package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/chaojimct/cli-agent-gateway/internal/acp"
	"github.com/chaojimct/cli-agent-gateway/internal/acpsession"
	"github.com/chaojimct/cli-agent-gateway/internal/agent"
	"github.com/chaojimct/cli-agent-gateway/internal/config"
	"github.com/chaojimct/cli-agent-gateway/internal/ir"
	"github.com/chaojimct/cli-agent-gateway/internal/session"
	"github.com/chaojimct/cli-agent-gateway/internal/toolloop"
	"github.com/chaojimct/cli-agent-gateway/internal/workspace"
)

// ACPProcess holds a long-lived ACP child for one agent profile.
type ACPProcess struct {
	mu       sync.Mutex
	turnMu   sync.Mutex // serialize turns on shared ACP stdio
	client   *acp.Client
	Profile  *agent.Profile
	Cfg      *config.CursorConfig
	Logger   *slog.Logger
	restarts atomic.Uint32
}

// NewACPProcess starts or connects to the default cursor ACP child.
func NewACPProcess(cfg *config.CursorConfig, logger *slog.Logger) (*ACPProcess, error) {
	profile := &agent.Profile{
		ID:               "cursor",
		Name:             "Cursor Agent",
		Binary:           cfg.BinaryPath,
		ACPArgs:          []string{"acp"},
		AuthMethod:       "cursor_login",
		SkipAuthEnv:      []string{"CURSOR_API_KEY", "CURSOR_AUTH_TOKEN"},
		ListModelsArgs:   []string{"--list-models"},
		ModelsSource:     "cli",
		ConfigKeys:       []string{"model", "mode"},
		CursorExtensions: true,
	}
	if profile.Binary == "" {
		profile.Binary = "cursor-agent"
	}
	return NewACPProcessForProfile(profile, cfg, logger)
}

// NewACPProcessForProfile starts or connects to an ACP child for the given agent.
func NewACPProcessForProfile(profile *agent.Profile, cfg *config.CursorConfig, logger *slog.Logger) (*ACPProcess, error) {
	p := &ACPProcess{Profile: profile, Cfg: cfg, Logger: logger}
	if err := p.ensureClient(context.Background()); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *ACPProcess) ensureClient(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.client != nil {
		return nil
	}
	profile := p.Profile
	if profile == nil {
		return fmt.Errorf("missing acp agent profile")
	}
	skipAuth := profile.ShouldSkipAuth()
	c, err := acp.NewClient(ctx, profile.ACPConfig(p.Cfg, p.Logger, skipAuth))
	if err != nil {
		return err
	}
	if err := c.Bootstrap(ctx); err != nil {
		_ = c.Close()
		return err
	}
	p.client = c
	return nil
}

func (p *ACPProcess) invalidate() {
	p.mu.Lock()
	if p.client != nil {
		_ = p.client.Close()
		p.client = nil
	}
	p.mu.Unlock()
	p.restarts.Add(1)
}

func (p *ACPProcess) Restarts() uint32 {
	return p.restarts.Load()
}

func (p *ACPProcess) clientLocked() *acp.Client {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.client
}

// ACPGateway is the ACP-only cursor-agent backend.
type ACPGateway struct {
	cfg       *config.CursorConfig
	pool      *session.Pool
	proc      *ACPProcess
	logger    *slog.Logger
	traceHook TraceHook
	active    atomic.Int32
	maxConc   int
}

func NewACPGateway(cfg *config.CursorConfig, pool *session.Pool, proc *ACPProcess, logger *slog.Logger) *ACPGateway {
	max := cfg.MaxConcurrent
	if max <= 0 {
		max = 8
	}
	return &ACPGateway{cfg: cfg, pool: pool, proc: proc, logger: logger, maxConc: max}
}

func (g *ACPGateway) Stats() Stats {
	return Stats{
		Active:        int(g.active.Load()),
		Queued:        0,
		MaxConcurrent: g.maxConc,
	}
}

func (g *ACPGateway) Close() error {
	return g.CloseProcessOnly()
}

// CloseProcessOnly shuts down the ACP child without clearing the shared session pool.
func (g *ACPGateway) CloseProcessOnly() error {
	if g.proc != nil {
		g.proc.invalidate()
	}
	return nil
}

func (g *ACPGateway) Run(ctx context.Context, prompt string, opts RunOpts) (*RunResult, error) {
	if err := g.proc.ensureClient(ctx); err != nil {
		return nil, err
	}
	g.active.Add(1)

	events := make(chan ir.Event, 64)
	errCh := make(chan error, 1)
	stopFlag := &atomic.Bool{}
	sessionID := opts.SessionID

	go func() {
		defer g.active.Add(-1)
		defer close(events)

		profile := toolloop.ProfileFromConfig(g.cfg.AgentProfile, opts.ClientTools)
		mode := g.resolveMode(opts)
		turnPrompt := prompt
		if opts.IncrementalPrompt != "" {
			turnPrompt = opts.IncrementalPrompt
		}

		sid, err := g.runTurn(ctx, turnPrompt, opts, profile, mode, events, stopFlag)
		if sid != "" {
			sessionID = sid
		}
		if err != nil && !stopFlag.Load() && ctx.Err() == nil {
			if !acp.IsBenignExit(err) {
				errCh <- err
			}
		}
		close(errCh)
	}()

	cancel := func(reason CancelReason) {
		stopFlag.Store(true)
		client := g.proc.clientLocked()
		if client != nil && sessionID != "" {
			_ = client.Notify("session/cancel", acp.CancelParams{SessionID: sessionID})
		}
	}

	return &RunResult{Events: events, ErrCh: errCh, Cancel: cancel, StopFlag: stopFlag}, nil
}

func (g *ACPGateway) resolveMode(opts RunOpts) acp.SessionMode {
	if opts.Mode != "" {
		return acp.SessionMode(opts.Mode)
	}
	if opts.ClientTools {
		m := g.cfg.ClientToolsAgentMode
		if m == "" || m == "agent" {
			return acp.ModePlan
		}
		return acp.SessionMode(m)
	}
	if g.cfg.IsModelProfile() {
		if g.cfg.AgentMode != "" {
			return acp.SessionMode(g.cfg.AgentMode)
		}
		return acp.ModeAsk
	}
	if g.cfg.AgentMode != "" {
		return acp.SessionMode(g.cfg.AgentMode)
	}
	return acp.ModeAgent
}

func (g *ACPProcess) withClient(ctx context.Context) (*acp.Client, error) {
	if err := pEnsure(ctx, g); err != nil {
		return nil, err
	}
	return g.clientLocked(), nil
}

func pEnsure(ctx context.Context, p *ACPProcess) error {
	return p.ensureClient(ctx)
}

func (g *ACPGateway) runTurn(ctx context.Context, prompt string, opts RunOpts, profile toolloop.Profile, mode acp.SessionMode, out chan<- ir.Event, stopFlag *atomic.Bool) (string, error) {
	g.proc.turnMu.Lock()
	defer g.proc.turnMu.Unlock()

	client, err := g.proc.withClient(ctx)
	if err != nil {
		return "", err
	}

	sessionID := opts.SessionID
	ws := workspace.Effective(g.cfg, opts.Workspace)
	if opts.ClientTools && g.cfg.Workspace != "" {
		ws = g.cfg.Workspace
	}
	_ = os.MkdirAll(ws, 0700)

	reused := false
	var poolEntry *session.Entry
	var sess *acpsession.Session
	if sessionID == "" && opts.ConversationID != "" && g.pool != nil {
		if e, ok := g.pool.Get(opts.ConversationID, opts.AgentID); ok {
			if e.Workspace != "" && e.Workspace != ws {
				g.pool.Delete(opts.ConversationID)
			} else {
				poolEntry = e
				sessionID = e.SessionID
				reused = true
			}
		}
	}
	if sessionID == "" {
		_ = workspace.EnsureModelSandbox(g.cfg)
		var err error
		sess, err = acpsession.NewSession(ctx, client, ws)
		if err != nil {
			g.proc.invalidate()
			if err2 := g.proc.ensureClient(ctx); err2 != nil {
				return "", Classify("", fmt.Errorf("session/new: %w", err))
			}
			client, _ = g.proc.withClient(ctx)
			sess, err = acpsession.NewSession(ctx, client, ws)
			if err != nil {
				return "", Classify("", fmt.Errorf("session/new: %w", err))
			}
		}
		sessionID = sess.SessionID
	}

	out <- ir.Event{Type: ir.EventSessionInit, SessionID: sessionID}

	model := opts.Model
	if model == "" {
		model = g.cfg.DefaultModel
	}
	_, bareModel := agent.ParseModel(model, g.registryDefault())
	agentModel := bareModel
	if p := g.proc.Profile; p != nil {
		agentModel = p.ResolveAgentModel(bareModel)
	} else {
		agentModel = acp.ResolveModel(bareModel)
	}
	modeStr := string(mode)
	needConfig := !reused
	if reused && poolEntry != nil {
		if poolEntry.Model != agentModel || poolEntry.Mode != modeStr {
			needConfig = true
		}
	}
	if needConfig {
		if sess == nil {
			sess = &acpsession.Session{SessionID: sessionID}
		}
		acpsession.ApplyConfig(ctx, client, sess, acpsession.ConfigureOpts{
			CWD:     ws,
			Model:   agentModel,
			Mode:    mode,
			Profile: agent.ACPSessionProfile(g.proc.Profile),
			Logger:  g.logger,
		})
	}

	allowExt := g.proc.Profile != nil && g.proc.Profile.CursorExtensions

	var pendingToolCalls []ir.ToolCallState
	handler := func(ctx context.Context, method string, id *int, params json.RawMessage) (interface{}, bool) {
		switch method {
		case "session/update":
			for _, ev := range toolloop.TranslateSessionUpdate(params, profile) {
				if ev.Type == ir.EventToolCall && ev.ToolCall != nil && profile == toolloop.ProfileClientTools {
					pendingToolCalls = append(pendingToolCalls, *ev.ToolCall)
				}
				if !emitEvent(out, ctx, ev) {
					return nil, false
				}
			}
		case "session/request_permission":
			if id == nil {
				return nil, false
			}
			dec := toolloop.DecidePermission(profile)
			// Native tool_calls already emitted on session/update; do not re-forward raw names here.
			pendingToolCalls = nil
			return dec.Response, true
		default:
			if allowExt && id != nil {
				if res, ok, extra := toolloop.HandleCursorExtension(method, params); ok {
					for _, ev := range extra {
						emitEvent(out, ctx, ev)
					}
					return res, true
				}
			}
		}
		return nil, false
	}
	client.SetHandler(handler)
	defer client.SetHandler(nil)

	if g.traceHook != nil && opts.TraceID != "" {
		traceID := opts.TraceID
		hook := g.traceHook
		client.SetRecorder(func(direction, method, payload string) {
			hook.RecordACP(traceID, direction, method, payload)
		})
		defer client.SetRecorder(nil)
	}

	raw, err := client.Request(ctx, "session/prompt", acp.PromptParams{
		SessionID: sessionID,
		Prompt:    []acp.ContentBlock{acp.TextBlock(prompt)},
	})
	if err != nil {
		if opts.ConversationID != "" && g.pool != nil && isSessionInvalidErr(err) {
			g.pool.Delete(opts.ConversationID)
		}
		return sessionID, Classify("", err)
	}
	var pr acp.PromptResult
	_ = json.Unmarshal(raw, &pr)
	out <- ir.Event{
		Type:       ir.EventDone,
		StopReason: string(pr.StopReason),
		SessionID:  sessionID,
	}

	if opts.ConversationID != "" && g.pool != nil {
		msgCount := opts.MessageCount
		if msgCount <= 0 {
			msgCount = 1
		}
		g.pool.Put(opts.ConversationID, opts.AgentID, sessionID, agentModel, modeStr, ws, hashPrompt(prompt), msgCount)
	}

	return sessionID, nil
}

func (g *ACPGateway) registryDefault() string {
	if g.proc != nil && g.proc.Profile != nil {
		return g.proc.Profile.ID
	}
	return "cursor"
}

func isSessionInvalidErr(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "session") && (strings.Contains(s, "not found") || strings.Contains(s, "invalid"))
}

func (g *ACPGateway) RunSync(ctx context.Context, prompt string, opts RunOpts) (string, error) {
	result, err := g.Run(ctx, prompt, opts)
	if err != nil {
		return "", err
	}

	var buf string
	var toolCalls []ir.ToolCallState
	for ev := range result.Events {
		switch ev.Type {
		case ir.EventContentDelta, ir.EventPlan:
			buf += ev.Text
		case ir.EventToolCall:
			if ev.ToolCall != nil {
				if opts.ClientTools && ev.ToolCall.Native {
					continue
				}
				toolCalls = append(toolCalls, *ev.ToolCall)
			}
		case ir.EventDone:
			if len(toolCalls) > 0 && opts.ClientTools {
				return toolloop.FormatClientToolCallsJSON(toolCalls), nil
			}
			return buf, nil
		case ir.EventError:
			if ev.Err != nil {
				return buf, ev.Err
			}
		}
	}
	select {
	case err := <-result.ErrCh:
		if err != nil && buf == "" && len(toolCalls) == 0 {
			return "", err
		}
	default:
	}
	if len(toolCalls) > 0 && opts.ClientTools {
		return toolloop.FormatClientToolCallsJSON(toolCalls), nil
	}
	return buf, nil
}

func hashPrompt(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:8])
}

func emitEvent(out chan<- ir.Event, ctx context.Context, ev ir.Event) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	select {
	case out <- ev:
		return true
	case <-ctx.Done():
		return false
	}
}

func (g *ACPGateway) StopDaemon() {}

func (g *ACPGateway) SetTraceHook(h TraceHook) {
	g.traceHook = h
}

func (g *ACPGateway) UpdateConfig(cfg *config.CursorConfig) {
	g.cfg = cfg
}

func (g *ACPGateway) Restarts() uint32 {
	if g.proc != nil {
		return g.proc.Restarts()
	}
	return 0
}

func (g *ACPGateway) ActiveSessions() int {
	if g.pool != nil {
		return g.pool.Active()
	}
	return 0
}

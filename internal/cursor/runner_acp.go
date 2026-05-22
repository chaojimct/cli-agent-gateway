package cursor

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/user/cursor-gateway/internal/agent"
	"github.com/user/cursor-gateway/internal/config"
	"github.com/user/cursor-gateway/internal/ir"
	"github.com/user/cursor-gateway/internal/runner"
	"github.com/user/cursor-gateway/internal/session"
)

// Runner v2: multi-agent ACP backend.
type Runner struct {
	router *runner.AgentRouter
	pool   *session.Pool
	logger *slog.Logger
	cfg    *config.Config
}

// NewRunner creates an ACP-backed runner with auto-discovered agents.
func NewRunner(cfg *config.Config, requestTimeout time.Duration, logger *slog.Logger) *Runner {
	if cfg == nil {
		cfg = config.Default()
	}
	if requestTimeout > 0 {
		cfg.Cursor.RequestTimeout = requestTimeout
	}
	pool := session.NewPool(30*time.Minute, 256)
	router := runner.NewAgentRouter(cfg, pool, logger)
	router.StartAsync()
	return &Runner{router: router, pool: pool, logger: logger, cfg: cfg}
}

func (r *Runner) IsReady() bool {
	if r.router == nil {
		return false
	}
	return r.router.IsReady()
}

// RunResult streams IR events from ACP.
type RunResult struct {
	Events   <-chan ir.Event
	ErrCh    <-chan error
	Cancel   func()
	stopFlag *atomic.Bool
}

func (r *RunResult) Stopped() bool {
	return r != nil && r.stopFlag != nil && r.stopFlag.Load()
}

func (r *Runner) SetTraceHook(h runner.TraceHook) {
	if r.router != nil {
		r.router.SetTraceHook(h)
	}
}

func (r *Runner) UpdateConfig(cfg *config.Config, requestTimeout time.Duration) {
	r.cfg = cfg
	if requestTimeout > 0 {
		cfg.Cursor.RequestTimeout = requestTimeout
	}
	r.router.UpdateConfig(cfg)
}

func (r *Runner) ResolveModel(requested string) agent.ResolvedModel {
	return r.router.ResolveModel(requested)
}

func (r *Runner) toRunOpts(opts RunOpts) runner.RunOpts {
	ro := runner.RunOpts{
		AgentID:           opts.AgentID,
		Model:             opts.Model,
		SessionID:         opts.SessionID,
		ConversationID:    opts.ConversationID,
		Workspace:         opts.Workspace,
		TraceID:           opts.TraceID,
		MessageCount:      len(opts.CursorMessages),
		ClientTools:       opts.ClientTools,
		IncrementalPrompt: opts.IncrementalPrompt,
	}
	if opts.MessageCount > 0 {
		ro.MessageCount = opts.MessageCount
	}
	return ro
}

func (r *Runner) Run(ctx context.Context, prompt string, opts RunOpts) (*RunResult, error) {
	if opts.AgentID == "" {
		resolved := r.ResolveModel(opts.Model)
		opts.AgentID = resolved.AgentID
		if opts.Model == "" {
			opts.Model = resolved.Model
		}
	}
	rr, err := r.router.Run(ctx, prompt, r.toRunOpts(opts))
	if err != nil {
		return nil, err
	}
	cancel := func() {
		if rr.Cancel != nil {
			rr.Cancel(runner.CancelUser)
		}
	}
	return &RunResult{
		Events:   rr.Events,
		ErrCh:    rr.ErrCh,
		Cancel:   cancel,
		stopFlag: rr.StopFlag,
	}, nil
}

func (r *Runner) RunSync(ctx context.Context, prompt string, opts RunOpts) (string, *Usage, error) {
	text, err := r.router.RunSync(ctx, prompt, r.toRunOpts(opts))
	return text, nil, err
}

func (r *Runner) Stats() ConcurrencyStats {
	if r.router == nil {
		return ConcurrencyStats{}
	}
	return r.router.ConcurrencyStats()
}

func (r *Runner) ListModels(ctx context.Context) ([]agent.ModelEntry, error) {
	return r.router.ListModels(ctx)
}

func (r *Runner) CreateChat(ctx context.Context) (string, error) {
	return "", nil
}

func (r *Runner) StopDaemon() {
	r.router.StopDaemon()
}

func (r *Runner) ACPRestarts() uint32 {
	return r.router.Restarts()
}

func (r *Runner) ActiveACPSessions() int {
	return r.router.ActiveSessions()
}

func (r *Runner) SessionEntry(conversationID, agentID string) (*session.Entry, bool) {
	if r.pool == nil || conversationID == "" {
		return nil, false
	}
	return r.pool.Get(conversationID, agentID)
}

func (r *Runner) Registry() *agent.Registry {
	if r.router == nil {
		return nil
	}
	return r.router.Registry()
}

func (r *Runner) Agents() []*agent.Profile {
	if r.router == nil {
		return nil
	}
	reg := r.router.Registry()
	if reg == nil {
		return nil
	}
	return reg.Profiles()
}

func (r *Runner) AgentRestarts(agentID string) uint32 {
	if r.router == nil {
		return 0
	}
	if gw, ok := r.router.Gateway(agentID); ok && gw != nil {
		return gw.Restarts()
	}
	return 0
}

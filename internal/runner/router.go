package runner

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/user/cursor-gateway/internal/agent"
	"github.com/user/cursor-gateway/internal/concurrency"
	"github.com/user/cursor-gateway/internal/config"
	"github.com/user/cursor-gateway/internal/session"
)

var agentRequestCounts sync.Map // agentID -> *atomic.Uint64

// AgentRouter routes ACP turns to discovered agent backends.
type AgentRouter struct {
	mu            sync.RWMutex
	cfg           *config.CursorConfig
	fullCfg       *config.Config
	registry      *agent.Registry
	pool          *session.Pool
	gateways      map[string]*ACPGateway
	logger        *slog.Logger
	traceHook     TraceHook
	maxConc       int
	ready         atomic.Bool
	concurrency   *concurrency.Controller
	debounceMu    sync.Mutex
	debounceTimer *time.Timer
}

func NewAgentRouter(fullCfg *config.Config, pool *session.Pool, logger *slog.Logger) *AgentRouter {
	cfg := &fullCfg.Cursor
	max := cfg.MaxConcurrent
	if max <= 0 {
		max = 8
	}
	r := &AgentRouter{
		cfg:         cfg,
		fullCfg:     fullCfg,
		registry:    agent.NewRegistry(fullCfg, logger),
		pool:        pool,
		gateways:    make(map[string]*ACPGateway),
		logger:      logger,
		maxConc:     max,
		concurrency: concurrency.NewController(max, 30*time.Second),
	}
	return r
}

// StartAsync runs agent discovery and gateway startup in the background.
func (r *AgentRouter) StartAsync() {
	go func() {
		r.registry.Refresh(context.Background())
		r.rebuildGateways()
		r.ready.Store(true)
		if r.logger != nil {
			r.logger.Info("agent router ready", "agents", len(r.registry.Profiles()))
		}
	}()
}

func (r *AgentRouter) IsReady() bool {
	return r.ready.Load()
}

func (r *AgentRouter) rebuildGateways() {
	profiles := r.registry.Profiles()
	next := make(map[string]*ACPGateway, len(profiles))
	for _, p := range profiles {
		cp := *p
		proc, err := NewACPProcessForProfile(&cp, r.cfg, r.logger)
		if err != nil {
			if r.logger != nil {
				r.logger.Warn("acp process start failed; will retry on first request", "agent", cp.ID, "error", err)
			}
			proc = &ACPProcess{Profile: &cp, Cfg: r.cfg, Logger: r.logger}
		}
		gw := NewACPGateway(r.cfg, r.pool, proc, r.logger)
		if r.traceHook != nil {
			gw.SetTraceHook(r.traceHook)
		}
		next[cp.ID] = gw
	}

	r.mu.Lock()
	old := r.gateways
	r.gateways = next
	r.mu.Unlock()

	for _, gw := range old {
		if gw != nil {
			gw.CloseProcessOnly()
		}
	}
}

func (r *AgentRouter) gateway(agentID string) (*ACPGateway, error) {
	r.mu.RLock()
	gw, ok := r.gateways[agentID]
	r.mu.RUnlock()
	if ok {
		return gw, nil
	}
	return nil, fmt.Errorf("acp agent not available: %s", agentID)
}

func (r *AgentRouter) ResolveModel(requested string) agent.ResolvedModel {
	return r.registry.Resolve(requested)
}

func (r *AgentRouter) ListModels(ctx context.Context) ([]agent.ModelEntry, error) {
	return r.registry.ListModels(ctx)
}

func (r *AgentRouter) ensureReady() error {
	if !r.ready.Load() {
		return fmt.Errorf("agent router not ready")
	}
	return nil
}

func (r *AgentRouter) Run(ctx context.Context, prompt string, opts RunOpts) (*RunResult, error) {
	if err := r.ensureReady(); err != nil {
		return nil, err
	}
	if opts.AgentID == "" {
		resolved := r.registry.Resolve(opts.Model)
		if !resolved.Valid {
			return nil, fmt.Errorf("%s", resolved.Err)
		}
		opts.AgentID = resolved.AgentID
		opts.Model = resolved.Model
	}
	if err := r.concurrency.Acquire(); err != nil {
		return nil, err
	}
	incAgentRequest(opts.AgentID)
	gw, err := r.gateway(opts.AgentID)
	if err != nil {
		r.concurrency.Release(0)
		return nil, err
	}
	start := time.Now()
	result, err := gw.Run(ctx, prompt, opts)
	if err != nil {
		r.concurrency.Release(0)
		return nil, err
	}
	go func() {
		<-result.ErrCh
		r.concurrency.Release(time.Since(start))
	}()
	return result, nil
}

func (r *AgentRouter) RunSync(ctx context.Context, prompt string, opts RunOpts) (string, error) {
	if err := r.ensureReady(); err != nil {
		return "", err
	}
	if opts.AgentID == "" {
		resolved := r.registry.Resolve(opts.Model)
		if !resolved.Valid {
			return "", fmt.Errorf("%s", resolved.Err)
		}
		opts.AgentID = resolved.AgentID
		opts.Model = resolved.Model
	}
	if err := r.concurrency.Acquire(); err != nil {
		return "", err
	}
	incAgentRequest(opts.AgentID)
	start := time.Now()
	defer r.concurrency.Release(time.Since(start))

	gw, err := r.gateway(opts.AgentID)
	if err != nil {
		return "", err
	}
	return gw.RunSync(ctx, prompt, opts)
}

func (r *AgentRouter) Stats() Stats {
	cc := r.concurrency.StatsSnapshot()
	r.mu.RLock()
	defer r.mu.RUnlock()
	var total Stats
	total.MaxConcurrent = r.maxConc
	total.Active = int(cc.Active)
	total.Queued = int(cc.Queued)
	return total
}

func (r *AgentRouter) ConcurrencyStats() concurrency.Stats {
	return r.concurrency.StatsSnapshot()
}

func (r *AgentRouter) SetTraceHook(h TraceHook) {
	r.traceHook = h
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, gw := range r.gateways {
		gw.SetTraceHook(h)
	}
}

func (r *AgentRouter) UpdateConfig(fullCfg *config.Config) {
	r.debounceMu.Lock()
	if r.debounceTimer != nil {
		r.debounceTimer.Stop()
	}
	pending := fullCfg
	r.debounceTimer = time.AfterFunc(500*time.Millisecond, func() {
		r.applyConfig(pending)
	})
	r.debounceMu.Unlock()
}

func (r *AgentRouter) applyConfig(fullCfg *config.Config) {
	r.mu.Lock()
	r.cfg = &fullCfg.Cursor
	r.fullCfg = fullCfg
	max := fullCfg.Cursor.MaxConcurrent
	if max <= 0 {
		max = 8
	}
	r.maxConc = max
	r.mu.Unlock()

	r.registry.UpdateConfig(fullCfg)
	r.rebuildGateways()
	if r.pool != nil {
		r.pool.InvalidateAll()
	}
}

func (r *AgentRouter) StopDaemon() {
	r.mu.RLock()
	gateways := r.gateways
	r.mu.RUnlock()
	for _, gw := range gateways {
		gw.CloseProcessOnly()
	}
	if r.pool != nil {
		r.pool.InvalidateAll()
	}
}

func (r *AgentRouter) Restarts() uint32 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var n uint32
	for _, gw := range r.gateways {
		n += gw.Restarts()
	}
	return n
}

func (r *AgentRouter) ActiveSessions() int {
	if r.pool != nil {
		return r.pool.Active()
	}
	return 0
}

func (r *AgentRouter) Registry() *agent.Registry {
	return r.registry
}

func (r *AgentRouter) Gateway(agentID string) (*ACPGateway, bool) {
	gw, err := r.gateway(agentID)
	return gw, err == nil
}

func incAgentRequest(agentID string) {
	if agentID == "" {
		agentID = "unknown"
	}
	v, _ := agentRequestCounts.LoadOrStore(agentID, &atomic.Uint64{})
	v.(*atomic.Uint64).Add(1)
}

// AgentRequestCounts returns per-agent turn counts since process start.
func AgentRequestCounts() map[string]uint64 {
	out := make(map[string]uint64)
	agentRequestCounts.Range(func(k, v interface{}) bool {
		out[k.(string)] = v.(*atomic.Uint64).Load()
		return true
	})
	return out
}

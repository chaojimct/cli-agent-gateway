package agent

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/user/cursor-gateway/internal/config"
)

// ModelEntry is one prefixed model for GET /v1/models.
type ModelEntry struct {
	ID      string
	OwnedBy string
	AgentID string
}

type modelCache struct {
	models []ModelEntry
	expiry time.Time
}

// Registry tracks discovered ACP agents and resolves prefixed model names.
type Registry struct {
	mu          sync.RWMutex
	cfg         *config.Config
	logger      *slog.Logger
	agents      map[string]*Profile
	defaultID   string
	lastRefresh time.Time
	modelCache  map[string]modelCache
}

func NewRegistry(cfg *config.Config, logger *slog.Logger) *Registry {
	r := &Registry{
		cfg:        cfg,
		logger:     logger,
		agents:     make(map[string]*Profile),
		defaultID:  "cursor",
		modelCache: make(map[string]modelCache),
	}
	if cfg != nil && cfg.Agents != nil && cfg.Agents.Default != "" {
		r.defaultID = cfg.Agents.Default
	}
	return r
}

// Refresh discovers agents and rebuilds the registry (blocking).
func (r *Registry) Refresh(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	found := Discover(ctx, r.cfg, r.logger)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents = make(map[string]*Profile, len(found))
	for _, p := range found {
		cp := *p
		r.agents[cp.ID] = &cp
	}
	if _, ok := r.agents[r.defaultID]; !ok && len(found) > 0 {
		r.defaultID = found[0].ID
	}
	r.lastRefresh = time.Now()
	r.modelCache = make(map[string]modelCache)
}

func (r *Registry) UpdateConfig(cfg *config.Config) {
	r.mu.Lock()
	r.cfg = cfg
	if cfg != nil && cfg.Agents != nil && cfg.Agents.Default != "" {
		r.defaultID = cfg.Agents.Default
	}
	r.mu.Unlock()
	r.Refresh(context.Background())
}

func (r *Registry) DefaultID() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.defaultID
}

func (r *Registry) Profile(agentID string) (*Profile, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.agents[agentID]
	if !ok {
		return nil, false
	}
	cp := *p
	return &cp, true
}

func (r *Registry) Profiles() []*Profile {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Profile, 0, len(r.agents))
	for _, p := range r.agents {
		cp := *p
		out = append(out, &cp)
	}
	return out
}

func (r *Registry) Resolve(requested string) ResolvedModel {
	r.mu.RLock()
	defaultID := r.defaultID
	cfg := r.cfg
	r.mu.RUnlock()

	agentID, model := ParseModel(requested, defaultID)
	display := FormatModel(agentID, model)

	r.mu.RLock()
	p, ok := r.agents[agentID]
	r.mu.RUnlock()

	if !ok {
		if HasExplicitPrefix(requested) && cfg != nil && cfg.Agents != nil && cfg.Agents.AllowFallbackToDefault() {
			return r.resolveForAgent(defaultID, model, FormatModel(defaultID, model))
		}
		if HasExplicitPrefix(requested) || (agentID != defaultID && requested != "") {
			return ResolvedModel{
				AgentID:   agentID,
				Model:     model,
				DisplayID: display,
				OwnedBy:   agentID,
				Valid:     false,
				Err:       "unknown agent: " + agentID,
			}
		}
		return r.resolveForAgent(defaultID, model, FormatModel(defaultID, model))
	}
	return r.resolveForAgent(p.ID, model, display)
}

func (r *Registry) resolveForAgent(agentID, model, display string) ResolvedModel {
	r.mu.RLock()
	p, ok := r.agents[agentID]
	r.mu.RUnlock()
	if !ok {
		return ResolvedModel{
			AgentID:   agentID,
			Model:     model,
			DisplayID: display,
			OwnedBy:   agentID,
			Valid:     false,
			Err:       "agent not available: " + agentID,
		}
	}
	agentModel := p.ResolveAgentModel(model)
	return ResolvedModel{
		AgentID:   p.ID,
		Model:     agentModel,
		DisplayID: display,
		OwnedBy:   p.ID,
		Valid:     true,
	}
}

func (r *Registry) cfgSnapshot() *config.Config {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.cfg
}

func (r *Registry) ListModels(ctx context.Context) ([]ModelEntry, error) {
	profiles := r.Profiles()
	if len(profiles) == 0 {
		return fallbackModels(r.defaultID), nil
	}
	var out []ModelEntry
	for _, p := range profiles {
		entries, err := r.modelsForProfile(ctx, p)
		if err != nil && r.logger != nil {
			r.logger.Warn("list models failed", "agent", p.ID, "error", err)
		}
		out = append(out, entries...)
	}
	if len(out) == 0 {
		return fallbackModels(r.defaultID), nil
	}
	return out, nil
}

func (r *Registry) modelsForProfile(ctx context.Context, p *Profile) ([]ModelEntry, error) {
	if p == nil {
		return nil, nil
	}
	if cached, ok := r.cachedModels(p.ID); ok {
		return cached, nil
	}

	var models []string
	var err error
	switch strings.ToLower(p.ModelsSource) {
	case "static":
		models = append([]string{}, p.StaticModels...)
	case "session_new":
		cfgSnap := r.cfgSnapshot()
		if cfgSnap != nil {
			models, err = FetchModelsFromSession(ctx, p, &cfgSnap.Cursor)
		}
		if len(models) == 0 {
			models = append([]string{}, p.StaticModels...)
		}
	default:
		cfgSnap := r.cfgSnapshot()
		var cursorCfg *config.CursorConfig
		if cfgSnap != nil {
			cursorCfg = &cfgSnap.Cursor
		}
		models, err = ListModelsCLI(ctx, p, cursorCfg)
		if len(models) == 0 && cursorCfg != nil {
			models, err = FetchModelsFromSession(ctx, p, cursorCfg)
		}
		if len(models) == 0 {
			models = append([]string{}, p.StaticModels...)
		}
	}

	out := make([]ModelEntry, 0, len(models))
	for _, m := range models {
		id := normalizePublicModelID(m)
		if id == "" {
			continue
		}
		out = append(out, ModelEntry{
			ID:      FormatModel(p.ID, id),
			OwnedBy: p.ID,
			AgentID: p.ID,
		})
	}
	r.storeModelCache(p.ID, out)
	return out, err
}

func (r *Registry) cacheTTL() time.Duration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.cacheTTLLocked()
}

func (r *Registry) cacheTTLLocked() time.Duration {
	ttl := 5 * time.Minute
	if r.cfg != nil && r.cfg.Agents != nil && r.cfg.Agents.ModelCacheTTL > 0 {
		ttl = r.cfg.Agents.ModelCacheTTL
	}
	return ttl
}

func (r *Registry) cachedModels(agentID string) ([]ModelEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.modelCache[agentID]
	if !ok || time.Now().After(c.expiry) {
		return nil, false
	}
	return append([]ModelEntry{}, c.models...), true
}

func (r *Registry) storeModelCache(agentID string, models []ModelEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.modelCache[agentID] = modelCache{
		models: append([]ModelEntry{}, models...),
		expiry: time.Now().Add(r.cacheTTLLocked()),
	}
}

func normalizePublicModelID(raw string) string {
	raw = trimModelLine(raw)
	if strings.HasPrefix(strings.ToLower(raw), "tip:") {
		return ""
	}
	if len(raw) >= 7 && raw[:7] == "auto - " {
		return "auto"
	}
	if i := findModelDash(raw); i > 0 {
		return trimModelLine(raw[:i])
	}
	return raw
}

func trimModelLine(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}

func findModelDash(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' && i+2 < len(s) && s[i+1] == '-' && s[i+2] == ' ' {
			return i
		}
	}
	return -1
}

func fallbackModels(defaultID string) []ModelEntry {
	return []ModelEntry{
		{ID: FormatModel(defaultID, "auto"), OwnedBy: defaultID, AgentID: defaultID},
	}
}

// LastRefreshAt returns when agents were last discovered.
func (r *Registry) LastRefreshAt() time.Time {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lastRefresh
}

// ModelCacheSize returns the number of cached agent model lists.
func (r *Registry) ModelCacheSize() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.modelCache)
}

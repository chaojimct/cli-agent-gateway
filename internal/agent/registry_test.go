package agent

import (
	"testing"
	"time"

	"github.com/chaojimct/cli-agent-gateway/internal/config"
)

func TestResolveUnknownExplicitAgent(t *testing.T) {
	cfg := config.Default()
	r := NewRegistry(cfg, nil)
	r.mu.Lock()
	r.agents["cursor"] = &Profile{ID: "cursor", StaticModels: []string{"auto"}}
	r.mu.Unlock()

	got := r.Resolve("claude/sonnet")
	if got.Valid {
		t.Fatalf("expected invalid resolve, got %+v", got)
	}
	if got.Err == "" {
		t.Fatal("expected error message")
	}
}

func TestResolveFallbackToDefaultWhenEnabled(t *testing.T) {
	cfg := config.Default()
	cfg.Agents = &config.AgentsConfig{
		Default:           "cursor",
		FallbackToDefault: true,
	}
	r := NewRegistry(cfg, nil)
	r.mu.Lock()
	r.agents["cursor"] = &Profile{ID: "cursor", StaticModels: []string{"auto"}}
	r.mu.Unlock()

	got := r.Resolve("claude/sonnet")
	if !got.Valid || got.AgentID != "cursor" {
		t.Fatalf("expected fallback to cursor, got %+v", got)
	}
}

func TestModelCacheHitAndExpiry(t *testing.T) {
	r := NewRegistry(config.Default(), nil)
	entries := []ModelEntry{{ID: "cursor/auto", AgentID: "cursor", OwnedBy: "cursor"}}
	r.storeModelCache("cursor", entries)

	cached, ok := r.cachedModels("cursor")
	if !ok || len(cached) != 1 || cached[0].ID != "cursor/auto" {
		t.Fatalf("cache miss: ok=%v cached=%+v", ok, cached)
	}

	r.mu.Lock()
	r.modelCache["cursor"] = modelCache{
		models: entries,
		expiry: time.Now().Add(-time.Second),
	}
	r.mu.Unlock()
	if _, ok := r.cachedModels("cursor"); ok {
		t.Fatal("expected expired cache entry")
	}
}

func TestRefreshClearsModelCache(t *testing.T) {
	r := NewRegistry(config.Default(), nil)
	r.storeModelCache("cursor", []ModelEntry{{ID: "cursor/auto"}})
	if r.ModelCacheSize() != 1 {
		t.Fatalf("cache size=%d want 1", r.ModelCacheSize())
	}
	r.mu.Lock()
	r.modelCache = make(map[string]modelCache)
	r.mu.Unlock()
	if r.ModelCacheSize() != 0 {
		t.Fatalf("cache size=%d want 0", r.ModelCacheSize())
	}
}

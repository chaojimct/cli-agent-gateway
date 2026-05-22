package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/chaojimct/cli-agent-gateway/internal/agent"
	"github.com/chaojimct/cli-agent-gateway/internal/config"
	"github.com/chaojimct/cli-agent-gateway/internal/session"
)

func TestGatewayUnknownAgent(t *testing.T) {
	cfg := config.Default()
	r := NewAgentRouter(cfg, session.NewPool(time.Minute, 8), nil)
	r.ready.Store(true)

	_, err := r.Run(context.Background(), "hello", RunOpts{AgentID: "missing-agent"})
	if err == nil {
		t.Fatal("expected error for unknown agent")
	}
	if !strings.Contains(err.Error(), "not available") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveUnknownExplicitAgent(t *testing.T) {
	cfg := config.Default()
	r := NewAgentRouter(cfg, session.NewPool(time.Minute, 8), nil)
	got := r.ResolveModel("unknown/model")
	if got.Valid {
		t.Fatalf("expected invalid model, got %+v", got)
	}
}

func TestApplyConfigInvalidatesPool(t *testing.T) {
	pool := session.NewPool(time.Minute, 8)
	pool.Put("conv-1", "cursor", "sess-1", "auto", "ask", "/ws", "hash", 1)

	cfg := config.Default()
	r := NewAgentRouter(cfg, pool, nil)
	r.mu.Lock()
	r.fullCfg = cfg
	r.mu.Unlock()
	if r.pool != nil {
		r.pool.InvalidateAll()
	}
	if pool.Active() != 0 {
		t.Fatalf("pool active=%d want 0 after invalidate", pool.Active())
	}
}

func TestRunNotReady(t *testing.T) {
	cfg := config.Default()
	r := NewAgentRouter(cfg, session.NewPool(time.Minute, 8), nil)
	_, err := r.Run(context.Background(), "hello", RunOpts{AgentID: "cursor"})
	if err == nil || !strings.Contains(err.Error(), "not ready") {
		t.Fatalf("expected not ready error, got %v", err)
	}
}

func TestCloseProcessOnlyIncrementsRestarts(t *testing.T) {
	proc := &ACPProcess{Profile: &agent.Profile{ID: "cursor", Binary: "cursor-agent"}}
	gw := &ACPGateway{proc: proc}
	gw.CloseProcessOnly()
	if proc.Restarts() != 1 {
		t.Fatalf("restarts=%d want 1", proc.Restarts())
	}
}

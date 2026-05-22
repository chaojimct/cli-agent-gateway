package agent

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/chaojimct/cli-agent-gateway/internal/config"
)

func TestDiscoverClaudeBridgeLive(t *testing.T) {
	if os.Getenv("CG_RUN_CLAUDE_PROBE") == "" {
		t.Skip("set CG_RUN_CLAUDE_PROBE=1 to run live claude bridge probe")
	}
	cfg := config.Default()
	cfg.Cursor.Proxy = "http://127.0.0.1:10809"
	if p := os.Getenv("CG_CURSOR_PROXY"); p != "" {
		cfg.Cursor.Proxy = p
	}
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Second)
	defer cancel()
	found := Discover(ctx, cfg, nil)
	for _, p := range found {
		if p.ID == "claude" {
			t.Logf("claude discovered: command=%s args=%v", p.Command(), p.Args())
			return
		}
	}
	t.Fatal("claude agent not discovered")
}

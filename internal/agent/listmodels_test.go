package agent

import (
	"os"
	"testing"

	"github.com/user/cursor-gateway/internal/config"
)

func TestListModelsEnvIncludesProxy(t *testing.T) {
	p := &Profile{ID: "cursor"}
	cfg := &config.CursorConfig{Proxy: "http://127.0.0.1:10809"}
	env := listModelsEnv(p, cfg)
	for _, key := range []string{
		"HTTP_PROXY=http://127.0.0.1:10809",
		"npm_config_proxy=http://127.0.0.1:10809",
		"NPX_YES=1",
	} {
		found := false
		for _, e := range env {
			if e == key {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing %s in env", key)
		}
	}
}

func TestListModelsEnvCursorKey(t *testing.T) {
	t.Setenv("CURSOR_API_KEY", "test-key")
	p := &Profile{ID: "cursor"}
	env := listModelsEnv(p, nil)
	found := false
	for _, e := range env {
		if e == "CURSOR_API_KEY=test-key" {
			found = true
		}
	}
	if !found {
		t.Fatalf("CURSOR_API_KEY not in env")
	}
	_ = os.Environ()
}

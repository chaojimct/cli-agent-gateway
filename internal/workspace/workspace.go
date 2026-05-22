package workspace

import (
	"os"
	"path/filepath"

	"github.com/user/cursor-gateway/internal/config"
)

// Effective returns the workspace directory for cursor-agent.
func Effective(cfg *config.CursorConfig, optsWorkspace string) string {
	if cfg != nil && cfg.IsModelProfile() {
		if cfg.ModelWorkspace != "" {
			return cfg.ModelWorkspace
		}
		return EnsureModelSandbox(cfg)
	}
	if optsWorkspace != "" {
		return optsWorkspace
	}
	if cfg != nil && cfg.Workspace != "" {
		return cfg.Workspace
	}
	return "."
}

// EnsureModelSandbox creates and returns the isolated model sandbox path.
func EnsureModelSandbox(cfg *config.CursorConfig) string {
	if cfg != nil && cfg.ModelWorkspace != "" {
		_ = os.MkdirAll(cfg.ModelWorkspace, 0700)
		return cfg.ModelWorkspace
	}
	home, err := os.UserHomeDir()
	if err != nil {
		dir := filepath.Join(os.TempDir(), "cursor-gateway-model-sandbox")
		_ = os.MkdirAll(dir, 0700)
		return dir
	}
	dir := filepath.Join(home, ".cursor-gateway", "model-sandbox")
	_ = os.MkdirAll(dir, 0700)
	return dir
}

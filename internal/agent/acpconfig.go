package agent

import (
	"log/slog"

	"github.com/chaojimct/cli-agent-gateway/internal/acp"
	"github.com/chaojimct/cli-agent-gateway/internal/config"
	"github.com/chaojimct/cli-agent-gateway/internal/workspace"
)

// ACPConfig builds acp.Client spawn settings for a profile.
func (p Profile) ACPConfig(cfg *config.CursorConfig, logger *slog.Logger, skipAuth bool) acp.Config {
	var dir string
	if cfg != nil {
		dir = workspace.Effective(cfg, "")
	}
	return acp.Config{
		Command:          p.Command(),
		Args:             p.Args(),
		Dir:              dir,
		Env:              SpawnEnv(p, cfg),
		Logger:           logger,
		SkipAuthenticate: skipAuth,
		AuthMethod:       p.AuthMethod,
		NoAppendACP:      p.ArgsComplete,
	}
}

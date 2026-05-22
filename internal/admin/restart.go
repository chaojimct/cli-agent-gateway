package admin

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/chaojimct/cli-agent-gateway/internal/config"
	"github.com/chaojimct/cli-agent-gateway/internal/cursor"
)

// Coordinator handles graceful shutdown and process restart.
type Coordinator struct {
	mu          sync.Mutex
	restarting  bool
	srv         *http.Server
	runner      *cursor.Runner
	sessionFlush func()
	grace       time.Duration
	logger      *slog.Logger
	configPath  string
}

func NewCoordinator(srv *http.Server, runner *cursor.Runner, sessionFlush func(), grace time.Duration, logger *slog.Logger, configPath string) *Coordinator {
	return &Coordinator{
		srv:          srv,
		runner:       runner,
		sessionFlush: sessionFlush,
		grace:        grace,
		logger:       logger,
		configPath:   configPath,
	}
}

// ScheduleRestart performs graceful shutdown then re-execs the binary.
func (c *Coordinator) ScheduleRestart() error {
	c.mu.Lock()
	if c.restarting {
		c.mu.Unlock()
		return nil
	}
	c.restarting = true
	c.mu.Unlock()

	go func() {
		c.logger.Info("scheduled restart")
		ctx, cancel := context.WithTimeout(context.Background(), c.grace)
		defer cancel()

		if c.sessionFlush != nil {
			c.sessionFlush()
		}
		if c.runner != nil {
			c.runner.StopDaemon()
		}
		if c.srv != nil {
			_ = c.srv.Shutdown(ctx)
		}

		exe, err := os.Executable()
		if err != nil {
			c.logger.Error("restart failed", "error", err)
			os.Exit(0)
			return
		}

		cmd := exec.Command(exe, os.Args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		cmd.Env = os.Environ()
		if err := cmd.Start(); err != nil {
			c.logger.Error("spawn restart failed", "error", err)
			os.Exit(1)
			return
		}
		os.Exit(0)
	}()

	return nil
}

// ApplyConfigFromManager reloads runtime after config change.
func ApplyConfigFromManager(cfg *config.Config, runner *cursor.Runner, authEnabled *bool, authKey *string) {
	if runner != nil {
		runner.UpdateConfig(cfg, cfg.Cursor.RequestTimeout)
	}
	if authEnabled != nil {
		*authEnabled = cfg.Auth.Enabled
	}
	if authKey != nil && cfg.Auth.APIKey != "" {
		*authKey = cfg.Auth.APIKey
	}
	_ = cursor.EnsureModelSandbox(&cfg.Cursor)
}

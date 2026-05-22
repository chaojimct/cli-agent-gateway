package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	"github.com/go-chi/chi/v5"
	"github.com/user/cursor-gateway/internal/admin"
	"github.com/user/cursor-gateway/internal/config"
	"github.com/user/cursor-gateway/internal/cursor"
	"github.com/user/cursor-gateway/internal/handler"
	"github.com/user/cursor-gateway/internal/middleware"
	"github.com/user/cursor-gateway/internal/webui"
)

var version = "dev"

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	showVersion := flag.Bool("version", false, "show version")
	flag.Parse()

	if *showVersion {
		fmt.Printf("cursor-gateway %s\n", version)
		os.Exit(0)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	_ = cursor.EnsureModelSandbox(&cfg.Cursor)

	logger := setupLogger(cfg.Log)

	runner := cursor.NewRunner(cfg, cfg.Cursor.RequestTimeout, logger)

	var sessionMgr *cursor.SessionManager
	if cfg.Session.Enabled {
		sessionMgr, err = cursor.NewSessionManager(cfg.Session.StoragePath, runner, logger, cfg.Session.LockTimeout)
		if err != nil {
			logger.Error("failed to create session manager", "error", err)
			os.Exit(1)
		}
	}

	hub := webui.NewHub(logger)
	maxTraces := cfg.WebUI.MaxTraces
	if maxTraces <= 0 {
		maxTraces = 1000
	}
	store := webui.NewStore(maxTraces, hub)
	store.SetMaxDeltaBytes(cfg.WebUI.MaxDeltaBytes)

	webuiHandler := webui.NewHandler(store, hub, logger)
	runner.SetTraceHook(store)

	var authEnabled atomic.Bool
	var authKey atomic.Value
	authEnabled.Store(cfg.Auth.Enabled)
	authKey.Store(cfg.Auth.APIKey)

	cfgMgr := config.NewManager(cfg, *configPath, func(c *config.Config) {
		runner.UpdateConfig(c, c.Cursor.RequestTimeout)
		authEnabled.Store(c.Auth.Enabled)
		if c.Auth.APIKey != "" {
			authKey.Store(c.Auth.APIKey)
		}
		_ = cursor.EnsureModelSandbox(&c.Cursor)
	})

	handlerEnv := handler.HandlerEnv{
		Runner:             runner,
		Sessions:           sessionMgr,
		Store:              store,
		Logger:             logger,
		CfgMgr:             cfgMgr,
		CursorCfg:          &cfg.Cursor,
		StreamPendingMode:  cfg.Cursor.StreamPendingMode,
		ThinkingVisibility: cfg.Cursor.ThinkingVisibility,
		MaxBody:            cfg.Server.MaxRequestBody,
	}

	chatHandler := handler.NewChatHandler(handlerEnv)
	messagesHandler := handler.NewMessagesHandler(handlerEnv)
	responsesHandler := handler.NewResponsesHandler(handlerEnv)
	geminiHandler := handler.NewGeminiHandler(handlerEnv)
	modelsHandler := handler.NewModelsHandler(runner, logger)
	healthHandler := handler.NewHealthHandler(version, runner)
	statusHandler := handler.NewStatusHandler(runner)
	metricsHandler := handler.NewMetricsHandler(runner)

	srv := &http.Server{
		Addr:         cfg.Addr(),
		Handler:      nil,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	var sessionFlush func()
	if sessionMgr != nil {
		sessionFlush = sessionMgr.Flush
	}

	restartCoord := admin.NewCoordinator(srv, runner, sessionFlush, cfg.Admin.RestartGrace, logger, *configPath)

	webuiHandler.SetAPIDeps(webui.APIDeps{
		ConfigMgr:         cfgMgr,
		Restart:           restartCoord,
		AuthEnabled:       cfg.Auth.Enabled,
		AllowUnauthConfig: cfg.WebUI.AllowUnauthenticatedConfig,
		AllowedOrigins:    cfg.Server.AllowedOrigins,
	})

	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(middleware.CORS(cfg.Server.AllowedOrigins))
		r.Use(middleware.Logging(logger))
		r.Use(middleware.Recovery(logger))
		r.Use(dynamicAuth(&authEnabled, &authKey))

		r.Get("/ws/events", webuiHandler.HandleWebSocket)
		r.Get("/", webuiHandler.ServeIndex)
		r.Get("/legacy", webuiHandler.ServeLegacyIndex)
		r.Get("/api/tap/events", webuiHandler.HandleTapSSE)
		r.Get("/api/tap/records", webuiHandler.GetTapRecords)
		r.Get("/api/traces", webuiHandler.GetTraces)
		r.Get("/api/traces/compare", webuiHandler.CompareTraces)
		r.Get("/api/traces/{id}", webuiHandler.GetTrace)
		r.Get("/api/traces/{id}/export", webuiHandler.ExportTrace)
		r.Get("/api/stats", webuiHandler.GetStats)
		r.Get("/api/config", webuiHandler.GetConfig)
		r.Put("/api/config", webuiHandler.PutConfig)
		r.Post("/api/admin/restart", webuiHandler.PostRestart)

		r.Post("/v1/chat/completions", chatHandler.ServeHTTP)
		r.Post("/v1/messages", messagesHandler.ServeHTTP)
		r.Post("/v1/responses", responsesHandler.ServeHTTP)
		r.Post("/v1beta/models/*", geminiHandler.ServeHTTP)
		r.Get("/v1/models", modelsHandler.ServeHTTP)
		r.Get("/healthz", healthHandler.ServeHTTP)
		r.Get("/status", statusHandler.ServeHTTP)
		r.Get("/metrics", metricsHandler.ServeHTTP)
	})

	srv.Handler = r

	go func() {
		logger.Info("starting cursor-gateway",
			"version", version,
			"addr", cfg.Addr(),
			"profile", cfg.Cursor.AgentProfile,
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	logger.Info("received signal, shutting down", "signal", sig)

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	if sessionMgr != nil {
		sessionMgr.Flush()
	}
	runner.StopDaemon()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("shutdown error", "error", err)
	}
	logger.Info("server stopped")
}

func dynamicAuth(enabled *atomic.Bool, key *atomic.Value) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			k, _ := key.Load().(string)
			middleware.Auth(enabled.Load(), k)(next).ServeHTTP(w, r)
		})
	}
}

func setupLogger(cfg config.LogConfig) *slog.Logger {
	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler
	if cfg.Format == "text" {
		h = slog.NewTextHandler(os.Stdout, opts)
	} else {
		h = slog.NewJSONHandler(os.Stdout, opts)
	}
	return slog.New(h)
}

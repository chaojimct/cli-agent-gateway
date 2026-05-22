package acpsession

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/chaojimct/cli-agent-gateway/internal/acp"
)

// ProfileConfig supplies agent-specific session config behavior.
type ProfileConfig struct {
	ResolveModel          func(string) string
	WantsConfigKey        func(string) bool
	UseCursorModeFallback bool
}

// ConfigureOpts controls session/new and config application.
type ConfigureOpts struct {
	CWD     string
	Model   string
	Mode    acp.SessionMode
	Profile ProfileConfig
	Logger  *slog.Logger
}

// Session holds a configured ACP session.
type Session struct {
	SessionID string
	NewResult acp.SessionNewResult
	Catalog   *ModelCatalog
}

// NewSession creates a session and builds the model catalog.
func NewSession(ctx context.Context, client *acp.Client, cwd string) (*Session, error) {
	raw, err := client.Request(ctx, "session/new", acp.SessionNewParams{
		CWD:        cwd,
		McpServers: []interface{}{},
	})
	if err != nil {
		return nil, err
	}
	var sn acp.SessionNewResult
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &sn)
	}
	if sn.SessionID == "" {
		return nil, errEmptySession
	}
	return &Session{
		SessionID: sn.SessionID,
		NewResult: sn,
		Catalog:   BuildModelCatalog(&sn),
	}, nil
}

// ConfigureNewSession runs session/new and applies model/mode config.
func ConfigureNewSession(ctx context.Context, client *acp.Client, opts ConfigureOpts) (*Session, error) {
	sess, err := NewSession(ctx, client, opts.CWD)
	if err != nil {
		return nil, err
	}
	ApplyConfig(ctx, client, sess, opts)
	return sess, nil
}

// ApplyConfig sets model and mode without aborting on failure.
func ApplyConfig(ctx context.Context, client *acp.Client, sess *Session, opts ConfigureOpts) {
	if sess == nil || client == nil {
		return
	}
	requested := opts.Model
	if opts.Profile.ResolveModel != nil {
		requested = opts.Profile.ResolveModel(requested)
	}
	if wantsKey(opts.Profile.WantsConfigKey, "model") {
		applyModel(ctx, client, sess, requested, opts.Logger)
	}
	if wantsKey(opts.Profile.WantsConfigKey, "mode") {
		if hasConfigOption(&sess.NewResult, "mode") || opts.Profile.UseCursorModeFallback {
			applyMode(ctx, client, sess.SessionID, string(opts.Mode), opts.Logger)
		}
	}
}

func wantsKey(fn func(string) bool, key string) bool {
	if fn == nil {
		return key == "model" || key == "mode"
	}
	return fn(key)
}

func applyModel(ctx context.Context, client *acp.Client, sess *Session, requested string, logger *slog.Logger) {
	valueID, value, _ := sess.Catalog.Resolve(requested)

	if valueID != "" {
		if err := setConfigOption(ctx, client, sess.SessionID, "model", valueID, true); err == nil {
			return
		}
	}
	if value != "" {
		if err := setConfigOption(ctx, client, sess.SessionID, "model", value, false); err == nil {
			return
		}
	}
	modelID := firstNonEmpty(valueID, value, requested)
	if modelID != "" && modelID != "auto" {
		if _, err := client.Request(ctx, "session/set_model", acp.SetModelParams{
			SessionID: sess.SessionID,
			ModelID:   modelID,
		}); err == nil {
			return
		}
	}
	if logger != nil && requested != "" && !stringsEqual(requested, "auto") {
		logger.Warn("acp model config failed; using session default", "requested", requested)
	}
}

func applyMode(ctx context.Context, client *acp.Client, sessionID, mode string, logger *slog.Logger) {
	if mode == "" {
		return
	}
	if err := setConfigOption(ctx, client, sessionID, "mode", mode, false); err != nil {
		if logger != nil {
			logger.Warn("acp mode config failed; using session default", "mode", mode, "error", err)
		}
	}
}

func setConfigOption(ctx context.Context, client *acp.Client, sessionID, configID, val string, useValueID bool) error {
	params := acp.SetConfigOptionParams{SessionID: sessionID, ConfigID: configID}
	if useValueID {
		params.ValueID = val
	} else {
		params.Value = val
	}
	_, err := client.Request(ctx, "session/set_config_option", params)
	return err
}

func hasConfigOption(sn *acp.SessionNewResult, id string) bool {
	if sn == nil {
		return false
	}
	for _, opt := range sn.ConfigOptions {
		if opt.ID == id {
			return true
		}
	}
	return false
}

func stringsEqual(a, b string) bool {
	return stringsTrim(a) == stringsTrim(b)
}

func stringsTrim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}

var errEmptySession = &sessionError{"empty session id from session/new"}

type sessionError struct{ msg string }

func (e *sessionError) Error() string { return e.msg }

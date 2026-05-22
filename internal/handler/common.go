package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"github.com/chaojimct/cli-agent-gateway/internal/config"
	"github.com/chaojimct/cli-agent-gateway/internal/cursor"
	"github.com/chaojimct/cli-agent-gateway/internal/agent"
	"github.com/chaojimct/cli-agent-gateway/internal/webui"
)

func generateShortID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func traceIDFromOpts(opts cursor.RunOpts) string {
	if opts.TraceID != "" {
		return opts.TraceID
	}
	return "chatcmpl-" + generateShortID()
}

// HandlerEnv shared dependencies for API handlers.
type HandlerEnv struct {
	Runner             *cursor.Runner
	Sessions           *cursor.SessionManager
	Store              *webui.Store
	Logger             *slog.Logger
	CfgMgr             *config.Manager
	CursorCfg          *config.CursorConfig
	StreamPendingMode  string
	ThinkingVisibility string
	MaxBody            int64
}

func (e *HandlerEnv) cursorCfg() *config.CursorConfig {
	if e.CfgMgr != nil {
		return &e.CfgMgr.Get().Cursor
	}
	return e.CursorCfg
}

func (e *HandlerEnv) webUICfg() config.WebUIConfig {
	if e.CfgMgr != nil {
		return e.CfgMgr.Get().WebUI
	}
	return config.WebUIConfig{}
}

const maxTraceRequestBody = 64 * 1024

func (e *HandlerEnv) traceRequestBody(v interface{}) string {
	if !e.webUICfg().StoreRequestBody || v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	if len(b) > maxTraceRequestBody {
		return string(b[:maxTraceRequestBody]) + "\n…(truncated)"
	}
	return string(b)
}

func (e *HandlerEnv) decodeJSON(w http.ResponseWriter, r *http.Request, v interface{}) error {
	max := e.MaxBody
	if max <= 0 {
		max = 8 << 20
	}
	body := http.MaxBytesReader(w, r.Body, max)
	defer body.Close()
	dec := json.NewDecoder(body)
	if err := dec.Decode(v); err != nil {
		if e.Logger != nil {
			e.Logger.Warn("request JSON decode failed", "path", r.URL.Path, "error", err.Error())
		}
		return err
	}
	return nil
}

func (e *HandlerEnv) validateResolved(w http.ResponseWriter, resolved agent.ResolvedModel) bool {
	if resolved.Valid {
		return true
	}
	msg := resolved.Err
	if msg == "" {
		msg = "model not found"
	}
	writeError(w, http.StatusBadRequest, "invalid_request_error", msg)
	return false
}

func (e *HandlerEnv) validateResolvedAnthropic(w http.ResponseWriter, resolved agent.ResolvedModel) bool {
	if resolved.Valid {
		return true
	}
	msg := resolved.Err
	if msg == "" {
		msg = "model not found"
	}
	writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", msg)
	return false
}

func (e *HandlerEnv) ensureRunnerReady(w http.ResponseWriter) bool {
	if e.Runner != nil && e.Runner.IsReady() {
		return true
	}
	writeError(w, http.StatusServiceUnavailable, "service_unavailable", "agent discovery in progress")
	return false
}

func (e *HandlerEnv) ensureRunnerReadyAnthropic(w http.ResponseWriter) bool {
	if e.Runner != nil && e.Runner.IsReady() {
		return true
	}
	writeAnthropicError(w, http.StatusServiceUnavailable, "service_unavailable", "agent discovery in progress")
	return false
}

func (e *HandlerEnv) runOpts(resolved agent.ResolvedModel, traceID string, msgs []cursor.Message, sessionID string, clientTools bool, conversationID, incremental string, msgCount int) cursor.RunOpts {
	ws := cursor.EffectiveWorkspace(e.cursorCfg(), "")
	if clientTools && e.cursorCfg().Workspace != "" {
		ws = e.cursorCfg().Workspace
	}
	return cursor.RunOpts{
		AgentID:           resolved.AgentID,
		Model:             resolved.Model,
		DisplayModel:      resolved.DisplayID,
		SessionID:         sessionID,
		ConversationID:    conversationID,
		IncrementalPrompt: incremental,
		MessageCount:      msgCount,
		TraceID:           traceID,
		CursorMessages:    msgs,
		Workspace:         ws,
		ClientTools:       clientTools,
	}
}

func displayModel(opts cursor.RunOpts) string {
	if opts.DisplayModel != "" {
		return opts.DisplayModel
	}
	return opts.Model
}

func (e *HandlerEnv) streamCfg(traceID string, opts cursor.RunOpts) StreamConfig {
	return StreamConfig{
		CursorCfg:       e.cursorCfg(),
		ThinkingVisMode: e.ThinkingVisibility,
		Store:           e.Store,
		Logger:          e.Logger,
		TraceID:         traceID,
		ClientTools:     opts.ClientTools,
	}
}

func (e *HandlerEnv) startTrace(traceID, endpoint, model string, messageCount, promptLen int, bodyPayload interface{}, conv ConversationResolve) {
	if e.Store == nil {
		return
	}
	profile := e.cursorCfg().AgentProfile
	e.Store.StartTrace(traceID, endpoint, model, profile, &webui.TraceRequest{
		Endpoint:           endpoint,
		Model:              model,
		MessageCount:       messageCount,
		PromptLen:          promptLen,
		AgentProfile:       profile,
		ConversationID:     conv.ID,
		ConversationSource: conv.Source,
	}, e.traceRequestBody(bodyPayload))
}

func waitForErr(result *cursor.RunResult) error {
	select {
	case err := <-result.ErrCh:
		return err
	default:
		return nil
	}
}

func isLocalRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	host = strings.Trim(host, "[]")
	return host == "127.0.0.1" || host == "::1" || host == "localhost"
}

func writeError(w http.ResponseWriter, status int, errType, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]string{
			"message": message,
			"type":    errType,
		},
	})
}

func writeAnthropicError(w http.ResponseWriter, status int, errType, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"type": "error",
		"error": map[string]string{
			"type":    errType,
			"message": message,
		},
	})
}

// drainBody consumes and discards request body.
func drainBody(r *http.Request) {
	if r.Body != nil {
		_, _ = io.Copy(io.Discard, r.Body)
		r.Body.Close()
	}
}

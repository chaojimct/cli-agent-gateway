package webui

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/chaojimct/cli-agent-gateway/internal/admin"
	"github.com/chaojimct/cli-agent-gateway/internal/config"
)

// APIDeps optional admin/config dependencies.
type APIDeps struct {
	ConfigMgr              *config.Manager
	Restart                *admin.Coordinator
	AuthEnabled            bool
	AllowUnauthConfig      bool
	AllowedOrigins         []string
}

func (h *Handler) SetAPIDeps(deps APIDeps) {
	if deps.ConfigMgr != nil {
		h.cfgMgr = deps.ConfigMgr
	}
	if deps.Restart != nil {
		h.restart = deps.Restart
	}
	h.authEnabled = deps.AuthEnabled
	h.allowUnauthConfig = deps.AllowUnauthConfig
	h.allowedOrigins = deps.AllowedOrigins
}

func (h *Handler) allowConfig(r *http.Request) bool {
	if h.authEnabled {
		return true
	}
	if h.allowUnauthConfig {
		return true
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	host = strings.Trim(host, "[]")
	return host == "127.0.0.1" || host == "::1"
}

func (h *Handler) checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	if len(h.allowedOrigins) == 0 {
		return true
	}
	for _, o := range h.allowedOrigins {
		if o == "*" || o == origin {
			return true
		}
	}
	return false
}

func (h *Handler) GetTraces(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	endpoint := r.URL.Query().Get("endpoint")
	model := r.URL.Query().Get("model")
	status := r.URL.Query().Get("status")
	traces := h.store.SearchTraces(q, endpoint, model, status)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(traces)
}

func (h *Handler) CompareTraces(w http.ResponseWriter, r *http.Request) {
	a := r.URL.Query().Get("a")
	b := r.URL.Query().Get("b")
	if a == "" || b == "" {
		http.Error(w, `{"error":"a and b required"}`, http.StatusBadRequest)
		return
	}
	result, err := h.store.Compare(a, b)
	if err != nil {
		http.Error(w, `{"error":"trace not found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (h *Handler) GetTapRecords(w http.ResponseWriter, r *http.Request) {
	records := TapRecordsFromStore(h.store)
	out := make([]json.RawMessage, 0, len(records))
	for _, rec := range records {
		out = append(out, rec)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (h *Handler) ExportTrace(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	trace := h.store.GetTrace(id)
	if trace == nil {
		http.Error(w, `{"error":"trace not found"}`, http.StatusNotFound)
		return
	}
	format := r.URL.Query().Get("format")
	if format == "html" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(renderTraceHTML(trace)))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(trace)
}

func (h *Handler) GetConfig(w http.ResponseWriter, r *http.Request) {
	if !h.allowConfig(r) || h.cfgMgr == nil {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"config":  h.cfgMgr.Snapshot(),
		"schema":  config.Schema(),
	})
}

func (h *Handler) PutConfig(w http.ResponseWriter, r *http.Request) {
	if !h.allowConfig(r) || h.cfgMgr == nil {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	var body struct {
		Patch   map[string]interface{} `json:"patch"`
		Restart bool                   `json:"restart"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	restartNeeded, err := h.cfgMgr.Apply(body.Patch)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}
	if body.Restart || r.URL.Query().Get("restart") == "true" {
		if h.restart != nil {
			_ = h.restart.ScheduleRestart()
		}
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":              true,
			"restarting":      true,
			"requires_restart": restartNeeded,
		})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":               true,
		"requires_restart": restartNeeded,
	})
}

func (h *Handler) PostRestart(w http.ResponseWriter, r *http.Request) {
	if !h.allowConfig(r) || h.restart == nil {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	_ = h.restart.ScheduleRestart()
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"restarting": true})
}

func renderTraceHTML(t *Trace) string {
	b, _ := json.MarshalIndent(t, "", "  ")
	return `<!DOCTYPE html><html><head><meta charset="utf-8"><title>Trace ` + t.ID + `</title></head><body><pre>` + string(b) + `</pre></body></html>`
}

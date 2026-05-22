package handler

import (
	"encoding/json"
	"net/http"

	"github.com/chaojimct/cli-agent-gateway/internal/cursor"
)

// HealthHandler handles GET /healthz.
type HealthHandler struct {
	version string
	runner  *cursor.Runner
}

// NewHealthHandler creates a new health handler.
func NewHealthHandler(version string, runner *cursor.Runner) *HealthHandler {
	return &HealthHandler{version: version, runner: runner}
}

// ServeHTTP returns the health status.
func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ready := h.runner != nil && h.runner.IsReady()
	status := "ok"
	if !ready {
		status = "starting"
	}
	resp := map[string]interface{}{
		"status":  status,
		"ready":   ready,
		"version": h.version,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

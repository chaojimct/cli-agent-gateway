package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/chaojimct/cli-agent-gateway/internal/cursor"
)

// ModelsHandler handles GET /v1/models.
type ModelsHandler struct {
	runner *cursor.Runner
	logger *slog.Logger
}

type modelEntry struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// NewModelsHandler creates a new models handler.
func NewModelsHandler(runner *cursor.Runner, logger *slog.Logger) *ModelsHandler {
	return &ModelsHandler{runner: runner, logger: logger}
}

// ServeHTTP returns the list of available models.
func (h *ModelsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.runner != nil && !h.runner.IsReady() {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "agent discovery in progress")
		return
	}

	var data []modelEntry
	if h.runner != nil {
		models, err := h.runner.ListModels(r.Context())
		if err != nil {
			if h.logger != nil {
				h.logger.Warn("failed to list models from acp agents, using fallback", "error", err)
			}
		} else {
			data = make([]modelEntry, len(models))
			for i, m := range models {
				data[i] = modelEntry{
					ID:      m.ID,
					Object:  "model",
					Created: time.Now().Unix(),
					OwnedBy: m.OwnedBy,
				}
			}
		}
	}

	if len(data) == 0 {
		staticModels := cursor.AvailableModels()
		data = make([]modelEntry, len(staticModels))
		for i, m := range staticModels {
			data[i] = modelEntry{
				ID:      "cursor/" + m.ID,
				Object:  "model",
				Created: time.Now().Unix(),
				OwnedBy: m.OwnedBy,
			}
		}
	}

	writeModels(w, data)
}

func writeModels(w http.ResponseWriter, data []modelEntry) {
	resp := map[string]interface{}{
		"object": "list",
		"data":   data,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

package handler

import (
	"encoding/json"
	"net/http"
	"runtime"
	"time"

	"github.com/chaojimct/cli-agent-gateway/internal/agent"
	"github.com/chaojimct/cli-agent-gateway/internal/cursor"
)

var startTime = time.Now()

// StatusHandler handles GET /status — shows gateway and concurrency metrics.
type StatusHandler struct {
	runner *cursor.Runner
}

// NewStatusHandler creates a new status handler.
func NewStatusHandler(runner *cursor.Runner) *StatusHandler {
	return &StatusHandler{runner: runner}
}

// statusResponse is the JSON response for /status.
type statusResponse struct {
	Status   string                  `json:"status"`
	Ready    bool                    `json:"ready"`
	Uptime   string                  `json:"uptime"`
	UptimeMs int64                   `json:"uptime_ms"`
	Go       statusGo                `json:"go"`
	Runner   cursor.ConcurrencyStats `json:"runner"`
	ACP      statusACP               `json:"acp"`
}

type statusACP struct {
	SessionsActive      int              `json:"sessions_active"`
	Restarts            uint32           `json:"restarts"`
	Backend             string           `json:"backend"`
	RegistryLastRefresh string           `json:"registry_last_refresh,omitempty"`
	ModelCacheSize      int              `json:"models_cache_size"`
	ProbeFailures       uint64           `json:"probe_failures_total"`
	Agents              []statusACPAgent `json:"agents"`
}

type statusACPAgent struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Binary       string `json:"binary"`
	Transport    string `json:"transport"`
	ModelsSource string `json:"models_source"`
	Restarts     uint32 `json:"restarts"`
	ProbeOK      bool   `json:"probe_ok"`
	ActiveTurns  int    `json:"active_turns"`
}

type statusGo struct {
	Version      string `json:"version"`
	NumCPU       int    `json:"num_cpu"`
	NumGoroutine int    `json:"num_goroutine"`
	HeapAllocMB  float64 `json:"heap_alloc_mb"`
}

// ServeHTTP handles GET /status.
func (h *StatusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	ready := h.runner != nil && h.runner.IsReady()
	status := "ok"
	if !ready {
		status = "starting"
	}
	resp := statusResponse{
		Status:   status,
		Ready:    ready,
		Uptime:   time.Since(startTime).Round(time.Second).String(),
		UptimeMs: time.Since(startTime).Milliseconds(),
		Go: statusGo{
			Version:      runtime.Version(),
			NumCPU:       runtime.NumCPU(),
			NumGoroutine: runtime.NumGoroutine(),
			HeapAllocMB:  float64(m.HeapAlloc) / 1024 / 1024,
		},
		Runner: h.runner.Stats(),
		ACP: statusACP{
			SessionsActive: h.runner.ActiveACPSessions(),
			Restarts:       h.runner.ACPRestarts(),
			Backend:        "acp",
			ModelCacheSize: registryModelCacheSize(h.runner),
			ProbeFailures:  agent.ProbeFailures(),
			Agents:         acpAgents(h.runner),
		},
	}
	if ts := registryLastRefresh(h.runner); !ts.IsZero() {
		resp.ACP.RegistryLastRefresh = ts.UTC().Format(time.RFC3339)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func acpAgents(r *cursor.Runner) []statusACPAgent {
	if r == nil {
		return nil
	}
	profiles := r.Agents()
	out := make([]statusACPAgent, 0, len(profiles))
	for _, p := range profiles {
		if p == nil {
			continue
		}
		out = append(out, statusACPAgent{
			ID:           p.ID,
			Name:         p.Name,
			Binary:       p.Command(),
			Transport:    p.Transport,
			ModelsSource: p.ModelsSource,
			Restarts:     agentRestarts(r, p.ID),
			ProbeOK:      true,
		})
	}
	return out
}

func agentRestarts(r *cursor.Runner, agentID string) uint32 {
	if r == nil {
		return 0
	}
	return r.AgentRestarts(agentID)
}

func registryLastRefresh(r *cursor.Runner) time.Time {
	if r == nil {
		return time.Time{}
	}
	reg := r.Registry()
	if reg == nil {
		return time.Time{}
	}
	return reg.LastRefreshAt()
}

func registryModelCacheSize(r *cursor.Runner) int {
	if r == nil {
		return 0
	}
	reg := r.Registry()
	if reg == nil {
		return 0
	}
	return reg.ModelCacheSize()
}

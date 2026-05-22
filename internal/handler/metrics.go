package handler

import (
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/user/cursor-gateway/internal/agent"
	"github.com/user/cursor-gateway/internal/cursor"
	"github.com/user/cursor-gateway/internal/runner"
)

var (
	metricsRequests  atomic.Uint64
	metricsToolCalls atomic.Uint64
)

// IncRequestMetrics increments API request counter.
func IncRequestMetrics() {
	metricsRequests.Add(1)
}

// IncToolCallMetrics increments tool call counter.
func IncToolCallMetrics() {
	metricsToolCalls.Add(1)
}

// MetricsHandler serves Prometheus text metrics at GET /metrics.
type MetricsHandler struct {
	runner *cursor.Runner
}

func NewMetricsHandler(runner *cursor.Runner) *MetricsHandler {
	return &MetricsHandler{runner: runner}
}

func (h *MetricsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	stats := h.runner.Stats()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write([]byte("# HELP gateway_requests_total Total HTTP API requests observed.\n"))
	_, _ = w.Write([]byte("# TYPE gateway_requests_total counter\n"))
	_, _ = w.Write([]byte("gateway_requests_total " + itoa(metricsRequests.Load()) + "\n"))
	_, _ = w.Write([]byte("# HELP gateway_tool_calls_total Tool calls emitted to clients.\n"))
	_, _ = w.Write([]byte("# TYPE gateway_tool_calls_total counter\n"))
	_, _ = w.Write([]byte("gateway_tool_calls_total " + itoa(metricsToolCalls.Load()) + "\n"))
	_, _ = w.Write([]byte("# HELP gateway_acp_sessions_active Active conversation session mappings.\n"))
	_, _ = w.Write([]byte("# TYPE gateway_acp_sessions_active gauge\n"))
	_, _ = w.Write([]byte("gateway_acp_sessions_active " + itoa(uint64(h.runner.ActiveACPSessions())) + "\n"))
	_, _ = w.Write([]byte("# HELP gateway_acp_restarts_total ACP process restarts.\n"))
	_, _ = w.Write([]byte("# TYPE gateway_acp_restarts_total counter\n"))
	_, _ = w.Write([]byte("gateway_acp_restarts_total " + itoa(uint64(h.runner.ACPRestarts())) + "\n"))
	_, _ = w.Write([]byte("# HELP gateway_runner_active In-flight ACP turns.\n"))
	_, _ = w.Write([]byte("# TYPE gateway_runner_active gauge\n"))
	_, _ = w.Write([]byte("gateway_runner_active " + itoa(uint64(stats.Active)) + "\n"))
	_, _ = w.Write([]byte("# HELP gateway_runner_queued Queued ACP turns waiting for capacity.\n"))
	_, _ = w.Write([]byte("# TYPE gateway_runner_queued gauge\n"))
	_, _ = w.Write([]byte("gateway_runner_queued " + itoa(uint64(stats.Queued)) + "\n"))
	_, _ = w.Write([]byte("# HELP gateway_uptime_seconds Process uptime.\n"))
	_, _ = w.Write([]byte("# TYPE gateway_uptime_seconds gauge\n"))
	_, _ = w.Write([]byte("gateway_uptime_seconds " + formatFloat(time.Since(startTime).Seconds()) + "\n"))
	_, _ = w.Write([]byte("# HELP gateway_agent_probe_failures_total Failed ACP agent probes during discovery.\n"))
	_, _ = w.Write([]byte("# TYPE gateway_agent_probe_failures_total counter\n"))
	_, _ = w.Write([]byte("gateway_agent_probe_failures_total " + itoa(agent.ProbeFailures()) + "\n"))
	_, _ = w.Write([]byte("# HELP gateway_agent_requests_total ACP turns routed per agent.\n"))
	_, _ = w.Write([]byte("# TYPE gateway_agent_requests_total counter\n"))
	for agentID, count := range runner.AgentRequestCounts() {
		_, _ = w.Write([]byte("gateway_agent_requests_total{agent=\"" + agentID + "\"} " + itoa(count) + "\n"))
	}
}

func itoa(n uint64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', 0, 64)
}

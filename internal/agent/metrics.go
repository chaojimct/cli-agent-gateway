package agent

import "sync/atomic"

var probeFailures atomic.Uint64

// IncProbeFailure records a failed ACP agent probe during discovery.
func IncProbeFailure() {
	probeFailures.Add(1)
}

// ProbeFailures returns total failed probes since process start.
func ProbeFailures() uint64 {
	return probeFailures.Load()
}

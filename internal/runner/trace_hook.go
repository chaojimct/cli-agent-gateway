package runner

// TraceHook records ACP JSON-RPC traffic for an active gateway trace.
type TraceHook interface {
	RecordACP(traceID, direction, method, payload string)
}

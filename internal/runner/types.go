package runner

import (
	"sync/atomic"

	"github.com/user/cursor-gateway/internal/ir"
)

// CancelReason explains why a turn was cancelled.
type CancelReason int

const (
	CancelUser CancelReason = iota
	CancelTimeout
	CancelNativeToolKill
	CancelProtocolError
)

// RunOpts contains options for an ACP turn.
type RunOpts struct {
	AgentID           string
	Model             string
	SessionID         string
	ConversationID    string
	Workspace         string
	TraceID           string
	MessageCount      int
	ClientTools       bool
	Mode              string
	IncrementalPrompt string // non-empty when reusing session: only new turn content
}

// RunResult streams IR events from an ACP turn.
type RunResult struct {
	Events   <-chan ir.Event
	ErrCh    <-chan error
	Cancel   func(reason CancelReason)
	StopFlag *atomic.Bool
}

func (r *RunResult) Stopped() bool {
	return r != nil && r.StopFlag != nil && r.StopFlag.Load()
}

// Stats for concurrency display.
type Stats struct {
	Active        int
	Queued        int
	MaxConcurrent int
}

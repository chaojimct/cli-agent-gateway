package ir

import "encoding/json"

// EventType classifies normalized gateway stream events.
type EventType int

const (
	EventSessionInit EventType = iota
	EventContentDelta
	EventReasoningDelta
	EventToolCall
	EventToolCallUpdate
	EventPlan
	EventModeUpdate
	EventTraceOnly
	EventDone
	EventError
)

// ToolCallState tracks an in-flight ACP tool call.
type ToolCallState struct {
	ID       string
	Name     string
	Title    string
	Args     json.RawMessage
	Status   string
	Native   bool // cursor native tool (not client OpenAI tool)
}

// Usage token stats (OpenAI-shaped).
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// Event is the internal representation after ACP translation.
type Event struct {
	Type       EventType
	Text       string
	ToolCall   *ToolCallState
	Usage      *Usage
	StopReason string
	SessionID  string
	ModeID     string
	Err        error
}

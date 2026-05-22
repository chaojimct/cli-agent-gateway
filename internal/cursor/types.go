package cursor

import (
	"encoding/json"
	"strings"
)

// Event types from cursor-agent stream-json output.
const (
	EventSystem    = "system"
	EventUser      = "user"
	EventAssistant = "assistant"
	EventToolCall  = "tool_call"
	EventResult    = "result"
)

// Subtypes.
const (
	SubtypeInit      = "init"
	SubtypeSuccess   = "success"
	SubtypeError     = "error"
	SubtypeStarted   = "started"
	SubtypeCompleted = "completed"
)

// CursorEvent represents any NDJSON event from cursor-agent.
type CursorEvent struct {
	Type        string          `json:"type"`
	Subtype     string          `json:"subtype,omitempty"`
	SessionID   string          `json:"session_id,omitempty"`
	Model       string          `json:"model,omitempty"`
	Message     *Message        `json:"message,omitempty"`
	CallID      string          `json:"call_id,omitempty"`
	ModelCallID string          `json:"model_call_id,omitempty"`
	TimestampMs int64           `json:"timestamp_ms,omitempty"`
	DurationMs  int64           `json:"duration_ms,omitempty"`
	DurationAPIMs int64         `json:"duration_api_ms,omitempty"`
	IsError     bool            `json:"is_error,omitempty"`
	Result      string          `json:"result,omitempty"`
	RequestID   string          `json:"request_id,omitempty"`
	Usage       *Usage          `json:"usage,omitempty"`
	ToolCall    json.RawMessage `json:"tool_call,omitempty"`
}

// Message represents a chat message in cursor-agent format.
type Message struct {
	Role    string        `json:"role"`
	Content []ContentPart `json:"content"`
}

// ContentPart represents a single content block.
type ContentPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Usage represents token usage statistics.
type Usage struct {
	InputTokens  int `json:"inputTokens"`
	OutputTokens int `json:"outputTokens"`
	CacheReadTokens  int `json:"cacheReadTokens"`
	CacheWriteTokens int `json:"cacheWriteTokens"`
}

// RunOpts contains options for a cursor-agent invocation.
type RunOpts struct {
	AgentID           string
	Model             string
	DisplayModel      string
	SessionID         string
	ConversationID    string
	IncrementalPrompt string
	MessageCount      int
	Workspace         string
	TraceID           string
	CursorMessages    []Message
	ClientTools       bool
}

// ToolCallInfo represents parsed tool call information.
type ToolCallInfo struct {
	CallID      string
	ToolType    string // e.g., "shellToolCall", "editToolCall"
	Subtype     string // "started" or "completed"
	Description string
	Args        json.RawMessage
	Result      json.RawMessage
}

// Phase represents the current phase of a Cursor Agent response stream.
// Cursor Agent events follow this pattern:
//   1. assistant deltas (thinking) — model reasoning, not meant for user
//   2. tool_call events — tool invocations (may be 0 or more rounds)
//   3. assistant deltas (output) — the actual response to the user
//   4. result — final completion
//
// If no tool_call occurs, all assistant events are output (no thinking phase).
type Phase int

const (
	PhaseInitial Phase = iota // No assistant events seen yet
	PhaseThinking            // Assistant deltas before first tool_call → skip in SSE
	PhaseOutput              // Assistant deltas after first tool_call (or all, if no tool_call) → emit in SSE
)

// PhaseTracker tracks which phase of the response stream we're in.
// It determines whether assistant events are thinking (reasoning_content) or output (content).
//
// Strategy: events in PhaseInitial are buffered as pending. When a tool_call arrives,
// pending events were thinking. When a final assistant event arrives (no tool_call),
// pending events were output.
//
// For streaming: the handler buffers SSE chunks during PhaseInitial and flushes them
// with the correct field when the phase is resolved.
type PhaseTracker struct {
	phase     Phase
	hasTools  bool   // whether any tool_call was seen
	thinking  string // accumulated thinking text
	output    string // accumulated output text
}

// NewPhaseTracker creates a new phase tracker.
func NewPhaseTracker() *PhaseTracker {
	return &PhaseTracker{phase: PhaseInitial}
}

// OnToolCall records that a tool_call was seen, transitioning to output phase.
func (pt *PhaseTracker) OnToolCall() {
	if !pt.hasTools {
		pt.hasTools = true
		pt.phase = PhaseOutput
	}
}

// IsPending returns true if we're still in PhaseInitial (don't know yet).
func (pt *PhaseTracker) IsPending() bool {
	return pt.phase == PhaseInitial
}

// IsOutput returns true if we're in the output phase.
func (pt *PhaseTracker) IsOutput() bool {
	return pt.phase == PhaseOutput
}

// OnAssistantDelta is called for each assistant delta event.
// Returns: 0 = pending (unknown), 1 = thinking, 2 = output
func (pt *PhaseTracker) OnAssistantDelta(text string) int {
	if pt.phase == PhaseInitial {
		return 0 // pending
	}
	if pt.phase == PhaseThinking {
		pt.thinking += text
		return 1 // thinking → reasoning_content
	}
	// PhaseOutput
	pt.output += text
	return 2 // output → content
}

// Phase returns the current phase (for debugging).
func (pt *PhaseTracker) Phase() Phase {
	return pt.phase
}

// ResolvePending resolves PhaseInitial based on whether tool_call was seen.
// Returns true if pending events were thinking (tool_call was seen).
func (pt *PhaseTracker) ResolvePending() bool {
	if pt.phase != PhaseInitial {
		return false
	}
	if pt.hasTools {
		// tool_call was seen → pending was thinking
		pt.phase = PhaseThinking
		return true
	}
	// No tool_call → pending was output
	pt.phase = PhaseOutput
	return false
}

// OnAssistantFinal is called for the final (non-delta) assistant event.
// Returns true if this is output.
func (pt *PhaseTracker) OnAssistantFinal(text string) bool {
	if pt.phase == PhaseInitial {
		// Resolve: no tool_call → everything is output
		wasThinking := pt.ResolvePending()
		pt.output = text
		return !wasThinking
	}
	if pt.phase == PhaseThinking {
		pt.thinking = text
		return false
	}
	// PhaseOutput
	pt.output = text
	return true
}

// Output returns the accumulated output text.
func (pt *PhaseTracker) Output() string {
	return pt.output
}

// Thinking returns the accumulated thinking text.
func (pt *PhaseTracker) Thinking() string {
	return pt.thinking
}

// HasTools returns whether any tool_call was observed.
func (pt *PhaseTracker) HasTools() bool {
	return pt.hasTools
}

// ExtractText returns concatenated text from a message.
func ExtractText(msg *Message) string {
	if msg == nil {
		return ""
	}
	var b strings.Builder
	for _, p := range msg.Content {
		if p.Type == "text" || p.Type == "" {
			b.WriteString(p.Text)
		}
	}
	return b.String()
}

// IsDelta reports assistant streaming delta (legacy; unused in ACP path).
func (e *CursorEvent) IsDelta() bool {
	return e != nil && e.Type == EventAssistant && e.Message != nil
}

func (e *CursorEvent) IsFinal() bool {
	return e != nil && e.Type == EventAssistant && e.Subtype == SubtypeCompleted
}

func (e *CursorEvent) IsDone() bool {
	return e != nil && e.Type == EventResult
}

func (e *CursorEvent) IsToolCall() bool {
	return e != nil && e.Type == EventToolCall
}

package translator

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/chaojimct/cli-agent-gateway/internal/cursor"
)

// OpenAIChatRequest represents an OpenAI Chat Completions request.
type OpenAIChatRequest struct {
	Model       string              `json:"model"`
	Messages    []OpenAIChatMessage `json:"messages"`
	Stream      bool                `json:"stream"`
	Tools       []OpenAITool        `json:"tools,omitempty"`
	ToolChoice  json.RawMessage     `json:"tool_choice,omitempty"`
	Temperature *float64            `json:"temperature,omitempty"`
	MaxTokens   *int                `json:"max_tokens,omitempty"`
	Stop        json.RawMessage     `json:"stop,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// HasClientTools returns true when the client supplied tool definitions (e.g. OpenCode skills).
func (r *OpenAIChatRequest) HasClientTools() bool {
	return len(r.Tools) > 0
}

// BuildPrompt converts messages into a prompt (alias for BuildPromptWithTools).
func BuildPrompt(messages []OpenAIChatMessage) string {
	return BuildPromptWithTools(messages, nil)
}

// ExtractSystemPrompt extracts system messages and returns the remaining messages.
func ExtractSystemPrompt(messages []ChatMessage) (string, []ChatMessage) {
	var systemParts []string
	var remaining []ChatMessage

	for _, msg := range messages {
		if msg.Role == "system" || msg.Role == "developer" {
			systemParts = append(systemParts, msg.Content)
		} else {
			remaining = append(remaining, msg)
		}
	}

	return strings.Join(systemParts, "\n\n"), remaining
}

// ConvertMessages converts cursor messages to OpenAI chat messages.
func ConvertMessages(messages []cursor.Message) []ChatMessage {
	var result []ChatMessage
	for _, msg := range messages {
		result = append(result, ChatMessage{
			Role:    msg.Role,
			Content: cursor.ExtractText(&msg),
		})
	}
	return result
}

// OpenAIChatSSEWriter writes OpenAI Chat Completions SSE events.
type OpenAIChatSSEWriter struct {
	w             http.ResponseWriter
	flusher       http.Flusher
	id            string
	model         string
	created       int64
	contentPrefix []byte // pre-built JSON prefix for content deltas
	reasonPrefix  []byte // pre-built JSON prefix for reasoning deltas
	donePrefix    []byte // pre-built JSON prefix for done chunk
	suffix        []byte // common suffix: }}]}\n\n
}

// NewOpenAIChatSSEWriter creates a new SSE writer for OpenAI Chat format.
func NewOpenAIChatSSEWriter(w http.ResponseWriter, model string) *OpenAIChatSSEWriter {
	flusher, _ := w.(http.Flusher)
	id := "chatcmpl-" + uuid.New().String()[:8]
	created := time.Now().Unix()

	// Pre-build static JSON prefixes to avoid re-serializing id/model/created on every chunk.
	// Format: {"id":"...","object":"chat.completion.chunk","created":N,"model":"...","choices":[{"index":0,"delta":{"
	base := fmt.Sprintf(`{"id":"%s","object":"chat.completion.chunk","created":%d,"model":"%s","choices":[{"index":0,"delta":{`, id, created, model)

	contentPrefix := []byte(base + `"content":"`)
	reasonPrefix := []byte(base + `"reasoning_content":"`)
	// done chunk: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}
	donePrefix := []byte(base + `},"finish_reason":"stop"}]}`)

	return &OpenAIChatSSEWriter{
		w:             w,
		flusher:       flusher,
		id:            id,
		model:         model,
		created:       created,
		contentPrefix: contentPrefix,
		reasonPrefix:  reasonPrefix,
		donePrefix:    donePrefix,
		suffix:        []byte(`"}}]}\n\n`),
	}
}

// WriteHeaders writes the SSE headers.
func (w *OpenAIChatSSEWriter) WriteHeaders() {
	w.w.Header().Set("Content-Type", "text/event-stream")
	w.w.Header().Set("Cache-Control", "no-cache")
	w.w.Header().Set("Connection", "keep-alive")
	w.w.Header().Set("X-Accel-Buffering", "no")
	w.w.Header().Set("X-Content-Type-Options", "nosniff")
	w.w.Header().Set("X-Request-Id", w.id)
	w.w.WriteHeader(http.StatusOK)
	// Flush immediately to ensure headers are sent
	if w.flusher != nil {
		w.flusher.Flush()
	}
	// OpenAI-compatible clients (e.g. OpenCode) expect an initial role chunk.
	_ = w.WriteRoleChunk()
}

// WriteRoleChunk emits the first SSE chunk with assistant role (OpenAI streaming convention).
func (w *OpenAIChatSSEWriter) WriteRoleChunk() error {
	chunk := map[string]interface{}{
		"id":      w.id,
		"object":  "chat.completion.chunk",
		"created": w.created,
		"model":   w.model,
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"delta": map[string]string{"role": "assistant"},
			},
		},
	}
	return w.writeSSE(chunk)
}

// WriteDelta writes a streaming text delta.
func (w *OpenAIChatSSEWriter) WriteDelta(text string) error {
	return w.writeDeltaFast(w.contentPrefix, text)
}

// WriteReasoningDelta writes a streaming reasoning/thinking delta.
// Uses the extended reasoning_content field (OpenAI o1/o3 style).
func (w *OpenAIChatSSEWriter) WriteReasoningDelta(text string) error {
	return w.writeDeltaFast(w.reasonPrefix, text)
}

// WriteDone writes the final chunk with finish_reason stop and [DONE].
func (w *OpenAIChatSSEWriter) WriteDone(usage *cursor.Usage) error {
	return w.writeFinish("stop", usage)
}

// WriteDoneToolCalls writes the final chunk when the model chose client tools.
func (w *OpenAIChatSSEWriter) WriteDoneToolCalls(usage *cursor.Usage) error {
	return w.writeFinish("tool_calls", usage)
}

func (w *OpenAIChatSSEWriter) writeFinish(reason string, usage *cursor.Usage) error {
	chunk := map[string]interface{}{
		"id":      w.id,
		"object":  "chat.completion.chunk",
		"created": w.created,
		"model":   w.model,
		"choices": []map[string]interface{}{
			{
				"index":         0,
				"delta":         map[string]interface{}{},
				"finish_reason": reason,
			},
		},
	}
	if usage != nil {
		chunk["usage"] = map[string]int{
			"prompt_tokens":     usage.InputTokens,
			"completion_tokens": usage.OutputTokens,
			"total_tokens":      usage.InputTokens + usage.OutputTokens,
		}
	}
	if err := w.writeSSE(chunk); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w.w, "data: [DONE]\n\n")
	if w.flusher != nil {
		w.flusher.Flush()
	}
	return err
}

// WriteToolCalls emits tool_calls in OpenAI streaming format.
func (w *OpenAIChatSSEWriter) WriteToolCalls(calls []OpenAIToolCall) error {
	toolDeltas := make([]map[string]interface{}, len(calls))
	for i, tc := range calls {
		toolDeltas[i] = map[string]interface{}{
			"index": i,
			"id":    tc.ID,
			"type":  tc.Type,
			"function": map[string]string{
				"name":      tc.Function.Name,
				"arguments": tc.Function.Arguments,
			},
		}
	}
	chunk := map[string]interface{}{
		"id":      w.id,
		"object":  "chat.completion.chunk",
		"created": w.created,
		"model":   w.model,
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"delta": map[string]interface{}{
					"tool_calls": toolDeltas,
				},
			},
		},
	}
	return w.writeSSE(chunk)
}

// WriteError writes an error event.
func (w *OpenAIChatSSEWriter) WriteError(message string) error {
	errMsg := "\n\n[ERROR: " + message + "]"
	err := w.writeDeltaFast(w.contentPrefix, errMsg)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w.w, "data: [DONE]\n\n")
	if w.flusher != nil {
		w.flusher.Flush()
	}
	return err
}

// writeDeltaFast writes an SSE chunk by escaping text and concatenating with pre-built prefix/suffix.
// Avoids full json.Marshal of the entire chunk on every call.
// prefix ends with opening quote (e.g. ..."content":") and suffix starts with closing quote.
func (w *OpenAIChatSSEWriter) writeDeltaFast(prefix []byte, text string) error {
	escaped, err := json.Marshal(text) // returns "..." with JSON-safe escaping
	if err != nil {
		return err
	}
	// escaped = "\"hello\\nworld\"" → strip outer quotes → hello\nworld (escaped)
	inner := escaped[1 : len(escaped)-1]
	w.w.Write([]byte("data: "))
	w.w.Write(prefix)
	w.w.Write(inner)
	w.w.Write([]byte(`"}}]}`))
	w.w.Write([]byte("\n\n"))
	if w.flusher != nil {
		w.flusher.Flush()
	}
	return nil
}

func (w *OpenAIChatSSEWriter) writeSSE(data interface{}) error {
	encoded, err := json.Marshal(data)
	if err != nil {
		return err
	}
	// Use direct write for better performance
	w.w.Write([]byte("data: "))
	w.w.Write(encoded)
	w.w.Write([]byte("\n\n"))
	if w.flusher != nil {
		w.flusher.Flush()
	}
	return nil
}

// OpenAIChatSyncResponse represents a non-streaming OpenAI Chat response.
type OpenAIChatSyncResponse struct {
	ID      string           `json:"id"`
	Object  string           `json:"object"`
	Created int64            `json:"created"`
	Model   string           `json:"model"`
	Choices []OpenAIChoice   `json:"choices"`
	Usage   *OpenAIUsage     `json:"usage,omitempty"`
}

type OpenAIChoice struct {
	Index        int            `json:"index"`
	Message      OpenAIMessage  `json:"message"`
	FinishReason string         `json:"finish_reason"`
}

type OpenAIMessage struct {
	Role       string           `json:"role"`
	Content    string             `json:"content,omitempty"`
	ToolCalls  []OpenAIToolCall   `json:"tool_calls,omitempty"`
}

type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// BuildSyncResponse builds a non-streaming response.
func BuildSyncResponse(text string, model string, usage *cursor.Usage) *OpenAIChatSyncResponse {
	resp := &OpenAIChatSyncResponse{
		ID:      "chatcmpl-" + uuid.New().String()[:8],
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []OpenAIChoice{
			{
				Index: 0,
				Message: OpenAIMessage{
					Role:    "assistant",
					Content: text,
				},
				FinishReason: "stop",
			},
		},
	}
	if usage != nil {
		resp.Usage = &OpenAIUsage{
			PromptTokens:     usage.InputTokens,
			CompletionTokens: usage.OutputTokens,
			TotalTokens:      usage.InputTokens + usage.OutputTokens,
		}
	}
	return resp
}

// BuildSyncResponseWithToolCalls builds a non-streaming response with tool_calls.
func BuildSyncResponseWithToolCalls(calls []OpenAIToolCall, model string, usage *cursor.Usage) *OpenAIChatSyncResponse {
	resp := &OpenAIChatSyncResponse{
		ID:      "chatcmpl-" + uuid.New().String()[:8],
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []OpenAIChoice{
			{
				Index: 0,
				Message: OpenAIMessage{
					Role:      "assistant",
					ToolCalls: calls,
				},
				FinishReason: "tool_calls",
			},
		},
	}
	if usage != nil {
		resp.Usage = &OpenAIUsage{
			PromptTokens:     usage.InputTokens,
			CompletionTokens: usage.OutputTokens,
			TotalTokens:      usage.InputTokens + usage.OutputTokens,
		}
	}
	return resp
}

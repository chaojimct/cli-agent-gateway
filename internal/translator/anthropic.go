package translator

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/chaojimct/cli-agent-gateway/internal/cursor"
)

// AnthropicRequest represents an Anthropic Messages API request.
type AnthropicRequest struct {
	Model     string              `json:"model"`
	Messages  []AnthropicMessage  `json:"messages"`
	MaxTokens int                 `json:"max_tokens"`
	Stream    bool                `json:"stream"`
	System    json.RawMessage     `json:"system,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// AnthropicMessage represents a message in Anthropic format.
type AnthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// BuildAnthropicPrompt converts Anthropic messages into a single prompt string.
func BuildAnthropicPrompt(system json.RawMessage, messages []AnthropicMessage) string {
	var b strings.Builder

	// Handle system prompt (can be string or array of content blocks)
	if len(system) > 0 {
		var sysStr string
		if err := json.Unmarshal(system, &sysStr); err == nil {
			b.WriteString("[System]\n")
			b.WriteString(sysStr)
			b.WriteString("\n\n")
		} else {
			// Try as array of content blocks
			var blocks []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			if err := json.Unmarshal(system, &blocks); err == nil {
				b.WriteString("[System]\n")
				for _, block := range blocks {
					if block.Type == "text" {
						b.WriteString(block.Text)
					}
				}
				b.WriteString("\n\n")
			}
		}
	}

	for _, msg := range messages {
		switch msg.Role {
		case "user":
			b.WriteString("[User]\n")
		case "assistant":
			b.WriteString("[Assistant]\n")
		}
		b.WriteString(msg.Content)
		b.WriteString("\n\n")
	}

	return strings.TrimSpace(b.String())
}

// AnthropicSSEWriter writes Anthropic Messages SSE events.
type AnthropicSSEWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
	id      string
	model   string
}

// NewAnthropicSSEWriter creates a new SSE writer for Anthropic format.
func NewAnthropicSSEWriter(w http.ResponseWriter, model string) *AnthropicSSEWriter {
	flusher, _ := w.(http.Flusher)
	return &AnthropicSSEWriter{
		w:       w,
		flusher: flusher,
		id:      "msg_" + uuid.New().String()[:8],
		model:   model,
	}
}

// WriteHeaders writes the SSE headers.
func (w *AnthropicSSEWriter) WriteHeaders() {
	w.w.Header().Set("Content-Type", "text/event-stream")
	w.w.Header().Set("Cache-Control", "no-cache")
	w.w.Header().Set("Connection", "keep-alive")
	w.w.WriteHeader(http.StatusOK)
}

// WriteMessageStart writes the message_start event.
func (w *AnthropicSSEWriter) WriteMessageStart() error {
	data := map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id":            w.id,
			"type":          "message",
			"role":          "assistant",
			"content":       []interface{}{},
			"model":         w.model,
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]int{
				"input_tokens":  0,
				"output_tokens": 0,
			},
		},
	}
	return w.writeSSE("message_start", data)
}

// WriteContentBlockStart writes the content_block_start event.
func (w *AnthropicSSEWriter) WriteContentBlockStart() error {
	data := map[string]interface{}{
		"type":         "content_block_start",
		"index":        0,
		"content_block": map[string]string{"type": "text", "text": ""},
	}
	return w.writeSSE("content_block_start", data)
}

// WriteDelta writes a content_block_delta event.
func (w *AnthropicSSEWriter) WriteDelta(text string) error {
	data := map[string]interface{}{
		"type":  "content_block_delta",
		"index": 0,
		"delta": map[string]string{
			"type": "text_delta",
			"text": text,
		},
	}
	return w.writeSSE("content_block_delta", data)
}

// WriteContentBlockStop writes the content_block_stop event.
func (w *AnthropicSSEWriter) WriteContentBlockStop() error {
	data := map[string]interface{}{
		"type":  "content_block_stop",
		"index": 0,
	}
	return w.writeSSE("content_block_stop", data)
}

// WriteMessageDelta writes the message_delta event with stop reason.
func (w *AnthropicSSEWriter) WriteMessageDelta(usage *cursor.Usage) error {
	data := map[string]interface{}{
		"type": "message_delta",
		"delta": map[string]string{
			"stop_reason": "end_turn",
		},
		"usage": map[string]int{
			"output_tokens": 0,
		},
	}
	if usage != nil {
		data["usage"] = map[string]int{
			"output_tokens": usage.OutputTokens,
		}
	}
	return w.writeSSE("message_delta", data)
}

// WriteMessageStop writes the message_stop event.
func (w *AnthropicSSEWriter) WriteMessageStop() error {
	return w.writeSSE("message_stop", map[string]string{"type": "message_stop"})
}

// WriteDone writes the complete stop sequence.
func (w *AnthropicSSEWriter) WriteDone(usage *cursor.Usage) error {
	if err := w.WriteContentBlockStop(); err != nil {
		return err
	}
	if err := w.WriteMessageDelta(usage); err != nil {
		return err
	}
	return w.WriteMessageStop()
}

// WriteError writes an error event.
func (w *AnthropicSSEWriter) WriteError(message string) error {
	data := map[string]interface{}{
		"type": "error",
		"error": map[string]string{
			"type":    "api_error",
			"message": message,
		},
	}
	return w.writeSSE("error", data)
}

func (w *AnthropicSSEWriter) writeSSE(event string, data interface{}) error {
	encoded, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if event != "" {
		fmt.Fprintf(w.w, "event: %s\n", event)
	}
	_, err = fmt.Fprintf(w.w, "data: %s\n\n", encoded)
	if w.flusher != nil {
		w.flusher.Flush()
	}
	return err
}

// AnthropicSyncResponse represents a non-streaming Anthropic response.
type AnthropicSyncResponse struct {
	ID           string               `json:"id"`
	Type         string               `json:"type"`
	Role         string               `json:"role"`
	Content      []AnthropicContent   `json:"content"`
	Model        string               `json:"model"`
	StopReason   string               `json:"stop_reason"`
	StopSequence *string              `json:"stop_sequence"`
	Usage        *AnthropicUsage      `json:"usage"`
}

type AnthropicContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// BuildAnthropicSyncResponse builds a non-streaming Anthropic response.
func BuildAnthropicSyncResponse(text string, model string, usage *cursor.Usage) *AnthropicSyncResponse {
	resp := &AnthropicSyncResponse{
		ID:   "msg_" + uuid.New().String()[:8],
		Type: "message",
		Role: "assistant",
		Content: []AnthropicContent{
			{Type: "text", Text: text},
		},
		Model:      model,
		StopReason: "end_turn",
	}
	if usage != nil {
		resp.Usage = &AnthropicUsage{
			InputTokens:  usage.InputTokens,
			OutputTokens: usage.OutputTokens,
		}
	}
	return resp
}

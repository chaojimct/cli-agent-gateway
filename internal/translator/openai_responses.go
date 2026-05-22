package translator

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/chaojimct/cli-agent-gateway/internal/cursor"
)

// ResponsesRequest represents an OpenAI Responses API request.
type ResponsesRequest struct {
	Model              string          `json:"model"`
	Input              json.RawMessage `json:"input"`
	Stream             bool            `json:"stream"`
	PreviousResponseID string          `json:"previous_response_id,omitempty"`
}

// ParseResponsesInput parses the input field which can be a string or array.
func ParseResponsesInput(input json.RawMessage) (string, []ChatMessage) {
	// Try as string first
	var str string
	if err := json.Unmarshal(input, &str); err == nil {
		return "", []ChatMessage{{Role: "user", Content: str}}
	}

	// Try as array of input items
	var items []struct {
		Type    string `json:"type"`
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(input, &items); err == nil {
		var messages []ChatMessage
		var systemPrompt string
		for _, item := range items {
			if item.Type == "message" {
				if item.Role == "system" {
					systemPrompt = item.Content
				} else {
					messages = append(messages, ChatMessage{
						Role:    item.Role,
						Content: item.Content,
					})
				}
			}
		}
		return systemPrompt, messages
	}

	return "", []ChatMessage{{Role: "user", Content: string(input)}}
}

// ResponsesSSEWriter writes OpenAI Responses SSE events.
type ResponsesSSEWriter struct {
	w        http.ResponseWriter
	flusher  http.Flusher
	respID   string
	itemID   string
	model    string
	fullText string
}

// NewResponsesSSEWriter creates a new SSE writer for Responses format.
func NewResponsesSSEWriter(w http.ResponseWriter, model string) *ResponsesSSEWriter {
	flusher, _ := w.(http.Flusher)
	return &ResponsesSSEWriter{
		w:      w,
		flusher: flusher,
		respID: "resp_" + uuid.New().String()[:8],
		itemID: "msg_" + uuid.New().String()[:8],
		model:  model,
	}
}

// WriteHeaders writes the SSE headers.
func (w *ResponsesSSEWriter) WriteHeaders() {
	w.w.Header().Set("Content-Type", "text/event-stream")
	w.w.Header().Set("Cache-Control", "no-cache")
	w.w.Header().Set("Connection", "keep-alive")
	w.w.WriteHeader(http.StatusOK)
}

// WriteCreated writes the response.created event.
func (w *ResponsesSSEWriter) WriteCreated() error {
	data := map[string]interface{}{
		"type": "response.created",
		"response": map[string]interface{}{
			"id":     w.respID,
			"object": "response",
			"status": "in_progress",
			"model":  w.model,
			"output": []interface{}{},
			"usage":  nil,
		},
	}
	return w.writeSSE(data)
}

// WriteOutputItemAdded writes the response.output_item.added event.
func (w *ResponsesSSEWriter) WriteOutputItemAdded() error {
	data := map[string]interface{}{
		"type":        "response.output_item.added",
		"output_index": 0,
		"item": map[string]interface{}{
			"id":      w.itemID,
			"type":    "message",
			"role":    "assistant",
			"content": []interface{}{},
			"status":  "in_progress",
		},
	}
	return w.writeSSE(data)
}

// WriteDelta writes a response.output_text.delta event.
func (w *ResponsesSSEWriter) WriteDelta(text string) error {
	w.fullText += text
	data := map[string]interface{}{
		"type":         "response.output_text.delta",
		"item_id":      w.itemID,
		"output_index": 0,
		"content_index": 0,
		"delta":        text,
	}
	return w.writeSSE(data)
}

// WriteTextDone writes the response.output_text.done event.
func (w *ResponsesSSEWriter) WriteTextDone() error {
	data := map[string]interface{}{
		"type":         "response.output_text.done",
		"item_id":      w.itemID,
		"output_index": 0,
		"content_index": 0,
		"text":         w.fullText,
	}
	return w.writeSSE(data)
}

// WriteOutputItemDone writes the response.output_item.done event.
func (w *ResponsesSSEWriter) WriteOutputItemDone() error {
	data := map[string]interface{}{
		"type":        "response.output_item.done",
		"output_index": 0,
		"item": map[string]interface{}{
			"id":      w.itemID,
			"type":    "message",
			"role":    "assistant",
			"content": []interface{}{
				map[string]interface{}{
					"type": "output_text",
					"text": w.fullText,
				},
			},
			"status": "completed",
		},
	}
	return w.writeSSE(data)
}

// WriteCompleted writes the response.completed event.
func (w *ResponsesSSEWriter) WriteCompleted(usage *cursor.Usage) error {
	var usageData interface{}
	if usage != nil {
		usageData = map[string]int{
			"input_tokens":  usage.InputTokens,
			"output_tokens": usage.OutputTokens,
			"total_tokens":  usage.InputTokens + usage.OutputTokens,
		}
	}

	data := map[string]interface{}{
		"type": "response.completed",
		"response": map[string]interface{}{
			"id":     w.respID,
			"object": "response",
			"status": "completed",
			"model":  w.model,
			"output": []interface{}{
				map[string]interface{}{
					"id":      w.itemID,
					"type":    "message",
					"role":    "assistant",
					"content": []interface{}{
						map[string]interface{}{
							"type": "output_text",
							"text": w.fullText,
						},
					},
					"status": "completed",
				},
			},
			"usage": usageData,
		},
	}
	return w.writeSSE(data)
}

// WriteDone writes the complete completion sequence.
func (w *ResponsesSSEWriter) WriteDone(usage *cursor.Usage) error {
	if err := w.WriteTextDone(); err != nil {
		return err
	}
	if err := w.WriteOutputItemDone(); err != nil {
		return err
	}
	return w.WriteCompleted(usage)
}

// WriteError writes an error event.
func (w *ResponsesSSEWriter) WriteError(message string) error {
	data := map[string]interface{}{
		"type": "response.failed",
		"response": map[string]interface{}{
			"id":     w.respID,
			"object": "response",
			"status": "failed",
			"error": map[string]string{
				"message": message,
				"type":    "server_error",
			},
		},
	}
	return w.writeSSE(data)
}

func (w *ResponsesSSEWriter) writeSSE(data interface{}) error {
	encoded, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w.w, "data: %s\n\n", encoded)
	if w.flusher != nil {
		w.flusher.Flush()
	}
	return err
}

// ResponsesSyncResponse represents a non-streaming Responses API response.
type ResponsesSyncResponse struct {
	ID     string              `json:"id"`
	Object string              `json:"object"`
	Status string              `json:"status"`
	Model  string              `json:"model"`
	Output []ResponsesOutput   `json:"output"`
	Usage  *ResponsesUsage     `json:"usage,omitempty"`
}

type ResponsesOutput struct {
	ID      string             `json:"id"`
	Type    string             `json:"type"`
	Role    string             `json:"role"`
	Content []ResponsesContent `json:"content"`
	Status  string             `json:"status"`
}

type ResponsesContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type ResponsesUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// BuildResponsesSyncResponse builds a non-streaming Responses API response.
func BuildResponsesSyncResponse(text string, model string, usage *cursor.Usage) *ResponsesSyncResponse {
	resp := &ResponsesSyncResponse{
		ID:     "resp_" + uuid.New().String()[:8],
		Object: "response",
		Status: "completed",
		Model:  model,
		Output: []ResponsesOutput{
			{
				ID:   "msg_" + uuid.New().String()[:8],
				Type: "message",
				Role: "assistant",
				Content: []ResponsesContent{
					{Type: "output_text", Text: text},
				},
				Status: "completed",
			},
		},
	}
	if usage != nil {
		resp.Usage = &ResponsesUsage{
			InputTokens:  usage.InputTokens,
			OutputTokens: usage.OutputTokens,
			TotalTokens:  usage.InputTokens + usage.OutputTokens,
		}
	}
	return resp
}

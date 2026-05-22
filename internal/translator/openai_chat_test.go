package translator

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIChatSSEDoneChunkValidJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	w := NewOpenAIChatSSEWriter(rec, "composer-2")
	w.WriteHeaders()
	_ = w.WriteDone(nil)

	body := rec.Body.String()
	var lastData string
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "data: ") && line != "data: [DONE]" {
			lastData = strings.TrimPrefix(line, "data: ")
		}
	}
	if lastData == "" {
		t.Fatal("no data chunk found")
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(lastData), &parsed); err != nil {
		t.Fatalf("invalid JSON in done chunk: %v\npayload: %s", err, lastData)
	}
	choices, ok := parsed["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		t.Fatal("missing choices")
	}
	choice := choices[0].(map[string]interface{})
	if choice["finish_reason"] != "stop" {
		t.Fatalf("finish_reason=%v", choice["finish_reason"])
	}
	delta, ok := choice["delta"].(map[string]interface{})
	if !ok {
		t.Fatalf("delta type %T", choice["delta"])
	}
	if len(delta) != 0 {
		t.Fatalf("expected empty delta object, got %v", delta)
	}
}

func TestOpenAIChatSSERoleChunkFirst(t *testing.T) {
	rec := httptest.NewRecorder()
	w := NewOpenAIChatSSEWriter(rec, "composer-2")
	w.WriteHeaders()

	body := rec.Body.String()
	idx := strings.Index(body, "data: ")
	if idx < 0 {
		t.Fatal("no SSE data")
	}
	end := strings.Index(body[idx:], "\n\n")
	first := strings.TrimPrefix(body[idx:idx+end], "data: ")
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(first), &parsed); err != nil {
		t.Fatalf("role chunk JSON: %v\n%s", err, first)
	}
	choices := parsed["choices"].([]interface{})
	choice := choices[0].(map[string]interface{})
	delta := choice["delta"].(map[string]interface{})
	if delta["role"] != "assistant" {
		t.Fatalf("role=%v", delta["role"])
	}
}

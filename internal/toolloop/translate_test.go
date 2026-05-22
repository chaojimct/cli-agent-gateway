package toolloop

import (
	"encoding/json"
	"testing"

	"github.com/chaojimct/cli-agent-gateway/internal/ir"
)

func TestTranslateToolCall(t *testing.T) {
	raw, _ := json.Marshal(map[string]interface{}{
		"sessionId": "s1",
		"update": map[string]interface{}{
			"sessionUpdate": "tool_call",
			"toolCallId":    "tc1",
			"title":         "bash",
			"content": map[string]interface{}{
				"name":      "bash",
				"arguments": map[string]string{"command": "Get-Date", "description": "time"},
			},
		},
	})
	events := TranslateSessionUpdate(raw, ProfileClientTools)
	if len(events) != 1 || events[0].Type != ir.EventToolCall {
		t.Fatalf("events=%+v", events)
	}
	tc := events[0].ToolCall
	if tc == nil || tc.Name != "bash" || tc.ID != "tc1" {
		t.Fatalf("tool call=%+v", tc)
	}
}

func TestStopReasonToFinish(t *testing.T) {
	if StopReasonToFinish(StopReasonFromString("max_tokens")) != "length" {
		t.Fatal("expected length")
	}
}

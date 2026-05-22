package webui

import (
	"encoding/json"
	"testing"
	"time"
)

func TestBuildTapRecord(t *testing.T) {
	dur := int64(1500)
	trace := &Trace{
		ID:           "req-abc",
		Endpoint:     "/v1/messages",
		Model:        "claude-sonnet",
		Status:       "completed",
		StartedAt:    time.Now().Add(-1500 * time.Millisecond),
		Duration:     &dur,
		RequestBody:  `{"model":"claude-sonnet","system":"hello","messages":[{"role":"user","content":"hi"}]}`,
		ResponseText: "world",
		Usage:        &UsageData{InputTokens: 10, OutputTokens: 5},
		ACPMessages: []ACPMessage{
			{Direction: "out", Method: "session/prompt", Payload: `{"method":"session/prompt"}`, Timestamp: time.Now()},
		},
	}

	rec := buildTapRecord(trace, 1)
	if rec["request_id"] != "req-abc" {
		t.Fatalf("request_id=%v", rec["request_id"])
	}
	req := rec["request"].(map[string]interface{})
	if req["path"] != "/v1/messages" {
		t.Fatalf("path=%v", req["path"])
	}
	body := req["body"].(map[string]interface{})
	if body["model"] != "claude-sonnet" {
		t.Fatalf("model=%v", body["model"])
	}
	resp := rec["response"].(map[string]interface{})
	respBody := resp["body"].(map[string]interface{})
	usage := respBody["usage"].(map[string]interface{})
	if usage["input_tokens"] != 10 {
		t.Fatalf("input_tokens=%v", usage["input_tokens"])
	}
	acp, ok := rec["acp_messages"].([]ACPMessage)
	if !ok || len(acp) != 1 {
		t.Fatalf("acp_messages=%T", rec["acp_messages"])
	}

	b := marshalTapRecord(trace, 1)
	var probe map[string]interface{}
	if err := json.Unmarshal(b, &probe); err != nil {
		t.Fatal(err)
	}
}

func TestEstimateUsage(t *testing.T) {
	trace := &Trace{
		Request: &TraceRequest{PromptLen: 400},
	}
	u := estimateUsage(trace, "abcd")
	if u == nil || u.InputTokens != 100 || u.OutputTokens != 1 {
		t.Fatalf("usage=%+v", u)
	}
}

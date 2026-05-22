package webui

import (
	"encoding/json"
	"strings"
	"time"
)

// marshalTapRecord converts an internal trace into a claude-tap viewer record.
func marshalTapRecord(trace *Trace, turn int) []byte {
	if trace == nil {
		return nil
	}
	record := buildTapRecord(trace, turn)
	b, err := json.Marshal(record)
	if err != nil {
		return nil
	}
	return b
}

func buildTapRecord(trace *Trace, turn int) map[string]interface{} {
	reqBody := parseRequestBody(trace.RequestBody)
	if trace.Model != "" {
		if _, ok := reqBody["model"]; !ok {
			reqBody["model"] = trace.Model
		}
	}

	durationMs := int64(0)
	if trace.Duration != nil {
		durationMs = *trace.Duration
	} else if trace.EndedAt != nil {
		durationMs = trace.EndedAt.Sub(trace.StartedAt).Milliseconds()
	}

	status := 200
	if trace.Status == "error" {
		status = 500
	}

	usage := trace.Usage
	if usage == nil {
		usage = estimateUsage(trace, trace.ResponseText)
	}

	respBody := map[string]interface{}{
		"model": trace.Model,
	}
	if usage != nil {
		respBody["usage"] = map[string]interface{}{
			"input_tokens":                usage.InputTokens,
			"output_tokens":               usage.OutputTokens,
			"cache_read_input_tokens":       usage.CacheReadInputTokens,
			"cache_creation_input_tokens":   usage.CacheCreationInputTokens,
		}
	}
	if trace.ResponseText != "" {
		respBody["content"] = []map[string]interface{}{
			{"type": "text", "text": trace.ResponseText},
		}
	}
	if trace.Error != "" {
		respBody["error"] = map[string]interface{}{"message": trace.Error}
	}

	record := map[string]interface{}{
		"turn":         turn,
		"request_id":   trace.ID,
		"timestamp":    trace.StartedAt.UTC().Format(time.RFC3339Nano),
		"duration_ms":  durationMs,
		"agent_profile": trace.AgentProfile,
		"request": map[string]interface{}{
			"method": "POST",
			"path":   trace.Endpoint,
			"body":   reqBody,
		},
		"response": map[string]interface{}{
			"status":      status,
			"body":        respBody,
			"sse_events":  buildSSEEvents(trace),
		},
	}
	if len(trace.ACPMessages) > 0 {
		record["acp_messages"] = trace.ACPMessages
	}
	return record
}

func parseRequestBody(raw string) map[string]interface{} {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]interface{}{}
	}
	var body map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return map[string]interface{}{"_raw": raw}
	}
	return body
}

func buildSSEEvents(trace *Trace) []map[string]interface{} {
	if trace == nil || len(trace.Events) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(trace.Events))
	for _, ev := range trace.Events {
		switch ev.Type {
		case EventTraceDelta:
			text, _ := ev.Data["text"].(string)
			out = append(out, map[string]interface{}{
				"event": "content_block_delta",
				"data": map[string]interface{}{
					"type":  "content_block_delta",
					"delta": map[string]interface{}{"type": "text_delta", "text": text},
				},
			})
		case EventTracePhase:
			out = append(out, map[string]interface{}{
				"event": "trace_phase",
				"data":  ev.Data,
			})
		case EventTraceTool, EventTraceBlocked:
			out = append(out, map[string]interface{}{
				"event": string(ev.Type),
				"data":  ev.Data,
			})
		case EventTraceACP:
			out = append(out, map[string]interface{}{
				"event": "trace_acp",
				"data":  ev.Data,
			})
		case EventTraceEnd:
			out = append(out, map[string]interface{}{
				"event": "message_stop",
				"data":  ev.Data,
			})
		}
	}
	return out
}

// TapRecordsFromStore returns tap records for all traces (newest first).
func TapRecordsFromStore(s *Store) [][]byte {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([][]byte, 0, len(s.order))
	for i, id := range s.order {
		if trace, ok := s.traces[id]; ok {
			turn := len(s.order) - i
			if b := marshalTapRecord(trace, turn); len(b) > 0 {
				out = append(out, b)
			}
		}
	}
	return out
}

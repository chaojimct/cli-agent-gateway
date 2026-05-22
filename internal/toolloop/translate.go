package toolloop

import (
	"encoding/json"

	"github.com/user/cursor-gateway/internal/acp"
	"github.com/user/cursor-gateway/internal/ir"
)

// TranslateSessionUpdate converts ACP session/update to IR events.
func TranslateSessionUpdate(params json.RawMessage, profile Profile) []ir.Event {
	var p acp.SessionUpdateParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil
	}
	u := p.Update
	var out []ir.Event

	switch u.SessionUpdate {
	case "user_message_chunk":
		out = append(out, ir.Event{Type: ir.EventTraceOnly, Text: acp.TextFromContent(u.Content)})

	case "agent_message_chunk":
		text := acp.TextFromContent(u.Content)
		if text != "" {
			out = append(out, ir.Event{Type: ir.EventContentDelta, Text: text})
		}

	case "agent_thought_chunk":
		text := acp.TextFromContent(u.Content)
		if text != "" {
			out = append(out, ir.Event{Type: ir.EventReasoningDelta, Text: text})
		}

	case "tool_call":
		tc := parseToolCall(u)
		tc.Native = true // ACP session/update tool_call is always cursor-agent built-in
		out = append(out, ir.Event{Type: ir.EventToolCall, ToolCall: tc})

	case "tool_call_update":
		tc := &ir.ToolCallState{
			ID:     u.ToolCallID,
			Status: u.Status,
		}
		if len(u.Content) > 0 {
			tc.Args = u.Content
		}
		out = append(out, ir.Event{Type: ir.EventToolCallUpdate, ToolCall: tc})

	case "plan":
		text := acp.TextFromContent(u.Content)
		if text == "" {
			text = string(u.Raw)
		}
		out = append(out, ir.Event{Type: ir.EventPlan, Text: text})

	case "available_commands_update":
		out = append(out, ir.Event{Type: ir.EventTraceOnly})

	case "current_mode_update":
		out = append(out, ir.Event{Type: ir.EventModeUpdate, ModeID: string(u.Raw)})

	default:
		out = append(out, ir.Event{Type: ir.EventTraceOnly})
	}
	return out
}

func parseToolCall(u acp.SessionUpdate) *ir.ToolCallState {
	tc := &ir.ToolCallState{
		ID:     u.ToolCallID,
		Status: u.Status,
		Title:  u.Title,
	}
	if len(u.Content) > 0 {
		tc.Args = u.Content
		var m map[string]interface{}
		if json.Unmarshal(u.Content, &m) == nil {
			if name, ok := m["name"].(string); ok {
				tc.Name = name
			}
			if title, ok := m["title"].(string); ok && tc.Title == "" {
				tc.Title = title
			}
			if args, ok := m["arguments"]; ok {
				if b, err := json.Marshal(args); err == nil {
					tc.Args = b
				}
			}
		}
	}
	if tc.Name == "" {
		tc.Name = tc.Title
	}
	return tc
}

// StopReasonToFinish maps ACP stop reason to OpenAI finish_reason.
func StopReasonToFinish(sr acp.StopReason) string {
	switch sr {
	case acp.StopMaxTokens:
		return "length"
	case acp.StopRefusal:
		return "content_filter"
	default:
		return "stop"
	}
}

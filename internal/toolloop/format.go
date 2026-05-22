package toolloop

import (
	"encoding/json"

	"github.com/user/cursor-gateway/internal/ir"
)

type openAIToolCallJSON struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// FormatClientToolCallsJSON builds OpenAI tool_calls JSON from IR tool states.
func FormatClientToolCallsJSON(calls []ir.ToolCallState) string {
	out := make([]openAIToolCallJSON, 0, len(calls))
	for _, tc := range calls {
		name := tc.Name
		if name == "" {
			name = tc.Title
		}
		args := string(tc.Args)
		if args == "" {
			args = "{}"
		}
		call := openAIToolCallJSON{ID: tc.ID, Type: "function"}
		call.Function.Name = name
		call.Function.Arguments = args
		if call.ID == "" {
			call.ID = "call_" + name
		}
		out = append(out, call)
	}
	raw, _ := json.Marshal(map[string]interface{}{"tool_calls": out})
	return string(raw)
}

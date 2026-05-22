package toolloop

import (
	"encoding/json"

	"github.com/user/cursor-gateway/internal/ir"
)

// HandleCursorExtension processes cursor/* blocking/notification methods.
func HandleCursorExtension(method string, params json.RawMessage) (interface{}, bool, []ir.Event) {
	switch method {
	case "cursor/ask_question":
		var req struct {
			Questions []struct {
				ID      string `json:"id"`
				Options []struct {
					ID string `json:"id"`
				} `json:"options"`
			} `json:"questions"`
		}
		_ = json.Unmarshal(params, &req)
		first := ""
		if len(req.Questions) > 0 && len(req.Questions[0].Options) > 0 {
			first = req.Questions[0].Options[0].ID
		}
		return map[string]interface{}{
			"outcome": map[string]interface{}{
				"outcome": "answered",
				"answers": []map[string]interface{}{
					{"questionId": req.Questions[0].ID, "selectedOptionIds": []string{first}},
				},
			},
		}, true, nil

	case "cursor/create_plan":
		var req struct {
			Plan string `json:"plan"`
			Name string `json:"name"`
		}
		_ = json.Unmarshal(params, &req)
		text := req.Plan
		if req.Name != "" {
			text = "## " + req.Name + "\n\n" + text
		}
		return map[string]interface{}{
			"outcome": map[string]interface{}{"outcome": "accepted"},
		}, true, []ir.Event{{Type: ir.EventPlan, Text: text}}

	case "cursor/update_todos", "cursor/task":
		return map[string]interface{}{
			"outcome": map[string]interface{}{"outcome": "completed"},
		}, true, []ir.Event{{Type: ir.EventTraceOnly}}

	case "cursor/generate_image":
		return map[string]interface{}{
			"outcome": map[string]interface{}{"outcome": "rejected", "reason": "not supported in gateway v2"},
		}, true, nil

	default:
		return map[string]interface{}{}, true, nil
	}
}

package agent

import "strings"

// ResolvedModel is the result of parsing a client model name.
type ResolvedModel struct {
	AgentID   string
	Model     string
	DisplayID string
	OwnedBy   string
	Valid     bool
	Err       string
}

// HasExplicitPrefix reports whether requested contains an agent prefix (e.g. claude/sonnet).
func HasExplicitPrefix(requested string) bool {
	requested = strings.TrimSpace(requested)
	i := strings.Index(requested, "/")
	return i > 0
}

// FormatModel builds a prefixed public model id: cursor/composer-2.5-fast.
func FormatModel(agentID, model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return agentID + "/auto"
	}
	if agentID == "" {
		return model
	}
	return agentID + "/" + model
}

// ParseModel splits "cursor/composer-2.5-fast" into agent and model.
// Unprefixed names use defaultAgent.
func ParseModel(requested, defaultAgent string) (agentID, model string) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return defaultAgent, "auto"
	}
	if i := strings.Index(requested, "/"); i > 0 {
		prefix := requested[:i]
		rest := strings.TrimSpace(requested[i+1:])
		if rest != "" {
			return prefix, rest
		}
		return prefix, "auto"
	}
	return defaultAgent, requested
}

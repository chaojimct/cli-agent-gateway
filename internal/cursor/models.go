package cursor

import (
	"github.com/user/cursor-gateway/internal/agent"
)
var modelMap = map[string]string{
	// OpenAI-style names
	"gpt-4":          "auto",
	"gpt-4o":         "auto",
	"gpt-4-turbo":    "auto",
	"gpt-4o-mini":    "auto",
	"gpt-3.5-turbo":  "auto",
	"gpt-5":          "auto",

	// Anthropic-style names
	"claude-sonnet-4-20250514":          "auto",
	"claude-sonnet-4":                   "auto",
	"claude-sonnet-4-thinking":          "auto",
	"claude-3-5-sonnet-20241022":        "auto",
	"claude-3-5-sonnet":                 "auto",
	"claude-3-opus-20240229":            "auto",
	"claude-3-haiku-20240307":           "auto",

	// Cursor-native names (pass through)
	"auto":              "auto",
	"composer-2-fast":   "composer-2-fast",
	"composer-2":        "composer-2",
	"composer-2.5":      "composer-2.5",
	"composer-2.5-fast": "composer-2.5-fast",
	"grok-4.3":          "grok-4.3",
	"kimi-k2.5":         "kimi-k2.5",

	// opencode-style names (no hyphen)
	"composer2":     "composer-2",
	"composer2fast": "composer-2-fast",
	"composer25":    "composer-2.5",
	"composer25fast": "composer-2.5-fast",
}

// ResolveModel maps a client-requested model name and selects the ACP agent.
func ResolveModel(requested string) string {
	// Backward-compatible helper: returns unprefixed agent model only.
	agentID, model := agent.ParseModel(requested, "cursor")
	_ = agentID
	if mapped, ok := modelMap[model]; ok {
		return mapped
	}
	return model
}

// AvailableModels returns the list of models that cursor-agent supports.
func AvailableModels() []ModelInfo {
	return []ModelInfo{
		{ID: "auto", Name: "Auto", OwnedBy: "cursor"},
		{ID: "gpt-5.2", Name: "GPT-5.2", OwnedBy: "openai"},
		{ID: "gpt-5.1", Name: "GPT-5.1", OwnedBy: "openai"},
		{ID: "gpt-5", Name: "GPT-5", OwnedBy: "openai"},
		{ID: "claude-4-sonnet", Name: "Claude 4 Sonnet", OwnedBy: "anthropic"},
		{ID: "claude-4.5-sonnet", Name: "Claude 4.5 Sonnet", OwnedBy: "anthropic"},
		{ID: "claude-4.6-opus-high", Name: "Claude 4.6 Opus", OwnedBy: "anthropic"},
		{ID: "gemini-3.1-pro", Name: "Gemini 3.1 Pro", OwnedBy: "google"},
		{ID: "gemini-3-flash", Name: "Gemini 3 Flash", OwnedBy: "google"},
		{ID: "grok-4.3", Name: "Grok 4.3", OwnedBy: "xai"},
		{ID: "kimi-k2.5", Name: "Kimi K2.5", OwnedBy: "moonshot"},
		{ID: "composer-2", Name: "Composer 2", OwnedBy: "cursor"},
		{ID: "composer-2.5", Name: "Composer 2.5", OwnedBy: "cursor"},
	}
}

// ModelInfo represents a model in the /v1/models response format.
type ModelInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	OwnedBy string `json:"owned_by"`
}

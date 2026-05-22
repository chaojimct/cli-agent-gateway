package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/user/cursor-gateway/internal/cursor"
	"github.com/user/cursor-gateway/internal/translator"
)

// ConversationResolve holds the resolved conversation key and how it was chosen.
type ConversationResolve struct {
	ID     string
	Source string // "explicit" or "auto"
}

func resolveConversationID(r *http.Request, req *translator.OpenAIChatRequest) ConversationResolve {
	if req != nil && req.Metadata != nil {
		if id, ok := req.Metadata["conversation_id"].(string); ok && id != "" {
			return ConversationResolve{ID: id, Source: "explicit"}
		}
	}
	if r != nil {
		if id := strings.TrimSpace(r.Header.Get("X-Conversation-Id")); id != "" {
			return ConversationResolve{ID: id, Source: "explicit"}
		}
	}
	if req != nil {
		if id := deriveConversationID(req.Messages); id != "" {
			return ConversationResolve{ID: id, Source: "auto"}
		}
	}
	return ConversationResolve{}
}

func resolveConversationIDAnthropic(r *http.Request, system json.RawMessage, messages []translator.AnthropicMessage, metadata map[string]interface{}) ConversationResolve {
	if metadata != nil {
		if id, ok := metadata["conversation_id"].(string); ok && id != "" {
			return ConversationResolve{ID: id, Source: "explicit"}
		}
		if id, ok := metadata["user_id"].(string); ok && id != "" {
			return ConversationResolve{ID: id, Source: "explicit"}
		}
	}
	if r != nil {
		if id := strings.TrimSpace(r.Header.Get("X-Conversation-Id")); id != "" {
			return ConversationResolve{ID: id, Source: "explicit"}
		}
	}
	sysText := anthropicSystemText(system)
	if id := deriveConversationKey(sysText, firstUserContent(messages)); id != "" {
		return ConversationResolve{ID: id, Source: "auto"}
	}
	return ConversationResolve{}
}

func anthropicSystemText(system json.RawMessage) string {
	if len(system) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(system, &s); err == nil {
		return s
	}
	return string(system)
}

func firstUserContent(messages []translator.AnthropicMessage) string {
	for _, m := range messages {
		if m.Role == "user" && m.Content != "" {
			return m.Content
		}
	}
	return ""
}

// deriveConversationID builds a stable thread key from system + first user message.
func deriveConversationID(messages []translator.OpenAIChatMessage) string {
	var system, firstUser string
	for _, m := range messages {
		switch m.Role {
		case "system", "developer":
			if system == "" {
				system = m.Content
			}
		case "user":
			if firstUser == "" {
				firstUser = m.Content
			}
		}
		if firstUser != "" {
			break
		}
	}
	return deriveConversationKey(system, firstUser)
}

func deriveConversationKey(system, firstUser string) string {
	if firstUser == "" {
		return ""
	}
	h := sha256.Sum256([]byte(system + "\n" + firstUser))
	return "auto:" + hex.EncodeToString(h[:16])
}

// BuildTurnPrompt returns full and optional incremental prompt for ACP session reuse.
func BuildTurnPrompt(runner *cursor.Runner, conversationID, agentID string, messages []translator.OpenAIChatMessage, tools []translator.OpenAITool) (full, incremental string, msgCount int) {
	full = translator.BuildPromptWithTools(messages, tools)
	msgCount = len(messages)
	if conversationID == "" || runner == nil {
		return full, "", msgCount
	}
	if e, ok := runner.SessionEntry(conversationID, agentID); ok && e.MessageCount > 0 && e.MessageCount < len(messages) {
		incremental = translator.BuildPromptIncremental(messages[e.MessageCount:], tools)
		return full, incremental, msgCount
	}
	return full, "", msgCount
}

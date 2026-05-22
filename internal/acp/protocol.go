package acp

import "encoding/json"

// JSON-RPC 2.0 envelope.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// StopReason from session/prompt result.
type StopReason string

const (
	StopEndTurn          StopReason = "end_turn"
	StopMaxTokens        StopReason = "max_tokens"
	StopMaxTurnRequests  StopReason = "max_turn_requests"
	StopRefusal          StopReason = "refusal"
	StopCancelled        StopReason = "cancelled"
)

// SessionMode for ACP sessions.
type SessionMode string

const (
	ModeAgent SessionMode = "agent"
	ModePlan  SessionMode = "plan"
	ModeAsk   SessionMode = "ask"
)

type InitializeParams struct {
	ProtocolVersion    int                `json:"protocolVersion"`
	ClientInfo         ClientInfo         `json:"clientInfo"`
	ClientCapabilities ClientCapabilities `json:"clientCapabilities"`
}

type InitializeResult struct {
	ProtocolVersion   int               `json:"protocolVersion"`
	AgentInfo         AgentInfo         `json:"agentInfo"`
	AuthMethods       []AuthMethod      `json:"authMethods"`
	AgentCapabilities AgentCapabilities `json:"agentCapabilities"`
}

type AgentInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type AuthMethod struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type AgentCapabilities struct {
	PromptCapabilities  *PromptCapabilities  `json:"promptCapabilities,omitempty"`
	SessionCapabilities *SessionCapabilities `json:"sessionCapabilities,omitempty"`
}

type PromptCapabilities struct {
	EmbeddedContext bool `json:"embeddedContext"`
}

type SessionCapabilities struct {
	Models *ModelsCapability `json:"models,omitempty"`
}

type ModelsCapability struct {
	AvailableModels []ModelDescriptor `json:"availableModels,omitempty"`
}

type ModelDescriptor struct {
	ModelID string `json:"modelId"`
	Name    string `json:"name"`
}

type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type ClientCapabilities struct {
	FS       *FSCapabilities `json:"fs,omitempty"`
	Terminal bool            `json:"terminal,omitempty"`
}

type FSCapabilities struct {
	ReadTextFile  bool `json:"readTextFile"`
	WriteTextFile bool `json:"writeTextFile"`
}

type AuthenticateParams struct {
	MethodID string `json:"methodId"`
}

type SessionNewParams struct {
	CWD        string        `json:"cwd"`
	McpServers []interface{} `json:"mcpServers"`
}

type SessionNewResult struct {
	SessionID     string         `json:"sessionId"`
	ConfigOptions []ConfigOption `json:"configOptions"`
	Models        *ModelList     `json:"models"`
}

type ConfigOption struct {
	ID      string        `json:"id"`
	Name    string        `json:"name,omitempty"`
	Options []ConfigValue `json:"options"`
}

type ConfigValue struct {
	Value     string `json:"value"`
	ValueID   string `json:"valueId"`
	Name      string `json:"name,omitempty"`
	IsDefault bool   `json:"isDefault"`
}

type ModelList struct {
	AvailableModels []ModelDescriptor `json:"availableModels"`
	CurrentModelID  string            `json:"currentModelId,omitempty"`
}

type SessionLoadParams struct {
	SessionID string `json:"sessionId"`
}

type SetConfigOptionParams struct {
	SessionID string `json:"sessionId"`
	ConfigID  string `json:"configId"`
	Value     string `json:"value,omitempty"`
	ValueID   string `json:"valueId,omitempty"`
}

type SetModelParams struct {
	SessionID string `json:"sessionId"`
	ModelID   string `json:"modelId"`
}

type PromptParams struct {
	SessionID string         `json:"sessionId"`
	Prompt    []ContentBlock `json:"prompt"`
}

type PromptResult struct {
	StopReason StopReason `json:"stopReason"`
}

type CancelParams struct {
	SessionID string `json:"sessionId"`
}

// ContentBlock for session/prompt.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

func TextBlock(text string) ContentBlock {
	return ContentBlock{Type: "text", Text: text}
}

// SessionUpdateParams wraps session/update notification.
type SessionUpdateParams struct {
	SessionID string          `json:"sessionId"`
	Update    SessionUpdate   `json:"update"`
}

// SessionUpdate is a tagged union parsed from JSON.
type SessionUpdate struct {
	SessionUpdate string          `json:"sessionUpdate"`
	Content       json.RawMessage `json:"content,omitempty"`
	ToolCallID    string          `json:"toolCallId,omitempty"`
	Title         string          `json:"title,omitempty"`
	Status        string          `json:"status,omitempty"`
	Raw           json.RawMessage `json:"-"`
}

func ParseSessionUpdate(raw json.RawMessage) (SessionUpdate, error) {
	var u SessionUpdate
	if err := json.Unmarshal(raw, &u); err != nil {
		return u, err
	}
	u.Raw = raw
	return u, nil
}

// TextFromContent extracts text from a content block JSON blob.
func TextFromContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]interface{}
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	if t, ok := m["text"].(string); ok {
		return t
	}
	if text, ok := m["text"].(map[string]interface{}); ok {
		if s, ok := text["text"].(string); ok {
			return s
		}
	}
	return ""
}

// PermissionRequest from agent.
type PermissionRequestParams struct {
	SessionID string                 `json:"sessionId"`
	ToolCall  map[string]interface{} `json:"toolCall,omitempty"`
	Options   []PermissionOption     `json:"options,omitempty"`
}

type PermissionOption struct {
	OptionID string `json:"optionId"`
	Kind     string `json:"kind,omitempty"`
	Name     string `json:"name,omitempty"`
}

type PermissionResponse struct {
	Outcome PermissionOutcome `json:"outcome"`
}

type PermissionOutcome struct {
	Outcome  string `json:"outcome"` // selected
	OptionID string `json:"optionId,omitempty"`
}

// Permission policy for gateway profiles.
type PermissionPolicy string

const (
	PermissionAllowOnce  PermissionPolicy = "allow-once"
	PermissionRejectOnce PermissionPolicy = "reject-once"
)

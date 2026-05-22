package translator

import (
	"encoding/json"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// OpenAITool is a function tool definition from the client (e.g. OpenCode).
type OpenAITool struct {
	Type     string         `json:"type"`
	Function OpenAIFunction `json:"function"`
}

// OpenAIFunction describes a callable function schema.
type OpenAIFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// OpenAIToolCall is an assistant tool invocation in OpenAI format.
type OpenAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// OpenAIChatMessage is a chat message in OpenAI format (including tool fields).
type OpenAIChatMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCalls  []OpenAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	Name       string           `json:"name,omitempty"`
}

// UnmarshalJSON accepts OpenAI string content or array-of-parts content (OpenCode / newer clients).
func (m *OpenAIChatMessage) UnmarshalJSON(data []byte) error {
	var raw struct {
		Role       string           `json:"role"`
		Content    json.RawMessage  `json:"content"`
		ToolCalls  []OpenAIToolCall `json:"tool_calls"`
		ToolCallID string           `json:"tool_call_id"`
		Name       string           `json:"name"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	m.Role = raw.Role
	m.ToolCalls = raw.ToolCalls
	m.ToolCallID = raw.ToolCallID
	m.Name = raw.Name
	m.Content = parseOpenAIMessageContent(raw.Content)
	return nil
}

func parseOpenAIMessageContent(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return ""
		}
		return s
	}
	if raw[0] != '[' {
		return strings.TrimSpace(string(raw))
	}
	var parts []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		Content  string `json:"content"`
		Refusal  string `json:"refusal"`
		Input    string `json:"input"`
		Output   string `json:"output"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return ""
	}
	var b strings.Builder
	for _, p := range parts {
		text := strings.TrimSpace(p.Text)
		if text == "" {
			text = strings.TrimSpace(p.Content)
		}
		if text == "" {
			text = strings.TrimSpace(p.Refusal)
		}
		if text == "" {
			text = strings.TrimSpace(p.Input)
		}
		if text == "" {
			text = strings.TrimSpace(p.Output)
		}
		if text == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(text)
	}
	return b.String()
}

// ChatMessage is an alias kept for internal callers.
type ChatMessage = OpenAIChatMessage

// BuildPromptWithTools converts messages and optional client tools into a cursor-agent prompt.
func BuildPromptWithTools(messages []OpenAIChatMessage, tools []OpenAITool) string {
	return buildPromptWithTools(messages, tools, true)
}

// BuildPromptIncremental converts only new messages (session reuse); skips tools appendix and duplicates.
func BuildPromptIncremental(messages []OpenAIChatMessage, tools []OpenAITool) string {
	return buildPromptWithTools(messages, tools, false)
}

func buildPromptWithTools(messages []OpenAIChatMessage, tools []OpenAITool, appendTools bool) string {
	var b strings.Builder

	for _, msg := range messages {
		switch msg.Role {
		case "system", "developer":
			b.WriteString("[System]\n")
			b.WriteString(msg.Content)
			b.WriteString("\n\n")
		case "user":
			b.WriteString("[User]\n")
			b.WriteString(msg.Content)
			b.WriteString("\n\n")
		case "assistant":
			b.WriteString("[Assistant]\n")
			if msg.Content != "" {
				b.WriteString(msg.Content)
				b.WriteString("\n")
			}
			if len(msg.ToolCalls) > 0 {
				raw, _ := json.Marshal(map[string]interface{}{"tool_calls": msg.ToolCalls})
				b.WriteString(string(raw))
				b.WriteString("\n")
			}
			b.WriteString("\n")
		case "tool":
			b.WriteString("[Tool Result — ")
			if msg.Name != "" {
				b.WriteString(msg.Name)
			} else if msg.ToolCallID != "" {
				b.WriteString(msg.ToolCallID)
			}
			b.WriteString("]\n")
			b.WriteString(msg.Content)
			b.WriteString("\n\n")
		case "function":
			b.WriteString("[Function Result]\n")
			b.WriteString(msg.Content)
			b.WriteString("\n\n")
		}
	}

	if shouldAppendPostToolHint(messages) {
		b.WriteString(`[System — 工具已执行完毕]
上方「工具输出」区块里已有命令的真实结果。请用自然语言直接回答用户。
不要再次输出 tool_calls JSON。

`)
	}

	if appendTools && len(tools) > 0 {
		appendCompactToolsSection(&b, messages, tools)
	}

	return strings.TrimSpace(b.String())
}

func appendCompactToolsSection(b *strings.Builder, messages []OpenAIChatMessage, tools []OpenAITool) {
	b.WriteString("[Available Functions — use ONLY these via tool_calls JSON]\n")
	if !systemAlreadyListsToolNames(messages, tools) {
		b.WriteString(formatToolNamesLine(tools))
	}
	if ShellToolName(tools) != "" {
		b.WriteString(formatTimeToolExample(tools))
	}
	b.WriteString("Compact schemas (name + required/optional fields only):\n")
	b.WriteString(formatCompactToolSchemas(tools))
	b.WriteString("\n\n")
}

// systemAlreadyListsToolNames reports whether the client system prompt already enumerates tool names.
func systemAlreadyListsToolNames(messages []OpenAIChatMessage, tools []OpenAITool) bool {
	if len(tools) == 0 {
		return false
	}
	var systemText strings.Builder
	for _, msg := range messages {
		if msg.Role == "system" || msg.Role == "developer" {
			systemText.WriteString(msg.Content)
			systemText.WriteByte('\n')
		}
	}
	body := systemText.String()
	if body == "" {
		return false
	}
	lower := strings.ToLower(body)
	if !strings.Contains(lower, "available tools") && !strings.Contains(body, "可用工具") &&
		!strings.Contains(lower, "registered for this turn") {
		return false
	}
	found := 0
	for _, t := range tools {
		if t.Function.Name == "" {
			continue
		}
		if strings.Contains(body, t.Function.Name) || strings.Contains(body, "`"+t.Function.Name+"`") {
			found++
		}
	}
	return found >= len(tools)/2 && found >= 2
}

type compactToolSchema struct {
	Name     string   `json:"name"`
	Required []string `json:"required,omitempty"`
	Optional []string `json:"optional,omitempty"`
}

func formatCompactToolSchemas(tools []OpenAITool) string {
	out := make([]compactToolSchema, 0, len(tools))
	for _, t := range tools {
		if t.Function.Name == "" {
			continue
		}
		ct := compactToolSchema{Name: t.Function.Name}
		if len(t.Function.Parameters) > 0 {
			var schema struct {
				Required   []string          `json:"required"`
				Properties map[string]json.RawMessage `json:"properties"`
			}
			if json.Unmarshal(t.Function.Parameters, &schema) == nil {
				ct.Required = schema.Required
				reqSet := make(map[string]struct{}, len(schema.Required))
				for _, r := range schema.Required {
					reqSet[r] = struct{}{}
				}
				for k := range schema.Properties {
					if _, ok := reqSet[k]; !ok {
						ct.Optional = append(ct.Optional, k)
					}
				}
				sortStrings(ct.Optional)
			}
		}
		out = append(out, ct)
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

func sortStrings(ss []string) {
	for i := 0; i < len(ss); i++ {
		for j := i + 1; j < len(ss); j++ {
			if ss[j] < ss[i] {
				ss[i], ss[j] = ss[j], ss[i]
			}
		}
	}
}

// MessagesHaveToolResults reports whether the conversation already includes tool output.
func MessagesHaveToolResults(messages []OpenAIChatMessage) bool {
	for _, msg := range messages {
		switch msg.Role {
		case "tool", "function":
			return true
		}
	}
	return false
}

// MessagesEndWithToolResult reports whether the last message is a tool/function result.
// Used to decide if the model should answer from tool output instead of emitting new tool_calls.
func MessagesEndWithToolResult(messages []OpenAIChatMessage) bool {
	if len(messages) == 0 {
		return false
	}
	switch messages[len(messages)-1].Role {
	case "tool", "function":
		return true
	default:
		return false
	}
}

func shouldAppendPostToolHint(messages []OpenAIChatMessage) bool {
	return MessagesEndWithToolResult(messages)
}

// LooksLikePendingToolCallsJSON reports whether assistant text may be an in-progress tool_calls JSON blob.
// Used to avoid streaming partial JSON to the client before ParseClientToolCalls can consume it.
func LooksLikePendingToolCallsJSON(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return false
	}
	if len(ParseClientToolCalls(t)) > 0 {
		return true
	}
	if !strings.HasPrefix(t, "{") {
		return false
	}
	if strings.Contains(t, `"tool_calls"`) {
		return true
	}
	// Early partial object: {" or {"tool...
	if len(t) < 512 && strings.Count(t, "}") < strings.Count(t, "{") {
		return true
	}
	return false
}

// PromptHasToolResults reports whether the prompt includes executed tool output blocks.
// Must not match the client-tools preamble mention of "[Tool Result] blocks".
func PromptHasToolResults(prompt string) bool {
	return strings.Contains(prompt, "[Tool Result — ") || strings.Contains(prompt, "[Function Result]\n")
}

func formatToolNamesLine(tools []OpenAITool) string {
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		if t.Function.Name != "" {
			names = append(names, t.Function.Name)
		}
	}
	if len(names) == 0 {
		return ""
	}
	return "Callable names: " + strings.Join(names, ", ") + "\n"
}

// ShellToolName picks the best client tool for running shell commands.
func ShellToolName(tools []OpenAITool) string {
	for _, t := range tools {
		n := strings.ToLower(t.Function.Name)
		switch n {
		case "bash", "shell", "run_terminal_cmd", "run_shell_command", "terminal", "exec", "run_command", "run":
			return t.Function.Name
		}
	}
	for _, t := range tools {
		n := strings.ToLower(t.Function.Name)
		if strings.Contains(n, "bash") || strings.Contains(n, "shell") || strings.Contains(n, "terminal") {
			return t.Function.Name
		}
	}
	if len(tools) > 0 {
		return tools[0].Function.Name
	}
	return "bash"
}

func formatTimeToolExample(tools []OpenAITool) string {
	name := ShellToolName(tools)
	args := marshalShellToolArgs("Get-Date -Format o", "Get local system date and time")
	return `Example for "what time is it" (use exact name "` + name + `"; arguments need command AND description):
{"tool_calls":[{"id":"call_time1","type":"function","function":{"name":"` + name + `","arguments":` + strconv.Quote(args) + `}}]}
`
}

func marshalShellToolArgs(command, description string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return "{}"
	}
	if strings.TrimSpace(description) == "" {
		description = defaultShellDescription(command)
	}
	raw, err := json.Marshal(map[string]string{
		"command":     command,
		"description": description,
	})
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func defaultShellDescription(command string) string {
	lower := strings.ToLower(command)
	if strings.Contains(lower, "get-date") || strings.Contains(lower, "date") {
		return "Get local system date and time"
	}
	return "Execute shell command"
}

// LastUserTextFromPrompt returns the last [User] block in a cursor-agent prompt.
func LastUserTextFromPrompt(prompt string) string {
	const marker = "[User]\n"
	idx := strings.LastIndex(prompt, marker)
	if idx < 0 {
		return strings.TrimSpace(prompt)
	}
	rest := prompt[idx+len(marker):]
	if end := strings.Index(rest, "\n\n"); end >= 0 {
		return strings.TrimSpace(rest[:end])
	}
	return strings.TrimSpace(rest)
}

// IsSimpleTimeQuery reports a short user message asking for local date/time.
func IsSimpleTimeQuery(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" || len(text) > 120 {
		return false
	}
	lower := strings.ToLower(text)
	keys := []string{
		"几点", "几点了", "什么时间", "现在时间", "当前时间", "本地时间", "几点钟",
		"what time", "current time", "local time", "what's the time", "whats the time",
	}
	for _, k := range keys {
		if strings.Contains(lower, k) {
			return true
		}
	}
	if strings.Contains(lower, "time") && (strings.Contains(lower, "now") || strings.Contains(lower, "current")) {
		return true
	}
	return false
}

// SynthesizeShellToolCall builds OpenAI tool_calls for the client's shell tool.
func SynthesizeShellToolCall(tools []OpenAITool, command string) []OpenAIToolCall {
	name := ShellToolName(tools)
	command = strings.TrimSpace(command)
	if name == "" || command == "" {
		return nil
	}
	return normalizeToolCalls([]OpenAIToolCall{{
		ID:   "call_" + uuid.New().String()[:12],
		Type: "function",
		Function: struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}{Name: name, Arguments: marshalShellToolArgs(command, "")},
	}})
}

// ExtractNativeShellCommand reads command from a suppressed cursor-agent shellToolCall.
func ExtractNativeShellCommand(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var envelope struct {
		ShellToolCall struct {
			Args struct {
				Command string `json:"command"`
			} `json:"args"`
		} `json:"shellToolCall"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return ""
	}
	return strings.TrimSpace(envelope.ShellToolCall.Args.Command)
}

// ShouldSynthesizeAfterSuppress decides whether to emit client tool_calls when the
// model tried a native shell tool or only returned a fetching stub.
func ShouldSynthesizeAfterSuppress(tools []OpenAITool, prompt, assistantText, suppressedCmd string) []OpenAIToolCall {
	if len(tools) == 0 || PromptHasToolResults(prompt) {
		return nil
	}
	if cmd := strings.TrimSpace(suppressedCmd); cmd != "" {
		return SynthesizeShellToolCall(tools, cmd)
	}
	if !IsSimpleTimeQuery(LastUserTextFromPrompt(prompt)) {
		return nil
	}
	if !LooksLikeIncompleteClientTool(assistantText) {
		return nil
	}
	return SynthesizeShellToolCall(tools, "Get-Date -Format o")
}

// StripToolCallsJSONText removes a leading tool_calls JSON blob from assistant text.
func StripToolCallsJSONText(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	if len(ParseClientToolCalls(trimmed)) > 0 {
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "```") {
			return ""
		}
	}
	return text
}

// LastToolResultText returns the most recent [Tool Result — name] body from a prompt.
func LastToolResultText(prompt string) string {
	const linePrefix = "[Tool Result — "
	parts := strings.Split(prompt, "\n"+linePrefix)
	if len(parts) < 2 {
		parts = strings.Split(prompt, linePrefix)
		if len(parts) < 2 {
			return lastFunctionResultText(prompt)
		}
	}
	block := parts[len(parts)-1]
	if nl := strings.Index(block, "\n"); nl >= 0 {
		block = block[nl+1:]
	} else {
		return ""
	}
	if end := strings.Index(block, "\n\n"); end >= 0 {
		block = block[:end]
	}
	if cut := strings.Index(block, "\n[System —"); cut >= 0 {
		block = block[:cut]
	}
	return strings.TrimSpace(block)
}

func lastFunctionResultText(prompt string) string {
	const marker = "[Function Result]\n"
	idx := strings.LastIndex(prompt, marker)
	if idx < 0 {
		return ""
	}
	rest := prompt[idx+len(marker):]
	if end := strings.Index(rest, "\n\n"); end >= 0 {
		return strings.TrimSpace(rest[:end])
	}
	return strings.TrimSpace(rest)
}

func looksLikeGatewayPostToolHint(text string) bool {
	return strings.Contains(text, "tool_calls JSON") ||
		strings.Contains(text, "工具已执行完毕") ||
		strings.Contains(text, "工具输出")
}

// LooksLikePlanModeRefusal detects cursor-agent plan-mode excuses in assistant text.
func LooksLikePlanModeRefusal(text string) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}
	lower := strings.ToLower(text)
	if strings.Contains(text, "计划模式") || strings.Contains(lower, "plan mode") {
		return strings.Contains(text, "不能") || strings.Contains(text, "无法") ||
			strings.Contains(lower, "cannot") || strings.Contains(lower, "can't")
	}
	return false
}

// ComposeAnswerAfterTool rewrites plan-mode refusals into an answer using tool output.
func ComposeAnswerAfterTool(prompt, modelText string) string {
	trimmed := strings.TrimSpace(modelText)
	if len(ParseClientToolCalls(trimmed)) > 0 {
		return trimmed
	}
	modelText = strings.TrimSpace(StripToolCallsJSONText(modelText))
	if !PromptHasToolResults(prompt) {
		return modelText
	}
	toolOut := LastToolResultText(prompt)
	if toolOut == "" {
		return modelText
	}
	if !LooksLikePlanModeRefusal(modelText) && modelText != "" && !looksLikeGatewayPostToolHint(modelText) {
		return modelText
	}
	if IsSimpleTimeQuery(LastUserTextFromPrompt(prompt)) {
		return "根据本机命令输出，当前时间是：" + toolOut
	}
	return "根据工具执行结果：\n" + toolOut
}

// LooksLikeIncompleteClientTool detects assistant text that promises tool use but has no tool_calls.
func LooksLikeIncompleteClientTool(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return true
	}
	if len(ParseClientToolCalls(text)) > 0 {
		return false
	}
	lower := strings.ToLower(text)
	stubs := []string{
		"正在获取", "正在查找", "正在读取", "正在执行", "正在运行",
		"fetching", "getting the current", "checking your",
		"无法获取", "拿不到", "没有工具", "no tool", "no clock",
	}
	for _, s := range stubs {
		if strings.Contains(lower, strings.ToLower(s)) {
			return true
		}
	}
	return len(text) < 120
}

var jsonFenceRE = regexp.MustCompile("(?s)```(?:json)?\\s*([\\s\\S]*?)```")

// ParseClientToolCalls extracts OpenAI tool_calls from model text output.
func ParseClientToolCalls(text string) []OpenAIToolCall {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	candidates := []string{text}
	if m := jsonFenceRE.FindStringSubmatch(text); len(m) > 1 {
		candidates = append([]string{strings.TrimSpace(m[1])}, candidates...)
	}

	for _, c := range candidates {
		if calls := parseToolCallsJSON(c); len(calls) > 0 {
			return normalizeToolCalls(calls)
		}
	}

	idx := strings.Index(text, `"tool_calls"`)
	if idx >= 0 {
		start := strings.LastIndex(text[:idx], "{")
		if start >= 0 {
			if calls := parseToolCallsJSON(extractBalancedJSON(text[start:])); len(calls) > 0 {
				return normalizeToolCalls(calls)
			}
		}
	}
	return nil
}

func parseToolCallsJSON(s string) []OpenAIToolCall {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}

	var withCalls struct {
		ToolCalls []OpenAIToolCall `json:"tool_calls"`
	}
	if err := json.Unmarshal([]byte(s), &withCalls); err == nil && len(withCalls.ToolCalls) > 0 {
		return withCalls.ToolCalls
	}

	var single struct {
		ToolCalls OpenAIToolCall `json:"tool_calls"`
	}
	if err := json.Unmarshal([]byte(s), &single); err == nil && single.ToolCalls.Function.Name != "" {
		return []OpenAIToolCall{single.ToolCalls}
	}

	var asMsg struct {
		ToolCalls []OpenAIToolCall `json:"tool_calls"`
	}
	if err := json.Unmarshal([]byte(s), &asMsg); err == nil && len(asMsg.ToolCalls) > 0 {
		return asMsg.ToolCalls
	}

	var arr []OpenAIToolCall
	if err := json.Unmarshal([]byte(s), &arr); err == nil && len(arr) > 0 {
		return arr
	}
	return nil
}

func extractBalancedJSON(s string) string {
	if !strings.HasPrefix(s, "{") {
		return s
	}
	depth := 0
	for i, r := range s {
		switch r {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[:i+1]
			}
		}
	}
	return s
}

func normalizeToolCalls(calls []OpenAIToolCall) []OpenAIToolCall {
	out := make([]OpenAIToolCall, 0, len(calls))
	for _, tc := range calls {
		if tc.Function.Name == "" {
			continue
		}
		if tc.ID == "" {
			tc.ID = "call_" + uuid.New().String()[:12]
		}
		if tc.Type == "" {
			tc.Type = "function"
		}
		if tc.Function.Arguments == "" {
			tc.Function.Arguments = "{}"
		}
		tc.Function.Arguments = enrichShellToolArguments(tc.Function.Name, tc.Function.Arguments)
		out = append(out, tc)
	}
	return out
}

func enrichShellToolArguments(toolName, argsJSON string) string {
	if !looksLikeShellTool(toolName) {
		return argsJSON
	}
	var args map[string]string
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return argsJSON
	}
	if strings.TrimSpace(args["command"]) == "" {
		return argsJSON
	}
	args["command"] = ensureShellUTF8Output(args["command"])
	if strings.TrimSpace(args["description"]) != "" {
		return argsJSON
	}
	args["description"] = defaultShellDescription(args["command"])
	raw, err := json.Marshal(args)
	if err != nil {
		return argsJSON
	}
	return string(raw)
}

func looksLikeShellTool(name string) bool {
	n := strings.ToLower(name)
	switch n {
	case "bash", "shell", "run_terminal_cmd", "terminal", "exec", "run_command", "run", "run_shell_command":
		return true
	}
	return strings.Contains(n, "bash") || strings.Contains(n, "shell") || strings.Contains(n, "terminal")
}

// ensureShellUTF8Output prefixes PowerShell commands on Windows so Chinese labels in stdout stay readable.
func ensureShellUTF8Output(command string) string {
	if runtime.GOOS != "windows" {
		return command
	}
	if strings.Contains(command, "OutputEncoding") || strings.Contains(command, "chcp 65001") {
		return command
	}
	lower := strings.ToLower(command)
	if strings.Contains(command, "$") || strings.Contains(lower, "get-date") ||
		strings.Contains(lower, "write-output") || strings.Contains(lower, "write-host") {
		return "[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false); " + command
	}
	return command
}

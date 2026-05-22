package translator

import (
	"encoding/json"
	"runtime"
	"strings"
	"testing"
)

func TestOpenAIChatMessageArrayContent(t *testing.T) {
	raw := `{"role":"user","content":[{"type":"text","text":"当前的时间"}]}`
	var msg OpenAIChatMessage
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if msg.Content != "当前的时间" {
		t.Fatalf("content=%q", msg.Content)
	}
}

func TestOpenAIChatMessageStringContent(t *testing.T) {
	raw := `{"role":"user","content":"hello"}`
	var msg OpenAIChatMessage
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if msg.Content != "hello" {
		t.Fatalf("content=%q", msg.Content)
	}
}

func TestParseClientToolCalls(t *testing.T) {
	raw := `{"tool_calls":[{"id":"call_abc","type":"function","function":{"name":"bash","arguments":"{\"command\":\"date\"}"}}]}`
	calls := ParseClientToolCalls(raw)
	if len(calls) != 1 {
		t.Fatalf("got %d calls", len(calls))
	}
	if calls[0].Function.Name != "bash" {
		t.Fatalf("name=%q", calls[0].Function.Name)
	}
}

func TestParseClientToolCallsFromFence(t *testing.T) {
	raw := "Here:\n```json\n{\"tool_calls\":[{\"id\":\"x\",\"type\":\"function\",\"function\":{\"name\":\"read\",\"arguments\":\"{}\"}}]}\n```"
	calls := ParseClientToolCalls(raw)
	if len(calls) != 1 || calls[0].Function.Name != "read" {
		t.Fatalf("got %+v", calls)
	}
}

func TestExtractNativeShellCommand(t *testing.T) {
	raw := []byte(`{"shellToolCall":{"args":{"command":"Get-Date -Format o"}}}`)
	if cmd := ExtractNativeShellCommand(raw); cmd != "Get-Date -Format o" {
		t.Fatalf("cmd=%q", cmd)
	}
}

func TestShouldSynthesizeAfterSuppress(t *testing.T) {
	tools := []OpenAITool{{Type: "function", Function: OpenAIFunction{Name: "bash"}}}
	calls := ShouldSynthesizeAfterSuppress(tools, "[User]\n几点\n\n", "正在获取本机当前时间。", "")
	if len(calls) != 1 {
		t.Fatalf("got %+v", calls)
	}
	if !strings.Contains(calls[0].Function.Arguments, `"description"`) {
		t.Fatalf("missing description: %q", calls[0].Function.Arguments)
	}
	calls2 := ShouldSynthesizeAfterSuppress(tools, "[User]\n几点\n\n", "", "Get-Date -Format o")
	if len(calls2) != 1 {
		t.Fatalf("got %+v", calls2)
	}
}

func TestComposeAnswerAfterTool(t *testing.T) {
	prompt := "[User]\n现在几点了\n\n[Tool Result — bash]\n2026-05-20 18:00:31 +08:00\n\n[System — 工具已执行完毕]\n上方「工具输出」区块里已有命令的真实结果。\n\n"
	refusal := "当前处于计划模式，不能执行命令读取实时时间。"
	got := ComposeAnswerAfterTool(prompt, refusal)
	if !strings.Contains(got, "18:00:31") {
		t.Fatalf("got %q", got)
	}
	if strings.Contains(got, "计划模式") || strings.Contains(got, "tool_calls JSON") {
		t.Fatalf("should not keep refusal/hint: %q", got)
	}
}

func TestComposeAnswerAfterToolPreservesToolCallsRetry(t *testing.T) {
	prompt := "[User]\n新闻\n\n[Tool Result — Web Search]\n{\"error\":\"unknown tool name\"}\n\n"
	retry := `{"tool_calls":[{"id":"call_news1","type":"function","function":{"name":"websearch","arguments":"{\"query\":\"news 2026\"}"}}]}`
	got := ComposeAnswerAfterTool(prompt, retry)
	if got != retry {
		t.Fatalf("got %q", got)
	}
}

func TestMapNativeWebSearchOpenCodeWebfetch(t *testing.T) {
	tools := []OpenAITool{
		{Function: OpenAIFunction{Name: "pencil_search_all_unique_properties"}},
		{Function: OpenAIFunction{Name: "webfetch"}},
		{Function: OpenAIFunction{Name: "grep"}},
	}
	calls := MapNativeToolToClient(tools, "Web Search", "Web Search", nil, "[User]\n看一下最新的美国新闻\n\n")
	if len(calls) != 1 {
		t.Fatalf("got %+v", calls)
	}
	if calls[0].Function.Name != "webfetch" {
		t.Fatalf("name=%q args=%q", calls[0].Function.Name, calls[0].Function.Arguments)
	}
	if !strings.Contains(calls[0].Function.Arguments, `"url"`) {
		t.Fatalf("expected url arg: %q", calls[0].Function.Arguments)
	}
	if strings.Contains(calls[0].Function.Arguments, "pencil") {
		t.Fatalf("must not use pencil tool: %q", calls[0].Function.Arguments)
	}
}

func TestWebSearchToolNameIgnoresPencil(t *testing.T) {
	tools := []OpenAITool{
		{Function: OpenAIFunction{Name: "pencil_search_all_unique_properties"}},
		{Function: OpenAIFunction{Name: "grep"}},
	}
	if WebSearchToolName(tools) != "" {
		t.Fatal("should not match pencil or grep")
	}
	if WebFetchToolName(tools) != "" {
		t.Fatal("no webfetch in list")
	}
}

func TestMapNativeWebSearch(t *testing.T) {
	tools := []OpenAITool{{Type: "function", Function: OpenAIFunction{Name: "websearch"}}}
	calls := MapNativeToolToClient(tools, "Web Search", "Web Search", nil, "[User]\n帮我总结最新的国际新闻\n\n")
	if len(calls) != 1 {
		t.Fatalf("got %+v", calls)
	}
	if calls[0].Function.Name != "websearch" {
		t.Fatalf("name=%q", calls[0].Function.Name)
	}
	if !strings.Contains(calls[0].Function.Arguments, "国际新闻") && !strings.Contains(calls[0].Function.Arguments, "2026") {
		t.Fatalf("args=%q", calls[0].Function.Arguments)
	}
}

func TestMapNativeShell(t *testing.T) {
	tools := []OpenAITool{{Type: "function", Function: OpenAIFunction{Name: "run_shell_command"}}}
	raw := []byte(`{"shellToolCall":{"args":{"command":"Get-Date -Format o"}}}`)
	calls := MapNativeToolToClient(tools, "Shell", "Shell", raw, "")
	if len(calls) != 1 || calls[0].Function.Name != "run_shell_command" {
		t.Fatalf("got %+v", calls)
	}
}

func TestLastToolResultTextIgnoresSystemHint(t *testing.T) {
	prompt := "[Tool Result — bash]\n2026-05-20 18:00:31 +08:00\n\n[System — 工具已执行完毕]\n上方 [Tool Result] 里已有命令。\n\n"
	got := LastToolResultText(prompt)
	if got != "2026-05-20 18:00:31 +08:00" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatCompactToolSchemas(t *testing.T) {
	tools := []OpenAITool{{
		Type: "function",
		Function: OpenAIFunction{
			Name:        "websearch",
			Parameters:  json.RawMessage(`{"type":"object","required":["query"],"properties":{"query":{"type":"string"},"numResults":{"type":"integer"}}}`),
		},
	}}
	raw := formatCompactToolSchemas(tools)
	if !strings.Contains(raw, `"name":"websearch"`) || !strings.Contains(raw, `"query"`) {
		t.Fatalf("got %s", raw)
	}
	if strings.Contains(raw, "Executes a given shell") {
		t.Fatalf("should not contain full description: %s", raw)
	}
}

func TestSystemAlreadyListsToolNames(t *testing.T) {
	msgs := []OpenAIChatMessage{{
		Role: "system",
		Content: "# Available Tools\n\n- `websearch`\n- `run_shell_command`\n- `grep`\n",
	}}
	tools := []OpenAITool{
		{Function: OpenAIFunction{Name: "websearch"}},
		{Function: OpenAIFunction{Name: "run_shell_command"}},
		{Function: OpenAIFunction{Name: "grep"}},
	}
	if !systemAlreadyListsToolNames(msgs, tools) {
		t.Fatal("expected true")
	}
}

func TestBuildPromptIncrementalOmitsToolsBlock(t *testing.T) {
	tools := []OpenAITool{{Function: OpenAIFunction{Name: "websearch", Description: strings.Repeat("x", 5000)}}}
	full := BuildPromptWithTools([]OpenAIChatMessage{{Role: "user", Content: "hi"}}, tools)
	inc := BuildPromptIncremental([]OpenAIChatMessage{{Role: "user", Content: "again"}}, tools)
	if !strings.Contains(full, "Compact schemas") {
		t.Fatal("full prompt should include compact tools")
	}
	if strings.Contains(inc, "Compact schemas") || strings.Contains(inc, strings.Repeat("x", 100)) {
		t.Fatal("incremental prompt should omit tools appendix")
	}
}

func TestBuildPromptIncrementalSecondTurnWithToolHistory(t *testing.T) {
	// Turn 2 incremental: replay assistant/tool/assistant from turn 1, then new user question.
	msgs := []OpenAIChatMessage{
		{Role: "assistant", ToolCalls: []OpenAIToolCall{{
			ID: "call_time1", Type: "function",
			Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{Name: "run_shell_command", Arguments: `{"command":"Get-Date"}`},
		}}},
		{Role: "tool", ToolCallID: "call_time1", Content: `{"output":"2026-05-21 18:31:09 +08:00"}`},
		{Role: "assistant", Content: "2026年5月21日 18:31:09（UTC+8）"},
		{Role: "user", Content: "帮我查一下 8080端口占用吧"},
	}
	inc := BuildPromptIncremental(msgs, nil)
	if strings.Contains(inc, "不要再次输出 tool_calls") {
		t.Fatalf("multi-turn incremental ending with user must allow tool_calls:\n%s", inc)
	}
	if !strings.Contains(inc, "8080") {
		t.Fatalf("missing new user message: %s", inc)
	}
}

func TestBuildPromptIncrementalEndsWithToolResultAddsHint(t *testing.T) {
	msgs := []OpenAIChatMessage{
		{Role: "user", Content: "现在几点"},
		{Role: "assistant", ToolCalls: []OpenAIToolCall{{
			ID: "call_1", Type: "function",
			Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{Name: "run_shell_command", Arguments: `{"command":"Get-Date"}`},
		}}},
		{Role: "tool", ToolCallID: "call_1", Content: "2026-05-21 18:31:09 +08:00"},
	}
	inc := BuildPromptIncremental(msgs, nil)
	if !strings.Contains(inc, "不要再次输出 tool_calls") {
		t.Fatalf("incremental ending with tool result should suppress new tool_calls:\n%s", inc)
	}
}

func TestMessagesEndWithToolResult(t *testing.T) {
	if !MessagesEndWithToolResult([]OpenAIChatMessage{{Role: "tool", Content: "ok"}}) {
		t.Fatal("expected true for trailing tool")
	}
	if MessagesEndWithToolResult([]OpenAIChatMessage{
		{Role: "tool", Content: "ok"},
		{Role: "user", Content: "next"},
	}) {
		t.Fatal("expected false when user follows tool history")
	}
}

func TestPromptHasToolResultsIgnoresPreamble(t *testing.T) {
	preamble := `[System — Client Tool API / OpenCode]
If [Tool Result] blocks already appear above, answer in plain text only.
`
	if PromptHasToolResults(preamble + "[User]\nhello") {
		t.Fatal("preamble mention should not count as tool output")
	}
	if !PromptHasToolResults("[Tool Result — bash]\n2026-05-20\n") {
		t.Fatal("actual tool result block should match")
	}
}

func TestLooksLikePendingToolCallsJSON(t *testing.T) {
	if !LooksLikePendingToolCallsJSON(`{"tool_calls":[`) {
		t.Fatal("partial tool_calls JSON should be pending")
	}
	if LooksLikePendingToolCallsJSON("当前时间是 12:00") {
		t.Fatal("plain text should not be pending")
	}
	if !LooksLikePendingToolCallsJSON(`{"tool_calls":[{"id":"x","type":"function","function":{"name":"bash","arguments":"{}"}}]}`) {
		t.Fatal("complete tool_calls JSON should be pending")
	}
}

func TestEnsureShellUTF8OutputWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows only")
	}
	got := ensureShellUTF8Output(`Get-Date -Format 'yyyy-MM-dd HH:mm:ss'`)
	if !strings.Contains(got, "OutputEncoding") {
		t.Fatalf("expected utf8 prefix, got %q", got)
	}
}

func TestEnrichShellToolArguments(t *testing.T) {
	raw := `{"command":"Get-Date -Format o"}`
	enriched := enrichShellToolArguments("bash", raw)
	if !strings.Contains(enriched, `"description"`) {
		t.Fatalf("got %q", enriched)
	}
}

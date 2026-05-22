package translator

import (
	"encoding/json"
	"net/url"
	"strings"
)

// MapNativeToolToClient converts a cursor-agent native ACP tool call into OpenWarp/OpenCode client tool_calls.
// Returns nil when the native tool cannot be mapped — caller must not forward the raw native name.
func MapNativeToolToClient(tools []OpenAITool, nativeName, title string, args []byte, prompt string) []OpenAIToolCall {
	label := strings.TrimSpace(nativeName)
	if label == "" {
		label = strings.TrimSpace(title)
	}
	key := normalizeNativeToolKey(label)

	if cmd := ExtractNativeShellCommand(args); cmd != "" || isShellNative(key) {
		if cmd == "" {
			cmd = extractCommandFromGenericArgs(args)
		}
		if cmd != "" {
			return SynthesizeShellToolCall(tools, cmd)
		}
	}

	if isWebSearchNative(key) {
		query := ExtractNativeWebSearchQuery(args)
		if query == "" {
			query = inferSearchQueryFromPrompt(prompt)
		}
		if query != "" {
			return SynthesizeWebToolCall(tools, query)
		}
	}

	return nil
}

func normalizeNativeToolKey(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	n = strings.ReplaceAll(n, " ", "")
	n = strings.ReplaceAll(n, "_", "")
	return n
}

func isShellNative(key string) bool {
	switch key {
	case "shell", "bash", "terminal", "runterminalcmd", "runcommand", "run_shell_command":
		return true
	}
	return strings.Contains(key, "shell") || strings.Contains(key, "terminal")
}

func isWebSearchNative(key string) bool {
	switch key {
	case "websearch", "webfetch", "search", "internetsearch":
		return true
	}
	return strings.Contains(key, "websearch") || strings.Contains(key, "search")
}

func extractCommandFromGenericArgs(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]interface{}
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	for _, k := range []string{"command", "cmd"} {
		if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// ExtractNativeWebSearchQuery reads search terms from cursor-agent native web search payloads.
func ExtractNativeWebSearchQuery(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var root map[string]interface{}
	if json.Unmarshal(raw, &root) != nil {
		return ""
	}
	// {"webSearchToolCall":{"args":{"searchTerm":"..."}}}
	for _, wrapper := range []string{"webSearchToolCall", "websearchToolCall", "searchToolCall"} {
		if w, ok := root[wrapper].(map[string]interface{}); ok {
			if q := queryFromMap(w); q != "" {
				return q
			}
			if args, ok := w["args"].(map[string]interface{}); ok {
				if q := queryFromMap(args); q != "" {
					return q
				}
			}
		}
	}
	return queryFromMap(root)
}

func queryFromMap(m map[string]interface{}) string {
	for _, k := range []string{"searchTerm", "search_term", "query", "q", "text", "input"} {
		if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func inferSearchQueryFromPrompt(prompt string) string {
	user := LastUserTextFromPrompt(prompt)
	user = strings.TrimSpace(user)
	if user == "" {
		return ""
	}
	// Strip common Chinese/English lead-ins so websearch gets a clean query.
	for _, prefix := range []string{
		"帮我", "请", "麻烦", "能不能", "可以", "总结一下", "总结", "汇总",
		"最新的", "最近", "当前", "今日", "今天",
		"please ", "summarize ", "summary of ", "latest ", "recent ",
	} {
		user = strings.TrimPrefix(user, prefix)
	}
	user = strings.TrimSpace(user)
	if user == "" {
		return LastUserTextFromPrompt(prompt)
	}
	// Append current year for news-style queries when missing.
	lower := strings.ToLower(user)
	if (strings.Contains(user, "新闻") || strings.Contains(lower, "news")) &&
		!strings.Contains(user, "202") {
		user = user + " 2026"
	}
	return user
}

// WebSearchToolName picks the client's dedicated web search tool (not file/design search).
func WebSearchToolName(tools []OpenAITool) string {
	for _, t := range tools {
		n := strings.ToLower(t.Function.Name)
		if n == "websearch" || n == "web_search" {
			return t.Function.Name
		}
	}
	return ""
}

// WebFetchToolName picks the client's web fetch tool.
func WebFetchToolName(tools []OpenAITool) string {
	for _, t := range tools {
		n := strings.ToLower(t.Function.Name)
		if n == "webfetch" || n == "web_fetch" {
			return t.Function.Name
		}
	}
	return ""
}

// SynthesizeWebToolCall maps a search query to websearch or webfetch client tool_calls.
func SynthesizeWebToolCall(tools []OpenAITool, query string) []OpenAIToolCall {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	if name := WebSearchToolName(tools); name != "" {
		return synthesizeWebSearchToolCall(name, query)
	}
	if name := WebFetchToolName(tools); name != "" {
		return synthesizeWebFetchToolCall(name, query)
	}
	return nil
}

// SynthesizeWebSearchToolCall builds client websearch or webfetch tool_calls.
func SynthesizeWebSearchToolCall(tools []OpenAITool, query string) []OpenAIToolCall {
	return SynthesizeWebToolCall(tools, query)
}

func synthesizeWebSearchToolCall(name, query string) []OpenAIToolCall {
	args, err := json.Marshal(map[string]interface{}{
		"query":      query,
		"type":       "deep",
		"numResults": 10,
	})
	if err != nil {
		return nil
	}
	return normalizeWebToolCall(name, string(args))
}

func synthesizeWebFetchToolCall(name, query string) []OpenAIToolCall {
	target := searchURLForQuery(query)
	if target == "" {
		return nil
	}
	args, err := json.Marshal(map[string]interface{}{
		"url":    target,
		"format": "markdown",
	})
	if err != nil {
		return nil
	}
	return normalizeWebToolCall(name, string(args))
}

func searchURLForQuery(query string) string {
	q := strings.TrimSpace(query)
	if q == "" {
		return ""
	}
	lower := strings.ToLower(q)
	if strings.Contains(q, "新闻") || strings.Contains(lower, "news") {
		return "https://news.google.com/search?q=" + url.QueryEscape(q)
	}
	return "https://www.google.com/search?q=" + url.QueryEscape(q)
}

func normalizeWebToolCall(name, args string) []OpenAIToolCall {
	return normalizeToolCalls([]OpenAIToolCall{{
		ID:   "call_" + strings.ReplaceAll(name, " ", "_"),
		Type: "function",
		Function: struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}{Name: name, Arguments: args},
	}})
}

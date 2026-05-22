package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/user/cursor-gateway/internal/cursor"
	"github.com/user/cursor-gateway/internal/translator"
	"github.com/user/cursor-gateway/internal/webui"
)

type ChatHandler struct {
	HandlerEnv
}

func NewChatHandler(env HandlerEnv) *ChatHandler {
	if env.ThinkingVisibility == "" {
		env.ThinkingVisibility = "reasoning_content"
	}
	return &ChatHandler{HandlerEnv: env}
}

func (h *ChatHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	IncRequestMetrics()
	if !h.ensureRunnerReady(w) {
		return
	}
	var req translator.OpenAIChatRequest
	if err := h.decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
		return
	}

	if len(req.Messages) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request_error", "messages is required")
		return
	}

	cfg := h.cursorCfg()
	clientTools := cfg.ClientToolsEnabled(req.HasClientTools())

	resolved := h.Runner.ResolveModel(req.Model)
	if !h.validateResolved(w, resolved) {
		return
	}
	model := resolved.DisplayID
	conv := resolveConversationID(r, &req)
	fullPrompt, incPrompt, msgCount := BuildTurnPrompt(h.Runner, conv.ID, resolved.AgentID, req.Messages, req.Tools)
	fullPrompt = cursor.WrapClientToolsPrompt(cfg, fullPrompt)
	fullPrompt = cursor.WrapModelAPIPrompt(cfg, fullPrompt, clientTools)
	if incPrompt != "" {
		incPrompt = cursor.WrapClientToolsPrompt(cfg, incPrompt)
		if !clientTools {
			incPrompt = cursor.WrapModelAPIPrompt(cfg, incPrompt, false)
		}
	}
	prompt := fullPrompt
	turnPrompt := fullPrompt
	if incPrompt != "" {
		turnPrompt = incPrompt
	}
	turnMessages := req.Messages
	if incPrompt != "" && conv.ID != "" {
		if e, ok := h.Runner.SessionEntry(conv.ID, resolved.AgentID); ok && e.MessageCount > 0 && e.MessageCount < len(req.Messages) {
			turnMessages = req.Messages[e.MessageCount:]
		}
	}
	afterToolResult := translator.MessagesEndWithToolResult(turnMessages)
	turnPromptLen := len(prompt)
	if incPrompt != "" {
		turnPromptLen = len(incPrompt)
	}
	traceID := "chatcmpl-" + generateShortID()

	h.startTrace(traceID, "/v1/chat/completions", model, len(req.Messages), turnPromptLen, &req, conv)
	if conv.ID != "" {
		h.Logger.Info("conversation resolved",
			"trace", traceID,
			"conversation_id", conv.ID,
			"source", conv.Source,
			"incremental", incPrompt != "",
			"turn_prompt_len", turnPromptLen,
		)
	}

	var sessionID string
	var cursorMessages []cursor.Message
	if h.Sessions != nil && !clientTools {
		cursorMessages = messagesToCursor(req.Messages)
		sessionID = h.Sessions.GetOrCreate(cursorMessages)
	}

	opts := h.runOpts(resolved, traceID, cursorMessages, sessionID, clientTools, conv.ID, incPrompt, msgCount)

	if req.Stream {
		h.handleStream(w, r, prompt, opts, model, clientTools, req.Tools, turnPrompt, afterToolResult)
	} else {
		h.handleSync(w, r, prompt, opts, model, traceID, clientTools, req.Tools)
	}
}

func (h *ChatHandler) handleStream(w http.ResponseWriter, r *http.Request, prompt string, opts cursor.RunOpts, model string, clientTools bool, tools []translator.OpenAITool, turnPrompt string, afterToolResult bool) {
	stats := h.Runner.Stats()
	w.Header().Set("X-Concurrency-Active", fmt.Sprintf("%d", stats.Active))
	w.Header().Set("X-Concurrency-Queued", fmt.Sprintf("%d", stats.Queued))
	w.Header().Set("X-Concurrency-Limit", fmt.Sprintf("%d", stats.MaxConcurrent))

	result, err := h.Runner.Run(r.Context(), prompt, opts)
	if err != nil {
		h.Logger.Error("failed to run cursor-agent", "error", err)
		if h.Store != nil {
			h.Store.ErrorTrace(opts.TraceID, err.Error())
		}
		writeError(w, http.StatusInternalServerError, "server_error", err.Error())
		return
	}

	writer := translator.NewOpenAIChatSSEWriter(w, displayModel(opts))
	writer.WriteHeaders()

	ctState := &ClientToolsState{
		Tools:           tools,
		Prompt:          turnPrompt,
		AfterToolResult: afterToolResult,
	}
	scfg := h.streamCfg(opts.TraceID, opts)
	scfg.ClientToolsState = ctState

	sink := &openAIChatSink{
		writer:           writer,
		thinkingVisMode:  h.ThinkingVisibility,
		clientTools:      clientTools,
		clientToolsState: ctState,
	}
	// 流式路径不在结束时再跑第二轮 agent（会卡住 OpenCode 数十秒）；重试仅走非流式 handleSync。

	start := time.Now()
	h.Logger.Info("chat stream start", "trace", opts.TraceID, "model", model, "client_tools", clientTools)
	fullText, usage, streamErr := ProcessEventStream(result, scfg, sink)
	callsN := len(translator.ParseClientToolCalls(sink.contentBuf.String()))
	if callsN == 0 && fullText != "" {
		callsN = len(translator.ParseClientToolCalls(fullText))
	}
	h.Logger.Info("chat stream end", "trace", opts.TraceID, "duration_ms", time.Since(start).Milliseconds(), "text_len", len(fullText), "tool_calls", callsN, "client_tools", clientTools, "stopped", result.Stopped())

	if streamErr != nil && !clientTools {
		if h.Store != nil {
			h.Store.ErrorTrace(opts.TraceID, streamErr.Error())
		}
		if fullText == "" {
			writeError(w, http.StatusBadRequest, "model_only_violation", streamErr.Error())
			return
		}
	}

	if err := waitForErr(result); err != nil && !result.Stopped() && !sink.finished && fullText == "" && sink.contentBuf.Len() == 0 {
		writer.WriteError(err.Error())
		if h.Store != nil {
			h.Store.ErrorTrace(opts.TraceID, err.Error())
		}
		return
	}

	if clientTools {
		if t := sink.contentBuf.String(); t != "" {
			fullText = t
		}
	}

	if h.Store != nil {
		var usageData *webui.UsageData
		if usage != nil {
			usageData = &webui.UsageData{
				InputTokens:  usage.InputTokens,
				OutputTokens: usage.OutputTokens,
			}
		}
		h.Store.CompleteTrace(opts.TraceID, usageData, time.Since(start).Milliseconds(), fullText)
	}

	if h.Sessions != nil && opts.CursorMessages != nil {
		h.Sessions.UpdateAfterResponse(opts.CursorMessages, fullText)
	}
}

func (h *ChatHandler) handleSync(w http.ResponseWriter, r *http.Request, prompt string, opts cursor.RunOpts, model, traceID string, clientTools bool, tools []translator.OpenAITool) {
	start := time.Now()
	text, usage, err := h.runClientToolsAware(r.Context(), prompt, opts, clientTools, tools)
	if err != nil {
		h.Logger.Error("failed to run cursor-agent sync", "error", err)
		if h.Store != nil {
			h.Store.ErrorTrace(traceID, err.Error())
		}
		status := http.StatusInternalServerError
		errType := "server_error"
		if err == cursor.ErrModelOnlyToolUse && !clientTools {
			status = http.StatusBadRequest
			errType = "model_only_violation"
		}
		writeError(w, status, errType, err.Error())
		return
	}

	if clientTools {
		if calls := translator.ParseClientToolCalls(text); len(calls) > 0 {
			if h.Store != nil {
				var usageData *webui.UsageData
				if usage != nil {
					usageData = &webui.UsageData{
						InputTokens:  usage.InputTokens,
						OutputTokens: usage.OutputTokens,
					}
				}
				h.Store.CompleteTrace(traceID, usageData, time.Since(start).Milliseconds(), text)
			}
			resp := translator.BuildSyncResponseWithToolCalls(calls, model, usage)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
	}

	if h.Store != nil {
		var usageData *webui.UsageData
		if usage != nil {
			usageData = &webui.UsageData{
				InputTokens:  usage.InputTokens,
				OutputTokens: usage.OutputTokens,
			}
		}
		h.Store.CompleteTrace(traceID, usageData, time.Since(start).Milliseconds(), text)
	}

	if h.Sessions != nil && opts.CursorMessages != nil {
		h.Sessions.UpdateAfterResponse(opts.CursorMessages, text)
	}

	resp := translator.BuildSyncResponse(text, model, usage)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *ChatHandler) runClientToolsAware(ctx context.Context, prompt string, opts cursor.RunOpts, clientTools bool, tools []translator.OpenAITool) (string, *cursor.Usage, error) {
	text, usage, err := h.Runner.RunSync(ctx, prompt, opts)
	if !clientTools {
		return text, usage, err
	}
	if calls := translator.ParseClientToolCalls(text); len(calls) > 0 {
		return text, usage, err
	}
	if strings.TrimSpace(text) != "" && strings.HasPrefix(strings.TrimSpace(text), "{") {
		if calls := translator.ParseClientToolCalls(text); len(calls) > 0 {
			return text, usage, err
		}
	}
	if err == nil && strings.TrimSpace(text) != "" && !translator.LooksLikeIncompleteClientTool(text) {
		return text, usage, err
	}
	return text, usage, err
}

type openAIChatSink struct {
	writer            *translator.OpenAIChatSSEWriter
	thinkingVisMode   string
	clientTools       bool
	clientToolsState  *ClientToolsState
	contentBuf        strings.Builder
	streamed          bool
	contentHold       bool
	finished          bool
	toolCalls         []translator.OpenAIToolCall
	suppressedShell   string
	retryRun          func() (string, *cursor.Usage, error)
}

func (s *openAIChatSink) OnInit(string) {}
func (s *openAIChatSink) OnContentDelta(text string) error {
	if s.clientTools {
		s.contentBuf.WriteString(text)
		if s.afterToolResult() {
			s.streamed = true
			return s.writer.WriteDelta(text)
		}
		buf := s.contentBuf.String()
		if s.contentHold || translator.LooksLikePendingToolCallsJSON(buf) {
			s.contentHold = true
			if len(translator.ParseClientToolCalls(buf)) > 0 {
				return nil
			}
			return nil
		}
		s.streamed = true
		return s.writer.WriteDelta(text)
	}
	return s.writer.WriteDelta(text)
}

func (s *openAIChatSink) afterToolResult() bool {
	return s.clientToolsState != nil && s.clientToolsState.AfterToolResult
}
func (s *openAIChatSink) OnReasoningDelta(text string) error {
	if s.clientTools {
		// 展示 thinking，但不写入 contentBuf，避免污染最终正文
		switch s.thinkingVisMode {
		case "off", "hidden", "none":
			return nil
		case "prefix", "content":
			return s.writer.WriteDelta(text)
		default:
			return s.writer.WriteReasoningDelta(text)
		}
	}
	switch s.thinkingVisMode {
	case "prefix", "content":
		return s.writer.WriteDelta(text)
	default:
		return s.writer.WriteReasoningDelta(text)
	}
}
func (s *openAIChatSink) OnToolCall(callID, name string, args []byte) error {
	if !s.clientTools {
		return nil
	}
	call := translator.OpenAIToolCall{ID: callID, Type: "function"}
	call.Function.Name = name
	if len(args) > 0 {
		call.Function.Arguments = string(args)
	} else {
		call.Function.Arguments = "{}"
	}
	return s.OnClientToolCalls([]translator.OpenAIToolCall{call})
}

func (s *openAIChatSink) OnClientToolCalls(calls []translator.OpenAIToolCall) error {
	if !s.clientTools || len(calls) == 0 {
		return nil
	}
	s.toolCalls = append(s.toolCalls, calls...)
	s.finished = true
	return s.writer.WriteToolCalls(calls)
}

func (s *openAIChatSink) NoteSuppressedNativeShell(command string) {
	command = strings.TrimSpace(command)
	if command != "" {
		s.suppressedShell = command
	}
}
func (s *openAIChatSink) OnToolBlocked(string) {}
func (s *openAIChatSink) OnDone(usage *cursor.Usage, fullText, finishReason string) error {
	if s.clientTools {
		return s.finishClientTools(usage, fullText)
	}
	return s.writer.WriteDone(usage)
}
func (s *openAIChatSink) OnError(err error) bool {
	_ = s.writer.WriteError(err.Error())
	return true
}

func (s *openAIChatSink) finishClientTools(usage *cursor.Usage, fullText string) error {
	if len(s.toolCalls) > 0 {
		s.finished = true
		return s.writer.WriteDoneToolCalls(usage)
	}

	text := s.contentBuf.String()
	if strings.TrimSpace(text) == "" && strings.TrimSpace(fullText) != "" {
		text = fullText
	}

	// Model-emitted tool_calls JSON (highest priority — restore original client-tools behavior).
	if calls := translator.ParseClientToolCalls(text); len(calls) > 0 {
		if err := s.writer.WriteToolCalls(calls); err != nil {
			return err
		}
		s.finished = true
		return s.writer.WriteDoneToolCalls(usage)
	}

	// Map suppressed native shell → client run_shell_command / bash.
	if s.clientToolsState != nil && !s.afterToolResult() {
		if calls := translator.ShouldSynthesizeAfterSuppress(
			s.clientToolsState.Tools, s.clientToolsState.Prompt, text, s.suppressedShell,
		); len(calls) > 0 {
			if err := s.writer.WriteToolCalls(calls); err != nil {
				return err
			}
			s.finished = true
			return s.writer.WriteDoneToolCalls(usage)
		}
	}

	if s.afterToolResult() && s.clientToolsState != nil {
		text = translator.ComposeAnswerAfterTool(s.clientToolsState.Prompt, text)
		if calls := translator.ParseClientToolCalls(text); len(calls) > 0 {
			if err := s.writer.WriteToolCalls(calls); err != nil {
				return err
			}
			s.finished = true
			return s.writer.WriteDoneToolCalls(usage)
		}
	}

	if text != "" && !s.streamed {
		if err := s.writer.WriteDelta(text); err != nil {
			return err
		}
		s.finished = true
	} else if text != "" {
		s.finished = true
	}
	return s.writer.WriteDone(usage)
}

func messagesToCursor(msgs []translator.OpenAIChatMessage) []cursor.Message {
	out := make([]cursor.Message, len(msgs))
	for i, m := range msgs {
		out[i] = cursor.Message{
			Role:    m.Role,
			Content: []cursor.ContentPart{{Type: "text", Text: m.Content}},
		}
	}
	return out
}

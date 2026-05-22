package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/chaojimct/cli-agent-gateway/internal/cursor"
	"github.com/chaojimct/cli-agent-gateway/internal/translator"
	"github.com/chaojimct/cli-agent-gateway/internal/webui"
)

type MessagesHandler struct {
	HandlerEnv
}

func NewMessagesHandler(env HandlerEnv) *MessagesHandler {
	return &MessagesHandler{HandlerEnv: env}
}

func (h *MessagesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	IncRequestMetrics()
	if !h.ensureRunnerReadyAnthropic(w) {
		return
	}
	var req translator.AnthropicRequest
	if err := h.decodeJSON(w, r, &req); err != nil {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
		return
	}

	if len(req.Messages) == 0 {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "messages is required")
		return
	}
	if req.MaxTokens == 0 {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "max_tokens is required")
		return
	}

	resolved := h.Runner.ResolveModel(req.Model)
	if !h.validateResolvedAnthropic(w, resolved) {
		return
	}
	model := resolved.DisplayID
	prompt := translator.BuildAnthropicPrompt(req.System, req.Messages)
	traceID := "msg_" + generateShortID()
	conv := resolveConversationIDAnthropic(r, req.System, req.Messages, req.Metadata)

	h.startTrace(traceID, "/v1/messages", model, len(req.Messages), len(prompt), &req, conv)

	var sessionID string
	var cursorMessages []cursor.Message
	if h.Sessions != nil {
		cursorMessages = anthropicToCursor(req.Messages)
		sessionID = h.Sessions.GetOrCreate(cursorMessages)
	}

	opts := h.runOpts(resolved, traceID, cursorMessages, sessionID, false, conv.ID, "", len(req.Messages))

	if req.Stream {
		h.handleStream(w, r, prompt, opts)
	} else {
		h.handleSync(w, r, prompt, opts, model, traceID)
	}
}

func (h *MessagesHandler) handleStream(w http.ResponseWriter, r *http.Request, prompt string, opts cursor.RunOpts) {
	result, err := h.Runner.Run(r.Context(), prompt, opts)
	if err != nil {
		writeAnthropicError(w, http.StatusInternalServerError, "api_error", err.Error())
		return
	}

	writer := translator.NewAnthropicSSEWriter(w, displayModel(opts))
	writer.WriteHeaders()
	_ = writer.WriteMessageStart()
	_ = writer.WriteContentBlockStart()

	sink := &anthropicSink{writer: writer}
	start := time.Now()
	fullText, usage, streamErr := ProcessEventStream(result, h.streamCfg(opts.TraceID, opts), sink)

	if streamErr != nil {
		if h.Store != nil {
			h.Store.ErrorTrace(opts.TraceID, streamErr.Error())
		}
		writeAnthropicError(w, http.StatusBadRequest, "model_only_violation", streamErr.Error())
		return
	}

	if err := waitForErr(result); err != nil && fullText == "" {
		writer.WriteError(err.Error())
		return
	}

	if h.Store != nil {
		var usageData *webui.UsageData
		if usage != nil {
			usageData = &webui.UsageData{InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens}
		}
		h.Store.CompleteTrace(opts.TraceID, usageData, time.Since(start).Milliseconds(), fullText)
	}

	if h.Sessions != nil && opts.CursorMessages != nil {
		h.Sessions.UpdateAfterResponse(opts.CursorMessages, fullText)
	}
}

func (h *MessagesHandler) handleSync(w http.ResponseWriter, r *http.Request, prompt string, opts cursor.RunOpts, model, traceID string) {
	start := time.Now()
	text, usage, err := h.Runner.RunSync(r.Context(), prompt, opts)
	if err != nil {
		if h.Store != nil {
			h.Store.ErrorTrace(traceID, err.Error())
		}
		status, errType := http.StatusInternalServerError, "api_error"
		if err == cursor.ErrModelOnlyToolUse {
			status = http.StatusBadRequest
			errType = "model_only_violation"
		}
		writeAnthropicError(w, status, errType, err.Error())
		return
	}

	if h.Store != nil {
		var usageData *webui.UsageData
		if usage != nil {
			usageData = &webui.UsageData{InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens}
		}
		h.Store.CompleteTrace(traceID, usageData, time.Since(start).Milliseconds(), text)
	}

	if h.Sessions != nil && opts.CursorMessages != nil {
		h.Sessions.UpdateAfterResponse(opts.CursorMessages, text)
	}

	resp := translator.BuildAnthropicSyncResponse(text, model, usage)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

type anthropicSink struct {
	writer *translator.AnthropicSSEWriter
}

func (s *anthropicSink) OnInit(string)                          {}
func (s *anthropicSink) OnContentDelta(text string) error       { return s.writer.WriteDelta(text) }
func (s *anthropicSink) OnReasoningDelta(text string) error     { return s.writer.WriteDelta(text) }
func (s *anthropicSink) OnToolCall(string, string, []byte) error { return nil }
func (s *anthropicSink) OnToolBlocked(string)                   {}
func (s *anthropicSink) OnDone(usage *cursor.Usage, _ string, _ string) error { return s.writer.WriteDone(usage) }
func (s *anthropicSink) OnError(err error) bool {
	_ = s.writer.WriteError(err.Error())
	return true
}

func anthropicToCursor(msgs []translator.AnthropicMessage) []cursor.Message {
	out := make([]cursor.Message, len(msgs))
	for i, m := range msgs {
		out[i] = cursor.Message{
			Role:    m.Role,
			Content: []cursor.ContentPart{{Type: "text", Text: m.Content}},
		}
	}
	return out
}

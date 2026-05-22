package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/user/cursor-gateway/internal/cursor"
	"github.com/user/cursor-gateway/internal/translator"
	"github.com/user/cursor-gateway/internal/webui"
)

type ResponsesHandler struct {
	HandlerEnv
}

func NewResponsesHandler(env HandlerEnv) *ResponsesHandler {
	return &ResponsesHandler{HandlerEnv: env}
}

func (h *ResponsesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	IncRequestMetrics()
	if !h.ensureRunnerReady(w) {
		return
	}
	var req translator.ResponsesRequest
	if err := h.decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
		return
	}

	if len(req.Input) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request_error", "input is required")
		return
	}

	resolved := h.Runner.ResolveModel(req.Model)
	if !h.validateResolved(w, resolved) {
		return
	}
	model := resolved.DisplayID
	systemPrompt, messages := translator.ParseResponsesInput(req.Input)

	var allMessages []translator.ChatMessage
	if systemPrompt != "" {
		allMessages = append(allMessages, translator.ChatMessage{Role: "system", Content: systemPrompt})
	}
	allMessages = append(allMessages, messages...)
	prompt := translator.BuildPrompt(allMessages)

	traceID := "resp_" + generateShortID()
	h.startTrace(traceID, "/v1/responses", model, len(allMessages), len(prompt), &req, ConversationResolve{})

	var sessionID string
	var cursorMessages []cursor.Message
	if h.Sessions != nil {
		cursorMessages = messagesToCursor(allMessages)
		sessionID = h.Sessions.GetOrCreate(cursorMessages)
	}

	convID := req.PreviousResponseID
	opts := h.runOpts(resolved, traceID, cursorMessages, sessionID, false, convID, "", len(allMessages))

	if req.Stream {
		h.handleStream(w, r, prompt, opts)
	} else {
		h.handleSync(w, r, prompt, opts, model, traceID)
	}
}

func (h *ResponsesHandler) handleStream(w http.ResponseWriter, r *http.Request, prompt string, opts cursor.RunOpts) {
	result, err := h.Runner.Run(r.Context(), prompt, opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", err.Error())
		return
	}

	writer := translator.NewResponsesSSEWriter(w, displayModel(opts))
	writer.WriteHeaders()
	_ = writer.WriteCreated()
	_ = writer.WriteOutputItemAdded()

	sink := &responsesSink{writer: writer}
	start := time.Now()
	fullText, usage, streamErr := ProcessEventStream(result, h.streamCfg(opts.TraceID, opts), sink)

	if streamErr != nil {
		if h.Store != nil {
			h.Store.ErrorTrace(opts.TraceID, streamErr.Error())
		}
		writeError(w, http.StatusBadRequest, "model_only_violation", streamErr.Error())
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

func (h *ResponsesHandler) handleSync(w http.ResponseWriter, r *http.Request, prompt string, opts cursor.RunOpts, model, traceID string) {
	start := time.Now()
	text, usage, err := h.Runner.RunSync(r.Context(), prompt, opts)
	if err != nil {
		if h.Store != nil {
			h.Store.ErrorTrace(traceID, err.Error())
		}
		status, errType := http.StatusInternalServerError, "server_error"
		if err == cursor.ErrModelOnlyToolUse {
			status = http.StatusBadRequest
			errType = "model_only_violation"
		}
		writeError(w, status, errType, err.Error())
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

	resp := translator.BuildResponsesSyncResponse(text, model, usage)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

type responsesSink struct {
	writer *translator.ResponsesSSEWriter
}

func (s *responsesSink) OnInit(string)                          {}
func (s *responsesSink) OnContentDelta(text string) error       { return s.writer.WriteDelta(text) }
func (s *responsesSink) OnReasoningDelta(text string) error     { return s.writer.WriteDelta(text) }
func (s *responsesSink) OnToolCall(string, string, []byte) error { return nil }
func (s *responsesSink) OnToolBlocked(string)                   {}
func (s *responsesSink) OnDone(usage *cursor.Usage, _ string, _ string) error { return s.writer.WriteDone(usage) }
func (s *responsesSink) OnError(err error) bool {
	_ = s.writer.WriteError(err.Error())
	return true
}

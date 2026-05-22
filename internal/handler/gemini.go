package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/user/cursor-gateway/internal/cursor"
	"github.com/user/cursor-gateway/internal/translator"
)

// GeminiHandler handles Gemini generateContent endpoints.
type GeminiHandler struct {
	HandlerEnv
}

func NewGeminiHandler(env HandlerEnv) *GeminiHandler {
	return &GeminiHandler{HandlerEnv: env}
}

type geminiRequest struct {
	Contents []struct {
		Role  string `json:"role"`
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	} `json:"contents"`
}

func (h *GeminiHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	IncRequestMetrics()
	if !h.ensureRunnerReady(w) {
		return
	}
	modelPath := strings.TrimPrefix(r.URL.Path, "/v1beta/models/")
	modelPath = strings.TrimPrefix(modelPath, "/")
	stream := strings.HasSuffix(modelPath, ":streamGenerateContent")
	model := strings.TrimSuffix(modelPath, ":streamGenerateContent")
	model = strings.TrimSuffix(model, ":generateContent")

	var req geminiRequest
	if err := h.decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
		return
	}

	var msgs []translator.OpenAIChatMessage
	for _, c := range req.Contents {
		role := c.Role
		if role == "model" {
			role = "assistant"
		}
		var text strings.Builder
		for _, p := range c.Parts {
			text.WriteString(p.Text)
		}
		msgs = append(msgs, translator.OpenAIChatMessage{Role: role, Content: text.String()})
	}
	if len(msgs) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request_error", "contents is required")
		return
	}

	resolved := h.Runner.ResolveModel(model)
	if !h.validateResolved(w, resolved) {
		return
	}
	displayModel := resolved.DisplayID
	prompt := translator.BuildPromptWithTools(msgs, nil)
	cfg := h.cursorCfg()
	prompt = cursor.WrapModelAPIPrompt(cfg, prompt, false)
	traceID := "gemini_" + generateShortID()
	h.startTrace(traceID, r.URL.Path, displayModel, len(msgs), len(prompt), &req, ConversationResolve{})

	opts := h.runOpts(resolved, traceID, messagesToCursor(msgs), "", false, "", "", len(msgs))

	if stream {
		result, err := h.Runner.Run(r.Context(), prompt, opts)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "server_error", err.Error())
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		sink := &geminiStreamSink{w: w, flusher: flusher, model: displayModel}
		_, _, _ = ProcessEventStream(result, h.streamCfg(traceID, opts), sink)
		return
	}

	text, _, err := h.Runner.RunSync(r.Context(), prompt, opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"candidates": []map[string]interface{}{
			{
				"content": map[string]interface{}{
					"role":  "model",
					"parts": []map[string]string{{"text": text}},
				},
				"finishReason": "STOP",
			},
		},
		"modelVersion": displayModel,
	})
}

type geminiStreamSink struct {
	w       http.ResponseWriter
	flusher http.Flusher
	model   string
	buf     strings.Builder
}

func (s *geminiStreamSink) OnInit(string)                          {}
func (s *geminiStreamSink) OnReasoningDelta(string) error          { return nil }
func (s *geminiStreamSink) OnToolCall(string, string, []byte) error { return nil }
func (s *geminiStreamSink) OnToolBlocked(string)                   {}

func (s *geminiStreamSink) OnContentDelta(text string) error {
	s.buf.WriteString(text)
	payload, _ := json.Marshal(map[string]interface{}{
		"candidates": []map[string]interface{}{
			{"content": map[string]interface{}{"parts": []map[string]string{{"text": text}}}},
		},
	})
	_, _ = s.w.Write([]byte("data: " + string(payload) + "\n\n"))
	if s.flusher != nil {
		s.flusher.Flush()
	}
	return nil
}

func (s *geminiStreamSink) OnDone(*cursor.Usage, string, string) error {
	return nil
}

func (s *geminiStreamSink) OnError(err error) bool {
	_, _ = s.w.Write([]byte("data: {\"error\":\"" + err.Error() + "\"}\n\n"))
	return true
}

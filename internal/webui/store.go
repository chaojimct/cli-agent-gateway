package webui

import (
	"strings"
	"sync"
	"time"
)

type EventType string

const (
	EventTraceStart  EventType = "trace_start"
	EventTraceDelta  EventType = "trace_delta"
	EventTraceTool   EventType = "trace_tool"
	EventTraceResult EventType = "trace_result"
	EventTraceError  EventType = "trace_error"
	EventTraceEnd    EventType = "trace_end"
	EventTracePhase  EventType = "trace_phase"
	EventTraceBlocked EventType = "trace_blocked"
	EventTraceACP     EventType = "trace_acp"
)

type TraceRequest struct {
	Endpoint           string `json:"endpoint"`
	Model              string `json:"model"`
	MessageCount       int    `json:"message_count"`
	PromptLen          int    `json:"prompt_len"`
	AgentProfile       string `json:"agent_profile,omitempty"`
	ConversationID     string `json:"conversation_id,omitempty"`
	ConversationSource string `json:"conversation_source,omitempty"`
}

type TraceEvent struct {
	Type      EventType              `json:"type"`
	ID        string                 `json:"id"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

type Trace struct {
	ID           string        `json:"id"`
	Endpoint     string        `json:"endpoint"`
	Model        string        `json:"model"`
	Status       string        `json:"status"`
	AgentProfile string        `json:"agent_profile,omitempty"`
	StartedAt    time.Time     `json:"started_at"`
	EndedAt      *time.Time    `json:"ended_at,omitempty"`
	Duration     *int64        `json:"duration_ms,omitempty"`
	Usage        *UsageData    `json:"usage,omitempty"`
	Error        string        `json:"error,omitempty"`
	ResponseText string        `json:"response_text,omitempty"`
	Request      *TraceRequest `json:"request,omitempty"`
	RequestBody  string        `json:"request_body,omitempty"`
	ACPMessages  []ACPMessage  `json:"acp_messages,omitempty"`
	Events       []TraceEvent  `json:"events"`
	searchText   string
}

type UsageData struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
}

// ACPMessage records one JSON-RPC line sent or received on the ACP stdio channel.
type ACPMessage struct {
	Direction string    `json:"direction"`
	Method    string    `json:"method,omitempty"`
	Payload   string    `json:"payload"`
	Timestamp time.Time `json:"timestamp"`
}

type Store struct {
	mu            sync.RWMutex
	traces        map[string]*Trace
	order         []string
	maxLen        int
	maxDeltaBytes int
	hub           *Hub
}

func NewStore(maxLen int, hub *Hub) *Store {
	if maxLen <= 0 {
		maxLen = 1000
	}
	return &Store{
		traces:        make(map[string]*Trace),
		maxLen:        maxLen,
		maxDeltaBytes: 65536,
		hub:           hub,
	}
}

func (s *Store) SetMaxDeltaBytes(n int) {
	if n > 0 {
		s.maxDeltaBytes = n
	}
}

func (s *Store) StartTrace(id, endpoint, model, agentProfile string, req *TraceRequest, requestBody string) {
	trace := &Trace{
		ID:           id,
		Endpoint:     endpoint,
		Model:        model,
		Status:       "active",
		AgentProfile: agentProfile,
		StartedAt:    time.Now(),
		Request:      req,
		RequestBody:  requestBody,
		Events:       make([]TraceEvent, 0),
	}
	if req != nil {
		trace.searchText = id + " " + endpoint + " " + model + " " + requestBody
	}

	s.mu.Lock()
	s.traces[id] = trace
	s.order = append([]string{id}, s.order...)
	if len(s.order) > s.maxLen {
		oldest := s.order[len(s.order)-1]
		s.order = s.order[:len(s.order)-1]
		delete(s.traces, oldest)
	}
	s.mu.Unlock()

	s.broadcast(TraceEvent{
		Type:      EventTraceStart,
		ID:        id,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"endpoint": endpoint,
			"model":    model,
		},
	})
}

func (s *Store) truncateDelta(text string) string {
	if s.maxDeltaBytes > 0 && len(text) > s.maxDeltaBytes {
		return text[:s.maxDeltaBytes]
	}
	return text
}

func (s *Store) AddDelta(id, text string) {
	text = s.truncateDelta(text)
	s.mu.Lock()
	if trace, ok := s.traces[id]; ok {
		trace.Events = append(trace.Events, TraceEvent{
			Type:      EventTraceDelta,
			ID:        id,
			Timestamp: time.Now(),
			Data:      map[string]interface{}{"text": text},
		})
		trace.ResponseText += text
		trace.searchText += " " + text
	}
	s.mu.Unlock()

	s.broadcast(TraceEvent{
		Type:      EventTraceDelta,
		ID:        id,
		Timestamp: time.Now(),
		Data:      map[string]interface{}{"text": text},
	})
}

func (s *Store) AddPhase(id, phase, preview string) {
	if len(preview) > 200 {
		preview = preview[:200]
	}
	s.mu.Lock()
	if trace, ok := s.traces[id]; ok {
		trace.Events = append(trace.Events, TraceEvent{
			Type:      EventTracePhase,
			ID:        id,
			Timestamp: time.Now(),
			Data:      map[string]interface{}{"phase": phase, "preview": preview},
		})
	}
	s.mu.Unlock()

	s.broadcast(TraceEvent{
		Type:      EventTracePhase,
		ID:        id,
		Timestamp: time.Now(),
		Data:      map[string]interface{}{"phase": phase, "preview": preview},
	})
}

func (s *Store) AddACPMessage(id, direction, method, payload string) {
	const maxPayload = 256 * 1024
	if len(payload) > maxPayload {
		payload = payload[:maxPayload] + "\n…(truncated)"
	}
	now := time.Now()
	msg := ACPMessage{
		Direction: direction,
		Method:    method,
		Payload:   payload,
		Timestamp: now,
	}

	s.mu.Lock()
	if trace, ok := s.traces[id]; ok {
		trace.ACPMessages = append(trace.ACPMessages, msg)
		trace.Events = append(trace.Events, TraceEvent{
			Type:      EventTraceACP,
			ID:        id,
			Timestamp: now,
			Data: map[string]interface{}{
				"direction": direction,
				"method":    method,
				"payload":   payload,
			},
		})
	}
	s.mu.Unlock()

	s.broadcast(TraceEvent{
		Type:      EventTraceACP,
		ID:        id,
		Timestamp: now,
		Data: map[string]interface{}{
			"direction": direction,
			"method":    method,
			"payload":   payload,
		},
	})
}

// RecordACP implements runner.TraceHook.
func (s *Store) RecordACP(traceID, direction, method, payload string) {
	if s == nil || traceID == "" {
		return
	}
	s.AddACPMessage(traceID, direction, method, payload)
}

func (s *Store) AddToolCall(id, toolType, status string, args interface{}) {
	s.mu.Lock()
	if trace, ok := s.traces[id]; ok {
		trace.Events = append(trace.Events, TraceEvent{
			Type:      EventTraceTool,
			ID:        id,
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"tool":   toolType,
				"status": status,
				"args":   args,
			},
		})
	}
	s.mu.Unlock()

	s.broadcast(TraceEvent{
		Type:      EventTraceTool,
		ID:        id,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"tool":   toolType,
			"status": status,
			"args":   args,
		},
	})
}

func (s *Store) AddToolBlocked(id, callID string) {
	s.mu.Lock()
	if trace, ok := s.traces[id]; ok {
		trace.Events = append(trace.Events, TraceEvent{
			Type:      EventTraceBlocked,
			ID:        id,
			Timestamp: time.Now(),
			Data:      map[string]interface{}{"call_id": callID},
		})
	}
	s.mu.Unlock()

	s.broadcast(TraceEvent{
		Type:      EventTraceBlocked,
		ID:        id,
		Timestamp: time.Now(),
		Data:      map[string]interface{}{"call_id": callID},
	})
}

func (s *Store) CompleteTrace(id string, usage *UsageData, durationMs int64, responseText string) {
	now := time.Now()
	var tapRecord []byte

	s.mu.Lock()
	if trace, ok := s.traces[id]; ok {
		trace.Status = "completed"
		trace.EndedAt = &now
		trace.Duration = &durationMs
		if usage == nil {
			usage = estimateUsage(trace, responseText)
		}
		trace.Usage = usage
		if responseText != "" {
			trace.ResponseText = responseText
		}
		tapRecord = marshalTapRecord(trace, s.tapTurnLocked(trace))
	}
	s.mu.Unlock()

	s.broadcast(TraceEvent{
		Type:      EventTraceEnd,
		ID:        id,
		Timestamp: now,
		Data: map[string]interface{}{
			"duration_ms": durationMs,
			"usage":       usage,
		},
	})
	if len(tapRecord) > 0 {
		s.broadcastTap(tapRecord)
	}
}

func estimateUsage(trace *Trace, responseText string) *UsageData {
	text := responseText
	if text == "" {
		text = trace.ResponseText
	}
	input := 0
	if trace.Request != nil && trace.Request.PromptLen > 0 {
		input = (trace.Request.PromptLen + 3) / 4
	}
	if input == 0 && trace.RequestBody != "" {
		input = (len(trace.RequestBody) + 3) / 4
	}
	output := (len(text) + 3) / 4
	if input == 0 && output == 0 {
		return nil
	}
	return &UsageData{InputTokens: input, OutputTokens: output}
}

func (s *Store) tapTurnLocked(trace *Trace) int {
	if trace == nil {
		return 0
	}
	turn := 0
	for _, id := range s.order {
		if t, ok := s.traces[id]; ok {
			turn++
			if t.ID == trace.ID {
				return turn
			}
		}
	}
	return turn
}

func (s *Store) ErrorTrace(id, errMsg string) {
	now := time.Now()
	var tapRecord []byte

	s.mu.Lock()
	if trace, ok := s.traces[id]; ok {
		trace.Status = "error"
		trace.EndedAt = &now
		trace.Error = errMsg
		if trace.Duration == nil {
			ms := now.Sub(trace.StartedAt).Milliseconds()
			trace.Duration = &ms
		}
		if trace.Usage == nil {
			trace.Usage = estimateUsage(trace, trace.ResponseText)
		}
		tapRecord = marshalTapRecord(trace, s.tapTurnLocked(trace))
	}
	s.mu.Unlock()

	s.broadcast(TraceEvent{
		Type:      EventTraceError,
		ID:        id,
		Timestamp: now,
		Data:      map[string]interface{}{"error": errMsg},
	})
	if len(tapRecord) > 0 {
		s.broadcastTap(tapRecord)
	}
}

func (s *Store) GetTraces() []*Trace {
	s.mu.RLock()
	defer s.mu.RUnlock()

	traces := make([]*Trace, 0, len(s.order))
	for _, id := range s.order {
		if trace, ok := s.traces[id]; ok {
			traces = append(traces, trace)
		}
	}
	return traces
}

func (s *Store) SearchTraces(q, endpoint, model, status string) []*Trace {
	q = strings.ToLower(strings.TrimSpace(q))
	all := s.GetTraces()
	out := make([]*Trace, 0, len(all))
	for _, t := range all {
		if endpoint != "" && t.Endpoint != endpoint {
			continue
		}
		if model != "" && t.Model != model {
			continue
		}
		if status != "" && t.Status != status {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(t.searchText+" "+t.ResponseText+" "+t.ID), q) {
			continue
		}
		out = append(out, t)
	}
	return out
}

func (s *Store) GetTrace(id string) *Trace {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.traces[id]
}

func (s *Store) Stats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	total := len(s.traces)
	active, completed, errors := 0, 0, 0
	var totalInput, totalOutput int

	for _, trace := range s.traces {
		switch trace.Status {
		case "active":
			active++
		case "completed":
			completed++
			if trace.Usage != nil {
				totalInput += trace.Usage.InputTokens
				totalOutput += trace.Usage.OutputTokens
			}
		case "error":
			errors++
		}
	}

	return map[string]interface{}{
		"total_traces":  total,
		"active":        active,
		"completed":     completed,
		"errors":        errors,
		"total_tokens":  totalInput + totalOutput,
		"input_tokens":  totalInput,
		"output_tokens": totalOutput,
	}
}

// CompareResult holds a structural comparison between two traces.
type CompareResult struct {
	TraceA      *Trace                 `json:"trace_a"`
	TraceB      *Trace                 `json:"trace_b"`
	Differences map[string]interface{} `json:"differences"`
}

func (s *Store) Compare(aID, bID string) (*CompareResult, error) {
	a := s.GetTrace(aID)
	b := s.GetTrace(bID)
	if a == nil || b == nil {
		return nil, jsonErr("trace not found")
	}

	diff := map[string]interface{}{
		"model":       diffField(a.Model, b.Model),
		"endpoint":    diffField(a.Endpoint, b.Endpoint),
		"status":      diffField(a.Status, b.Status),
		"duration_ms": diffField(ptrInt64(a.Duration), ptrInt64(b.Duration)),
		"tokens": map[string]interface{}{
			"a": a.Usage,
			"b": b.Usage,
		},
		"response_text": map[string]string{
			"a": a.ResponseText,
			"b": b.ResponseText,
		},
	}
	if a.Request != nil && b.Request != nil {
		diff["message_count"] = diffField(a.Request.MessageCount, b.Request.MessageCount)
	}

	return &CompareResult{TraceA: a, TraceB: b, Differences: diff}, nil
}

func diffField(a, b interface{}) map[string]interface{} {
	return map[string]interface{}{"a": a, "b": b, "changed": a != b}
}

func ptrInt64(p *int64) interface{} {
	if p == nil {
		return nil
	}
	return *p
}

type jsonError string

func (e jsonError) Error() string { return string(e) }

func jsonErr(msg string) error { return jsonError(msg) }

func (s *Store) broadcast(event TraceEvent) {
	if s.hub != nil {
		s.hub.Broadcast(event)
	}
}

func (s *Store) broadcastTap(record []byte) {
	if s.hub != nil && len(record) > 0 {
		s.hub.BroadcastTap(record)
	}
}

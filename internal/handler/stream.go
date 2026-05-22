package handler

import (
	"log/slog"
	"time"

	"github.com/chaojimct/cli-agent-gateway/internal/config"
	"github.com/chaojimct/cli-agent-gateway/internal/cursor"
	"github.com/chaojimct/cli-agent-gateway/internal/ir"
	"github.com/chaojimct/cli-agent-gateway/internal/runner"
	"github.com/chaojimct/cli-agent-gateway/internal/toolloop"
	"github.com/chaojimct/cli-agent-gateway/internal/translator"
	"github.com/chaojimct/cli-agent-gateway/internal/webui"
)

// ClientToolsState shares client-tool metadata between stream processing and the sink.
type ClientToolsState struct {
	Tools           []translator.OpenAITool
	Prompt          string
	AfterToolResult bool
}

// StreamSink receives processed stream output.
type StreamSink interface {
	OnInit(sessionID string)
	OnContentDelta(text string) error
	OnReasoningDelta(text string) error
	OnToolCall(callID, name string, args []byte) error
	OnToolBlocked(callID string)
	OnDone(usage *cursor.Usage, fullText string, finishReason string) error
	OnError(err error) bool
}

// ClientToolsStreamSink extends StreamSink for OpenCode/OpenWarp client-tool routing.
type ClientToolsStreamSink interface {
	StreamSink
	OnClientToolCalls(calls []translator.OpenAIToolCall) error
	NoteSuppressedNativeShell(command string)
}

// StreamConfig controls stream processing behavior.
type StreamConfig struct {
	CursorCfg        *config.CursorConfig
	ThinkingVisMode  string
	Store            *webui.Store
	Logger           *slog.Logger
	TraceID          string
	ClientTools      bool
	ClientToolsState *ClientToolsState
}

// ProcessEventStream reads IR events from result and drives the sink.
func ProcessEventStream(result *cursor.RunResult, cfg StreamConfig, sink StreamSink) (fullText string, usage *cursor.Usage, streamErr error) {
	for ev := range result.Events {
		switch ev.Type {
		case ir.EventSessionInit:
			sink.OnInit(ev.SessionID)
			if cfg.Store != nil {
				cfg.Store.AddPhase(cfg.TraceID, "session", ev.SessionID)
			}

		case ir.EventContentDelta:
			fullText += ev.Text
			if cfg.Store != nil {
				cfg.Store.AddDelta(cfg.TraceID, ev.Text)
			}
			_ = sink.OnContentDelta(ev.Text)

		case ir.EventReasoningDelta:
			if cfg.Store != nil {
				cfg.Store.AddPhase(cfg.TraceID, "reasoning", ev.Text)
			}
			_ = sink.OnReasoningDelta(ev.Text)

		case ir.EventToolCall:
			if ev.ToolCall == nil {
				continue
			}
			IncToolCallMetrics()
			name := ev.ToolCall.Name
			if name == "" {
				name = ev.ToolCall.Title
			}
			if cfg.ClientTools && cfg.ClientToolsState != nil {
				if ev.ToolCall.Native {
					if cts, ok := sink.(ClientToolsStreamSink); ok {
						mapped := translator.MapNativeToolToClient(
							cfg.ClientToolsState.Tools,
							ev.ToolCall.Name, ev.ToolCall.Title,
							ev.ToolCall.Args, cfg.ClientToolsState.Prompt,
						)
						if len(mapped) > 0 {
							if err := cts.OnClientToolCalls(mapped); err != nil {
								streamErr = err
							}
						} else if cmd := translator.ExtractNativeShellCommand(ev.ToolCall.Args); cmd != "" {
							cts.NoteSuppressedNativeShell(cmd)
						} else if cfg.Logger != nil {
							cfg.Logger.Debug("cursor native tool suppressed; use client tool_calls",
								"name", name, "call_id", ev.ToolCall.ID)
						}
					}
					if cfg.Store != nil {
						cfg.Store.AddToolCall(cfg.TraceID, name, "native_suppressed", ev.ToolCall.ID)
					}
					continue
				}
				if err := sink.OnToolCall(ev.ToolCall.ID, name, ev.ToolCall.Args); err != nil {
					streamErr = err
				}
				if cfg.Store != nil {
					cfg.Store.AddToolCall(cfg.TraceID, ev.ToolCall.Title, "pending", ev.ToolCall.ID)
				}
				continue
			}
			if cfg.CursorCfg != nil && cfg.CursorCfg.IsModelProfile() {
				sink.OnToolBlocked(ev.ToolCall.ID)
				streamErr = cursor.ErrModelOnlyToolUse
				break
			}

		case ir.EventToolCallUpdate:
			// trace only for v2
			if cfg.Store != nil && ev.ToolCall != nil {
				cfg.Store.AddToolCall(cfg.TraceID, "tool_update", ev.ToolCall.Status, ev.ToolCall.ID)
			}

		case ir.EventPlan:
			if cfg.CursorCfg == nil || cfg.CursorCfg.AgentProfile != "model" {
				fullText += ev.Text
				_ = sink.OnContentDelta(ev.Text)
			}

		case ir.EventModeUpdate:
			if cfg.Store != nil {
				cfg.Store.AddPhase(cfg.TraceID, "mode", ev.ModeID)
			}

		case ir.EventDone:
			fr := toolloop.StopReasonToFinish(toolloop.StopReasonFromString(ev.StopReason))
			_ = sink.OnDone(usage, fullText, fr)

		case ir.EventError:
			if ev.Err != nil && sink.OnError(ev.Err) {
				streamErr = ev.Err
			}
		}
	}

	if streamErr != nil {
		return fullText, usage, streamErr
	}

	select {
	case err := <-result.ErrCh:
		if err != nil && !result.Stopped() {
			if ce, ok := err.(*runner.ClassifiedErr); ok {
				_ = ce
			}
			if sink.OnError(err) {
				return fullText, usage, err
			}
		}
	case <-time.After(150 * time.Millisecond):
	}

	return fullText, usage, nil
}

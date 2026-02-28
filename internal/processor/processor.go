// Package processor handles one LLM turn: consuming the SSE stream,
// executing tool calls, and persisting parts to the session store.
//
// This mirrors OpenCode's SessionProcessor.process().
package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/nolouch/gcode/internal/bus"
	"github.com/nolouch/gcode/internal/llm"
	"github.com/nolouch/gcode/internal/model"
	"github.com/nolouch/gcode/internal/session"
	"github.com/nolouch/gcode/internal/tool"
)

const doomLoopThreshold = 3

// Processor handles one assistant message turn.
type Processor struct {
	store   *session.Store
	Bus     *bus.Bus
	Message *model.Message
}

// New creates a Processor for the given (pre-saved) assistant message.
func New(store *session.Store, b *bus.Bus, msg *model.Message) *Processor {
	return &Processor{store: store, Bus: b, Message: msg}
}

func (p *Processor) publish(e bus.Event) {
	if p.Bus != nil {
		p.Bus.Publish(e)
	}
}

// Process consumes a stream from the LLM, executes tools, and updates the
// message in the store. Returns a ProcessResult for the outer loop.
func (p *Processor) Process(
	ctx context.Context,
	streamCh <-chan llm.StreamEvent,
	tools map[string]tool.Tool,
	workDir string,
) (model.ProcessResult, []model.ChatMessage) {
	var (
		currentTextPart *model.Part
		toolParts       = make(map[string]*model.Part)
		toolArgsAcc     = make(map[string]*strings.Builder)
		toolMessages    []model.ChatMessage

		thinkingStart   time.Time
		thinkingContent strings.Builder
		toolStartTimes  = make(map[string]time.Time)
	)

	partID := func() string { return session.NewID() }
	sid := p.Message.SessionID
	mid := p.Message.ID

	for event := range streamCh {
		select {
		case <-ctx.Done():
			p.Message.Finish = "abort"
			p.Message.Error = &model.AgentError{Name: "AbortedError", Message: "cancelled"}
			p.store.UpdateMessage(p.Message)
			return model.ProcessResultStop, nil
		default:
		}

		switch event.Type {
		case llm.TypeReasoningDelta:
			if thinkingContent.Len() == 0 {
				thinkingStart = time.Now()
			}
			thinkingContent.WriteString(event.ReasoningDelta)
			p.publish(bus.Event{
				Type: bus.EventThinking, SessionID: sid, MessageID: mid,
				Payload: bus.ThinkingPayload{Delta: event.ReasoningDelta},
			})

		case llm.TypeTextDelta:
			if thinkingContent.Len() > 0 {
				durationMs := float64(time.Since(thinkingStart).Milliseconds())
				p.publish(bus.Event{
					Type: bus.EventThinkingDone, SessionID: sid, MessageID: mid,
					Payload: bus.ThinkingPayload{Duration: durationMs},
				})
				thinkingContent.Reset()
			}
			if currentTextPart == nil {
				pt := model.Part{
					ID: partID(), SessionID: sid, MessageID: mid,
					Type: model.PartTypeText,
				}
				p.Message.Parts = append(p.Message.Parts, pt)
				currentTextPart = &p.Message.Parts[len(p.Message.Parts)-1]
				p.store.UpdateMessage(p.Message)
			}
			currentTextPart.Text += event.TextDelta
			p.store.UpsertPart(sid, mid, *currentTextPart)
			p.publish(bus.Event{
				Type: bus.EventTextDelta, SessionID: sid, MessageID: mid,
				Payload: bus.TextDeltaPayload{Delta: event.TextDelta},
			})

		case llm.TypeToolCallStart:
			if thinkingContent.Len() > 0 {
				durationMs := float64(time.Since(thinkingStart).Milliseconds())
				p.publish(bus.Event{
					Type: bus.EventThinkingDone, SessionID: sid, MessageID: mid,
					Payload: bus.ThinkingPayload{Duration: durationMs},
				})
				thinkingContent.Reset()
			}
			toolArgsAcc[event.ToolCallID] = &strings.Builder{}
			toolStartTimes[event.ToolCallID] = time.Now()
			p.publish(bus.Event{
				Type: bus.EventToolStart, SessionID: sid, MessageID: mid,
				Payload: bus.ToolPayload{CallID: event.ToolCallID, Tool: event.ToolCallName},
			})
			pt := model.Part{
				ID: partID(), SessionID: sid, MessageID: mid,
				Type: model.PartTypeTool,
				Tool: &model.ToolPart{
					CallID: event.ToolCallID,
					Tool:   event.ToolCallName,
					State:  model.ToolStatePending,
				},
			}
			p.Message.Parts = append(p.Message.Parts, pt)
			toolParts[event.ToolCallID] = &p.Message.Parts[len(p.Message.Parts)-1]
			p.store.UpdateMessage(p.Message)

		case llm.TypeToolCallDelta:
			if b, ok := toolArgsAcc[event.ToolCallID]; ok {
				b.WriteString(event.ArgsDelta)
			}

		case llm.TypeToolCallDone:
			tp, ok := toolParts[event.ToolCallID]
			if !ok {
				continue
			}
			argsRaw := strings.TrimSpace(toolArgsAcc[event.ToolCallID].String())
			if argsRaw == "" {
				argsRaw = strings.TrimSpace(event.ArgsDelta)
			}
			if argsRaw == "" {
				argsRaw = "{}"
			}
			var args map[string]any
			if err := json.Unmarshal([]byte(argsRaw), &args); err != nil {
				errMsg := fmt.Sprintf("invalid tool args JSON: %v; raw=%q", err, argsRaw)
				tp.Tool.State = model.ToolStateError
				tp.Tool.Input = map[string]any{}
				tp.Tool.Error = errMsg
				tp.Tool.EndAt = time.Now()
				p.store.UpsertPart(sid, mid, *tp)
				p.publish(bus.Event{
					Type: bus.EventToolError, SessionID: sid, MessageID: mid,
					Payload: bus.ToolPayload{CallID: event.ToolCallID, Tool: event.ToolCallName, Output: errMsg, IsError: true},
				})
				toolMessages = append(toolMessages, model.ChatMessage{
					Role: "tool", ToolCallID: event.ToolCallID,
					Name: event.ToolCallName, Content: errMsg,
				})
				continue
			}
			if args == nil {
				args = map[string]any{}
			}

			tp.Tool.State = model.ToolStateRunning
			tp.Tool.Input = args
			tp.Tool.StartAt = time.Now()
			p.store.UpsertPart(sid, mid, *tp)

			if p.doomLoop(event.ToolCallName, argsRaw) {
				errMsg := "doom loop detected"
				tp.Tool.State = model.ToolStateError
				tp.Tool.Error = errMsg
				tp.Tool.EndAt = time.Now()
				p.store.UpsertPart(sid, mid, *tp)
				p.Message.Finish = "stop"
				p.store.UpdateMessage(p.Message)
				p.publish(bus.Event{
					Type: bus.EventToolError, SessionID: sid, MessageID: mid,
					Payload: bus.ToolPayload{CallID: event.ToolCallID, Tool: event.ToolCallName, Output: errMsg, IsError: true},
				})
				return model.ProcessResultStop, nil
			}

			// Execute tool
			t, exists := tools[event.ToolCallName]
			var result tool.Result
			if !exists {
				result = tool.Result{IsError: true, Output: fmt.Sprintf("unknown tool: %s", event.ToolCallName)}
			} else {
				tctx := tool.Context{
					SessionID: sid, MessageID: mid,
					CallID: event.ToolCallID, WorkDir: workDir, Ctx: ctx,
				}
				result, _ = t.Execute(tctx, args)
			}

			startTime := toolStartTimes[event.ToolCallID]
			durationMs := time.Since(startTime).Milliseconds()
			delete(toolStartTimes, event.ToolCallID)
			tp.Tool.EndAt = time.Now()

			if result.IsError {
				tp.Tool.State = model.ToolStateError
				tp.Tool.Error = result.Output
				p.store.UpsertPart(sid, mid, *tp)
				p.publish(bus.Event{
					Type: bus.EventToolError, SessionID: sid, MessageID: mid,
					Payload: bus.ToolPayload{
						CallID: event.ToolCallID, Tool: event.ToolCallName,
						Input: args, Output: result.Output, IsError: true, DurationMs: durationMs,
					},
				})
			} else {
				tp.Tool.State = model.ToolStateCompleted
				tp.Tool.Output = result.Output
				p.store.UpsertPart(sid, mid, *tp)
				p.publish(bus.Event{
					Type: bus.EventToolDone, SessionID: sid, MessageID: mid,
					Payload: bus.ToolPayload{
						CallID: event.ToolCallID, Tool: event.ToolCallName,
						Input: args, Output: result.Output, DurationMs: durationMs,
					},
				})
			}

			toolMessages = append(toolMessages, model.ChatMessage{
				Role: "tool", ToolCallID: event.ToolCallID,
				Name: event.ToolCallName, Content: result.Output,
			})

		case llm.TypeStepFinish:
			if event.FinishReason != "" {
				p.Message.Finish = event.FinishReason
			}
			p.Message.Tokens.Input += event.Usage.Input
			p.Message.Tokens.Output += event.Usage.Output
			sfPart := model.Part{
				ID: partID(), SessionID: sid, MessageID: mid,
				Type: model.PartTypeStepFinish,
				StepFinish: &model.StepFinishPart{
					Reason: event.FinishReason,
					Tokens: event.Usage,
				},
			}
			p.Message.Parts = append(p.Message.Parts, sfPart)
			p.store.UpdateMessage(p.Message)
			p.publish(bus.Event{
				Type: bus.EventTurnDone, SessionID: sid, MessageID: mid,
				Payload: bus.TurnDonePayload{FinishReason: event.FinishReason},
			})

		case llm.TypeError:
			p.Message.Error = &model.AgentError{
				Name: "APIError", Message: event.Err.Error(), Retryable: true,
			}
			p.store.UpdateMessage(p.Message)
			p.publish(bus.Event{
				Type: bus.EventTurnError, SessionID: sid, MessageID: mid,
				Payload: bus.TurnDonePayload{FinishReason: "error"},
			})
			return model.ProcessResultStop, nil
		}
	}

	p.Message.Parts = compactTextParts(p.Message.Parts)
	p.store.UpdateMessage(p.Message)

	if p.Message.Finish == "tool_calls" || p.Message.Finish == "tool-calls" {
		return model.ProcessResultContinue, toolMessages
	}
	return model.ProcessResultStop, nil
}

func (p *Processor) doomLoop(toolName, argsRaw string) bool {
	var count int
	for i := len(p.Message.Parts) - 1; i >= 0; i-- {
		pt := p.Message.Parts[i]
		if pt.Type != model.PartTypeTool || pt.Tool == nil {
			continue
		}
		if pt.Tool.State == model.ToolStatePending {
			continue
		}
		argBytes, _ := json.Marshal(pt.Tool.Input)
		if pt.Tool.Tool == toolName && string(argBytes) == argsRaw {
			count++
		} else {
			break
		}
		if count >= doomLoopThreshold {
			return true
		}
	}
	return false
}

func compactTextParts(parts []model.Part) []model.Part {
	for i := range parts {
		if parts[i].Type == model.PartTypeText {
			parts[i].Text = strings.TrimRight(parts[i].Text, "\n ")
		}
	}
	return parts
}

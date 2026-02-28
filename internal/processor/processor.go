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
	"github.com/nolouch/gcode/internal/permission"
	"github.com/nolouch/gcode/internal/session"
	"github.com/nolouch/gcode/internal/tool"
)

const doomLoopThreshold = 3

// Processor handles one assistant message turn.
type Processor struct {
	store   session.StoreAPI
	Bus     *bus.Bus
	Message *model.Message
	// Authorize optionally overrides tool policy checks.
	Authorize func(agentName, toolName string, args map[string]any) (bool, string)
}

// New creates a Processor for the given (pre-saved) assistant message.
func New(store session.StoreAPI, b *bus.Bus, msg *model.Message) *Processor {
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
		currentTextPart      *model.Part
		currentReasoningPart *model.Part
		toolParts            = make(map[string]*model.Part)
		toolArgsAcc          = make(map[string]*strings.Builder)
		toolMessages         []model.ChatMessage

		thinkingStart  time.Time
		toolStartTimes = make(map[string]time.Time)
	)

	partID := func() string { return session.NewID() }
	sid := p.Message.SessionID
	mid := p.Message.ID
	publishPartUpsert := func(part model.Part) {
		p.publish(bus.Event{
			Type: bus.EventPartUpsert, SessionID: sid, MessageID: mid,
			Payload: bus.PartUpsertPayload{Part: part},
		})
	}
	publishPartDelta := func(partID string, partType model.PartType, field, delta string) {
		if delta == "" {
			return
		}
		p.publish(bus.Event{
			Type: bus.EventPartDelta, SessionID: sid, MessageID: mid,
			Payload: bus.PartDeltaPayload{PartID: partID, PartType: partType, Field: field, Delta: delta},
		})
	}
	flushThinking := func() {
		if currentReasoningPart == nil {
			return
		}
		currentReasoningPart.Text = strings.TrimRight(currentReasoningPart.Text, " \t\n\r")
		p.store.UpsertPart(sid, mid, *currentReasoningPart)
		publishPartUpsert(*currentReasoningPart)

		durationMs := 0.0
		if !thinkingStart.IsZero() {
			durationMs = float64(time.Since(thinkingStart).Milliseconds())
		}
		p.publish(bus.Event{
			Type: bus.EventPartDone, SessionID: sid, MessageID: mid,
			Payload: bus.PartDonePayload{PartID: currentReasoningPart.ID, PartType: currentReasoningPart.Type, DurationMs: durationMs},
		})

		currentReasoningPart = nil
		thinkingStart = time.Time{}
	}

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
			if currentReasoningPart == nil {
				thinkingStart = time.Now()
				rp := model.Part{
					ID: partID(), SessionID: sid, MessageID: mid,
					Type: model.PartTypeReasoning,
				}
				p.Message.Parts = append(p.Message.Parts, rp)
				currentReasoningPart = &p.Message.Parts[len(p.Message.Parts)-1]
				p.store.UpdateMessage(p.Message)
				publishPartUpsert(*currentReasoningPart)
			}
			currentReasoningPart.Text += event.ReasoningDelta
			p.store.UpsertPart(sid, mid, *currentReasoningPart)
			publishPartDelta(currentReasoningPart.ID, currentReasoningPart.Type, "text", event.ReasoningDelta)

		case llm.TypeTextDelta:
			flushThinking()
			if currentTextPart == nil {
				pt := model.Part{
					ID: partID(), SessionID: sid, MessageID: mid,
					Type: model.PartTypeText,
				}
				p.Message.Parts = append(p.Message.Parts, pt)
				currentTextPart = &p.Message.Parts[len(p.Message.Parts)-1]
				p.store.UpdateMessage(p.Message)
				publishPartUpsert(*currentTextPart)
			}
			currentTextPart.Text += event.TextDelta
			p.store.UpsertPart(sid, mid, *currentTextPart)
			publishPartDelta(currentTextPart.ID, currentTextPart.Type, "text", event.TextDelta)

		case llm.TypeToolCallStart:
			flushThinking()
			toolArgsAcc[event.ToolCallID] = &strings.Builder{}
			toolStartTimes[event.ToolCallID] = time.Now()
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
			publishPartUpsert(*toolParts[event.ToolCallID])

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
				publishPartUpsert(*tp)
				p.publish(bus.Event{
					Type: bus.EventPartDone, SessionID: sid, MessageID: mid,
					Payload: bus.PartDonePayload{PartID: tp.ID, PartType: tp.Type},
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
			publishPartUpsert(*tp)

			if p.doomLoop(event.ToolCallName, argsRaw) {
				errMsg := "doom loop detected"
				tp.Tool.State = model.ToolStateError
				tp.Tool.Error = errMsg
				tp.Tool.EndAt = time.Now()
				p.store.UpsertPart(sid, mid, *tp)
				publishPartUpsert(*tp)
				p.Message.Finish = "stop"
				p.store.UpdateMessage(p.Message)
				p.publish(bus.Event{
					Type: bus.EventPartDone, SessionID: sid, MessageID: mid,
					Payload: bus.PartDonePayload{PartID: tp.ID, PartType: tp.Type},
				})
				return model.ProcessResultStop, nil
			}

			// Execute tool
			t, exists := tools[event.ToolCallName]
			var result tool.Result
			authorize := p.Authorize
			if authorize == nil {
				authorize = denyToolByPolicy
			}
			if denied, reason := authorize(p.Message.Agent, event.ToolCallName, args); denied {
				result = tool.Result{IsError: true, Output: reason}
			} else if !exists {
				result = tool.Result{IsError: true, Output: fmt.Sprintf("unknown tool: %s", event.ToolCallName)}
			} else if err := tool.ValidateArgs(t.Schema(), args); err != nil {
				result = tool.Result{IsError: true, Output: fmt.Sprintf("invalid tool args for %s: %v", event.ToolCallName, err)}
			} else {
				tctx := tool.Context{
					SessionID: sid, MessageID: mid,
					CallID: event.ToolCallID, Agent: p.Message.Agent,
					WorkDir: workDir, Ctx: ctx,
				}
				result, _ = t.Execute(tctx, args)
			}
			result = tool.NormalizeResult(result)

			startTime := toolStartTimes[event.ToolCallID]
			durationMs := time.Since(startTime).Milliseconds()
			delete(toolStartTimes, event.ToolCallID)
			tp.Tool.EndAt = time.Now()

			if result.IsError {
				tp.Tool.State = model.ToolStateError
				tp.Tool.Error = result.Output
				p.store.UpsertPart(sid, mid, *tp)
				publishPartUpsert(*tp)
			} else {
				tp.Tool.State = model.ToolStateCompleted
				tp.Tool.Output = result.Output
				p.store.UpsertPart(sid, mid, *tp)
				publishPartUpsert(*tp)
			}
			p.publish(bus.Event{
				Type: bus.EventPartDone, SessionID: sid, MessageID: mid,
				Payload: bus.PartDonePayload{PartID: tp.ID, PartType: tp.Type, DurationMs: float64(durationMs)},
			})

			toolMessages = append(toolMessages, model.ChatMessage{
				Role: "tool", ToolCallID: event.ToolCallID,
				Name: event.ToolCallName, Content: result.Output,
			})

		case llm.TypeStepFinish:
			flushThinking()
			if event.FinishReason != "" {
				p.Message.Finish = normalizeFinishReason(event.FinishReason)
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
			publishPartUpsert(sfPart)
			p.publish(bus.Event{
				Type: bus.EventTurnDone, SessionID: sid, MessageID: mid,
				Payload: bus.TurnDonePayload{FinishReason: event.FinishReason},
			})

		case llm.TypeError:
			flushThinking()
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

	flushThinking()

	p.Message.Parts = compactTextParts(p.Message.Parts)
	p.store.UpdateMessage(p.Message)

	finish := normalizeFinishReason(p.Message.Finish)
	p.Message.Finish = finish
	p.store.UpdateMessage(p.Message)

	if finish == "tool-calls" {
		return model.ProcessResultContinue, toolMessages
	}
	if finish == "length" {
		return model.ProcessResultCompact, toolMessages
	}
	if finish == "unknown" && len(toolMessages) > 0 {
		return model.ProcessResultContinue, toolMessages
	}
	return model.ProcessResultStop, nil
}

func normalizeFinishReason(reason string) string {
	s := strings.ToLower(strings.TrimSpace(reason))
	s = strings.ReplaceAll(s, "_", "-")
	if s == "" {
		return "unknown"
	}
	if s == "tool-calls" {
		return "tool-calls"
	}
	if s == "max-tokens" {
		return "length"
	}
	return s
}

func denyToolByPolicy(agentName, toolName string, args map[string]any) (bool, string) {
	return permission.AuthorizeTool(agentName, toolName, args, nil)
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

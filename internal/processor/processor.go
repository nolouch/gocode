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

	"github.com/nolouch/gcode/internal/llm"
	"github.com/nolouch/gcode/internal/model"
	"github.com/nolouch/gcode/internal/session"
	"github.com/nolouch/gcode/internal/tool"
)

const doomLoopThreshold = 3

// Processor handles one assistant message turn.
type Processor struct {
	store   *session.Store
	Message *model.Message
}

// New creates a Processor for the given (pre-saved) assistant message.
func New(store *session.Store, msg *model.Message) *Processor {
	return &Processor{store: store, Message: msg}
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
		toolParts       = make(map[string]*model.Part) // callID -> part
		toolArgsAcc     = make(map[string]*strings.Builder)
		toolMessages    []model.ChatMessage // tool results to append to history

		// Thinking tracking
		thinkingStart   time.Time
		thinkingContent strings.Builder

		// Tool tracking
		toolStartTimes = make(map[string]time.Time)
	)

	partID := func() string { return session.NewID() }

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
			// Stream thinking content
			if thinkingContent.Len() == 0 {
				thinkingStart = time.Now()
				fmt.Print(ThinkingHeader.Render("💭 Thinking: "))
			}
			thinkingContent.WriteString(event.ReasoningDelta)
			// Print content chunk
			fmt.Print(event.ReasoningDelta)

		case llm.TypeTextDelta:
			// Close thinking block if it was open
			if thinkingContent.Len() > 0 {
				duration := time.Since(thinkingStart).Round(time.Millisecond * 100)
				// Print duration
				fmt.Println()
				fmt.Println(ThinkingDone.Render("✓ done (" + duration.String() + ")"))
				thinkingContent.Reset()
			}
			if currentTextPart == nil {
				pt := model.Part{
					ID:        partID(),
					SessionID: p.Message.SessionID,
					MessageID: p.Message.ID,
					Type:      model.PartTypeText,
				}
				p.Message.Parts = append(p.Message.Parts, pt)
				currentTextPart = &p.Message.Parts[len(p.Message.Parts)-1]
				p.store.UpdateMessage(p.Message)
			}
			currentTextPart.Text += event.TextDelta
			// Live-update the part in the store
			p.store.UpsertPart(p.Message.SessionID, p.Message.ID, *currentTextPart)
			fmt.Print(event.TextDelta) // stream to stdout

		case llm.TypeToolCallStart:
			// Close thinking block if it was open
			if thinkingContent.Len() > 0 {
				duration := time.Since(thinkingStart).Round(time.Millisecond * 100)
				fmt.Println()
				fmt.Println(ThinkingDone.Render("✓ done (" + duration.String() + ")"))
				thinkingContent.Reset()
			}

			toolArgsAcc[event.ToolCallID] = &strings.Builder{}
			toolStartTimes[event.ToolCallID] = time.Now()
			// Print tool with running state
			fmt.Printf("\n%s %s\n",
				RunningIcon.Render("●"),
				ToolHeader.Render("Tool: "+event.ToolCallName))
			pt := model.Part{
				ID:        partID(),
				SessionID: p.Message.SessionID,
				MessageID: p.Message.ID,
				Type:      model.PartTypeTool,
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
				// Some providers only send full arguments in the final tool-done event.
				argsRaw = strings.TrimSpace(event.ArgsDelta)
			}
			if argsRaw == "" {
				argsRaw = "{}"
			}
			var args map[string]any
			if err := json.Unmarshal([]byte(argsRaw), &args); err != nil {
				errMsg := fmt.Sprintf("invalid tool args JSON: %v; raw=%q", err, argsRaw)
				fmt.Printf("%s %s\n", ErrorIcon.Render("✗"), ToolHeader.Render("Tool: "+event.ToolCallName))
				fmt.Printf("[tool-error] %s\n", errMsg)

				tp.Tool.State = model.ToolStateError
				tp.Tool.Input = map[string]any{}
				tp.Tool.Error = errMsg
				tp.Tool.EndAt = time.Now()
				p.store.UpsertPart(p.Message.SessionID, p.Message.ID, *tp)

				toolMessages = append(toolMessages, model.ChatMessage{
					Role:       "tool",
					ToolCallID: event.ToolCallID,
					Name:       event.ToolCallName,
					Content:    errMsg,
				})
				continue
			}
			if args == nil {
				args = map[string]any{}
			}

			// Mark running
			tp.Tool.State = model.ToolStateRunning
			tp.Tool.Input = args
			tp.Tool.StartAt = time.Now()
			p.store.UpsertPart(p.Message.SessionID, p.Message.ID, *tp)

			// Doom-loop detection (same tool+args 3 times in a row)
			if p.doomLoop(event.ToolCallName, argsRaw) {
				fmt.Printf("\n[warn] doom loop detected for tool %s, stopping\n", event.ToolCallName)
				tp.Tool.State = model.ToolStateError
				tp.Tool.Error = "doom loop detected"
				tp.Tool.EndAt = time.Now()
				p.store.UpsertPart(p.Message.SessionID, p.Message.ID, *tp)
				p.Message.Finish = "stop"
				p.store.UpdateMessage(p.Message)
				return model.ProcessResultStop, nil
			}

			// Execute tool
			t, exists := tools[event.ToolCallName]
			var result tool.Result
			if !exists {
				result = tool.Result{IsError: true, Output: fmt.Sprintf("unknown tool: %s", event.ToolCallName)}
			} else {
				tctx := tool.Context{
					SessionID: p.Message.SessionID,
					MessageID: p.Message.ID,
					CallID:    event.ToolCallID,
					WorkDir:   workDir,
					Ctx:       ctx,
				}
				result, _ = t.Execute(tctx, args)
			}

			// Mark completed / error
			startTime, hasStart := toolStartTimes[event.ToolCallID]
			var duration string
			if hasStart {
				duration = time.Since(startTime).Round(time.Millisecond * 100).String()
			}
			if result.IsError {
				fmt.Printf("%s %s (%s)\n",
					ErrorIcon.Render("✗"),
					ToolHeader.Render("Tool: "+event.ToolCallName),
					duration)
				fmt.Printf("[tool-error] %s\n", result.Output)
				tp.Tool.State = model.ToolStateError
				tp.Tool.Error = result.Output
			} else {
				fmt.Printf("%s %s (%s)\n",
					SuccessIcon.Render("✓"),
					ToolHeader.Render("Tool: "+event.ToolCallName),
					duration)
				// Print tool output (truncated if too long)
				output := result.Output
				if len(output) > 500 {
					output = output[:500] + "\n... (truncated)"
				}
				fmt.Printf("%s\n", output)
				tp.Tool.State = model.ToolStateCompleted
				tp.Tool.Output = result.Output
			}
			delete(toolStartTimes, event.ToolCallID)
			tp.Tool.EndAt = time.Now()
			p.store.UpsertPart(p.Message.SessionID, p.Message.ID, *tp)

			// Build tool result message for the next LLM turn
			toolMessages = append(toolMessages, model.ChatMessage{
				Role:       "tool",
				ToolCallID: event.ToolCallID,
				Name:       event.ToolCallName,
				Content:    result.Output,
			})

		case llm.TypeStepFinish:
			if event.FinishReason != "" {
				p.Message.Finish = event.FinishReason
			}
			p.Message.Tokens.Input += event.Usage.Input
			p.Message.Tokens.Output += event.Usage.Output
			// persist step-finish part
			sfPart := model.Part{
				ID:        partID(),
				SessionID: p.Message.SessionID,
				MessageID: p.Message.ID,
				Type:      model.PartTypeStepFinish,
				StepFinish: &model.StepFinishPart{
					Reason: event.FinishReason,
					Tokens: event.Usage,
				},
			}
			p.Message.Parts = append(p.Message.Parts, sfPart)
			p.store.UpdateMessage(p.Message)
			fmt.Printf("\n[step] finish_reason=%s input=%d output=%d\n",
				event.FinishReason, event.Usage.Input, event.Usage.Output)

		case llm.TypeError:
			fmt.Printf("\n[error] LLM API error: %v\n", event.Err)
			p.Message.Error = &model.AgentError{
				Name:      "APIError",
				Message:   event.Err.Error(),
				Retryable: true,
			}
			p.store.UpdateMessage(p.Message)
			return model.ProcessResultStop, nil
		}
	}

	// Finalise message
	p.Message.Parts = compactTextParts(p.Message.Parts) // trim trailing newlines
	p.store.UpdateMessage(p.Message)

	if p.Message.Finish == "tool_calls" || p.Message.Finish == "tool-calls" {
		return model.ProcessResultContinue, toolMessages
	}
	return model.ProcessResultStop, nil
}

// doomLoop checks whether the last 3 tool parts have the same tool+args.
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

// compactTextParts trims trailing whitespace from text parts.
func compactTextParts(parts []model.Part) []model.Part {
	for i := range parts {
		if parts[i].Type == model.PartTypeText {
			parts[i].Text = strings.TrimRight(parts[i].Text, "\n ")
		}
	}
	return parts
}

// Package loop implements the core agent loop, mirroring OpenCode's
// SessionPrompt.loop(). Each iteration:
//  1. Finds the last user message.
//  2. Checks if the last assistant message is already done (exit condition).
//  3. Creates a new assistant message.
//  4. Calls the LLM (streaming).
//  5. Runs the Processor to handle tool calls and text deltas.
//  6. Decides: stop | continue (more tool calls) | compact (not yet implemented).
package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/nolouch/gcode/internal/agent"
	"github.com/nolouch/gcode/internal/llm"
	"github.com/nolouch/gcode/internal/model"
	"github.com/nolouch/gcode/internal/processor"
	"github.com/nolouch/gcode/internal/session"
	"github.com/nolouch/gcode/internal/tool"
)

const maxRetries = 3

// Runner is the top-level agent loop runner.
type Runner struct {
	Store             *session.Store
	LLM               *llm.Client
	Agents            *agent.Registry
	Tools             map[string]tool.Tool // built-ins + MCP
	SystemPromptExtra []string             // skill system prompt fragments
}

// Run executes the agent loop for the given session + user message text.
// It blocks until the agent is done or ctx is cancelled.
func (r *Runner) Run(ctx context.Context, sessionID string, userText string, agentName string) error {
	store := r.Store

	// 1. Save the user message
	userMsg := &model.Message{
		ID:        session.NewID(),
		SessionID: sessionID,
		Role:      model.RoleUser,
		CreatedAt: time.Now(),
		Text:      userText,
	}
	userMsg.Parts = []model.Part{{
		ID:        session.NewID(),
		SessionID: sessionID,
		MessageID: userMsg.ID,
		Type:      model.PartTypeText,
		Text:      userText,
	}}
	store.AddMessage(userMsg)
	store.TouchSession(sessionID)

	sess, err := store.GetSession(sessionID)
	if err != nil {
		return err
	}

	ag := r.Agents.Get(agentName)
	maxSteps := ag.MaxSteps
	if maxSteps == 0 {
		maxSteps = 100 // safety cap
	}

	// ─── Build the effective tool set for this agent ───────────────
	effectiveTools := make(map[string]tool.Tool)
	for id, t := range r.Tools {
		if ag.DeniedTools[id] {
			continue
		}
		if sess.DeniedTools[id] {
			continue
		}
		effectiveTools[id] = t
	}

	// ─── Build base system prompt ──────────────────────────────────
	baseSystem := buildSystemPrompt(ag, r.SystemPromptExtra, sess.Directory)

	// ─── Accumulated conversation history (beyond what's in the store) ─
	// We reconstruct the full history from the store on each step so
	// the LLM always sees tool results.
	var extraMessages []model.ChatMessage // tool-result messages from previous step

	step := 0
	for {
		step++
		if step > maxSteps {
			fmt.Printf("[loop] max steps (%d) reached\n", maxSteps)
			break
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Check exit: if the last assistant message is fully finished
		lastAsst := store.LastAssistantMessage(sessionID)
		if lastAsst != nil && lastAsst.Finish != "" &&
			lastAsst.Finish != "tool_calls" && lastAsst.Finish != "tool-calls" &&
			lastAsst.Error == nil {
			fmt.Printf("\n[loop] done at step %d (finish=%s)\n", step, lastAsst.Finish)
			break
		}

		fmt.Printf("\n[loop] step %d\n", step)

		// ── Build message history for this LLM call ──────────────
		history := buildHistory(store.Messages(sessionID))
		history = append(history, extraMessages...)
		extraMessages = nil

		// ── Create the assistant message placeholder ──────────────
		asstMsg := &model.Message{
			ID:        session.NewID(),
			SessionID: sessionID,
			Role:      model.RoleAssistant,
			CreatedAt: time.Now(),
			Agent:     ag.Name,
		}
		store.AddMessage(asstMsg)

		// ── Stream from LLM ────────────────────────────────────────
		streamInput := llm.StreamInput{
			Messages:  history,
			Tools:     toolList(effectiveTools),
			System:    baseSystem,
			MaxTokens: 8192,
			Abort:     ctx,
		}

		var streamCh <-chan llm.StreamEvent
		var streamErr error
		for attempt := 0; attempt <= maxRetries; attempt++ {
			streamCh, streamErr = r.LLM.Stream(streamInput)
			if streamErr == nil {
				break
			}
			if attempt < maxRetries {
				wait := time.Duration(1<<uint(attempt)) * time.Second
				fmt.Printf("[loop] LLM error (attempt %d): %v, retrying in %v\n", attempt+1, streamErr, wait)
				select {
				case <-time.After(wait):
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
		if streamErr != nil {
			return fmt.Errorf("LLM stream failed: %w", streamErr)
		}

		// ── Process stream ─────────────────────────────────────────
		proc := processor.New(store, asstMsg)
		result, toolMsgs := proc.Process(ctx, streamCh, effectiveTools, sess.Directory)
		extraMessages = toolMsgs

		switch result {
		case model.ProcessResultStop:
			break // inner
		case model.ProcessResultContinue:
			// Append assistant tool-call message to the history for the next step
			extraMessages = append([]model.ChatMessage{buildAssistantToolCallMsg(asstMsg)}, extraMessages...)
			continue
		case model.ProcessResultCompact:
			// Simple compaction: keep only the last user message
			// (MVP: a real implementation would summarise)
			fmt.Printf("[loop] context compaction triggered (not fully implemented)\n")
		}

		// Stop
		break
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

// buildSystemPrompt assembles the system messages array.
func buildSystemPrompt(ag *agent.Info, extras []string, workDir string) []string {
	base := fmt.Sprintf(`You are gcode, an expert AI coding assistant.
Current working directory: %s
You can use tools to read files, write files, run commands, and explore the codebase.
Think step-by-step. Be concise.`, workDir)

	if ag.Prompt != "" {
		base += "\n" + ag.Prompt
	}

	result := []string{base}
	result = append(result, extras...)
	return result
}

// buildHistory converts stored messages to the ChatMessage format for the LLM.
func buildHistory(msgs []*model.Message) []model.ChatMessage {
	var out []model.ChatMessage
	for _, m := range msgs {
		switch m.Role {
		case model.RoleUser:
			text := m.Text
			if text == "" {
				for _, p := range m.Parts {
					if p.Type == model.PartTypeText {
						text = p.Text
						break
					}
				}
			}
			if text == "" {
				continue
			}
			out = append(out, model.ChatMessage{Role: "user", Content: text})

		case model.RoleAssistant:
			if m.Error != nil {
				continue // skip errored turns
			}
			if m.Finish == "" {
				continue // in-progress, skip
			}
			// Text parts
			var textBuilder strings.Builder
			var toolCalls []model.ToolCall
			for _, p := range m.Parts {
				switch p.Type {
				case model.PartTypeText:
					textBuilder.WriteString(p.Text)
				case model.PartTypeTool:
					if p.Tool == nil {
						continue
					}
					argsB, _ := json.Marshal(p.Tool.Input)
					toolCalls = append(toolCalls, model.ToolCall{
						ID:   p.Tool.CallID,
						Type: "function",
						Function: model.FunctionCall{
							Name:      p.Tool.Tool,
							Arguments: string(argsB),
						},
					})
				}
			}
			msg := model.ChatMessage{Role: "assistant"}
			if len(toolCalls) > 0 {
				msg.ToolCalls = toolCalls
				msg.Content = textBuilder.String()
			} else {
				msg.Content = textBuilder.String()
			}
			out = append(out, msg)

			// Tool results (completed tool parts become tool messages)
			for _, p := range m.Parts {
				if p.Type != model.PartTypeTool || p.Tool == nil {
					continue
				}
				output := p.Tool.Output
				if p.Tool.State == model.ToolStateError {
					output = "[error] " + p.Tool.Error
				}
				out = append(out, model.ChatMessage{
					Role:       "tool",
					ToolCallID: p.Tool.CallID,
					Name:       p.Tool.Tool,
					Content:    output,
				})
			}
		}
	}
	return out
}

// buildAssistantToolCallMsg creates the assistant message containing tool calls
// to prepend to the next LLM turn when the finish reason is "tool_calls".
func buildAssistantToolCallMsg(m *model.Message) model.ChatMessage {
	var toolCalls []model.ToolCall
	for _, p := range m.Parts {
		if p.Type != model.PartTypeTool || p.Tool == nil {
			continue
		}
		argsB, _ := json.Marshal(p.Tool.Input)
		toolCalls = append(toolCalls, model.ToolCall{
			ID:   p.Tool.CallID,
			Type: "function",
			Function: model.FunctionCall{
				Name:      p.Tool.Tool,
				Arguments: string(argsB),
			},
		})
	}
	return model.ChatMessage{Role: "assistant", ToolCalls: toolCalls}
}

// toolList converts a map of Tool to a slice for LLM input.
func toolList(m map[string]tool.Tool) []tool.Tool {
	out := make([]tool.Tool, 0, len(m))
	for _, t := range m {
		out = append(out, t)
	}
	return out
}

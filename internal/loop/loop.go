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
	"sort"
	"strings"
	"time"

	"github.com/nolouch/gcode/internal/agent"
	"github.com/nolouch/gcode/internal/bus"
	"github.com/nolouch/gcode/internal/llm"
	"github.com/nolouch/gcode/internal/model"
	"github.com/nolouch/gcode/internal/permission"
	"github.com/nolouch/gcode/internal/processor"
	"github.com/nolouch/gcode/internal/session"
	"github.com/nolouch/gcode/internal/tool"
)

const (
	maxRetries         = 3
	historyCompactKeep = 24
)

// Runner is the top-level agent loop runner.
type Runner struct {
	Store             session.StoreAPI
	LLM               *llm.Client
	Agents            *agent.Registry
	Tools             map[string]tool.Tool // built-ins + MCP
	SystemPromptExtra []string             // skill system prompt fragments
	Bus               *bus.Bus             // event bus (nil = no events)
	Logf              func(format string, args ...any)
	Debug             bool
}

func (r *Runner) logf(format string, args ...any) {
	if r.Logf != nil {
		r.Logf(format, args...)
	}
}

func (r *Runner) debugf(format string, args ...any) {
	if r.Debug {
		r.logf("[debug] "+format, args...)
	}
}

// Run executes the agent loop for the given session + user message text.
// It blocks until the agent is done or ctx is cancelled.
func (r *Runner) Run(ctx context.Context, sessionID string, userText string, agentName string) error {
	store := r.Store
	r.debugf("run start session=%s agent=%s user_len=%d\n", sessionID, agentName, len(userText))

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
		if !permission.ToolAllowed(id, ag.Permissions) {
			continue
		}
		if sess.DeniedTools[id] {
			continue
		}
		effectiveTools[id] = t
	}
	toolNames := make([]string, 0, len(effectiveTools))
	for id := range effectiveTools {
		toolNames = append(toolNames, id)
	}
	sort.Strings(toolNames)
	r.debugf("effective tools=%d [%s]\n", len(toolNames), strings.Join(toolNames, ","))

	// ─── Build base system prompt ──────────────────────────────────
	baseSystem := buildSystemPrompt(ag, r.SystemPromptExtra, sess.Directory)

	// ─── Accumulated conversation history (beyond what's in the store) ─
	// We reconstruct the full history from the store on each step so
	// the LLM always sees tool results.
	var extraMessages []model.ChatMessage // tool-result messages from previous step

	// Cross-step doom loop detection: track last tool signature
	type stepSig struct{ tool, output string }
	var lastSig stepSig
	consecutiveSame := 0
	const crossStepThreshold = 3

	step := 0
	compactNext := false
	for {
		step++
		if step > maxSteps {
			r.logf("[loop] max steps (%d) reached\n", maxSteps)
			break
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		r.logf("\n[loop] step %d\n", step)

		// ── Build message history for this LLM call ──────────────
		history := buildHistory(store.Messages(sessionID))
		if compactNext && len(history) > historyCompactKeep {
			history = history[len(history)-historyCompactKeep:]
			r.logf("[loop] compacted history to last %d messages\n", len(history))
			compactNext = false
		}
		history = append(history, extraMessages...)
		extraMessages = nil
		r.debugf("step=%d history_messages=%d\n", step, len(history))

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
		r.debugf("step=%d llm stream request tools=%d system_parts=%d max_tokens=%d\n", step, len(streamInput.Tools), len(streamInput.System), streamInput.MaxTokens)

		var streamCh <-chan llm.StreamEvent
		var streamErr error
		for attempt := 0; attempt <= maxRetries; attempt++ {
			streamCh, streamErr = r.LLM.Stream(streamInput)
			if streamErr == nil {
				break
			}
			if attempt < maxRetries {
				wait := time.Duration(1<<uint(attempt)) * time.Second
				r.logf("[loop] LLM error (attempt %d): %v, retrying in %v\n", attempt+1, streamErr, wait)
				select {
				case <-time.After(wait):
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
		if streamErr != nil {
			r.debugf("step=%d llm stream failed err=%v\n", step, streamErr)
			return fmt.Errorf("LLM stream failed: %w", streamErr)
		}

		// ── Process stream ─────────────────────────────────────────
		proc := processor.New(store, r.Bus, asstMsg)
		proc.Authorize = func(agentName, toolName string, args map[string]any) (bool, string) {
			return permission.AuthorizeTool(agentName, toolName, args, ag.Permissions)
		}
		proc.RunTask = func(runCtx context.Context, req tool.TaskRequest) (tool.TaskResult, error) {
			start := time.Now()
			sub, ok := r.Agents.Lookup(req.Subagent)
			if !ok {
				return tool.TaskResult{}, fmt.Errorf("unknown agent type: %s", req.Subagent)
			}
			if sub.Mode == agent.ModePrimary {
				return tool.TaskResult{}, fmt.Errorf("agent %q cannot be used as subagent", req.Subagent)
			}

			targetSessionID := strings.TrimSpace(req.TaskID)
			if targetSessionID == "" {
				subSession := store.CreateSession(sess.Directory)
				store.SetSessionParent(subSession.ID, sessionID)
				subSession.DeniedTools["task"] = true
				targetSessionID = subSession.ID
			} else {
				subSession, err := store.GetSession(targetSessionID)
				if err != nil {
					return tool.TaskResult{}, fmt.Errorf("task_id not found: %s", targetSessionID)
				}
				if err := validateTaskTargetSession(sessionID, sess.Directory, subSession); err != nil {
					return tool.TaskResult{}, err
				}
				subSession.DeniedTools["task"] = true
			}

			if err := r.Run(runCtx, targetSessionID, req.Prompt, req.Subagent); err != nil {
				return tool.TaskResult{}, err
			}

			msgs := store.Messages(targetSessionID)
			return tool.TaskResult{
				TaskID:     targetSessionID,
				Output:     finalAssistantText(msgs),
				Subagent:   req.Subagent,
				DurationMs: time.Since(start).Milliseconds(),
			}, nil
		}
		result, toolMsgs := proc.Process(ctx, streamCh, effectiveTools, sess.Directory)
		extraMessages = toolMsgs
		r.debugf("step=%d process result=%s tool_messages=%d assistant_finish=%q\n", step, result, len(toolMsgs), asstMsg.Finish)

		// Cross-step doom loop detection
		if len(toolMsgs) == 1 {
			contentStr, _ := toolMsgs[0].Content.(string)
			sig := stepSig{
				tool:   toolMsgs[0].Name,
				output: contentStr,
			}
			if sig == lastSig {
				consecutiveSame++
				if consecutiveSame >= crossStepThreshold {
					r.logf("\n[warn] cross-step doom loop detected for tool %s, stopping\n", sig.tool)
					break
				}
			} else {
				lastSig = sig
				consecutiveSame = 1
			}
		} else {
			lastSig = stepSig{}
			consecutiveSame = 0
		}

		switch result {
		case model.ProcessResultStop:
			break // inner
		case model.ProcessResultContinue:
			// Append assistant tool-call message to the history for the next step
			extraMessages = append([]model.ChatMessage{buildAssistantToolCallMsg(asstMsg)}, extraMessages...)
			r.debugf("step=%d continue with extra_messages=%d\n", step, len(extraMessages))
			continue
		case model.ProcessResultCompact:
			compactNext = true
			r.logf("[loop] context compaction triggered\n")
			continue
		}

		// Stop
		break
	}
	r.debugf("run complete session=%s agent=%s\n", sessionID, agentName)
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
Think step-by-step. Be concise.

IMPORTANT - Tool Calling Format:
When you call a tool, you MUST provide ALL required arguments in JSON format.
Example:
{"name": "read", "parameters": {"path": "main.go"}}
{"name": "list", "parameters": {"path": "."}}
{"name": "bash", "parameters": {"command": "ls -la"}}

NEVER call a tool without arguments. Always include the required parameters.`, workDir)

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

func finalAssistantText(msgs []*model.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.Role != model.RoleAssistant {
			continue
		}
		if m.Error != nil {
			continue
		}
		var sb strings.Builder
		for _, p := range m.Parts {
			if p.Type == model.PartTypeText && strings.TrimSpace(p.Text) != "" {
				sb.WriteString(p.Text)
			}
		}
		text := strings.TrimSpace(sb.String())
		if text != "" {
			return text
		}
		if strings.TrimSpace(m.Text) != "" {
			return strings.TrimSpace(m.Text)
		}
	}
	return ""
}

func validateTaskTargetSession(currentSessionID, currentDir string, target *model.Session) error {
	if target == nil {
		return fmt.Errorf("task_id session is nil")
	}
	if target.ID == currentSessionID {
		return fmt.Errorf("task_id must not be the current session")
	}
	if strings.TrimSpace(target.Directory) != strings.TrimSpace(currentDir) {
		return fmt.Errorf("task_id %s belongs to a different working directory", target.ID)
	}
	if target.ParentID != "" && target.ParentID != currentSessionID {
		return fmt.Errorf("task_id %s belongs to a different parent session", target.ID)
	}
	return nil
}

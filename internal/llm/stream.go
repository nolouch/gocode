// Package llm provides a multi-provider streaming LLM client using charm.land/fantasy.
//
// It supports OpenAI, Anthropic, Google, OpenRouter, and any OpenAI-compatible provider.
package llm

import (
	"context"
	"fmt"

	"charm.land/fantasy"
	"github.com/nolouch/gcode/internal/model"
	"github.com/nolouch/gcode/internal/tool"
)

// Config holds provider connection details.
type Config struct {
	ProviderName string // "openai", "anthropic", "google", "openrouter", "openai-compat"
	BaseURL      string // for openai-compat or custom endpoints
	APIKey       string
	Model        string // e.g. "gpt-4o", "claude-3-5-sonnet-20241022"
}

// StreamEvent is one parsed event from the stream.
type StreamEvent struct {
	Type string

	// TypeTextDelta
	TextDelta string

	// TypeReasoningDelta - for models that output thinking/reasoning
	ReasoningDelta string

	// TypeToolCallStart / TypeToolCallDelta / TypeToolCallDone
	ToolCallID   string
	ToolCallName string
	ArgsDelta    string

	// TypeStepFinish
	FinishReason string
	Usage        model.TokenUsage

	// TypeError
	Err error
}

// Event type constants.
const (
	TypeTextDelta      = "text-delta"
	TypeReasoningDelta = "reasoning-delta"
	TypeToolCallStart  = "tool-call-start"
	TypeToolCallDelta  = "tool-call-delta"
	TypeToolCallDone   = "tool-call-done"
	TypeStepFinish     = "step-finish"
	TypeError          = "error"
)

// Client wraps a fantasy.LanguageModel.
type Client struct {
	model fantasy.LanguageModel
	cfg   Config
}

// New creates a new LLM Client using fantasy SDK.
func New(cfg Config) (*Client, error) {
	provider, err := BuildProvider(cfg)
	if err != nil {
		return nil, fmt.Errorf("build provider: %w", err)
	}

	model, err := provider.LanguageModel(context.Background(), cfg.Model)
	if err != nil {
		return nil, fmt.Errorf("get language model: %w", err)
	}

	return &Client{
		model: model,
		cfg:   cfg,
	}, nil
}

// StreamInput is the full set of parameters for one LLM call.
type StreamInput struct {
	Messages  []model.ChatMessage
	Tools     []tool.Tool
	System    []string
	MaxTokens int
	Abort     context.Context
}

// Stream calls the LLM and returns a channel of StreamEvents.
// The caller must drain the channel until it is closed.
func (c *Client) Stream(input StreamInput) (<-chan StreamEvent, error) {
	call, err := c.buildCall(input)
	if err != nil {
		return nil, fmt.Errorf("build call: %w", err)
	}

	streamResp, err := c.model.Stream(input.Abort, call)
	if err != nil {
		return nil, fmt.Errorf("stream: %w", err)
	}

	Debug("Stream started with model=%s provider=%s", c.model.Model(), c.model.Provider())

	ch := make(chan StreamEvent, 64)
	go c.processStream(streamResp, ch)
	return ch, nil
}

// buildCall converts StreamInput to fantasy.Call
func (c *Client) buildCall(input StreamInput) (fantasy.Call, error) {
	// Build prompt (messages)
	var prompt fantasy.Prompt

	// System messages
	for _, s := range input.System {
		if s != "" {
			prompt = append(prompt, fantasy.NewSystemMessage(s))
		}
	}

	// Conversation messages
	for _, m := range input.Messages {
		msg, err := c.convertMessage(m)
		if err != nil {
			return fantasy.Call{}, fmt.Errorf("convert message: %w", err)
		}
		prompt = append(prompt, msg)
	}

	// Build tools
	var tools []fantasy.Tool
	for _, t := range input.Tools {
		tools = append(tools, fantasy.FunctionTool{
			Name:        t.ID(),
			Description: t.Description(),
			InputSchema: t.Schema(),
		})
	}

	// Build call
	call := fantasy.Call{
		Prompt: prompt,
		Tools:  tools,
	}
	if input.MaxTokens > 0 {
		maxTokens := int64(input.MaxTokens)
		call.MaxOutputTokens = &maxTokens
	}

	return call, nil
}

// convertMessage converts model.ChatMessage to fantasy.Message
func (c *Client) convertMessage(m model.ChatMessage) (fantasy.Message, error) {
	msg := fantasy.Message{
		Role: fantasy.MessageRole(m.Role),
	}

	// Handle content
	switch content := m.Content.(type) {
	case string:
		if content != "" {
			msg.Content = append(msg.Content, fantasy.TextPart{Text: content})
		}
	case []interface{}:
		// Multi-part content (images, etc.) - for now just extract text
		for _, part := range content {
			if partMap, ok := part.(map[string]interface{}); ok {
				if partMap["type"] == "text" {
					if text, ok := partMap["text"].(string); ok {
						msg.Content = append(msg.Content, fantasy.TextPart{Text: text})
					}
				}
			}
		}
	}

	// Handle tool calls (assistant messages)
	if len(m.ToolCalls) > 0 {
		for _, tc := range m.ToolCalls {
			// Ensure arguments is a valid JSON object, not "null" string
			args := tc.Function.Arguments
			if args == "" || args == "null" {
				args = "{}"
			}
			Debug("tool call: id=%s name=%s args=%s", tc.ID, tc.Function.Name, args)
			msg.Content = append(msg.Content, fantasy.ToolCallPart{
				ToolCallID: tc.ID,
				ToolName:   tc.Function.Name,
				Input:      args,
			})
		}
	}

	// Handle tool results (tool messages)
	if m.ToolCallID != "" {
		// This is a tool result message
		contentStr := ""
		if str, ok := m.Content.(string); ok {
			contentStr = str
		}
		msg.Content = append(msg.Content, fantasy.ToolResultPart{
			ToolCallID: m.ToolCallID,
			Output:     fantasy.ToolResultOutputContentText{Text: contentStr},
		})
	}

	return msg, nil
}

// processStream converts fantasy.StreamPart to StreamEvent
func (c *Client) processStream(streamResp fantasy.StreamResponse, ch chan<- StreamEvent) {
	defer close(ch)

	// Track tool calls across stream parts
	toolCalls := make(map[string]*toolCallState)

	// Use range syntax - Go 1.23+ transforms this to function call
	for part := range streamResp {
		Debug("stream part: type=%s delta=%q id=%q toolName=%q toolInput=%q finish=%q",
			part.Type, part.Delta, part.ID, part.ToolCallName, part.ToolCallInput, part.FinishReason)

		// Handle errors
		if part.Error != nil {
			Debug("stream error: %v", part.Error)
			ch <- StreamEvent{Type: TypeError, Err: part.Error}
			continue
		}

		switch part.Type {
		case fantasy.StreamPartTypeReasoningDelta:
			ch <- StreamEvent{
				Type:           TypeReasoningDelta,
				ReasoningDelta: part.Delta,
			}

		case fantasy.StreamPartTypeTextDelta:
			ch <- StreamEvent{
				Type:      TypeTextDelta,
				TextDelta: part.Delta,
			}

		case fantasy.StreamPartTypeToolInputStart:
			// New tool call starting
			toolCalls[part.ID] = &toolCallState{
				id:   part.ID,
				name: part.ToolCallName,
			}
			ch <- StreamEvent{
				Type:         TypeToolCallStart,
				ToolCallID:   part.ID,
				ToolCallName: part.ToolCallName,
			}

		case fantasy.StreamPartTypeToolInputDelta:
			// Tool input delta
			delta := part.Delta
			if delta == "" {
				// Some providers send tool input in ToolCallInput rather than Delta.
				delta = part.ToolCallInput
			}
			if tc, ok := toolCalls[part.ID]; ok {
				tc.input += delta
			}
			ch <- StreamEvent{
				Type:       TypeToolCallDelta,
				ToolCallID: part.ID,
				ArgsDelta:  delta,
			}

		case fantasy.StreamPartTypeToolInputEnd, fantasy.StreamPartTypeToolCall:
			// Tool call complete
			if tc, ok := toolCalls[part.ID]; ok {
				if tc.name == "" && part.ToolCallName != "" {
					tc.name = part.ToolCallName
				}
				// Prefer part.ToolCallInput (full input from provider) over accumulated deltas.
				// Some models (e.g. MiniMax) send the full input only in the final ToolCall
				// event without emitting any delta events first.
				finalInput := tc.input
				if part.ToolCallInput != "" {
					finalInput = part.ToolCallInput
				}
				if part.Type == fantasy.StreamPartTypeToolInputEnd && finalInput == "" {
					// Wait for a subsequent ToolCall event that may carry full input.
					continue
				}
				ch <- StreamEvent{
					Type:         TypeToolCallDone,
					ToolCallID:   tc.id,
					ToolCallName: tc.name,
					ArgsDelta:    finalInput,
				}
				delete(toolCalls, part.ID)
			}

		case fantasy.StreamPartTypeFinish:
			// Stream finished
			ch <- StreamEvent{
				Type:         TypeStepFinish,
				FinishReason: string(part.FinishReason),
				Usage: model.TokenUsage{
					Input:  int(part.Usage.InputTokens),
					Output: int(part.Usage.OutputTokens),
				},
			}
		}
	}
}

// toolCallState tracks partial tool call data during streaming
type toolCallState struct {
	id    string
	name  string
	input string
}

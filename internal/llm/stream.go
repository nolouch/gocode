// Package llm provides an OpenAI-compatible streaming LLM client using raw
// HTTP SSE, mirroring OpenCode's LLM.stream().
//
// It supports any OpenAI-compatible provider (OpenAI, Anthropic via proxy,
// Ollama, etc.) by accepting a base URL + model ID.
package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nolouch/gcode/internal/model"
	"github.com/nolouch/gcode/internal/tool"
)

// Config holds provider connection details.
type Config struct {
	BaseURL string // e.g. "https://api.openai.com/v1"
	APIKey  string
	Model   string // e.g. "gpt-4o"
}

// ToolDef is the JSON representation of a tool for the LLM API.
type ToolDef struct {
	Type     string          `json:"type"`
	Function ToolFunctionDef `json:"function"`
}

// ToolFunctionDef describes a single function tool.
type ToolFunctionDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// StreamEvent is one parsed event from the SSE stream.
type StreamEvent struct {
	Type string

	// TypeTextDelta
	TextDelta string

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
	TypeTextDelta     = "text-delta"
	TypeToolCallStart = "tool-call-start"
	TypeToolCallDelta = "tool-call-delta"
	TypeToolCallDone  = "tool-call-done"
	TypeStepFinish    = "step-finish"
	TypeError         = "error"
)

// ─── OpenAI SSE wire types ────────────────────

type chatCompletionChunk struct {
	Choices []struct {
		Delta struct {
			Role      string          `json:"role"`
			Content   string          `json:"content"`
			ToolCalls []toolCallChunk `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *usageChunk `json:"usage"`
}

type toolCallChunk struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type usageChunk struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ─────────────────────────────────────────────
// Client
// ─────────────────────────────────────────────

// Client wraps an OpenAI-compatible API endpoint.
type Client struct {
	cfg        Config
	httpClient *http.Client
}

// New creates a new LLM Client.
func New(cfg Config) *Client {
	return &Client{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 10 * time.Minute},
	}
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
	// Build the request body
	body, err := c.buildRequest(input)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(input.Abort,
		http.MethodPost,
		c.cfg.BaseURL+"/chat/completions",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("LLM API error %d: %s", resp.StatusCode, data)
	}

	ch := make(chan StreamEvent, 64)
	go c.readSSE(resp.Body, ch)
	return ch, nil
}

// buildRequest serialises the API request body.
func (c *Client) buildRequest(input StreamInput) ([]byte, error) {
	messages := make([]map[string]any, 0)

	// System messages
	for _, s := range input.System {
		if s != "" {
			messages = append(messages, map[string]any{
				"role":    "system",
				"content": s,
			})
		}
	}

	// Conversation messages
	for _, m := range input.Messages {
		msg := map[string]any{
			"role":    m.Role,
			"content": m.Content,
		}
		if len(m.ToolCalls) > 0 {
			msg["tool_calls"] = m.ToolCalls
		}
		if m.ToolCallID != "" {
			msg["tool_call_id"] = m.ToolCallID
		}
		if m.Name != "" {
			msg["name"] = m.Name
		}
		messages = append(messages, msg)
	}

	// Tool definitions
	var tools []ToolDef
	for _, t := range input.Tools {
		tools = append(tools, ToolDef{
			Type: "function",
			Function: ToolFunctionDef{
				Name:        t.ID(),
				Description: t.Description(),
				Parameters:  t.Schema(),
			},
		})
	}

	req := map[string]any{
		"model":    c.cfg.Model,
		"messages": messages,
		"stream":   true,
		"stream_options": map[string]any{
			"include_usage": true,
		},
	}
	if len(tools) > 0 {
		req["tools"] = tools
	}
	if input.MaxTokens > 0 {
		req["max_tokens"] = input.MaxTokens
	}

	return json.Marshal(req)
}

// readSSE parses the SSE stream and sends events to ch.
func (c *Client) readSSE(body io.ReadCloser, ch chan<- StreamEvent) {
	defer body.Close()
	defer close(ch)

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)

	// Track partial tool calls across chunks
	toolCalls := make(map[int]struct {
		id   string
		name string
		args strings.Builder
	})

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk chatCompletionChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		// Usage at end of stream
		if chunk.Usage != nil {
			ch <- StreamEvent{
				Type: TypeStepFinish,
				Usage: model.TokenUsage{
					Input:  chunk.Usage.PromptTokens,
					Output: chunk.Usage.CompletionTokens,
				},
			}
		}

		for _, choice := range chunk.Choices {
			// Text delta
			if choice.Delta.Content != "" {
				ch <- StreamEvent{Type: TypeTextDelta, TextDelta: choice.Delta.Content}
			}

			// Tool call chunks
			for _, tc := range choice.Delta.ToolCalls {
				idx := tc.Index
				entry := toolCalls[idx]
				if tc.ID != "" {
					entry.id = tc.ID
				}
				if tc.Function.Name != "" {
					entry.name = tc.Function.Name
					ch <- StreamEvent{
						Type:         TypeToolCallStart,
						ToolCallID:   entry.id,
						ToolCallName: entry.name,
					}
				}
				if tc.Function.Arguments != "" {
					entry.args.WriteString(tc.Function.Arguments)
					ch <- StreamEvent{
						Type:       TypeToolCallDelta,
						ToolCallID: entry.id,
						ArgsDelta:  tc.Function.Arguments,
					}
				}
				toolCalls[idx] = entry
			}

			// Finish
			if choice.FinishReason != nil {
				reason := *choice.FinishReason

				// Emit tool-call-done for each pending tool call
				for _, tc := range toolCalls {
					ch <- StreamEvent{
						Type:         TypeToolCallDone,
						ToolCallID:   tc.id,
						ToolCallName: tc.name,
						ArgsDelta:    tc.args.String(),
					}
				}
				toolCalls = make(map[int]struct {
					id   string
					name string
					args strings.Builder
				})

				ch <- StreamEvent{
					Type:         TypeStepFinish,
					FinishReason: reason,
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		ch <- StreamEvent{Type: TypeError, Err: err}
	}
}

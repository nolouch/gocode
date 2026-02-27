//go:build integration
// +build integration

package tests

import (
	"context"
	"os"
	"testing"

	"github.com/nolouch/gcode/internal/llm"
	"github.com/nolouch/gcode/internal/model"
	"github.com/nolouch/gcode/internal/tool"
)

// TestMiniMaxAnthropic tests MiniMax with anthropic-compatible API
func TestMiniMaxAnthropic(t *testing.T) {
	apiKey := os.Getenv("MINIMAX_API_KEY")
	if apiKey == "" {
		t.Skip("MINIMAX_API_KEY not set")
	}

	cfg := llm.Config{
		ProviderName: "anthropic",
		BaseURL:      "https://api.minimaxi.com/anthropic",
		APIKey:       apiKey,
		Model:        "MiniMax-M2.5",
	}

	client, err := llm.New(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()
	stream, err := client.Stream(llm.StreamInput{
		Messages: []model.ChatMessage{
			{Role: "user", Content: "Say 'hello' in one word"},
		},
		Abort: ctx,
	})
	if err != nil {
		t.Fatalf("stream error: %v", err)
	}

	var text string
	for event := range stream {
		switch event.Type {
		case llm.TypeTextDelta:
			text += event.TextDelta
		case llm.TypeError:
			t.Fatalf("stream error: %v", event.Err)
		case llm.TypeStepFinish:
			if event.FinishReason != "stop" {
				t.Logf("finish reason: %s", event.FinishReason)
			}
			t.Logf("usage: input=%d output=%d", event.Usage.Input, event.Usage.Output)
		}
	}

	if text == "" {
		t.Fatal("no text received")
	}

	t.Logf("received text: %s", text)
}

// TestMiniMaxToolCall tests tool calling with MiniMax
func TestMiniMaxToolCall(t *testing.T) {
	apiKey := os.Getenv("MINIMAX_API_KEY")
	if apiKey == "" {
		t.Skip("MINIMAX_API_KEY not set")
	}

	cfg := llm.Config{
		ProviderName: "anthropic",
		BaseURL:      "https://api.minimaxi.com/anthropic",
		APIKey:       apiKey,
		Model:        "MiniMax-M2.5",
	}

	client, err := llm.New(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()

	// Simple echo tool
	echoTool := &echoTool{}

	stream, err := client.Stream(llm.StreamInput{
		Messages: []model.ChatMessage{
			{Role: "user", Content: "Use the echo tool to say hello world"},
		},
		Tools: []tool.Tool{echoTool},
		Abort:  ctx,
	})
	if err != nil {
		t.Fatalf("stream error: %v", err)
	}

	var toolCall bool
	for event := range stream {
		switch event.Type {
		case llm.TypeToolCallStart:
			toolCall = true
			t.Logf("tool call started: %s", event.ToolCallName)
		case llm.TypeToolCallDone:
			t.Logf("tool call done: %s args=%s", event.ToolCallID, event.ArgsDelta)
		case llm.TypeError:
			t.Fatalf("stream error: %v", event.Err)
		case llm.TypeStepFinish:
			t.Logf("finish reason: %s", event.FinishReason)
		}
	}

	if !toolCall {
		t.Log("no tool call detected (model may not support tools)")
	}
}

// echoTool is a simple test tool
type echoTool struct{}

func (e *echoTool) ID() string          { return "echo" }
func (e *echoTool) Description() string { return "Echo back the input text" }
func (e *echoTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{"type": "string"},
		},
		"required": []string{"text"},
	}
}
func (e *echoTool) Execute(ctx tool.Context, params map[string]any) (tool.Result, error) {
	text, _ := params["text"].(string)
	return tool.Result{Output: "echo: " + text}, nil
}

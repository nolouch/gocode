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

// TestOpenAI tests OpenAI provider as baseline
func TestOpenAI(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set")
	}

	cfg := llm.Config{
		ProviderName: "openai",
		APIKey:       apiKey,
		Model:        "gpt-4o",
	}

	client, err := llm.New(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()
	stream, err := client.Stream(llm.StreamInput{
		Messages: []model.ChatMessage{
			{Role: "user", Content: "Say 'hi' in one word"},
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
			t.Logf("usage: input=%d output=%d", event.Usage.Input, event.Usage.Output)
		}
	}

	if text == "" {
		t.Fatal("no text received")
	}

	t.Logf("received text: %s", text)
}

// TestOpenAIToolCall tests OpenAI tool calling
func TestOpenAIToolCall(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set")
	}

	cfg := llm.Config{
		ProviderName: "openai",
		APIKey:       apiKey,
		Model:        "gpt-4o",
	}

	client, err := llm.New(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()

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
		}
	}

	if !toolCall {
		t.Fatal("no tool call detected")
	}
}

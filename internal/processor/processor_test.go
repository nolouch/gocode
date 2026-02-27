package processor

import (
	"context"
	"testing"
	"time"

	"github.com/nolouch/gcode/internal/llm"
	"github.com/nolouch/gcode/internal/model"
	"github.com/nolouch/gcode/internal/session"
	"github.com/nolouch/gcode/internal/tool"
)

func TestProcess_UsesArgsFromToolCallDoneEvent(t *testing.T) {
	store := session.NewStore()
	sess := store.CreateSession(".")

	msg := &model.Message{
		ID:        session.NewID(),
		SessionID: sess.ID,
		Role:      model.RoleAssistant,
		CreatedAt: time.Now(),
	}
	store.AddMessage(msg)

	streamCh := make(chan llm.StreamEvent, 4)
	streamCh <- llm.StreamEvent{Type: llm.TypeToolCallStart, ToolCallID: "call-1", ToolCallName: "echo"}
	streamCh <- llm.StreamEvent{Type: llm.TypeToolCallDone, ToolCallID: "call-1", ToolCallName: "echo", ArgsDelta: `{"text":"hello"}`}
	streamCh <- llm.StreamEvent{Type: llm.TypeStepFinish, FinishReason: "tool_calls"}
	close(streamCh)

	mt := &mockTool{}
	proc := New(store, msg)

	result, toolMsgs := proc.Process(context.Background(), streamCh, map[string]tool.Tool{"echo": mt}, ".")

	if result != model.ProcessResultContinue {
		t.Fatalf("expected continue, got %q", result)
	}
	if mt.lastArgs == nil {
		t.Fatal("expected tool to be executed")
	}
	if got, _ := mt.lastArgs["text"].(string); got != "hello" {
		t.Fatalf("expected parsed arg text=hello, got %#v", mt.lastArgs)
	}
	if len(toolMsgs) != 1 {
		t.Fatalf("expected 1 tool message, got %d", len(toolMsgs))
	}
}

type mockTool struct {
	lastArgs map[string]any
}

func (m *mockTool) ID() string          { return "echo" }
func (m *mockTool) Description() string { return "test tool" }
func (m *mockTool) Schema() map[string]any {
	return map[string]any{"type": "object"}
}
func (m *mockTool) Execute(_ tool.Context, args map[string]any) (tool.Result, error) {
	m.lastArgs = args
	return tool.Result{Output: "ok"}, nil
}

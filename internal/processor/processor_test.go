package processor

import (
	"context"
	"testing"
	"time"

	"github.com/nolouch/gcode/internal/bus"
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
	proc := New(store, nil, msg)

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

func TestProcess_ReasoningPartDeltaAndDoneArePublished(t *testing.T) {
	store := session.NewStore()
	sess := store.CreateSession(".")

	msg := &model.Message{
		ID:        session.NewID(),
		SessionID: sess.ID,
		Role:      model.RoleAssistant,
		CreatedAt: time.Now(),
	}
	store.AddMessage(msg)

	b := bus.New()
	var events []bus.Event
	b.Subscribe(func(e bus.Event) {
		events = append(events, e)
	})

	streamCh := make(chan llm.StreamEvent, 8)
	streamCh <- llm.StreamEvent{Type: llm.TypeReasoningDelta, ReasoningDelta: "first "}
	streamCh <- llm.StreamEvent{Type: llm.TypeReasoningDelta, ReasoningDelta: "second"}
	streamCh <- llm.StreamEvent{Type: llm.TypeTextDelta, TextDelta: "answer"}
	streamCh <- llm.StreamEvent{Type: llm.TypeStepFinish, FinishReason: "stop"}
	close(streamCh)

	proc := New(store, b, msg)
	result, _ := proc.Process(context.Background(), streamCh, map[string]tool.Tool{}, ".")

	if result != model.ProcessResultStop {
		t.Fatalf("expected stop, got %q", result)
	}

	var done *bus.PartDonePayload
	var sawReasoningPartDelta bool
	for i := range events {
		if events[i].Type == bus.EventPartDelta {
			p, ok := events[i].Payload.(bus.PartDeltaPayload)
			if ok && p.PartType == model.PartTypeReasoning && p.Field == "text" {
				sawReasoningPartDelta = true
			}
		}
		if events[i].Type != bus.EventPartDone {
			continue
		}
		p, ok := events[i].Payload.(bus.PartDonePayload)
		if !ok {
			t.Fatalf("part.done payload type = %T", events[i].Payload)
		}
		if p.PartType != model.PartTypeReasoning {
			continue
		}
		done = &p
		break
	}
	if done == nil {
		t.Fatal("expected reasoning message.part.done event")
	}
	if !sawReasoningPartDelta {
		t.Fatal("expected message.part.delta for reasoning")
	}
	if done.DurationMs < 0 {
		t.Fatalf("reasoning duration_ms = %f, want >= 0", done.DurationMs)
	}

	msgs := store.Messages(sess.ID)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	var reasoning string
	for _, part := range msgs[0].Parts {
		if part.Type == model.PartTypeReasoning {
			reasoning = part.Text
			break
		}
	}
	if reasoning != "first second" {
		t.Fatalf("reasoning part text = %q, want %q", reasoning, "first second")
	}
}

func TestProcess_ReasoningPartDoneFlushesOnStepFinish(t *testing.T) {
	store := session.NewStore()
	sess := store.CreateSession(".")

	msg := &model.Message{
		ID:        session.NewID(),
		SessionID: sess.ID,
		Role:      model.RoleAssistant,
		CreatedAt: time.Now(),
	}
	store.AddMessage(msg)

	b := bus.New()
	var sawDone bool
	b.Subscribe(func(e bus.Event) {
		if e.Type != bus.EventPartDone {
			return
		}
		p, ok := e.Payload.(bus.PartDonePayload)
		if !ok {
			t.Fatalf("part.done payload type = %T", e.Payload)
		}
		if p.PartType == model.PartTypeReasoning {
			sawDone = true
		}
	})

	streamCh := make(chan llm.StreamEvent, 4)
	streamCh <- llm.StreamEvent{Type: llm.TypeReasoningDelta, ReasoningDelta: "plan"}
	streamCh <- llm.StreamEvent{Type: llm.TypeStepFinish, FinishReason: "stop"}
	close(streamCh)

	proc := New(store, b, msg)
	_, _ = proc.Process(context.Background(), streamCh, map[string]tool.Tool{}, ".")

	if !sawDone {
		t.Fatal("expected message.part.done reasoning on step-finish flush")
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

// Package bus provides a simple type-safe in-process event bus.
// Processor publishes events; TUI/server subscribes to render or forward them.
package bus

import (
	"sync"
)

// EventType identifies the kind of event.
type EventType string

const (
	EventTextDelta    EventType = "text.delta"
	EventThinking     EventType = "thinking.delta"
	EventThinkingDone EventType = "thinking.done"
	EventToolStart    EventType = "tool.start"
	EventToolDone     EventType = "tool.done"
	EventToolError    EventType = "tool.error"
	EventTurnDone     EventType = "turn.done"
	EventTurnError    EventType = "turn.error"
)

// Event is a single bus event.
type Event struct {
	Type      EventType `json:"type"`
	SessionID string    `json:"session_id"`
	MessageID string    `json:"message_id"`
	Payload   any       `json:"payload"`
}

// TextDeltaPayload carries a streaming text chunk.
type TextDeltaPayload struct {
	Delta string `json:"delta"`
}

// ThinkingPayload carries a reasoning/thinking chunk or duration.
type ThinkingPayload struct {
	Delta    string  `json:"delta,omitempty"`
	Duration float64 `json:"duration_ms,omitempty"` // only on done
}

// ToolPayload carries tool call metadata.
type ToolPayload struct {
	CallID   string         `json:"call_id"`
	Tool     string         `json:"tool"`
	Input    map[string]any `json:"input,omitempty"`
	Output   string         `json:"output,omitempty"`
	IsError  bool           `json:"is_error,omitempty"`
	DurationMs int64        `json:"duration_ms,omitempty"`
}

// TurnDonePayload signals the agent turn finished.
type TurnDonePayload struct {
	FinishReason string `json:"finish_reason"`
}

// Handler is a function that receives events.
type Handler func(Event)

// Bus is a simple pub/sub event bus.
type Bus struct {
	mu       sync.RWMutex
	handlers []Handler
}

// New creates a new Bus.
func New() *Bus {
	return &Bus{}
}

// Subscribe registers a handler for all events.
// Returns an unsubscribe function.
func (b *Bus) Subscribe(h Handler) func() {
	b.mu.Lock()
	b.handlers = append(b.handlers, h)
	idx := len(b.handlers) - 1
	b.mu.Unlock()

	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		b.handlers[idx] = nil
	}
}

// Publish sends an event to all subscribers.
func (b *Bus) Publish(e Event) {
	b.mu.RLock()
	handlers := make([]Handler, len(b.handlers))
	copy(handlers, b.handlers)
	b.mu.RUnlock()

	for _, h := range handlers {
		if h != nil {
			h(e)
		}
	}
}

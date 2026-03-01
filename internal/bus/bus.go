// Package bus provides a simple type-safe in-process event bus.
// Processor publishes events; TUI/server subscribes to render or forward them.
package bus

import (
	"sync"

	"github.com/nolouch/gocode/internal/model"
)

// EventType identifies the kind of event.
type EventType string

const (
	EventTurnDone   EventType = "turn.done"
	EventTurnError  EventType = "turn.error"
	EventPartUpsert EventType = "message.part.upsert"
	EventPartDelta  EventType = "message.part.delta"
	EventPartDone   EventType = "message.part.done"
)

// Event is a single bus event.
type Event struct {
	Type      EventType `json:"type"`
	SessionID string    `json:"session_id"`
	MessageID string    `json:"message_id"`
	Payload   any       `json:"payload"`
}

// TurnDonePayload signals the agent turn finished.
type TurnDonePayload struct {
	FinishReason string `json:"finish_reason"`
}

// PartUpsertPayload carries a full part snapshot.
type PartUpsertPayload struct {
	Part model.Part `json:"part"`
}

// PartDeltaPayload carries an incremental part text update.
type PartDeltaPayload struct {
	PartID   string         `json:"part_id"`
	PartType model.PartType `json:"part_type"`
	Field    string         `json:"field"`
	Delta    string         `json:"delta"`
}

// PartDonePayload marks a part lifecycle boundary completion.
type PartDonePayload struct {
	PartID     string         `json:"part_id"`
	PartType   model.PartType `json:"part_type"`
	DurationMs float64        `json:"duration_ms,omitempty"`
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

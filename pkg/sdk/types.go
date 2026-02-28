package sdk

import "time"

type Session struct {
	ID          string
	Title       string
	Directory   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeniedTools map[string]bool
}

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type Message struct {
	ID        string
	SessionID string
	Role      Role
	CreatedAt time.Time

	Text string

	ProviderID string
	ModelID    string
	Agent      string
	Finish     string

	Cost   float64
	Tokens TokenUsage

	Error *AgentError
	Parts []Part
}

type TokenUsage struct {
	Input      int
	Output     int
	Reasoning  int
	CacheRead  int
	CacheWrite int
}

type AgentError struct {
	Name      string
	Message   string
	Retryable bool
}

type PartType string

const (
	PartTypeText       PartType = "text"
	PartTypeReasoning  PartType = "reasoning"
	PartTypeTool       PartType = "tool"
	PartTypeStepStart  PartType = "step-start"
	PartTypeStepFinish PartType = "step-finish"
)

type Part struct {
	ID        string
	SessionID string
	MessageID string
	Type      PartType

	Text string

	Tool *ToolPart

	StepFinish *StepFinishPart
}

type ToolPart struct {
	CallID  string
	Tool    string
	State   ToolState
	Input   map[string]any
	Output  string
	Error   string
	StartAt time.Time
	EndAt   time.Time
}

type ToolState string

const (
	ToolStatePending   ToolState = "pending"
	ToolStateRunning   ToolState = "running"
	ToolStateCompleted ToolState = "completed"
	ToolStateError     ToolState = "error"
)

type StepFinishPart struct {
	Reason string
	Cost   float64
	Tokens TokenUsage
}

type EventType string

const (
	EventTurnDone   EventType = "turn.done"
	EventTurnError  EventType = "turn.error"
	EventPartUpsert EventType = "message.part.upsert"
	EventPartDelta  EventType = "message.part.delta"
	EventPartDone   EventType = "message.part.done"
)

type Event struct {
	Type      EventType `json:"type"`
	SessionID string    `json:"session_id"`
	MessageID string    `json:"message_id"`
	Payload   any       `json:"payload"`
}

type TurnDonePayload struct {
	FinishReason string `json:"finish_reason"`
}

type PartUpsertPayload struct {
	Part Part `json:"part"`
}

type PartDeltaPayload struct {
	PartID   string   `json:"part_id"`
	PartType PartType `json:"part_type"`
	Field    string   `json:"field"`
	Delta    string   `json:"delta"`
}

type PartDonePayload struct {
	PartID     string   `json:"part_id"`
	PartType   PartType `json:"part_type"`
	DurationMs float64  `json:"duration_ms,omitempty"`
}

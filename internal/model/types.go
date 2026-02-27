// Package model defines the core data structures for gcode agent,
// inspired by OpenCode's MessageV2 / Session types.
package model

import "time"

// ─────────────────────────────────────────────
// Session
// ─────────────────────────────────────────────

// Session holds the state for one conversation with the agent.
type Session struct {
	ID        string
	Title     string
	Directory string // working directory (cwd)
	CreatedAt time.Time
	UpdatedAt time.Time

	// Permission ruleset (simplified: set of denied tool names)
	DeniedTools map[string]bool
}

// ─────────────────────────────────────────────
// Messages
// ─────────────────────────────────────────────

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message is a single turn in the conversation.
type Message struct {
	ID        string
	SessionID string
	Role      Role
	CreatedAt time.Time

	// User fields
	Text string // primary text content (user side)

	// Assistant fields
	ProviderID string
	ModelID    string
	Agent      string
	Finish     string // "stop", "tool-calls", "length", "error"

	// Cost / tokens (assistant)
	Cost   float64
	Tokens TokenUsage

	// Error if the LLM call failed
	Error *AgentError

	// Parts holds streaming output parts
	Parts []Part
}

// TokenUsage summarises LLM token consumption.
type TokenUsage struct {
	Input      int
	Output     int
	Reasoning  int
	CacheRead  int
	CacheWrite int
}

// AgentError is a structured error attached to an assistant message.
type AgentError struct {
	Name      string
	Message   string
	Retryable bool
}

// ─────────────────────────────────────────────
// Parts  (streaming output pieces)
// A Part is one visual / semantic chunk within an assistant message.
// ─────────────────────────────────────────────

type PartType string

const (
	PartTypeText       PartType = "text"
	PartTypeReasoning  PartType = "reasoning"
	PartTypeTool       PartType = "tool"
	PartTypeStepStart  PartType = "step-start"
	PartTypeStepFinish PartType = "step-finish"
)

// Part is a polymorphic piece of an assistant response.
type Part struct {
	ID        string
	SessionID string
	MessageID string
	Type      PartType

	// PartTypeText / PartTypeReasoning
	Text string

	// PartTypeTool
	Tool *ToolPart

	// PartTypeStepFinish
	StepFinish *StepFinishPart
}

// ToolPart tracks one tool call within a message.
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

// ToolState is the status of a tool call.
type ToolState string

const (
	ToolStatePending   ToolState = "pending"
	ToolStateRunning   ToolState = "running"
	ToolStateCompleted ToolState = "completed"
	ToolStateError     ToolState = "error"
)

// StepFinishPart summarises token usage at the end of an LLM step.
type StepFinishPart struct {
	Reason string
	Cost   float64
	Tokens TokenUsage
}

// ─────────────────────────────────────────────
// LLM conversation representation
// ─────────────────────────────────────────────

// ChatMessage is the format sent to the LLM API.
type ChatMessage struct {
	Role       string      `json:"role"`
	Content    interface{} `json:"content"` // string or []ContentPart
	ToolCallID string      `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	Name       string      `json:"name,omitempty"`
}

// ContentPart is one piece of multi-modal message content.
type ContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}

// ToolCall is a function call requested by the LLM.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall holds the name + raw JSON arguments.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ─────────────────────────────────────────────
// ProcessResult  (returned by processor / loop)
// ─────────────────────────────────────────────

// ProcessResult tells the outer loop what to do next.
type ProcessResult string

const (
	ProcessResultContinue ProcessResult = "continue" // more tool calls needed
	ProcessResultStop     ProcessResult = "stop"     // done or permission denied
	ProcessResultCompact  ProcessResult = "compact"  // context too large, summarise
)

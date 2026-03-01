package runs

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nolouch/opengocode/internal/session"
)

type Status string

const (
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusAborted   Status = "aborted"
)

type Info struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Agent     string    `json:"agent"`
	Prompt    string    `json:"prompt"`
	Status    Status    `json:"status"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at,omitempty"`
}

type run struct {
	info   Info
	cancel context.CancelFunc
}

type Manager struct {
	mu   sync.RWMutex
	runs map[string]*run
}

func NewManager() *Manager {
	return &Manager{runs: make(map[string]*run)}
}

func (m *Manager) Start(parent context.Context, sessionID, prompt, agent string, fn func(context.Context) error) Info {
	ctx, cancel := context.WithCancel(parent)
	now := time.Now()
	id := session.NewID()

	r := &run{info: Info{
		ID:        id,
		SessionID: sessionID,
		Agent:     agent,
		Prompt:    prompt,
		Status:    StatusRunning,
		CreatedAt: now,
		StartedAt: now,
	}, cancel: cancel}

	m.mu.Lock()
	m.runs[id] = r
	m.mu.Unlock()

	go func() {
		err := fn(ctx)
		m.mu.Lock()
		defer m.mu.Unlock()
		curr, ok := m.runs[id]
		if !ok {
			return
		}
		if curr.info.Status == StatusAborted {
			if curr.info.EndedAt.IsZero() {
				curr.info.EndedAt = time.Now()
			}
			return
		}
		if err != nil {
			if ctx.Err() == context.Canceled {
				curr.info.Status = StatusAborted
				curr.info.Error = "run aborted"
			} else {
				curr.info.Status = StatusFailed
				curr.info.Error = err.Error()
			}
		} else {
			curr.info.Status = StatusCompleted
		}
		curr.info.EndedAt = time.Now()
	}()

	return r.info
}

func (m *Manager) Get(id string) (Info, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.runs[id]
	if !ok {
		return Info{}, false
	}
	return r.info, true
}

func (m *Manager) Abort(id string) (Info, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.runs[id]
	if !ok {
		return Info{}, fmt.Errorf("run not found: %s", id)
	}
	if r.info.Status == StatusCompleted || r.info.Status == StatusFailed || r.info.Status == StatusAborted {
		return r.info, nil
	}
	r.info.Status = StatusAborted
	r.info.Error = "run aborted"
	r.info.EndedAt = time.Now()
	r.cancel()
	return r.info, nil
}

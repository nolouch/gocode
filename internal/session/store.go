// Package session provides in-memory session and message storage.
package session

import (
	"fmt"
	"sync"
	"time"

	"github.com/nolouch/gcode/internal/model"
	"github.com/oklog/ulid/v2"
)

// Store is an in-memory store for sessions and messages.
type Store struct {
	mu       sync.RWMutex
	sessions map[string]*model.Session
	messages map[string][]*model.Message // sessionID -> ordered messages
}

// NewStore creates a new in-memory store.
func NewStore() *Store {
	return &Store{
		sessions: make(map[string]*model.Session),
		messages: make(map[string][]*model.Message),
	}
}

// NewID returns a time-sortable unique ID.
func NewID() string {
	return ulid.Make().String()
}

// ─────────────────── Session ───────────────────

// CreateSession creates and stores a new session.
func (s *Store) CreateSession(dir string) *model.Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := &model.Session{
		ID:          NewID(),
		Title:       "New session",
		Directory:   dir,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		DeniedTools: make(map[string]bool),
	}
	s.sessions[sess.ID] = sess
	return sess
}

// GetSession returns a session by ID.
func (s *Store) GetSession(id string) (*model.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", id)
	}
	return sess, nil
}

// TouchSession updates the session timestamp.
func (s *Store) TouchSession(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[id]; ok {
		sess.UpdatedAt = time.Now()
	}
}

// ─────────────────── Messages ───────────────────

// AddMessage appends a message to the session.
func (s *Store) AddMessage(msg *model.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages[msg.SessionID] = append(s.messages[msg.SessionID], msg)
}

// UpdateMessage replaces or appends the message (upsert by ID).
func (s *Store) UpdateMessage(msg *model.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	msgs := s.messages[msg.SessionID]
	for i, m := range msgs {
		if m.ID == msg.ID {
			msgs[i] = msg
			return
		}
	}
	s.messages[msg.SessionID] = append(msgs, msg)
}

// Messages returns all messages for a session (oldest first).
func (s *Store) Messages(sessionID string) []*model.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]*model.Message(nil), s.messages[sessionID]...)
}

// LastUserMessage returns the most recent user message in a session.
func (s *Store) LastUserMessage(sessionID string) *model.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	msgs := s.messages[sessionID]
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == model.RoleUser {
			return msgs[i]
		}
	}
	return nil
}

// LastAssistantMessage returns the most recent assistant message.
func (s *Store) LastAssistantMessage(sessionID string) *model.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	msgs := s.messages[sessionID]
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == model.RoleAssistant {
			return msgs[i]
		}
	}
	return nil
}

// ─────────────────── Parts ───────────────────

// UpsertPart adds or replaces a part within a message.
func (s *Store) UpsertPart(sessionID string, msgID string, part model.Part) {
	s.mu.Lock()
	defer s.mu.Unlock()
	msgs := s.messages[sessionID]
	for _, msg := range msgs {
		if msg.ID != msgID {
			continue
		}
		for i, p := range msg.Parts {
			if p.ID == part.ID {
				msg.Parts[i] = part
				return
			}
		}
		msg.Parts = append(msg.Parts, part)
		return
	}
}

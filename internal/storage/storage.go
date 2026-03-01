// Package storage provides SQLite-backed persistence for sessions and messages.
package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/nolouch/gocode/internal/model"
	"github.com/nolouch/gocode/internal/session"
)

const schema = `
CREATE TABLE IF NOT EXISTS sessions (
	id         TEXT PRIMARY KEY,
	title      TEXT NOT NULL DEFAULT 'New session',
	directory  TEXT NOT NULL DEFAULT '',
	parent_id  TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS messages (
	id         TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	role       TEXT NOT NULL,
	text       TEXT NOT NULL DEFAULT '',
	agent      TEXT NOT NULL DEFAULT '',
	finish     TEXT NOT NULL DEFAULT '',
	error_json TEXT,
	tokens_json TEXT,
	parts_json TEXT,
	created_at INTEGER NOT NULL,
	FOREIGN KEY(session_id) REFERENCES sessions(id)
);

CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id, created_at);
`

// DB wraps a SQLite database and implements persistence for sessions/messages.
type DB struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at the given path.
func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("storage: mkdir: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("storage: open: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite is single-writer
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("storage: schema: %w", err)
	}
	if err := ensureSessionParentColumn(db); err != nil {
		return nil, fmt.Errorf("storage: migrate parent_id: %w", err)
	}
	return &DB{db: db}, nil
}

func ensureSessionParentColumn(db *sql.DB) error {
	_, err := db.Exec(`ALTER TABLE sessions ADD COLUMN parent_id TEXT NOT NULL DEFAULT ''`)
	if err == nil {
		return nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		return nil
	}
	return err
}

// DefaultPath returns ~/.gocode/sessions.db
func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".gocode", "sessions.db")
}

// Close closes the database.
func (d *DB) Close() error { return d.db.Close() }

// ── Sessions ──────────────────────────────────────────────────────────────

func (d *DB) SaveSession(s *model.Session) error {
	_, err := d.db.Exec(`
		INSERT INTO sessions(id, title, directory, parent_id, created_at, updated_at)
		VALUES(?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET title=excluded.title, directory=excluded.directory, parent_id=excluded.parent_id, updated_at=excluded.updated_at`,
		s.ID, s.Title, s.Directory, s.ParentID,
		s.CreatedAt.UnixMilli(), s.UpdatedAt.UnixMilli(),
	)
	return err
}

func (d *DB) ListSessions() ([]*model.Session, error) {
	rows, err := d.db.Query(`SELECT id, title, directory, parent_id, created_at, updated_at FROM sessions ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Session
	for rows.Next() {
		var s model.Session
		var createdMs, updatedMs int64
		if err := rows.Scan(&s.ID, &s.Title, &s.Directory, &s.ParentID, &createdMs, &updatedMs); err != nil {
			return nil, err
		}
		s.CreatedAt = time.UnixMilli(createdMs)
		s.UpdatedAt = time.UnixMilli(updatedMs)
		s.DeniedTools = make(map[string]bool)
		out = append(out, &s)
	}
	return out, rows.Err()
}

func (d *DB) GetSession(id string) (*model.Session, error) {
	var s model.Session
	var createdMs, updatedMs int64
	err := d.db.QueryRow(`SELECT id, title, directory, parent_id, created_at, updated_at FROM sessions WHERE id=?`, id).
		Scan(&s.ID, &s.Title, &s.Directory, &s.ParentID, &createdMs, &updatedMs)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	s.CreatedAt = time.UnixMilli(createdMs)
	s.UpdatedAt = time.UnixMilli(updatedMs)
	s.DeniedTools = make(map[string]bool)
	return &s, nil
}

// ── Messages ──────────────────────────────────────────────────────────────

func (d *DB) SaveMessage(m *model.Message) error {
	errJSON, _ := json.Marshal(m.Error)
	tokJSON, _ := json.Marshal(m.Tokens)
	partsJSON, _ := json.Marshal(m.Parts)
	_, err := d.db.Exec(`
		INSERT INTO messages(id, session_id, role, text, agent, finish, error_json, tokens_json, parts_json, created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			finish=excluded.finish, error_json=excluded.error_json,
			tokens_json=excluded.tokens_json, parts_json=excluded.parts_json`,
		m.ID, m.SessionID, string(m.Role), m.Text, m.Agent, m.Finish,
		string(errJSON), string(tokJSON), string(partsJSON),
		m.CreatedAt.UnixMilli(),
	)
	return err
}

func (d *DB) ListMessages(sessionID string) ([]*model.Message, error) {
	rows, err := d.db.Query(`
		SELECT id, session_id, role, text, agent, finish, error_json, tokens_json, parts_json, created_at
		FROM messages WHERE session_id=? ORDER BY created_at ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Message
	for rows.Next() {
		var msg model.Message
		var createdMs int64
		var errJSON, tokJSON, partsJSON string
		var role string
		if err := rows.Scan(&msg.ID, &msg.SessionID, &role, &msg.Text, &msg.Agent,
			&msg.Finish, &errJSON, &tokJSON, &partsJSON, &createdMs); err != nil {
			return nil, err
		}
		msg.Role = model.Role(role)
		msg.CreatedAt = time.UnixMilli(createdMs)
		json.Unmarshal([]byte(errJSON), &msg.Error)
		json.Unmarshal([]byte(tokJSON), &msg.Tokens)
		json.Unmarshal([]byte(partsJSON), &msg.Parts)
		out = append(out, &msg)
	}
	return out, rows.Err()
}

// ── PersistentStore wraps session.Store + DB ──────────────────────────────

// PersistentStore is a session.Store that also writes to SQLite.
type PersistentStore struct {
	*session.Store
	db *DB
}

// NewPersistentStore creates a PersistentStore, loading existing sessions from DB.
func NewPersistentStore(db *DB) (*PersistentStore, error) {
	store := session.NewStore()
	ps := &PersistentStore{Store: store, db: db}

	// Load existing sessions into memory
	sessions, err := db.ListSessions()
	if err != nil {
		return nil, err
	}
	for _, s := range sessions {
		msgs, err := db.ListMessages(s.ID)
		if err != nil {
			return nil, err
		}
		store.RestoreSession(s, msgs)
	}
	return ps, nil
}

// CreateSession creates a session and persists it.
func (ps *PersistentStore) CreateSession(dir string) *model.Session {
	s := ps.Store.CreateSession(dir)
	ps.db.SaveSession(s)
	return s
}

// SetSessionParent updates parent_id and persists it.
func (ps *PersistentStore) SetSessionParent(id, parentID string) {
	ps.Store.SetSessionParent(id, parentID)
	if s, err := ps.Store.GetSession(id); err == nil {
		ps.db.SaveSession(s)
	}
}

// Children returns child sessions from in-memory state.
func (ps *PersistentStore) Children(parentID string) []*model.Session {
	return ps.Store.Children(parentID)
}

// UpdateMessage persists a message update.
func (ps *PersistentStore) UpdateMessage(msg *model.Message) {
	ps.Store.UpdateMessage(msg)
	ps.db.SaveMessage(msg)
}

// AddMessage persists a new message.
func (ps *PersistentStore) AddMessage(msg *model.Message) {
	ps.Store.AddMessage(msg)
	ps.db.SaveMessage(msg)
}

// TouchSession persists the updated timestamp.
func (ps *PersistentStore) TouchSession(id string) {
	ps.Store.TouchSession(id)
	if s, err := ps.Store.GetSession(id); err == nil {
		ps.db.SaveSession(s)
	}
}

// UpsertPart updates a message part and persists the full message snapshot.
func (ps *PersistentStore) UpsertPart(sessionID string, msgID string, part model.Part) {
	ps.Store.UpsertPart(sessionID, msgID, part)
	for _, msg := range ps.Store.Messages(sessionID) {
		if msg.ID == msgID {
			ps.db.SaveMessage(msg)
			return
		}
	}
}

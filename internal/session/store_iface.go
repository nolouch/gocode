package session

import "github.com/nolouch/opengocode/internal/model"

// StoreAPI is the shared storage contract used by runtime components.
// It is implemented by the in-memory Store and the SQLite-backed PersistentStore.
type StoreAPI interface {
	CreateSession(dir string) *model.Session
	SetSessionParent(id, parentID string)
	Children(parentID string) []*model.Session
	ListSessions() []*model.Session
	GetSession(id string) (*model.Session, error)
	TouchSession(id string)

	AddMessage(msg *model.Message)
	UpdateMessage(msg *model.Message)
	Messages(sessionID string) []*model.Message
	LastUserMessage(sessionID string) *model.Message
	LastAssistantMessage(sessionID string) *model.Message
	UpsertPart(sessionID string, msgID string, part model.Part)
}

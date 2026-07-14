package session

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/lease"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function string `json:"function"`
}

type ExecutionContext struct {
	SystemPrompt   string     `json:"system_prompt"`
	Summary        string     `json:"summary"`
	PinnedFacts    []string   `json:"pinned_facts"`
	RecentMessages []Message  `json:"recent_messages"`
	TerminalState  string     `json:"terminal_state"`
	ToolState      []ToolCall `json:"tool_state"`
}

type VirtualSession struct {
	ID             string            `json:"id"`
	OwnerID        string            `json:"owner_id"`
	ConversationID string            `json:"conversation_id"`
	State          string            `json:"state"` // new, active, rebinding, queued, closed
	Version        int               `json:"version"`
	Lease          *lease.Lease      `json:"lease,omitempty"`
	Context        *ExecutionContext `json:"context,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

type SessionManager struct {
	store *storage.Storage
}

func NewSessionManager(store *storage.Storage) *SessionManager {
	return &SessionManager{store: store}
}

func (sm *SessionManager) GetSession(ctx context.Context, sessionID string) (*VirtualSession, error) {
	key := storage.SessionKey(sessionID)
	data, err := sm.store.Redis().Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}
	var sess VirtualSession
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, err
	}
	return &sess, nil
}

func (sm *SessionManager) SaveSession(ctx context.Context, sess *VirtualSession) error {
	key := storage.SessionKey(sess.ID)
	data, err := json.Marshal(sess)
	if err != nil {
		return err
	}
	return sm.store.Redis().Set(ctx, key, data, 0).Err()
}

func (sm *SessionManager) DeleteSession(ctx context.Context, sessionID string) error {
	key := storage.SessionKey(sessionID)
	return sm.store.Redis().Del(ctx, key).Err()
}

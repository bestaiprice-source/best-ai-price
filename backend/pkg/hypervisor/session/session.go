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

type ProjectSnapshot struct {
	WorkspaceID string    `json:"workspace_id"`
	GitBranch   string    `json:"git_branch"`
	GitCommit   string    `json:"git_commit"`
	DirtyFiles  []string  `json:"dirty_files"`
	OpenFiles   []string  `json:"open_files"`
	LastCommand string    `json:"last_command"`
	SnapshotRef string    `json:"snapshot_ref"`
	CreatedAt   time.Time `json:"created_at"`
}

type ExecutionContext struct {
	SystemPrompt   string           `json:"system_prompt"`
	Summary        string           `json:"summary"`
	PinnedFacts    []string         `json:"pinned_facts"`
	RecentMessages []Message        `json:"recent_messages"`
	TerminalState  string           `json:"terminal_state"`
	ToolState      []ToolCall       `json:"tool_state"`
	ProjectState   *ProjectSnapshot `json:"project_state,omitempty"`
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
	if err == nil {
		var sess VirtualSession
		if err := json.Unmarshal(data, &sess); err == nil {
			return &sess, nil
		}
	}

	// Fallback to DB
	if sm.store.DB() != nil {
		dbSess, err := sm.store.DB().AshSession.Get(ctx, sessionID)
		if err == nil && dbSess != nil {
			// Convert back to VirtualSession
			sess := &VirtualSession{
				ID:             dbSess.ID,
				OwnerID:        dbSess.OwnerID,
				ConversationID: dbSess.ConversationID,
				State:          dbSess.State,
				CreatedAt:      dbSess.CreatedAt,
				UpdatedAt:      dbSess.UpdatedAt,
			}
			if dbSess.Metadata != nil {
				// We don't restore Version and Lease from DB right now since they are transient, but we could.
			}
			// Write back to Redis for hot cache
			_ = sm.SaveSession(ctx, sess)
			return sess, nil
		}
	}

	return nil, err
}

func (sm *SessionManager) SaveSession(ctx context.Context, sess *VirtualSession) error {
	key := storage.SessionKey(sess.ID)
	data, err := json.Marshal(sess)
	if err != nil {
		return err
	}
	
	// Write to Redis (hot state)
	redisErr := sm.store.Redis().Set(ctx, key, data, 24*time.Hour).Err()
	
	// Write to PostgreSQL
	if sm.store.DB() != nil {
		dbErr := sm.store.DB().AshSession.Create().
			SetID(sess.ID).
			SetOwnerID(sess.OwnerID).
			SetConversationID(sess.ConversationID).
			SetState(sess.State).
			SetCreatedAt(sess.CreatedAt).
			SetUpdatedAt(sess.UpdatedAt).
			OnConflict().
			UpdateNewValues().
			Exec(ctx)
			
		if dbErr != nil {
			// Ignore DB error for MVP if Redis succeeded, but we should log it
		}
	}
	
	return redisErr
}

func (sm *SessionManager) DeleteSession(ctx context.Context, sessionID string) error {
	key := storage.SessionKey(sessionID)
	redisErr := sm.store.Redis().Del(ctx, key).Err()
	
	if sm.store.DB() != nil {
		_ = sm.store.DB().AshSession.DeleteOneID(sessionID).Exec(ctx)
	}
	
	return redisErr
}

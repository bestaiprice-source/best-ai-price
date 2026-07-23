package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/session"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
)

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

type BackupManager struct {
	store *storage.Storage
	sm    *session.SessionManager
}

func NewBackupManager(store *storage.Storage) *BackupManager {
	return &BackupManager{
		store: store,
		sm:    session.NewSessionManager(store),
	}
}

func SnapshotKey(sessionID string) string {
	return fmt.Sprintf("ash:snapshot:%s", sessionID)
}

func (bm *BackupManager) CreateSnapshot(ctx context.Context, sessionID string, snapshot *ProjectSnapshot) error {
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Now()
	}
	key := SnapshotKey(sessionID)
	data, err := json.Marshal(snapshot)
	if err != nil {
		slog.Error("[BackupManager] Failed to marshal project snapshot", "session_id", sessionID, "error", err)
		return err
	}

	err = bm.store.Redis().Set(ctx, key, data, 7*24*time.Hour).Err()
	if err != nil {
		slog.Error("[BackupManager] Failed to save project snapshot to Redis", "session_id", sessionID, "error", err)
		return err
	}

	slog.Info("[BackupManager] Created project snapshot",
		"session_id", sessionID,
		"workspace_id", snapshot.WorkspaceID,
		"git_branch", snapshot.GitBranch,
		"dirty_files_count", len(snapshot.DirtyFiles),
	)

	_, _ = bm.store.LogEvent(ctx, "system", &sessionID, nil, "SnapshotCreated", map[string]any{
		"workspace_id": snapshot.WorkspaceID,
		"git_branch":   snapshot.GitBranch,
		"snapshot_ref": snapshot.SnapshotRef,
	})

	return nil
}

func (bm *BackupManager) GetLatestSnapshot(ctx context.Context, sessionID string) (*ProjectSnapshot, error) {
	key := SnapshotKey(sessionID)
	data, err := bm.store.Redis().Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}
	var snap ProjectSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, err
	}
	return &snap, nil
}

func (bm *BackupManager) RestoreSessionFromSnapshot(ctx context.Context, sessionID string) (*session.VirtualSession, error) {
	snap, err := bm.GetLatestSnapshot(ctx, sessionID)
	if err != nil {
		slog.Warn("[BackupManager] No snapshot found for session restoration", "session_id", sessionID)
		return nil, fmt.Errorf("no project snapshot found for session %s: %w", sessionID, err)
	}

	sess, err := bm.sm.GetSession(ctx, sessionID)
	if err != nil || sess == nil {
		sess = &session.VirtualSession{
			ID:        sessionID,
			State:     "active",
			Version:   1,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
	}

	sess.State = "active"
	sess.UpdatedAt = time.Now()
	_ = bm.sm.SaveSession(ctx, sess)

	slog.Info("[BackupManager] Restored session from snapshot",
		"session_id", sessionID,
		"workspace_id", snap.WorkspaceID,
		"git_branch", snap.GitBranch,
	)

	return sess, nil
}

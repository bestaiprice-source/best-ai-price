package backup

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
	_ "modernc.org/sqlite"
)

func newTestStorage(t *testing.T) *storage.Storage {
	t.Helper()

	name := strings.ReplaceAll(t.Name(), "/", "_")
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", name)
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return storage.NewStorage(client, rdb)
}

func TestBackupManagerCreateAndRestore(t *testing.T) {
	store := newTestStorage(t)
	bm := NewBackupManager(store)

	ctx := context.Background()
	sessID := "sess-backup-1"

	snap := &ProjectSnapshot{
		WorkspaceID: "ws-dev",
		GitBranch:   "main",
		GitCommit:   "abc1234",
		DirtyFiles:  []string{"main.go"},
		OpenFiles:   []string{"app.go"},
		LastCommand: "go test",
		SnapshotRef: "ref-1",
		CreatedAt:   time.Now(),
	}

	err := bm.CreateSnapshot(ctx, sessID, snap)
	require.NoError(t, err)

	fetched, err := bm.GetLatestSnapshot(ctx, sessID)
	require.NoError(t, err)
	assert.Equal(t, "ws-dev", fetched.WorkspaceID)
	assert.Equal(t, "main", fetched.GitBranch)

	restoredSess, err := bm.RestoreSessionFromSnapshot(ctx, sessID)
	require.NoError(t, err)
	assert.Equal(t, sessID, restoredSess.ID)
	assert.Equal(t, "active", restoredSess.State)

	// Test missing snapshot
	_, err = bm.RestoreSessionFromSnapshot(ctx, "non-existent-session")
	assert.Error(t, err)

	// Test snapshot with zero CreatedAt
	zeroSnap := &ProjectSnapshot{WorkspaceID: "ws-zero"}
	err = bm.CreateSnapshot(ctx, "sess-zero", zeroSnap)
	require.NoError(t, err)
	savedZero, err := bm.GetLatestSnapshot(ctx, "sess-zero")
	require.NoError(t, err)
	assert.False(t, savedZero.CreatedAt.IsZero())
}

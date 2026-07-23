package session_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/session"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
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

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	t.Cleanup(func() { _ = rdb.Close() })

	return storage.NewStorage(client, rdb)
}

func TestSessionManager(t *testing.T) {
	store := newTestStorage(t)
	ctx := context.Background()

	sm := session.NewSessionManager(store)
	sessID := "session-abc"
	sess := &session.VirtualSession{
		ID:             sessID,
		OwnerID:        "owner-1",
		ConversationID: "conv-1",
		State:          "active",
		Version:        1,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	err := sm.SaveSession(ctx, sess)
	require.NoError(t, err)

	savedSess, err := sm.GetSession(ctx, sessID)
	require.NoError(t, err)
	require.Equal(t, "owner-1", savedSess.OwnerID)
	require.Equal(t, "conv-1", savedSess.ConversationID)

	err = sm.DeleteSession(ctx, sessID)
	require.NoError(t, err)

	_, err = sm.GetSession(ctx, sessID)
	require.Error(t, err)
}

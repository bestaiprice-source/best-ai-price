package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func newTestStorage(t *testing.T) *Storage {
	t.Helper()

	// Setup SQLite in-memory DB
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

	// Setup miniredis
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	t.Cleanup(func() { _ = rdb.Close() })

	return NewStorage(client, rdb)
}

func TestRedisKeys(t *testing.T) {
	require.Equal(t, "ash:session:sess1", SessionKey("sess1"))
	require.Equal(t, "ash:lease:sess1", LeaseKey("sess1"))
	require.Equal(t, "ash:node:node1", NodeKey("node1"))
	require.Equal(t, "ash:lock:session:sess1", LockSessionKey("sess1"))
	require.Equal(t, "ash:affinity:owner1", AffinityKey("owner1"))
	require.Equal(t, "ash:queue:5", QueueKey(5))
}

func TestRedisLock(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	// Try acquiring lock
	ok, err := s.AcquireLock(ctx, "session1", 10*time.Second)
	require.NoError(t, err)
	require.True(t, ok)

	// Try acquiring again (should fail)
	ok, err = s.AcquireLock(ctx, "session1", 10*time.Second)
	require.NoError(t, err)
	require.False(t, ok)

	// Release lock
	err = s.ReleaseLock(ctx, "session1")
	require.NoError(t, err)

	// Acquire again (should succeed)
	ok, err = s.AcquireLock(ctx, "session1", 10*time.Second)
	require.NoError(t, err)
	require.True(t, ok)
}

func TestPostgresLogEvent(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	sessID := "sess1"
	nodeID := "node1"
	payload := map[string]any{"reason": "timeout"}

	event, err := s.LogEvent(ctx, "owner1", &sessID, &nodeID, "RebindingStarted", payload)
	require.NoError(t, err)
	require.NotNil(t, event)
	require.Equal(t, "owner1", event.OwnerID)
	require.Equal(t, sessID, *event.SessionID)
	require.Equal(t, nodeID, *event.NodeID)
	require.Equal(t, "RebindingStarted", event.EventType)
	require.Equal(t, "timeout", event.Payload["reason"])

	// Test nil session and node
	event2, err := s.LogEvent(ctx, "owner2", nil, nil, "SystemStartup", map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, event2)
	require.Equal(t, "owner2", event2.OwnerID)
	require.Nil(t, event2.SessionID)
	require.Nil(t, event2.NodeID)
}

func TestPostgresRecordUsage(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	sessID := "sess1"
	nodeID := "node1"

	usage, err := s.RecordUsage(ctx, "owner1", &sessID, &nodeID, 100, 200, 5)
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.Equal(t, "owner1", usage.OwnerID)
	require.Equal(t, sessID, *usage.SessionID)
	require.Equal(t, nodeID, *usage.NodeID)
	require.Equal(t, int64(100), usage.InputTokens)
	require.Equal(t, int64(200), usage.OutputTokens)
	require.Equal(t, int64(5), usage.CostUnits)

	// Test nil session and node
	usage2, err := s.RecordUsage(ctx, "owner2", nil, nil, 50, 60, 2)
	require.NoError(t, err)
	require.NotNil(t, usage2)
	require.Equal(t, "owner2", usage2.OwnerID)
	require.Nil(t, usage2.SessionID)
	require.Nil(t, usage2.NodeID)
}

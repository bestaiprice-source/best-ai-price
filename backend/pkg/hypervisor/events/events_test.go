package events

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

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

func TestEventWriterLogEvent(t *testing.T) {
	store := newTestStorage(t)
	ew := NewEventWriter(store)

	sessID := "sess-100"
	nodeID := "node-200"

	event, err := ew.LogEvent(context.Background(), "user-1", &sessID, &nodeID, "RebindingSucceeded", map[string]any{
		"retry_count": 1,
	})
	require.NoError(t, err)
	assert.Equal(t, "user-1", event.OwnerID)
	assert.Equal(t, "RebindingSucceeded", event.EventType)
	assert.Equal(t, "sess-100", *event.SessionID)
	assert.Equal(t, "node-200", *event.NodeID)
}

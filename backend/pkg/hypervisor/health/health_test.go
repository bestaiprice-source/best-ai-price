package health_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
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

func TestNodeManagerAndHealthController(t *testing.T) {
	store := newTestStorage(t)
	ctx := context.Background()

	nm := health.NewNodeManager(store)
	hc := health.NewHealthController(store)

	node := &health.ExecutionNode{
		NodeID:      "node-1",
		AccountName: "Test Node 1",
		Provider:    "anthropic",
		State:       "healthy",
	}

	err := nm.SaveNode(ctx, node)
	require.NoError(t, err)

	savedNode, err := nm.GetNode(ctx, "node-1")
	require.NoError(t, err)
	require.Equal(t, "Test Node 1", savedNode.AccountName)

	now := time.Now()
	state := &health.NodeRuntimeState{
		NodeID:        "node-1",
		SuccessRate:   0.95,
		AvgLatencyMs:  150,
		Recent429:     1,
		LastErrorAt:   &now,
	}

	err = nm.SaveNodeRuntimeState(ctx, state)
	require.NoError(t, err)

	savedState, err := nm.GetNodeRuntimeState(ctx, "node-1")
	require.NoError(t, err)
	require.Equal(t, 0.95, savedState.SuccessRate)

	systemState := hc.GetSystemState(ctx, "anthropic")
	require.Equal(t, "NORMAL", systemState)
}

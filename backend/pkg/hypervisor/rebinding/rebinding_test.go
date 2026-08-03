package rebinding

import (
	"context"
	"database/sql"
	"errors"
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
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/session"
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

func TestExecuteRebindingSuccess(t *testing.T) {
	store := newTestStorage(t)
	rm := NewRebindingManager(store)

	sess := &session.VirtualSession{
		ID:             "sess-rebinding-1",
		OwnerID:        "user-1",
		ConversationID: "conv-1",
		State:          "active",
	}

	nodes := []health.ExecutionNode{
		{NodeID: "1", AccountName: "node-1", State: "healthy"},
		{NodeID: "2", AccountName: "node-2", State: "healthy"},
	}
	states := map[string]health.NodeRuntimeState{
		"1": {NodeID: "1", SuccessRate: 1.0},
		"2": {NodeID: "2", SuccessRate: 1.0},
	}

	selectFn := func(ctx context.Context, nodes []health.ExecutionNode, states map[string]health.NodeRuntimeState, userPriority int, activeLease string, systemState string) (*health.ExecutionNode, error) {
		return &nodes[1], nil
	}

	upstreamErr := errors.New("upstream error status 429 Too Many Requests")
	newNode, err := rm.RebindSession(
		context.Background(),
		sess,
		"1",
		nodes,
		states,
		1,
		selectFn,
		"NORMAL",
		upstreamErr,
	)

	require.NoError(t, err)
	assert.Equal(t, "2", newNode.NodeID)
}

func TestExecuteRebindingLockConflict(t *testing.T) {
	store := newTestStorage(t)
	rm := NewRebindingManager(store)

	sess := &session.VirtualSession{
		ID:             "sess-locked",
		OwnerID:        "user-1",
		ConversationID: "conv-1",
		State:          "active",
	}

	// Pre-acquire session lock
	locked, err := store.AcquireLock(context.Background(), sess.ID, 0)
	require.NoError(t, err)
	require.True(t, locked)

	selectFn := func(ctx context.Context, nodes []health.ExecutionNode, states map[string]health.NodeRuntimeState, userPriority int, activeLease string, systemState string) (*health.ExecutionNode, error) {
		return &nodes[0], nil
	}

	_, err = rm.RebindSession(
		context.Background(),
		sess,
		"1",
		nil,
		nil,
		1,
		selectFn,
		"NORMAL",
		errors.New("429 error"),
	)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to acquire session lock")
}

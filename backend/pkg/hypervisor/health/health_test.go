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
	"github.com/stretchr/testify/assert"
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
		NodeID:       "node-1",
		SuccessRate:  0.95,
		AvgLatencyMs: 150,
		Recent429:    1,
		LastErrorAt:  &now,
	}

	err = nm.SaveNodeRuntimeState(ctx, state)
	require.NoError(t, err)

	savedState, err := nm.GetNodeRuntimeState(ctx, "node-1")
	require.NoError(t, err)
	require.Equal(t, 0.95, savedState.SuccessRate)

	systemState := hc.GetSystemState(ctx, "anthropic")
	require.Equal(t, "NORMAL", systemState)

	// Reconcile cycle
	err = hc.Reconcile(ctx)
	require.NoError(t, err)

	// Start / Stop background worker
	hc.Start(ctx, 10*time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	hc.Stop()

	// Delete node and runtime state
	err = nm.DeleteNodeRuntimeState(ctx, "node-1")
	require.NoError(t, err)

	err = nm.DeleteNode(ctx, "node-1")
	require.NoError(t, err)

	_, err = nm.GetNode(ctx, "node-1")
	assert.Error(t, err)
}

func TestHealthControllerCooldownResetAndLimitedState(t *testing.T) {
	store := newTestStorage(t)
	ctx := context.Background()

	nm := health.NewNodeManager(store)
	hc := health.NewHealthController(store)

	// Create an account in DB so Reconcile sees it
	acc, err := store.DB().Account.Create().
		SetName("acc-cooldown").
		SetPlatform("anthropic").
		SetType("api_key").
		SetStatus("active").
		Save(ctx)
	require.NoError(t, err)

	nodeID := fmt.Sprintf("%d", acc.ID)

	// expired cooldown
	pastTime := time.Now().Add(-10 * time.Second)
	state := &health.NodeRuntimeState{
		NodeID:        nodeID,
		CooldownUntil: &pastTime,
		Recent429:     4,
	}
	require.NoError(t, nm.SaveNodeRuntimeState(ctx, state))

	require.NoError(t, hc.Reconcile(ctx))

	// Reconcile should have reset cooldown to healthy
	savedNode, err := nm.GetNode(ctx, nodeID)
	require.NoError(t, err)
	require.Equal(t, "healthy", savedNode.State)
}

func TestHealthControllerDegradationStates(t *testing.T) {
	store := newTestStorage(t)
	ctx := context.Background()

	nm := health.NewNodeManager(store)
	hc := health.NewHealthController(store)

	// Create 4 accounts
	for i := 1; i <= 4; i++ {
		acc, err := store.DB().Account.Create().
			SetName(fmt.Sprintf("acc-%d", i)).
			SetPlatform("openai").
			SetType("api_key").
			SetStatus("active").
			Save(ctx)
		require.NoError(t, err)
		nodeID := fmt.Sprintf("%d", acc.ID)

		nodeState := "healthy"
		if i > 1 {
			// Mark nodes 2, 3, 4 with active cooldown so healthyRatio < 0.25
			futureTime := time.Now().Add(10 * time.Minute)
			_ = nm.SaveNodeRuntimeState(ctx, &health.NodeRuntimeState{
				NodeID:        nodeID,
				CooldownUntil: &futureTime,
			})
			nodeState = "cooldown"
		}
		_ = nm.SaveNode(ctx, &health.ExecutionNode{
			NodeID:   nodeID,
			Provider: "openai",
			State:    nodeState,
		})
	}

	require.NoError(t, hc.Reconcile(ctx))
	sysState := hc.GetSystemState(ctx, "openai")
	require.Equal(t, "DEGRADED", sysState)
}


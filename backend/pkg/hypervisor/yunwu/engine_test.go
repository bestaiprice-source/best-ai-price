package yunwu

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/quota"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/runtime"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
)

type mockYunwuRuntime struct{}

func (m *mockYunwuRuntime) Execute(ctx context.Context, nodeID string, reqPayload []byte, stream io.Writer) (*runtime.ResponseMetadata, error) {
	return &runtime.ResponseMetadata{
		StatusCode:   200,
		InputTokens:  150,
		OutputTokens: 250,
	}, nil
}

func setupTestYunwuStore(t *testing.T) *storage.Storage {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	return storage.NewStorage(nil, rdb)
}

func TestYunwuHypervisorEndToEnd(t *testing.T) {
	store := setupTestYunwuStore(t)
	rt := &mockYunwuRuntime{}
	yh := NewYunwuHypervisor(store, rt)

	// Add test node in Redis
	nm := health.NewNodeManager(store)
	err := nm.SaveNode(context.Background(), &health.ExecutionNode{
		NodeID:      "yunwu-101",
		AccountName: "Yunwu Channel Test 1",
		Provider:    "yunwu",
		State:       "healthy",
	})
	require.NoError(t, err)

	err = store.Redis().SAdd(context.Background(), "ash:yunwu:nodes", "yunwu-101").Err()
	require.NoError(t, err)

	// Set test credit balance
	err = store.Redis().Set(context.Background(), "ash:yunwu:credit:yunwu-101", 50.0, 24*time.Hour).Err()
	require.NoError(t, err)

	// Save test user access profile
	err = yh.qm.SaveUserAccess(context.Background(), &quota.UserAccess{
		OwnerID:      "test-owner-1",
		PlanID:       "pool-basic",
		MonthlyLimit: 1000000,
		UsedUnits:    100,
		Priority:     1,
		IsActive:     true,
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	meta, err := yh.HandleRequest(context.Background(), "test-owner-1", "conv-debate-1", []byte(`{"model":"claude-3-5-sonnet-20241022"}`), &buf, "anthropic")
	require.NoError(t, err)
	assert.NotNil(t, meta)
	assert.Equal(t, 200, meta.StatusCode)
	assert.Equal(t, int64(150), meta.InputTokens)
	assert.Equal(t, int64(250), meta.OutputTokens)

	// Verify Health Status API
	status := yh.GetHealthStatus(context.Background())
	assert.Equal(t, "ok", status["status"])
	assert.Equal(t, "yunwu", status["platform"])
	assert.Equal(t, 1, status["total_nodes"])
	assert.Equal(t, 1, status["healthy_nodes"])
}

func TestYunwuRebindingOnFailure(t *testing.T) {
	store := setupTestYunwuStore(t)
	nm := health.NewNodeManager(store)

	node1 := health.ExecutionNode{NodeID: "yw-1", AccountName: "Yunwu 1", State: "healthy"}
	node2 := health.ExecutionNode{NodeID: "yw-2", AccountName: "Yunwu 2", State: "healthy"}

	_ = nm.SaveNode(context.Background(), &node1)
	_ = nm.SaveNode(context.Background(), &node2)

	rm := NewYunwuRebindingManager(store)
	reboundNode, err := rm.HandleYunwuFailure(context.Background(), "yw-1", assert.AnError, []health.ExecutionNode{node1, node2})

	require.NoError(t, err)
	assert.NotNil(t, reboundNode)
	assert.Equal(t, "yw-2", reboundNode.NodeID)

	// Verify yw-1 is now in cooldown
	failedState, err := nm.GetNodeRuntimeState(context.Background(), "yw-1")
	require.NoError(t, err)
	assert.NotNil(t, failedState.CooldownUntil)
}

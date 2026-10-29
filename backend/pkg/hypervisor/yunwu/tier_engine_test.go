package yunwu

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/quota"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/runtime"
)

// TestYunwuBillingTierRouting proves a fast request is served only by a node
// that holds a fast key, while a smart (default) request still uses the pool.
func TestYunwuBillingTierRouting(t *testing.T) {
	t.Setenv("ASH_YUNWU_TEST_MODE", "true")
	store := setupTestYunwuStore(t)
	rt := &mockYunwuRuntime{}
	yh := NewYunwuHypervisor(store, rt)
	nm := health.NewNodeManager(store)

	nodes := []health.ExecutionNode{
		{NodeID: "smart-node", AccountName: "Smart", Provider: "yunwu", State: "healthy", Tiers: []string{runtime.TierSmart}},
		{NodeID: "fast-node", AccountName: "Fast", Provider: "yunwu", State: "healthy", Tiers: []string{runtime.TierSmart, runtime.TierFast}},
	}
	for i := range nodes {
		node := nodes[i]
		require.NoError(t, nm.SaveNode(context.Background(), &node))
		require.NoError(t, store.Redis().SAdd(context.Background(), "ash:yunwu:nodes", node.NodeID).Err())
		setTestBalance(t, store, node.NodeID, 50_000_000)
		require.NoError(t, nm.SaveNodeRuntimeState(context.Background(), &health.NodeRuntimeState{NodeID: node.NodeID, SuccessRate: 1.0}))
	}
	require.NoError(t, yh.qm.SaveUserAccess(context.Background(), &quota.UserAccess{
		OwnerID: "tier-owner", MonthlyLimit: 1000000, Priority: 1, IsActive: true,
	}, "yunwu"))

	payload := []byte(`{"model":"claude-3-5-sonnet-20241022"}`)

	// Default (no tier in context) == smart: served from the pool.
	_, err := yh.HandleRequest(context.Background(), "tier-owner", "conv-smart", payload, io.Discard, "anthropic")
	require.NoError(t, err)

	// Fast request: the smart-only node must be skipped.
	fastCtx := runtime.ContextWithTier(context.Background(), runtime.TierFast)
	_, err = yh.HandleRequest(fastCtx, "tier-owner", "conv-fast", payload, io.Discard, "anthropic")
	require.NoError(t, err)
	assert.Equal(t, "fast-node", rt.lastNodeID, "a fast request must be served by the fast node")
}

// TestYunwuBillingTierNotConfigured proves the engine fails explicitly instead of
// silently downgrading a fast request onto a smart-priced key.
func TestYunwuBillingTierNotConfigured(t *testing.T) {
	t.Setenv("ASH_YUNWU_TEST_MODE", "true")
	store := setupTestYunwuStore(t)
	rt := &mockYunwuRuntime{}
	yh := NewYunwuHypervisor(store, rt)
	nm := health.NewNodeManager(store)

	require.NoError(t, nm.SaveNode(context.Background(), &health.ExecutionNode{
		NodeID: "only-smart", AccountName: "OnlySmart", Provider: "yunwu", State: "healthy",
		Tiers: []string{runtime.TierSmart},
	}))
	require.NoError(t, store.Redis().SAdd(context.Background(), "ash:yunwu:nodes", "only-smart").Err())
	setTestBalance(t, store, "only-smart", 50_000_000)
	require.NoError(t, nm.SaveNodeRuntimeState(context.Background(), &health.NodeRuntimeState{NodeID: "only-smart", SuccessRate: 1.0}))
	require.NoError(t, yh.qm.SaveUserAccess(context.Background(), &quota.UserAccess{
		OwnerID: "tier-owner-2", MonthlyLimit: 1000000, Priority: 1, IsActive: true,
	}, "yunwu"))

	fastCtx := runtime.ContextWithTier(context.Background(), runtime.TierFast)
	_, err := yh.HandleRequest(fastCtx, "tier-owner-2", "conv-fast", []byte(`{"model":"claude-3-5-sonnet-20241022"}`), io.Discard, "anthropic")
	require.ErrorIs(t, err, ErrTierNotConfigured)
}

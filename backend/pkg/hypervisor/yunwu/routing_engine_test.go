package yunwu

import (
	"context"
	"io"
	"testing"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/quota"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestYunwuExplicitModelRouting verifies that an administrator-configured
// Primary/Backup route (as produced by promoting a cheaper provider) is honored
// by the engine, overriding the default cheapest-first pool scheduler.
func TestYunwuExplicitModelRouting(t *testing.T) {
	t.Setenv("ASH_YUNWU_TEST_MODE", "true")

	store := setupTestYunwuStore(t)
	rt := &mockYunwuRuntime{}
	yh := NewYunwuHypervisor(store, rt)
	nm := health.NewNodeManager(store)
	ctx := context.Background()

	// Two numeric account IDs, both supporting Claude with valid pricing.
	for _, id := range []string{"101", "102"} {
		require.NoError(t, nm.SaveNode(ctx, &health.ExecutionNode{
			NodeID:       id,
			AccountName:  "node-" + id,
			Provider:     "openlux",
			State:        "healthy",
			Capabilities: []string{FamilyClaude},
		}))
		require.NoError(t, store.Redis().SAdd(ctx, "ash:yunwu:nodes", id).Err())
		setTestBalance(t, store, id, 10_000_000)
	}

	ppm := yh.PriceManager()
	require.NotNil(t, ppm)
	// 101 is cheap, 102 is expensive.
	ppm.SetProviderPrices("101", map[string]ProviderModelPrice{
		"claude-3-7-sonnet": {InputPricePer1k: 0.003, OutputPricePer1k: 0.015, Available: true},
	})
	ppm.SetProviderPrices("102", map[string]ProviderModelPrice{
		"claude-3-7-sonnet": {InputPricePer1k: 0.030, OutputPricePer1k: 0.150, Available: true},
	})

	require.NoError(t, yh.qm.SaveUserAccess(ctx, &quota.UserAccess{
		OwnerID:      "routing-user",
		PlanID:       "basic",
		MonthlyLimit: 1_000_000,
		Priority:     1,
		IsActive:     true,
	}, "yunwu"))

	payload := []byte(`{"model": "claude-3-7-sonnet", "messages": [{"role": "user", "content": "hi"}]}`)

	// Default pool scheduling would pick the cheaper node 101. Explicit routing
	// to [102] must force node 102.
	routedCtx := ContextWithModelRouting(ctx, []int64{102})
	_, err := yh.HandleRequest(routedCtx, "routing-user", "conv-routed", payload, io.Discard, "yunwu")
	require.NoError(t, err)
	assert.Equal(t, "102", rt.lastNodeID, "explicit routing must override cheapest-first scheduling")

	// Routing [101, 102] must prefer the Primary (101) first.
	routedCtx2 := ContextWithModelRouting(ctx, []int64{101, 102})
	_, err = yh.HandleRequest(routedCtx2, "routing-user", "conv-routed-2", payload, io.Discard, "yunwu")
	require.NoError(t, err)
	assert.Equal(t, "101", rt.lastNodeID, "Primary route must be preferred over Backup")

	// Without routing, cheapest-first scheduling picks node 101.
	_, err = yh.HandleRequest(ctx, "routing-user", "conv-pool", payload, io.Discard, "yunwu")
	require.NoError(t, err)
	assert.Equal(t, "101", rt.lastNodeID, "pool scheduling should pick the cheapest node")
}

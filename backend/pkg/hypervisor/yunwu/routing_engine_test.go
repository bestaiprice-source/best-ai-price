package yunwu

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/quota"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/runtime"
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

// recordingFailoverRuntime records the last node that successfully served a
// request and fails a configurable node to exercise failover.
type recordingFailoverRuntime struct {
	failNode   string
	lastNodeID string
}

func (r *recordingFailoverRuntime) Execute(ctx context.Context, nodeID string, reqPayload []byte, stream io.Writer) (*runtime.ResponseMetadata, error) {
	if r.failNode != "" && nodeID == r.failNode {
		return nil, errors.New("upstream error status 500: provider down")
	}
	r.lastNodeID = nodeID
	return &runtime.ResponseMetadata{StatusCode: 200, InputTokens: 10, OutputTokens: 10, NodeID: nodeID}, nil
}

// TestYunwuPriceBasedPrimaryBackup verifies that Primary/Backup are assigned by
// price: the cheapest provider for the model is Primary and the next cheapest is
// Backup, used automatically on failover.
func TestYunwuPriceBasedPrimaryBackup(t *testing.T) {
	t.Setenv("ASH_YUNWU_TEST_MODE", "true")

	store := setupTestYunwuStore(t)
	rt := &recordingFailoverRuntime{}
	yh := NewYunwuHypervisor(store, rt)
	nm := health.NewNodeManager(store)
	ctx := context.Background()

	// Three nodes, all supporting Claude. Totals: 201=0.018 (cheapest),
	// 203=0.060 (mid), 202=0.180 (most expensive).
	for _, id := range []string{"201", "202", "203"} {
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
	ppm.SetProviderPrices("201", map[string]ProviderModelPrice{"claude-3-7-sonnet": {InputPricePer1k: 0.003, OutputPricePer1k: 0.015, Available: true}})
	ppm.SetProviderPrices("203", map[string]ProviderModelPrice{"claude-3-7-sonnet": {InputPricePer1k: 0.010, OutputPricePer1k: 0.050, Available: true}})
	ppm.SetProviderPrices("202", map[string]ProviderModelPrice{"claude-3-7-sonnet": {InputPricePer1k: 0.030, OutputPricePer1k: 0.150, Available: true}})

	require.NoError(t, yh.qm.SaveUserAccess(ctx, &quota.UserAccess{
		OwnerID:      "price-route-user",
		PlanID:       "basic",
		MonthlyLimit: 1_000_000,
		Priority:     1,
		IsActive:     true,
	}, "yunwu"))

	payload := []byte(`{"model": "claude-3-7-sonnet", "messages": [{"role": "user", "content": "hi"}]}`)
	priceNodes := []health.ExecutionNode{{NodeID: "201"}, {NodeID: "203"}, {NodeID: "202"}}
	primary, backup := pricePrimaryBackup(priceNodes, map[string]float64{"201": 0.018, "203": 0.060, "202": 0.180})
	assert.Equal(t, "201", primary, "cheapest provider must be Primary")
	assert.Equal(t, "203", backup, "next cheapest provider must be Backup")

	// 1. No failure: Primary (cheapest) serves the request.
	_, err := yh.HandleRequest(ctx, "price-route-user", "conv-primary", payload, io.Discard, "yunwu")
	require.NoError(t, err)
	assert.Equal(t, "201", rt.lastNodeID, "Primary (cheapest) must serve the request")

	// 2. Primary fails: request must fail over to Backup (next cheapest = 203),
	// not to the most expensive node 202.
	rt.failNode = "201"
	_, err = yh.HandleRequest(ctx, "price-route-user", "conv-backup", payload, io.Discard, "yunwu")
	require.NoError(t, err)
	assert.Equal(t, "203", rt.lastNodeID, "failover must go to Backup (next cheapest)")
}

// TestYunwuExplicitRoutingRespectsRecoveryStreak — §6.1 applies to the explicit
// administrator route too: a configured Primary that just failed and has not yet
// proven a healthy streak must be deferred to the Backup instead of being
// re-selected immediately (pool scheduling already defers such nodes).
func TestYunwuExplicitRoutingRespectsRecoveryStreak(t *testing.T) {
	t.Setenv("ASH_YUNWU_TEST_MODE", "true")
	t.Setenv("ASH_YUNWU_REQUIRED_HEALTHY_STREAK", "2")

	store := setupTestYunwuStore(t)
	rt := &mockYunwuRuntime{}
	yh := NewYunwuHypervisor(store, rt)
	nm := health.NewNodeManager(store)
	ctx := context.Background()

	for _, id := range []string{"111", "112"} {
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
	for _, id := range []string{"111", "112"} {
		ppm.SetProviderPrices(id, map[string]ProviderModelPrice{
			"claude-3-7-sonnet": {InputPricePer1k: 0.003, OutputPricePer1k: 0.015, Available: true},
		})
	}

	// 111 is the configured Primary, but it just failed and its success streak is 0.
	failedAt := time.Now()
	require.NoError(t, nm.SaveNodeRuntimeState(ctx, &health.NodeRuntimeState{
		NodeID:               "111",
		LastErrorAt:          &failedAt,
		ConsecutiveSuccesses: 0,
		TotalRequests:        1,
		SuccessRate:          0,
	}))

	require.NoError(t, yh.qm.SaveUserAccess(ctx, &quota.UserAccess{
		OwnerID:      "streak-user",
		PlanID:       "basic",
		MonthlyLimit: 1_000_000,
		Priority:     1,
		IsActive:     true,
	}, "yunwu"))

	payload := []byte(`{"model": "claude-3-7-sonnet", "messages": [{"role": "user", "content": "hi"}]}`)
	routed := ContextWithModelRouting(ctx, []int64{111, 112})
	_, err := yh.HandleRequest(routed, "streak-user", "conv-streak", payload, io.Discard, "yunwu")
	require.NoError(t, err)
	assert.Equal(t, "112", rt.lastNodeID,
		"a recovering Primary must be deferred to the Backup even on an explicit route")
}

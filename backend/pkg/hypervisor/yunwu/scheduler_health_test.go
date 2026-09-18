package yunwu

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthControllerReconcileHealthStates(t *testing.T) {
	store := setupTestYunwuStore(t)
	ctx := context.Background()
	nm := health.NewNodeManager(store)
	hc := NewYunwuHealthController(store)

	// 1. Cooldown expiry and threshold transitions
	now := time.Now()
	past := now.Add(-5 * time.Minute)
	future := now.Add(5 * time.Minute)

	nodes := []health.ExecutionNode{
		{NodeID: "n1", State: "healthy"},
		{NodeID: "n2", State: "healthy"},
		{NodeID: "n3", State: "healthy"},
		{NodeID: "n4", State: "cooldown"},
	}
	for _, n := range nodes {
		_ = nm.SaveNode(ctx, &n)
	}

	// n4 has expired cooldown -> should be reset to healthy
	_ = nm.SaveNodeRuntimeState(ctx, &health.NodeRuntimeState{
		NodeID:        "n4",
		CooldownUntil: &past,
		Recent429:     5,
	})

	sysState := hc.ReconcileHealth(ctx, nodes)
	assert.Equal(t, "NORMAL", sysState)
	assert.Equal(t, "NORMAL", hc.GetSystemState(ctx))

	n4, err := nm.GetNode(ctx, "n4")
	require.NoError(t, err)
	assert.Equal(t, "healthy", n4.State)

	// 2. Active cooldown and limited thresholds
	_ = nm.SaveNodeRuntimeState(ctx, &health.NodeRuntimeState{
		NodeID:        "n1",
		CooldownUntil: &future,
	})
	_ = nm.SaveNodeRuntimeState(ctx, &health.NodeRuntimeState{
		NodeID:    "n2",
		Recent429: 4,
	})
	_ = nm.SaveNodeRuntimeState(ctx, &health.NodeRuntimeState{
		NodeID:         "n3",
		RecentTimeouts: 6,
	})

	sysState = hc.ReconcileHealth(ctx, nodes)
	assert.Equal(t, "DEGRADED", sysState)

	// 3. Survival state with >= 5 nodes and < 25% healthy
	survivalNodes := []health.ExecutionNode{
		{NodeID: "s1", State: "unavailable"},
		{NodeID: "s2", State: "unavailable"},
		{NodeID: "s3", State: "unavailable"},
		{NodeID: "s4", State: "unavailable"},
		{NodeID: "s5", State: "healthy"},
	}
	for _, n := range survivalNodes {
		_ = nm.SaveNode(ctx, &n)
	}
	t.Setenv("ASH_DISABLE_SURVIVAL_MODE", "false")
	sysState = hc.ReconcileHealth(ctx, survivalNodes)
	assert.Equal(t, "SURVIVAL", sysState)

	// With ASH_DISABLE_SURVIVAL_MODE=true -> DEGRADED
	t.Setenv("ASH_DISABLE_SURVIVAL_MODE", "true")
	sysState = hc.ReconcileHealth(ctx, survivalNodes)
	assert.Equal(t, "DEGRADED", sysState)

	// 4. Start controller loop briefly
	loopCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hc.Start(loopCtx, 10*time.Millisecond, func(ctx context.Context) []health.ExecutionNode {
		return nodes
	})
}

func TestSchedulerSelectNodeScoringAndPenalties(t *testing.T) {
	ctx := context.Background()
	sch := NewYunwuScheduler(true)

	// 1. Empty candidates -> ErrNoYunwuNodesAvailable
	_, err := sch.SelectNode(ctx, nil, nil, nil, 1, "", "NORMAL")
	assert.ErrorIs(t, err, ErrNoYunwuNodesAvailable)

	// All unavailable
	unavailNodes := []health.ExecutionNode{
		{NodeID: "u1", State: "unavailable"},
	}
	_, err = sch.SelectNode(ctx, unavailNodes, nil, nil, 1, "", "NORMAL")
	assert.ErrorIs(t, err, ErrNoYunwuNodesAvailable)

	// 2. Cooldown node excluded if len > 1, but picked if only 1 node
	future := time.Now().Add(5 * time.Minute)
	cooldownNodes := []health.ExecutionNode{
		{NodeID: "cd1", State: "cooldown"},
		{NodeID: "h1", State: "healthy"},
	}
	states := map[string]*health.NodeRuntimeState{
		"cd1": {NodeID: "cd1", CooldownUntil: &future, SuccessRate: 1.0},
		"h1":  {NodeID: "h1", SuccessRate: 1.0},
	}
	credits := map[string]float64{
		"cd1": 100.0,
		"h1":  100.0,
	}
	selected, err := sch.SelectNode(ctx, cooldownNodes, states, credits, 1, "", "NORMAL")
	require.NoError(t, err)
	assert.Equal(t, "h1", selected.NodeID)

	// Single node in cooldown
	singleCooldown := []health.ExecutionNode{{NodeID: "cd1", State: "cooldown"}}
	selected, err = sch.SelectNode(ctx, singleCooldown, states, credits, 1, "", "NORMAL")
	require.NoError(t, err)
	assert.Equal(t, "cd1", selected.NodeID)

	// 3. Limited candidates fallback when healthy not present
	limitedNodes := []health.ExecutionNode{
		{NodeID: "lim1", State: "limited"},
	}
	statesLimited := map[string]*health.NodeRuntimeState{
		"lim1": {NodeID: "lim1", SuccessRate: 0.9, QueueDepth: 2, AvgLatencyMs: 300, Recent429: 1, RecentTimeouts: 1},
	}
	selected, err = sch.SelectNode(ctx, limitedNodes, statesLimited, map[string]float64{"lim1": 50.0}, 2, "lim1", "DEGRADED")
	require.NoError(t, err)
	assert.Equal(t, "lim1", selected.NodeID)

	// 4. Survival system state penalty
	selected, err = sch.SelectNode(ctx, limitedNodes, statesLimited, map[string]float64{"lim1": 50.0}, 2, "lim1", "SURVIVAL")
	require.NoError(t, err)
	assert.Equal(t, "lim1", selected.NodeID)

	// 5. Fresh balance fallback when all candidates excluded due to balance
	exhaustedNodes := []health.ExecutionNode{
		{NodeID: "ex1", State: "healthy"},
	}
	selected, err = sch.SelectNode(ctx, exhaustedNodes, nil, map[string]float64{"ex1": 0.0}, 1, "", "NORMAL")
	require.NoError(t, err)
	assert.Equal(t, "ex1", selected.NodeID)

	// Scheduler with requireFreshBalance=false
	schNoFresh := NewYunwuScheduler(false)
	selected, err = schNoFresh.SelectNode(ctx, exhaustedNodes, nil, map[string]float64{"ex1": 0.0}, 1, "", "NORMAL")
	require.NoError(t, err)
	assert.Equal(t, "ex1", selected.NodeID)
}

func TestRebindingManagerFailureHandling(t *testing.T) {
	store := setupTestYunwuStore(t)
	rm := NewYunwuRebindingManager(store)
	ctx := context.Background()

	// 1. Non-retryable failure (context canceled)
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	rebound, err := rm.HandleYunwuFailure(canceledCtx, "sess-1", "node-1", context.Canceled, []health.ExecutionNode{{NodeID: "node-2"}}, 1, map[string]float64{"node-2": 100.0}, "NORMAL")
	assert.Nil(t, rebound)
	assert.Error(t, err)

	// 2. Successful rebinding with 429 error
	nodeFail := health.ExecutionNode{NodeID: "node-fail", State: "healthy"}
	nodeSucc := health.ExecutionNode{NodeID: "node-succ", State: "healthy"}
	_ = rm.nm.SaveNode(ctx, &nodeFail)
	_ = rm.nm.SaveNode(ctx, &nodeSucc)
	nodes := []health.ExecutionNode{nodeFail, nodeSucc}

	rebound, err = rm.HandleYunwuFailure(ctx, "sess-rebind-1", "node-fail", errors.New("upstream returned 429 too many requests"), nodes, 1, map[string]float64{"node-succ": 50.0}, "NORMAL")
	require.NoError(t, err)
	require.NotNil(t, rebound)
	assert.Equal(t, "node-succ", rebound.NodeID)

	// Verify failed node state became cooldown
	failedState, _ := rm.nm.GetNode(ctx, "node-fail")
	assert.Equal(t, "cooldown", failedState.State)

	// 3. Exhausted rebinding candidates
	_, err = rm.HandleYunwuFailure(ctx, "sess-rebind-exhausted", "node-succ", errors.New("timeout"), []health.ExecutionNode{nodeSucc}, 1, nil, "NORMAL")
	assert.ErrorIs(t, err, ErrYunwuRebindingExhausted)

	// 4. 24h rebinding counter
	count, err := rm.GetRebinding24hCount(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, int64(1))
}

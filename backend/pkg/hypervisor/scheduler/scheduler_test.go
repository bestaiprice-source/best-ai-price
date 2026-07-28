package scheduler_test

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/scheduler"
	"github.com/stretchr/testify/require"
)

func TestSchedulerSelectNode(t *testing.T) {
	s := scheduler.NewScheduler()
	ctx := context.Background()

	nodes := []health.ExecutionNode{
		{NodeID: "1", AccountName: "Node 1", State: "healthy"},
		{NodeID: "2", AccountName: "Node 2", State: "healthy"},
		{NodeID: "3", AccountName: "Node 3", State: "unavailable"},
	}

	states := map[string]health.NodeRuntimeState{
		"1": {
			NodeID:      "1",
			SuccessRate: 0.99,
			AvgLatencyMs: 200,
		},
		"2": {
			NodeID:      "2",
			SuccessRate: 0.80,
			Recent429:   2,
			AvgLatencyMs: 800,
		},
	}

	selected, err := s.SelectNode(ctx, nodes, states, 1, "", "NORMAL")
	require.NoError(t, err)
	require.NotNil(t, selected)
	require.Equal(t, "1", selected.NodeID)
}

func TestSchedulerSingleNodeCooldownFallback(t *testing.T) {
	s := scheduler.NewScheduler()
	ctx := context.Background()

	cooldownTime := time.Now().Add(5 * time.Minute)
	nodes := []health.ExecutionNode{
		{NodeID: "1", AccountName: "Only Node", State: "cooldown"},
	}

	states := map[string]health.NodeRuntimeState{
		"1": {
			NodeID:        "1",
			CooldownUntil: &cooldownTime,
		},
	}

	selected, err := s.SelectNode(ctx, nodes, states, 1, "", "NORMAL")
	require.NoError(t, err)
	require.NotNil(t, selected)
	require.Equal(t, "1", selected.NodeID)
}

func TestSchedulerNoNodesAvailable(t *testing.T) {
	s := scheduler.NewScheduler()
	ctx := context.Background()

	// Empty list
	_, err := s.SelectNode(ctx, nil, nil, 1, "", "NORMAL")
	require.ErrorIs(t, err, scheduler.ErrNoNodesAvailable)

	// Only unavailable nodes
	nodes := []health.ExecutionNode{
		{NodeID: "1", State: "unavailable"},
	}
	_, err = s.SelectNode(ctx, nodes, nil, 1, "", "NORMAL")
	require.ErrorIs(t, err, scheduler.ErrNoNodesAvailable)
}

func TestSchedulerLimitedCandidatesFallback(t *testing.T) {
	s := scheduler.NewScheduler()
	ctx := context.Background()

	nodes := []health.ExecutionNode{
		{NodeID: "1", AccountName: "Node 1", State: "limited"},
		{NodeID: "2", AccountName: "Node 2", State: "limited"},
	}

	selected, err := s.SelectNode(ctx, nodes, nil, 1, "", "NORMAL")
	require.NoError(t, err)
	require.NotNil(t, selected)
}

func TestSchedulerAffinityAndRecovery(t *testing.T) {
	s := scheduler.NewScheduler()
	ctx := context.Background()

	lastErr := time.Now().Add(-1 * time.Minute)
	nodes := []health.ExecutionNode{
		{NodeID: "1", AccountName: "Node 1", State: "healthy"},
		{NodeID: "2", AccountName: "Node 2", State: "healthy"},
	}

	states := map[string]health.NodeRuntimeState{
		"1": {NodeID: "1", SuccessRate: 0.99, LastErrorAt: &lastErr},
		"2": {NodeID: "2", SuccessRate: 0.99},
	}

	// Affinity for node 1
	selected, err := s.SelectNode(ctx, nodes, states, 2, "1", "NORMAL")
	require.NoError(t, err)
	require.Equal(t, "1", selected.NodeID)

	// RECOVERY state penalizes node 1 recent error
	selectedRec, err := s.SelectNode(ctx, nodes, states, 1, "", "RECOVERY")
	require.NoError(t, err)
	require.Equal(t, "2", selectedRec.NodeID)
}


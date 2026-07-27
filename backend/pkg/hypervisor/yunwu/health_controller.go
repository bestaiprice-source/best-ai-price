package yunwu

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
)

type YunwuHealthController struct {
	store *storage.Storage
	nm    *health.NodeManager
}

func NewYunwuHealthController(store *storage.Storage) *YunwuHealthController {
	return &YunwuHealthController{
		store: store,
		nm:    health.NewNodeManager(store),
	}
}

// ReconcileHealth evaluates Yunwu node states and computes Yunwu pool degradation state
func (hc *YunwuHealthController) ReconcileHealth(ctx context.Context, nodes []health.ExecutionNode) string {
	now := time.Now()
	totalCount := len(nodes)
	healthyCount := 0

	for _, node := range nodes {
		state, err := hc.nm.GetNodeRuntimeState(ctx, node.NodeID)
		if err != nil || state == nil {
			state = &health.NodeRuntimeState{NodeID: node.NodeID}
		}

		// Cooldown check
		if state.CooldownUntil != nil && now.After(*state.CooldownUntil) {
			slog.Info("[YunwuHealthController] Yunwu node cooldown expired, resetting to healthy", "node_id", node.NodeID)
			state.CooldownUntil = nil
			state.Recent429 = 0
			state.RecentTimeouts = 0
			node.State = "healthy"
			_ = hc.nm.SaveNode(ctx, &node)
			_ = hc.nm.SaveNodeRuntimeState(ctx, state)
		}

		// Determine current node state
		if state.CooldownUntil != nil && now.Before(*state.CooldownUntil) {
			node.State = "cooldown"
		} else if state.Recent429 > 3 || state.RecentTimeouts > 5 {
			node.State = "limited"
		} else if node.State == "" {
			node.State = "healthy"
		}

		_ = hc.nm.SaveNode(ctx, &node)

		if node.State == "healthy" {
			healthyCount++
		}
	}

	// Compute Yunwu System State
	systemState := "NORMAL"
	if totalCount > 0 {
		healthyRatio := float64(healthyCount) / float64(totalCount)
		if healthyRatio >= 0.75 {
			systemState = "NORMAL"
		} else if healthyRatio >= 0.50 {
			systemState = "WARNING"
		} else if healthyRatio >= 0.25 {
			systemState = "DEGRADED"
		} else {
			if totalCount >= 5 && os.Getenv("ASH_DISABLE_SURVIVAL_MODE") != "true" {
				systemState = "SURVIVAL"
			} else {
				systemState = "DEGRADED"
			}
		}
	}

	slog.Info("[YunwuHealthController] Reconciled Yunwu pool health",
		"total_nodes", totalCount,
		"healthy_nodes", healthyCount,
		"system_state", systemState,
	)

	_ = hc.store.Redis().Set(ctx, "ash:yunwu:system:state", systemState, 0).Err()
	return systemState
}

// GetSystemState returns the cached Yunwu system state
func (hc *YunwuHealthController) GetSystemState(ctx context.Context) string {
	val, err := hc.store.Redis().Get(ctx, "ash:yunwu:system:state").Result()
	if err != nil || val == "" {
		return "NORMAL"
	}
	return val
}

// Start runs background reconciliation every interval
func (hc *YunwuHealthController) Start(ctx context.Context, interval time.Duration, getNodes func(ctx context.Context) []health.ExecutionNode) {
	ticker := time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				nodes := getNodes(ctx)
				hc.ReconcileHealth(ctx, nodes)
			}
		}
	}()
}

package health

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/ent/account"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
)

type HealthController struct {
	store *storage.Storage
	nm    *NodeManager
	stop  chan struct{}
}

func NewHealthController(store *storage.Storage) *HealthController {
	return &HealthController{
		store: store,
		nm:    NewNodeManager(store),
		stop:  make(chan struct{}),
	}
}

func (hc *HealthController) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-ticker.C:
				_ = hc.Reconcile(ctx)
			case <-hc.stop:
				ticker.Stop()
				return
			case <-ctx.Done():
				ticker.Stop()
				return
			}
		}
	}()
}

func (hc *HealthController) Stop() {
	close(hc.stop)
}

func (hc *HealthController) Reconcile(ctx context.Context) error {
	var nodeIDs []string

	// Query PostgreSQL active accounts first
	dbAccounts, dbErr := hc.store.DB().Account.Query().
		Where(
			account.Platform("anthropic"),
			account.Status("active"),
		).
		All(ctx)

	if dbErr == nil && len(dbAccounts) > 0 {
		for _, acc := range dbAccounts {
			nodeIDs = append(nodeIDs, fmt.Sprintf("%d", acc.ID))
		}
	} else {
		// Fallback to Redis set for tests
		nodeIDs, _ = hc.store.Redis().SMembers(ctx, "ash:nodes").Result()
	}

	var healthyCount int
	var totalCount = len(nodeIDs)

	for _, nodeID := range nodeIDs {
		node, err := hc.nm.GetNode(ctx, nodeID)
		if err != nil || node == nil {
			node = &ExecutionNode{
				NodeID:       nodeID,
				State:        "healthy",
				Capabilities: []string{"claude-3-5-sonnet"},
				UpdatedAt:    time.Now(),
			}
		}

		state, err := hc.nm.GetNodeRuntimeState(ctx, nodeID)
		if err != nil || state == nil {
			state = &NodeRuntimeState{NodeID: nodeID}
		}

		now := time.Now()

		// Cooldown check
		if state.CooldownUntil != nil && now.After(*state.CooldownUntil) {
			// Cooldown expired, transition back to healthy
			state.CooldownUntil = nil
			node.State = "healthy"
			state.Recent429 = 0
			state.RecentTimeouts = 0
			_ = hc.nm.SaveNode(ctx, node)
			_ = hc.nm.SaveNodeRuntimeState(ctx, state)
		}

		// Determine current state
		if state.CooldownUntil != nil && now.Before(*state.CooldownUntil) {
			node.State = "cooldown"
		} else if state.Recent429 > 3 || state.RecentTimeouts > 5 {
			node.State = "limited"
		} else if node.State == "" {
			node.State = "healthy"
		}

		_ = hc.nm.SaveNode(ctx, node)

		if node.State == "healthy" {
			healthyCount++
		}
	}

	// 2. Compute System Degradation State
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
			systemState = "SURVIVAL"
		}
	}

	// Save system state in Redis
	err := hc.store.Redis().Set(ctx, "ash:system:state", systemState, 0).Err()
	return err
}

func (hc *HealthController) GetSystemState(ctx context.Context) string {
	state, err := hc.store.Redis().Get(ctx, "ash:system:state").Result()
	if err != nil || state == "" {
		return "NORMAL"
	}
	return state
}

package health

import (
	"context"
	"fmt"
	"log/slog"
	"os"
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

func (hc *HealthController) NodeManager() *NodeManager {
	return hc.nm
}

func (hc *HealthController) Start(ctx context.Context, interval time.Duration) {
	slog.Info("[HealthController] Starting background health reconciliation loop", "interval", interval)
	ticker := time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-ticker.C:
				_ = hc.Reconcile(ctx)
			case <-hc.stop:
				ticker.Stop()
				slog.Info("[HealthController] Stopped health reconciliation loop")
				return
			case <-ctx.Done():
				ticker.Stop()
				slog.Info("[HealthController] Stopped health reconciliation loop via context done")
				return
			}
		}
	}()
}

func (hc *HealthController) Stop() {
	close(hc.stop)
}

func (hc *HealthController) Reconcile(ctx context.Context) error {
	platforms := []string{"anthropic", "openai"}

	for _, plat := range platforms {
		var nodeIDs []string

		// Query PostgreSQL active accounts first
		dbAccounts, dbErr := hc.store.DB().Account.Query().
			Where(
				account.Platform(plat),
				account.StatusIn("active", "error"),
			).
			All(ctx)

		if dbErr == nil && len(dbAccounts) > 0 {
			for _, acc := range dbAccounts {
				nodeIDs = append(nodeIDs, fmt.Sprintf("%d", acc.ID))
			}
		} else {
			// Fallback to Redis set
			allNodes, _ := hc.store.Redis().SMembers(ctx, "ash:nodes").Result()
			for _, nodeID := range allNodes {
				node, err := hc.nm.GetNode(ctx, nodeID)
				if err == nil && node != nil && (node.Provider == plat || node.Provider == "") {
					nodeIDs = append(nodeIDs, nodeID)
				}
			}
		}

		var healthyCount int
		var totalCount = len(nodeIDs)

		for _, nodeID := range nodeIDs {
			node, err := hc.nm.GetNode(ctx, nodeID)
			if err != nil || node == nil {
				node = &ExecutionNode{
					NodeID:       nodeID,
					State:        "healthy",
					Capabilities: []string{plat},
					Provider:     plat,
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
				slog.Info("[HealthController] Node cooldown expired, resetting to healthy", "node_id", nodeID)
				state.CooldownUntil = nil
				node.State = "healthy"
				state.Recent429 = 0
				state.RecentTimeouts = 0
				state.ConsecutiveSuccesses = 0
				_ = hc.nm.SaveNode(ctx, node)
				_ = hc.nm.SaveNodeRuntimeState(ctx, state)
			}

			// Determine current state
			if state.CooldownUntil != nil && now.Before(*state.CooldownUntil) {
				node.State = "cooldown"
			} else if state.Recent429 > 3 || state.RecentTimeouts > 5 {
				if node.State != "limited" {
					slog.Warn("[HealthController] Node marked as limited due to recent errors",
						"node_id", nodeID,
						"recent_429", state.Recent429,
						"recent_timeouts", state.RecentTimeouts,
					)
					_, _ = hc.store.LogEvent(ctx, "system", nil, &nodeID, "NodeLimited", map[string]any{
						"recent_429":      state.Recent429,
						"recent_timeouts": state.RecentTimeouts,
					})
				}
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
				if totalCount >= 5 && os.Getenv("ASH_DISABLE_SURVIVAL_MODE") != "true" {
					systemState = "SURVIVAL"
				} else {
					systemState = "DEGRADED"
				}
			}
		}

		// Check for RECOVERY transition from SURVIVAL/DEGRADED to NORMAL/WARNING
		prevKey := "ash:system:prev_state:" + plat
		prevState, _ := hc.store.Redis().Get(ctx, prevKey).Result()
		recoveryUntilKey := "ash:system:recovery_until:" + plat
		recVal, _ := hc.store.Redis().Get(ctx, recoveryUntilKey).Result()

		if (prevState == "SURVIVAL" || prevState == "DEGRADED") && (systemState == "NORMAL" || systemState == "WARNING") {
			recUntil := time.Now().Add(5 * time.Minute)
			_ = hc.store.Redis().Set(ctx, recoveryUntilKey, recUntil.Format(time.RFC3339), 5*time.Minute).Err()
			systemState = "RECOVERY"
		} else if recVal != "" {
			if recTime, parseErr := time.Parse(time.RFC3339, recVal); parseErr == nil && time.Now().Before(recTime) {
				if systemState == "NORMAL" || systemState == "WARNING" {
					systemState = "RECOVERY"
				}
			}
		}

		_ = hc.store.Redis().Set(ctx, prevKey, systemState, 0).Err()

		slog.Debug("[HealthController] Reconciled platform pool health",
			"platform", plat,
			"total_nodes", totalCount,
			"healthy_nodes", healthyCount,
			"system_state", systemState,
		)

		_ = hc.store.Redis().Set(ctx, "ash:system:state:"+plat, systemState, 0).Err()
	}

	// Also update global state (worst of the two)
	gState := "NORMAL"
	anthropicState, _ := hc.store.Redis().Get(ctx, "ash:system:state:anthropic").Result()
	openaiState, _ := hc.store.Redis().Get(ctx, "ash:system:state:openai").Result()

	statesPriority := map[string]int{"SURVIVAL": 5, "DEGRADED": 4, "WARNING": 3, "RECOVERY": 2, "NORMAL": 1}
	if statesPriority[anthropicState] > statesPriority[gState] {
		gState = anthropicState
	}
	if statesPriority[openaiState] > statesPriority[gState] {
		gState = openaiState
	}
	_ = hc.store.Redis().Set(ctx, "ash:system:state", gState, 0).Err()

	return nil
}

func (hc *HealthController) GetSystemState(ctx context.Context, platform string) string {
	if platform == "" {
		platform = "anthropic"
	}
	state, err := hc.store.Redis().Get(ctx, "ash:system:state:"+platform).Result()
	if err != nil || state == "" {
		// Fallback to global state
		gState, err := hc.store.Redis().Get(ctx, "ash:system:state").Result()
		if err != nil || gState == "" {
			return "NORMAL"
		}
		return gState
	}
	return state
}

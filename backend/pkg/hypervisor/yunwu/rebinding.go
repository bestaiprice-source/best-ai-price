package yunwu

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
	"github.com/redis/go-redis/v9"
)

var (
	ErrYunwuRebindingExhausted = errors.New("all Yunwu channel nodes exhausted during rebinding")
)

type YunwuRebindingManager struct {
	store *storage.Storage
	nm    *health.NodeManager
}

func NewYunwuRebindingManager(store *storage.Storage) *YunwuRebindingManager {
	return &YunwuRebindingManager{
		store: store,
		nm:    health.NewNodeManager(store),
	}
}

// HandleYunwuFailure sets 10m cooldown on failing node and returns alternative Yunwu candidate
func (rm *YunwuRebindingManager) HandleYunwuFailure(
	ctx context.Context,
	failedNodeID string,
	errReason error,
	allNodes []health.ExecutionNode,
) (*health.ExecutionNode, error) {
	slog.Warn("[YunwuRebinding] Handling Yunwu channel failure",
		"failed_node_id", failedNodeID,
		"error", errReason,
	)

	// 1. Put failed Yunwu node into 10m cooldown
	cooldownUntil := time.Now().Add(10 * time.Minute)
	state, err := rm.nm.GetNodeRuntimeState(ctx, failedNodeID)
	if err != nil || state == nil {
		state = &health.NodeRuntimeState{
			NodeID: failedNodeID,
		}
	}

	state.Recent429++
	state.CooldownUntil = &cooldownUntil
	_ = rm.nm.SaveNodeRuntimeState(ctx, state)

	// Mark node state as cooldown
	failedNode, _ := rm.nm.GetNode(ctx, failedNodeID)
	if failedNode != nil {
		failedNode.State = "cooldown"
		_ = rm.nm.SaveNode(ctx, failedNode)
	}

	// Track 24h rebinding counter in Redis ash:yunwu:rebinding:24h
	now := time.Now().Unix()
	_ = rm.store.Redis().ZAdd(ctx, "ash:yunwu:rebinding:24h", redis.Z{
		Score:  float64(now),
		Member: fmt.Sprintf("rebind-%s-%d", failedNodeID, now),
	}).Err()
	// Cleanup entries older than 24h
	_ = rm.store.Redis().ZRemRangeByScore(ctx, "ash:yunwu:rebinding:24h", "-inf", fmt.Sprintf("%d", now-86400)).Err()

	// 2. Select alternative Yunwu node
	var candidates []health.ExecutionNode
	for _, node := range allNodes {
		if node.NodeID == failedNodeID {
			continue
		}
		if node.State == "unavailable" || node.State == "cooldown" {
			continue
		}
		nodeState, err := rm.nm.GetNodeRuntimeState(ctx, node.NodeID)
		if err == nil && nodeState != nil && nodeState.CooldownUntil != nil && time.Now().Before(*nodeState.CooldownUntil) {
			continue
		}
		candidates = append(candidates, node)
	}

	if len(candidates) == 0 {
		slog.Error("[YunwuRebinding] No alternative Yunwu nodes available for rebinding")
		return nil, ErrYunwuRebindingExhausted
	}

	selected := &candidates[0]
	slog.Info("[YunwuRebinding] Successfully rebound session to alternative Yunwu channel",
		"failed_node_id", failedNodeID,
		"rebound_node_id", selected.NodeID,
		"account_name", selected.AccountName,
	)

	return selected, nil
}

func (rm *YunwuRebindingManager) GetRebinding24hCount(ctx context.Context) (int64, error) {
	now := time.Now().Unix()
	_ = rm.store.Redis().ZRemRangeByScore(ctx, "ash:yunwu:rebinding:24h", "-inf", fmt.Sprintf("%d", now-86400)).Err()
	return rm.store.Redis().ZCard(ctx, "ash:yunwu:rebinding:24h").Result()
}

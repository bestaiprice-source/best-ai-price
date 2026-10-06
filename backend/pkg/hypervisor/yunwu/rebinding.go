package yunwu

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/config"
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
	cfg   *config.HypervisorConfig
}

func NewYunwuRebindingManager(store *storage.Storage, cfg ...*config.HypervisorConfig) *YunwuRebindingManager {
	var c *config.HypervisorConfig
	if len(cfg) > 0 && cfg[0] != nil {
		c = cfg[0]
	} else {
		c = config.LoadFromEnv()
	}
	return &YunwuRebindingManager{
		store: store,
		nm:    health.NewNodeManager(store),
		cfg:   c,
	}
}

// HandleYunwuFailure sets 10m cooldown on failing node and returns alternative Yunwu candidate via YunwuScheduler
func (rm *YunwuRebindingManager) HandleYunwuFailure(
	ctx context.Context,
	sessionID string,
	failedNodeID string,
	errReason error,
	allNodes []health.ExecutionNode,
	userPriority int,
	credits map[string]float64,
	systemState string,
	prices ...map[string]float64,
) (*health.ExecutionNode, error) {
	slog.Warn("[YunwuRebinding] Handling Yunwu channel failure",
		"session_id", sessionID,
		"failed_node_id", failedNodeID,
		"error", errReason,
		"user_priority", userPriority,
		"system_state", systemState,
	)

	// 1. Acquire Redis distributed lock specifically for this session to enforce one active rebinding per VirtualSession
	lockSessionID := sessionID
	if lockSessionID == "" {
		lockSessionID = fmt.Sprintf("node-%s", failedNodeID)
	}
	lockKey := fmt.Sprintf("lock:yunwu:rebind:session:%s", lockSessionID)
	locked, lockErr := rm.store.AcquireLock(ctx, lockKey, 60*time.Second)
	if lockErr != nil || !locked {
		slog.Warn("[YunwuRebinding] Failed to acquire lock for Yunwu session rebinding", "session_id", lockSessionID)
		return nil, fmt.Errorf("concurrent rebinding lock contention for session %s", lockSessionID)
	}
	defer func() {
		_ = rm.store.ReleaseLock(ctx, lockKey)
	}()

	// Put failed Yunwu node into 10m cooldown
	cooldownUntil := time.Now().Add(10 * time.Minute)
	state, err := rm.nm.GetNodeRuntimeState(ctx, failedNodeID)
	if err != nil || state == nil {
		state = &health.NodeRuntimeState{
			NodeID: failedNodeID,
		}
	}

	errorText := strings.ToLower(errReason.Error())
	if strings.Contains(errorText, "429") || strings.Contains(errorText, "too many requests") {
		state.Recent429++
	}
	if errors.Is(errReason, context.DeadlineExceeded) || strings.Contains(errorText, "timeout") || strings.Contains(errorText, "deadline exceeded") {
		state.RecentTimeouts++
	}
	state.ConsecutiveSuccesses = 0
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

	// 2. Select alternative Yunwu node via YunwuScheduler
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

	requireFresh := false
	if rm.cfg != nil {
		requireFresh = rm.cfg.YunwuRequireFreshBalance
	}
	sch := NewYunwuScheduler(requireFresh)
	nodeStates := make(map[string]*health.NodeRuntimeState)
	for _, node := range candidates {
		if st, err := rm.nm.GetNodeRuntimeState(ctx, node.NodeID); err == nil && st != nil {
			nodeStates[node.NodeID] = st
		}
	}
	if credits == nil {
		credits = make(map[string]float64)
	}
	if systemState == "" {
		systemState = "NORMAL"
	}

	selected, selectErr := sch.SelectNode(ctx, candidates, nodeStates, credits, userPriority, "", systemState, prices...)
	if selectErr != nil {
		slog.Error("[YunwuRebinding] Yunwu scheduler selection failed during rebinding", "error", selectErr)
		return nil, ErrYunwuRebindingExhausted
	}

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

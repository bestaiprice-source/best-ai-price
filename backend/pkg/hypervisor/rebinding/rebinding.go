package rebinding

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/config"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/lease"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/session"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
)

type RebindingManager struct {
	store *storage.Storage
	lm    *lease.LeaseManager
	sm    *session.SessionManager
	nm    *health.NodeManager
	cfg   *config.HypervisorConfig
}

func NewRebindingManager(store *storage.Storage, cfg *config.HypervisorConfig) *RebindingManager {
	return &RebindingManager{
		store: store,
		lm:    lease.NewLeaseManager(store),
		sm:    session.NewSessionManager(store),
		nm:    health.NewNodeManager(store),
		cfg:   cfg,
	}
}

func (rm *RebindingManager) RebindSession(
	ctx context.Context,
	sess *session.VirtualSession,
	failedNodeID string,
	nodes []health.ExecutionNode,
	states map[string]health.NodeRuntimeState,
	userPriority int,
	selectNodeFn func(ctx context.Context, nodes []health.ExecutionNode, states map[string]health.NodeRuntimeState, userPriority int, activeLease string, systemState string) (*health.ExecutionNode, error),
	systemState string,
	execErr error,
) (*health.ExecutionNode, error) {
	slog.Info("[Rebinding] Initiating session rebinding workflow",
		"session_id", sess.ID,
		"failed_node_id", failedNodeID,
		"exec_error", execErr,
	)

	// 1. Acquire Redis distributed lock
	lockTTL := time.Duration(rm.cfg.SessionLockTTLSeconds) * time.Second
	locked, err := rm.store.AcquireLock(ctx, sess.ID, lockTTL)
	if err != nil || !locked {
		slog.Warn("[Rebinding] Could not acquire distributed lock for session", "session_id", sess.ID)
		return nil, fmt.Errorf("failed to acquire session lock for rebinding")
	}
	defer func() {
		_ = rm.store.ReleaseLock(ctx, sess.ID)
		slog.Debug("[Rebinding] Released session lock", "session_id", sess.ID)
	}()

	// 2. Put failed node on cooldown (10 minutes) ONLY if caused by a real upstream Claude/OpenAI error (429/5xx)
	errMsg := ""
	if execErr != nil {
		errMsg = execErr.Error()
	}
	isUpstreamError := strings.Contains(errMsg, "upstream error status 429") ||
		strings.Contains(errMsg, "upstream error status 401") ||
		strings.Contains(errMsg, "upstream error status 403") ||
		strings.Contains(errMsg, "upstream error status 404") ||
		strings.Contains(errMsg, "upstream error status 5") ||
		strings.Contains(errMsg, "rate_limit_error") ||
		strings.Contains(errMsg, "permission_error") ||
		strings.Contains(errMsg, "overloaded_error") ||
		strings.Contains(errMsg, "model_not_found") ||
		strings.Contains(errMsg, "no available channel")

	if isUpstreamError {
		cooldownUntil := time.Now().Add(10 * time.Minute)
		currentState, exists := states[failedNodeID]
		if !exists {
			currentState = health.NodeRuntimeState{NodeID: failedNodeID}
		}
		currentState.CooldownUntil = &cooldownUntil
		currentState.Recent429++
		_ = rm.nm.SaveNodeRuntimeState(ctx, &currentState)
		states[failedNodeID] = currentState
		slog.Warn("[Rebinding] Placed failed node on cooldown",
			"node_id", failedNodeID,
			"cooldown_until", cooldownUntil,
			"recent_429", currentState.Recent429,
		)
	}

	// 3. Select backup node
	newNode, err := selectNodeFn(ctx, nodes, states, userPriority, "", systemState)
	if err != nil {
		slog.Error("[Rebinding] Backup node selection failed", "session_id", sess.ID, "error", err)
		return nil, fmt.Errorf("no backup nodes available: %w", err)
	}
	slog.Info("[Rebinding] Selected backup node for rebinding",
		"session_id", sess.ID,
		"new_node_id", newNode.NodeID,
	)

	// 4. Grant new lease
	expiresAt := time.Now().Add(30 * time.Minute)
	newLease := &lease.Lease{
		LeaseID:   fmt.Sprintf("lease-%d-retry", time.Now().UnixNano()),
		SessionID: sess.ID,
		NodeID:    newNode.NodeID,
		GrantedAt: time.Now(),
		ExpiresAt: &expiresAt,
	}
	if err := rm.lm.SaveLease(ctx, newLease); err != nil {
		slog.Error("[Rebinding] Failed to save new lease", "session_id", sess.ID, "error", err)
		return nil, err
	}

	sess.Lease = newLease
	sess.UpdatedAt = time.Now()
	sess.State = "active"
	if err := rm.sm.SaveSession(ctx, sess); err != nil {
		slog.Error("[Rebinding] Failed to update session after rebinding", "session_id", sess.ID, "error", err)
		return nil, err
	}

	return newNode, nil
}

package rebinding

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/lease"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/runtime"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/session"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
)

type RebindingManager struct {
	store *storage.Storage
	lm    *lease.LeaseManager
	sm    *session.SessionManager
	nm    *health.NodeManager
}

func NewRebindingManager(store *storage.Storage) *RebindingManager {
	return &RebindingManager{
		store: store,
		lm:    lease.NewLeaseManager(store),
		sm:    session.NewSessionManager(store),
		nm:    health.NewNodeManager(store),
	}
}

func (rm *RebindingManager) ExecuteRebinding(
	ctx context.Context,
	sess *session.VirtualSession,
	failedNodeID string,
	reqPayload []byte,
	stream io.Writer,
	rt runtime.Runtime,
	nodes []health.ExecutionNode,
	states map[string]health.NodeRuntimeState,
	userPriority int,
	selectNodeFn func(ctx context.Context, nodes []health.ExecutionNode, states map[string]health.NodeRuntimeState, userPriority int, activeLease string, systemState string) (*health.ExecutionNode, error),
	systemState string,
	execErr error,
) (*runtime.ResponseMetadata, *health.ExecutionNode, error) {
	// 1. Acquire Redis distributed lock
	locked, err := rm.store.AcquireLock(ctx, sess.ID, 30*time.Second)
	if err != nil || !locked {
		return nil, nil, fmt.Errorf("failed to acquire session lock for rebinding")
	}
	defer func() {
		_ = rm.store.ReleaseLock(ctx, sess.ID)
	}()

	// 2. Put failed node on cooldown (10 minutes) ONLY if caused by a real upstream Claude/OpenAI error (429/5xx)
	errMsg := ""
	if execErr != nil {
		errMsg = execErr.Error()
	}
	isUpstreamError := strings.Contains(errMsg, "upstream error status 429") ||
		strings.Contains(errMsg, "upstream error status 401") ||
		strings.Contains(errMsg, "upstream error status 403") ||
		strings.Contains(errMsg, "upstream error status 5") ||
		strings.Contains(errMsg, "rate_limit_error") ||
		strings.Contains(errMsg, "permission_error") ||
		strings.Contains(errMsg, "overloaded_error")

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
	}

	// 3. Select backup node
	newNode, err := selectNodeFn(ctx, nodes, states, userPriority, "", systemState)
	if err != nil {
		return nil, nil, fmt.Errorf("no backup nodes available: %w", err)
	}

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
		return nil, nil, err
	}

	sess.Lease = newLease
	sess.UpdatedAt = time.Now()
	sess.State = "active"
	if err := rm.sm.SaveSession(ctx, sess); err != nil {
		return nil, nil, err
	}

	// 5. Retry request once
	res, execErr := rt.Execute(ctx, newNode.NodeID, reqPayload, stream)
	if execErr != nil {
		return nil, newNode, execErr
	}

	return res, newNode, nil
}

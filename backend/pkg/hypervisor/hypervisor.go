package hypervisor

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/Wei-Shaw/sub2api/ent/account"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/compiler"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/config"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/events"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/lease"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/quota"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/rebinding"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/runtime"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/scheduler"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/session"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/yunwu"
)

type Hypervisor struct {
	store *storage.Storage
	sm    *session.SessionManager
	lm    *lease.LeaseManager
	nm    *health.NodeManager
	ew    *events.EventWriter
	qm    *quota.QuotaManager
	sch   *scheduler.Scheduler
	rt    runtime.Runtime
	rm    *rebinding.RebindingManager
	comp  *compiler.Compiler
	hc    *health.HealthController
	yh    *yunwu.YunwuHypervisor
}

func NewHypervisor(store *storage.Storage, rt runtime.Runtime) *Hypervisor {
	h := &Hypervisor{
		store: store,
		sm:    session.NewSessionManager(store),
		lm:    lease.NewLeaseManager(store),
		nm:    health.NewNodeManager(store),
		ew:    events.NewEventWriter(store),
		qm:    quota.NewQuotaManager(store),
		sch:   scheduler.NewScheduler(),
		rt:    rt,
		rm:    rebinding.NewRebindingManager(store),
		comp:  compiler.NewCompiler(),
		hc:    health.NewHealthController(store),
		yh:    yunwu.NewYunwuHypervisor(store, rt),
	}

	cfg := config.LoadFromEnv()
	if cfg.EnableHealthController {
		h.hc.Start(context.Background(), 10*time.Second)
	}

	slog.Info("[Hypervisor] Initialized engine",
		"health_controller_enabled", cfg.EnableHealthController,
		"rebinding_enabled", cfg.EnableRebinding,
		"quota_manager_enabled", cfg.EnableQuotaManager,
		"yunwu_engine_initialized", h.yh != nil,
	)

	return h
}

func (h *Hypervisor) YunwuEngine() *yunwu.YunwuHypervisor {
	return h.yh
}

func (h *Hypervisor) HandleRequest(ctx context.Context, ownerID string, conversationID string, reqPayload []byte, stream io.Writer, platform string) (*runtime.ResponseMetadata, error) {
	slog.Info("[Hypervisor] Handling request",
		"owner_id", ownerID,
		"conversation_id", conversationID,
		"platform", platform,
		"payload_size", len(reqPayload),
	)

	if platform == "yunwu" && h.yh != nil {
		slog.Info("[Hypervisor] Dispatching request to dedicated Yunwu Hypervisor Engine",
			"owner_id", ownerID,
			"conversation_id", conversationID,
			"platform", platform,
		)
		return h.yh.HandleRequest(ctx, ownerID, conversationID, reqPayload, stream, platform)
	}

	// 1. Check Quota
	allowed, err := h.qm.CheckQuota(ctx, ownerID, "standard")
	if err != nil {
		slog.Error("[Hypervisor] Quota check failed with error", "owner_id", ownerID, "error", err)
		return nil, err
	}
	if !allowed {
		slog.Warn("[Hypervisor] User quota exceeded", "owner_id", ownerID)
		_ = h.logEvent(ctx, ownerID, nil, nil, "QuotaExceeded", map[string]any{"reason": "quota limit reached"})
		return nil, quota.ErrQuotaExceeded
	}

	// 2. Resolve or create Session
	if conversationID == "" {
		conversationID = "default"
	}
	sessID := fmt.Sprintf("session-%s-%s", ownerID, conversationID)

	sess, err := h.sm.GetSession(ctx, sessID)
	if err != nil || sess == nil {
		sess = &session.VirtualSession{
			ID:             sessID,
			OwnerID:        ownerID,
			ConversationID: conversationID,
			State:          "new",
			Version:        1,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		slog.Info("[Hypervisor] Created new virtual session", "session_id", sessID, "owner_id", ownerID)
	} else {
		slog.Debug("[Hypervisor] Resolved existing virtual session", "session_id", sessID, "version", sess.Version)
	}

	// 3. Resolve active lease if any
	var activeLeaseNodeID string
	currentLease, err := h.lm.GetLease(ctx, sess.ID)
	if err == nil && currentLease != nil {
		activeLeaseNodeID = currentLease.NodeID
		slog.Debug("[Hypervisor] Resolved active lease", "session_id", sess.ID, "lease_node_id", activeLeaseNodeID)
	}

	// 4. Load pool nodes and states
	nodes, err := h.getAllNodes(ctx, platform)
	if err != nil {
		slog.Error("[Hypervisor] Failed to load execution nodes", "platform", platform, "error", err)
		return nil, err
	}
	states, err := h.getAllNodeRuntimeStates(ctx, nodes)
	if err != nil {
		slog.Error("[Hypervisor] Failed to load node runtime states", "error", err)
		return nil, err
	}

	// Resolve user priority
	userPriority := 1
	if ua, err := h.qm.GetUserAccess(ctx, ownerID, "standard"); err == nil && ua != nil {
		userPriority = ua.Priority
	}

	// Fetch system state from Redis
	systemState := h.hc.GetSystemState(ctx, platform)
	slog.Info("[Hypervisor] Loaded pool status",
		"total_nodes", len(nodes),
		"user_priority", userPriority,
		"system_state", systemState,
	)

	// Enforce degradation policies
	disableSurvival := os.Getenv("ASH_DISABLE_SURVIVAL_MODE") == "true"
	if systemState == "SURVIVAL" && !disableSurvival {
		if userPriority < 2 {
			slog.Warn("[Hypervisor] Request rejected due to SURVIVAL mode", "owner_id", ownerID, "priority", userPriority)
			_ = h.logEvent(ctx, ownerID, &sess.ID, nil, "QuotaExceeded", map[string]any{"reason": "system in survival mode: low priority requests rejected"})
			return nil, fmt.Errorf("system in survival mode: only high priority requests allowed")
		}
	}

	// 5. Select Best Node via Scheduler
	selectedNode, err := h.sch.SelectNode(ctx, nodes, states, userPriority, activeLeaseNodeID, systemState)
	if err != nil {
		slog.Error("[Hypervisor] Scheduler node selection failed", "session_id", sess.ID, "error", err)
		return nil, err
	}
	slog.Info("[Hypervisor] Scheduler selected node",
		"session_id", sess.ID,
		"selected_node_id", selectedNode.NodeID,
		"account_name", selectedNode.AccountName,
	)

	// 6. Grant lease
	expiresAt := time.Now().Add(30 * time.Minute)
	newLease := &lease.Lease{
		LeaseID:   fmt.Sprintf("lease-%d", time.Now().UnixNano()),
		SessionID: sess.ID,
		NodeID:    selectedNode.NodeID,
		GrantedAt: time.Now(),
		ExpiresAt: &expiresAt,
	}

	if err := h.lm.SaveLease(ctx, newLease); err != nil {
		slog.Error("[Hypervisor] Failed to save lease", "lease_id", newLease.LeaseID, "error", err)
		return nil, err
	}

	sess.Lease = newLease
	sess.State = "active"
	sess.UpdatedAt = time.Now()
	if err := h.sm.SaveSession(ctx, sess); err != nil {
		slog.Error("[Hypervisor] Failed to update session state", "session_id", sess.ID, "error", err)
		return nil, err
	}

	_ = h.logEvent(ctx, ownerID, &sess.ID, &selectedNode.NodeID, "RequestStarted", nil)

	// 7. Execute request using Runtime
	slog.Info("[Hypervisor] Executing upstream request", "node_id", selectedNode.NodeID, "session_id", sess.ID)
	res, execErr := h.rt.Execute(ctx, selectedNode.NodeID, reqPayload, stream)

	// 8. If failed, handle Rebinding (Auto retry on another node)
	if execErr != nil {
		slog.Warn("[Hypervisor] Upstream request execution failed, triggering rebinding",
			"node_id", selectedNode.NodeID,
			"session_id", sess.ID,
			"error", execErr,
		)
		_ = h.logEvent(ctx, ownerID, &sess.ID, &selectedNode.NodeID, "RequestFailed", map[string]any{"error": execErr.Error()})
		_ = h.logEvent(ctx, ownerID, &sess.ID, &selectedNode.NodeID, "RebindingStarted", map[string]any{"reason": "upstream request failed"})

		newRes, newSelectedNode, err := h.rm.ExecuteRebinding(
			ctx,
			sess,
			selectedNode.NodeID,
			reqPayload,
			stream,
			h.rt,
			nodes,
			states,
			userPriority,
			h.sch.SelectNode,
			systemState,
			execErr,
		)
		if err != nil {
			slog.Error("[Hypervisor] Rebinding workflow failed", "session_id", sess.ID, "error", err)
			_ = h.logEvent(ctx, ownerID, &sess.ID, nil, "RebindingFailed", map[string]any{"reason": err.Error()})
			return nil, execErr
		}

		slog.Info("[Hypervisor] Rebinding succeeded",
			"session_id", sess.ID,
			"old_node_id", selectedNode.NodeID,
			"new_node_id", newSelectedNode.NodeID,
		)
		_ = h.logEvent(ctx, ownerID, &sess.ID, &newSelectedNode.NodeID, "RebindingSucceeded", nil)
		res = newRes
		selectedNode = newSelectedNode
	}

	// 9. Consume Quota on Success & Record Usage in Database
	if res != nil {
		totalTokens := res.InputTokens + res.OutputTokens
		costUnits := totalTokens
		if totalTokens > 0 {
			_ = h.qm.ConsumeQuota(ctx, ownerID, "standard", totalTokens)
			_, _ = h.store.RecordUsage(ctx, ownerID, &sess.ID, &selectedNode.NodeID, res.InputTokens, res.OutputTokens, costUnits)
			slog.Info("[Hypervisor] Recorded request usage",
				"owner_id", ownerID,
				"session_id", sess.ID,
				"node_id", selectedNode.NodeID,
				"input_tokens", res.InputTokens,
				"output_tokens", res.OutputTokens,
				"cost_units", costUnits,
			)
			_ = h.logEvent(ctx, ownerID, &sess.ID, &selectedNode.NodeID, "UsageRecorded", map[string]any{
				"input_tokens":  res.InputTokens,
				"output_tokens": res.OutputTokens,
				"cost_units":    costUnits,
			})
		}
		_ = h.logEvent(ctx, ownerID, &sess.ID, &selectedNode.NodeID, "RequestSucceeded", nil)
	}

	return res, nil
}

func (h *Hypervisor) Storage() *storage.Storage {
	return h.store
}

func (h *Hypervisor) HealthController() *health.HealthController {
	return h.hc
}

func (h *Hypervisor) getAllNodes(ctx context.Context, platform string) ([]health.ExecutionNode, error) {
	if platform == "" {
		platform = "anthropic"
	}
	// Try loading from DB first
	dbAccounts, _ := h.store.DB().Account.Query().
		Where(
			account.Platform(platform),
			account.StatusIn("active", "error"),
		).
		All(ctx)

	if len(dbAccounts) == 0 {
		dbAccounts, _ = h.store.DB().Account.Query().
			Where(
				account.StatusIn("active", "error"),
				account.PlatformNEQ("yunwu"),
			).
			All(ctx)
	}

	var list []health.ExecutionNode
	for _, acc := range dbAccounts {
		nodeID := fmt.Sprintf("%d", acc.ID)
		node, err := h.nm.GetNode(ctx, nodeID)
		if err != nil || node == nil {
			node = &health.ExecutionNode{
				NodeID:       nodeID,
				AccountName:  acc.Name,
				Provider:     acc.Platform,
				State:        "healthy",
				Capabilities: []string{platform},
				Priority:     acc.Priority,
				UpdatedAt:    time.Now(),
			}
			_ = h.nm.SaveNode(ctx, node)
		}
		list = append(list, *node)
	}

	// Fallback to Redis nodes (for unit tests where DB is empty)
	if len(list) == 0 {
		nodeIDs, _ := h.store.Redis().SMembers(ctx, "ash:nodes").Result()
		for _, id := range nodeIDs {
			node, err := h.nm.GetNode(ctx, id)
			if err == nil && node != nil && (node.Provider == platform || node.Provider == "") {
				list = append(list, *node)
			}
		}
	}

	return list, nil
}

func (h *Hypervisor) getAllNodeRuntimeStates(ctx context.Context, nodes []health.ExecutionNode) (map[string]health.NodeRuntimeState, error) {
	states := make(map[string]health.NodeRuntimeState)
	for _, node := range nodes {
		state, err := h.nm.GetNodeRuntimeState(ctx, node.NodeID)
		if err == nil && state != nil {
			states[node.NodeID] = *state
		}
	}
	return states, nil
}

func (h *Hypervisor) logEvent(ctx context.Context, ownerID string, sessID *string, nodeID *string, eventType string, payload map[string]any) error {
	if payload == nil {
		payload = make(map[string]any)
	}
	_, err := h.ew.LogEvent(ctx, ownerID, sessID, nodeID, eventType, payload)
	return err
}

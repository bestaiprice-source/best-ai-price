package yunwu

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/account"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/compiler"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/events"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/lease"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/quota"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/runtime"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/session"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
)

type YunwuHypervisor struct {
	store *storage.Storage
	sm    *session.SessionManager
	lm    *lease.LeaseManager
	nm    *health.NodeManager
	ew    *events.EventWriter
	qm    *quota.QuotaManager
	sch   *YunwuScheduler
	rt    runtime.Runtime
	rm    *YunwuRebindingManager
	comp  *compiler.Compiler
	hc    *YunwuHealthController
	sw    *StatsWorker
}

func NewYunwuHypervisor(store *storage.Storage, rt runtime.Runtime) *YunwuHypervisor {
	yh := &YunwuHypervisor{
		store: store,
		sm:    session.NewSessionManager(store),
		lm:    lease.NewLeaseManager(store),
		nm:    health.NewNodeManager(store),
		ew:    events.NewEventWriter(store),
		qm:    quota.NewQuotaManager(store),
		sch:   NewYunwuScheduler(),
		rt:    rt,
		rm:    NewYunwuRebindingManager(store),
		comp:  compiler.NewCompiler(),
		hc:    NewYunwuHealthController(store),
		sw:    NewStatsWorker(store),
	}

	yh.hc.Start(context.Background(), 10*time.Second, func(ctx context.Context) []health.ExecutionNode {
		nodes, _ := yh.getAllYunwuNodes(ctx)
		return nodes
	})

	slog.Info("[YunwuHypervisor] Initialized dedicated Yunwu Engine")
	return yh
}

func (yh *YunwuHypervisor) HandleRequest(
	ctx context.Context,
	ownerID string,
	conversationID string,
	reqPayload []byte,
	stream io.Writer,
	platform string,
) (*runtime.ResponseMetadata, error) {
	slog.Info("[YunwuHypervisor] Handling request via Yunwu Engine",
		"owner_id", ownerID,
		"conversation_id", conversationID,
		"platform", platform,
	)

	// 1. Quota Check
	allowed, err := yh.qm.CheckQuota(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		slog.Warn("[YunwuHypervisor] User quota exceeded", "owner_id", ownerID)
		return nil, quota.ErrQuotaExceeded
	}

	// 2. Resolve VirtualSession
	if conversationID == "" {
		conversationID = "default"
	}
	sessID := fmt.Sprintf("session-yunwu-%s-%s", ownerID, conversationID)
	sess, err := yh.sm.GetSession(ctx, sessID)
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
	}

	// 3. Resolve active lease
	var activeLeaseNodeID string
	currentLease, err := yh.lm.GetLease(ctx, sess.ID)
	if err == nil && currentLease != nil {
		activeLeaseNodeID = currentLease.NodeID
	}

	// 4. Load Yunwu nodes and runtime credits
	nodes, err := yh.getAllYunwuNodes(ctx)
	if err != nil {
		return nil, err
	}
	states := yh.getAllYunwuNodeStates(ctx, nodes)
	credits := yh.getAllYunwuNodeCredits(ctx, nodes)
	systemState := yh.hc.GetSystemState(ctx)

	userPriority := 1
	if ua, err := yh.qm.GetUserAccess(ctx, ownerID); err == nil && ua != nil {
		userPriority = ua.Priority
	}

	// 5. Select Best Node via YunwuScheduler
	selectedNode, err := yh.sch.SelectNode(ctx, nodes, states, credits, userPriority, activeLeaseNodeID, systemState)
	if err != nil {
		slog.Error("[YunwuHypervisor] YunwuScheduler selection failed", "error", err)
		return nil, err
	}

	// 6. Grant lease
	expiresAt := time.Now().Add(30 * time.Minute)
	newLease := &lease.Lease{
		LeaseID:   fmt.Sprintf("lease-yunwu-%d", time.Now().UnixNano()),
		SessionID: sess.ID,
		NodeID:    selectedNode.NodeID,
		GrantedAt: time.Now(),
		ExpiresAt: &expiresAt,
	}
	_ = yh.lm.SaveLease(ctx, newLease)

	// 7. Execute Runtime Request
	execStart := time.Now()
	meta, err := yh.rt.Execute(ctx, selectedNode.NodeID, reqPayload, stream)

	// 8. Rebinding & Fallback handling if error
	if err != nil {
		slog.Warn("[YunwuHypervisor] Runtime request failed on selected Yunwu node, attempting Rebinding",
			"failed_node_id", selectedNode.NodeID,
			"error", err,
		)
		reboundNode, rebindErr := yh.rm.HandleYunwuFailure(ctx, selectedNode.NodeID, err, nodes)
		if rebindErr == nil && reboundNode != nil {
			meta, err = yh.rt.Execute(ctx, reboundNode.NodeID, reqPayload, stream)
			if err == nil {
				selectedNode = reboundNode
			}
		}
	}

	if err != nil {
		return nil, err
	}

	latencyMs := time.Since(execStart).Milliseconds()

	// Update node runtime metrics
	state := states[selectedNode.NodeID]
	if state == nil {
		state = &health.NodeRuntimeState{NodeID: selectedNode.NodeID}
	}
	state.AvgLatencyMs = (state.AvgLatencyMs + latencyMs) / 2
	_ = yh.nm.SaveNodeRuntimeState(ctx, state)

	// 9. Record usage
	if meta != nil {
		units := meta.InputTokens + meta.OutputTokens
		if units == 0 {
			units = 100
		}
		_ = yh.qm.ConsumeQuota(ctx, ownerID, units)
	}

	return meta, nil
}

func (yh *YunwuHypervisor) getAllYunwuNodes(ctx context.Context) ([]health.ExecutionNode, error) {
	var dbAccounts []*ent.Account

	if yh.store != nil && yh.store.DB() != nil {
		dbAccounts, _ = yh.store.DB().Account.Query().
			Where(account.PlatformEqualFold("yunwu")).
			All(ctx)

		if len(dbAccounts) == 0 {
			// Try fallback to accounts containing "yunwu" in name or type
			dbAccounts, _ = yh.store.DB().Account.Query().
				Where(account.NameContainsFold("yunwu")).
				All(ctx)
		}
	}

	var list []health.ExecutionNode
	for _, acc := range dbAccounts {
		nodeID := fmt.Sprintf("%d", acc.ID)
		node, err := yh.nm.GetNode(ctx, nodeID)
		if err != nil || node == nil {
			node = &health.ExecutionNode{
				NodeID:       nodeID,
				AccountName:  acc.Name,
				Provider:     "yunwu",
				State:        "healthy",
				Capabilities: []string{"anthropic", "openai"},
				Priority:     acc.Priority,
				UpdatedAt:    time.Now(),
			}
			_ = yh.nm.SaveNode(ctx, node)
		}
		list = append(list, *node)
	}

	// Fallback to Redis for unit testing
	if len(list) == 0 {
		nodeIDs, _ := yh.store.Redis().SMembers(ctx, "ash:yunwu:nodes").Result()
		for _, id := range nodeIDs {
			node, err := yh.nm.GetNode(ctx, id)
			if err == nil && node != nil {
				list = append(list, *node)
			}
		}
	}

	return list, nil
}

func (yh *YunwuHypervisor) getAllYunwuNodeStates(ctx context.Context, nodes []health.ExecutionNode) map[string]*health.NodeRuntimeState {
	states := make(map[string]*health.NodeRuntimeState)
	for _, node := range nodes {
		state, err := yh.nm.GetNodeRuntimeState(ctx, node.NodeID)
		if err == nil && state != nil {
			states[node.NodeID] = state
		}
	}
	return states
}

func (yh *YunwuHypervisor) getAllYunwuNodeCredits(ctx context.Context, nodes []health.ExecutionNode) map[string]float64 {
	credits := make(map[string]float64)
	for _, node := range nodes {
		valStr, err := yh.store.Redis().Get(ctx, fmt.Sprintf("ash:yunwu:credit:%s", node.NodeID)).Result()
		if err == nil && valStr != "" {
			if val, err := strconv.ParseFloat(valStr, 64); err == nil {
				credits[node.NodeID] = val
			}
		} else {
			credits[node.NodeID] = 100.0 // Default 100.0 if not yet synced
		}
	}
	return credits
}

// GetHealthStatus returns the status summary of Yunwu Hypervisor Engine
func (yh *YunwuHypervisor) GetHealthStatus(ctx context.Context) map[string]any {
	nodes, _ := yh.getAllYunwuNodes(ctx)
	systemState := yh.hc.GetSystemState(ctx)
	healthy := 0
	cooldown := 0

	for _, n := range nodes {
		if n.State == "healthy" {
			healthy++
		} else if n.State == "cooldown" {
			cooldown++
		}
	}

	rebinding24h, _ := yh.store.Redis().ZCard(ctx, "ash:yunwu:rebinding:24h").Result()

	return map[string]any{
		"status":            "ok",
		"system_state":      systemState,
		"platform":          "yunwu",
		"total_nodes":       len(nodes),
		"healthy_nodes":     healthy,
		"cooldown_nodes":    cooldown,
		"rebinding_24h_cnt": rebinding24h,
	}
}

// SyncNodeBalance triggers an immediate balance sync for a Yunwu node
func (yh *YunwuHypervisor) SyncNodeBalance(ctx context.Context, nodeID string, apiKey string, baseURL string) (*YunwuCreditGrantsResponse, error) {
	node, err := yh.nm.GetNode(ctx, nodeID)
	if err != nil || node == nil {
		node = &health.ExecutionNode{
			NodeID:      nodeID,
			AccountName: "Yunwu-Node-" + nodeID,
			Provider:    "yunwu",
			State:       "healthy",
		}
	}
	return yh.sw.SyncNodeStats(ctx, node, apiKey, baseURL)
}

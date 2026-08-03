package yunwu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
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
		"payload_size_bytes", len(reqPayload),
	)

	// 1. Quota Check
	allowed, err := yh.qm.CheckQuota(ctx, ownerID, "yunwu")
	if err != nil {
		slog.Error("[YunwuHypervisor] Quota check failed", "owner_id", ownerID, "error", err)
		return nil, err
	}
	if !allowed {
		slog.Warn("[YunwuHypervisor] User quota exceeded", "owner_id", ownerID)
		return nil, quota.ErrQuotaExceeded
	}
	slog.Debug("[YunwuHypervisor] Quota check passed", "owner_id", ownerID)

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
		slog.Debug("[YunwuHypervisor] Created new virtual session", "session_id", sessID)
	} else {
		slog.Debug("[YunwuHypervisor] Reusing existing virtual session", "session_id", sessID, "state", sess.State)
	}

	// 2.1 Extract and store session context from request payload
	yh.extractAndStoreContext(sess, reqPayload)
	_ = yh.sm.SaveSession(ctx, sess)

	// 3. Resolve active lease
	var activeLeaseNodeID string
	currentLease, err := yh.lm.GetLease(ctx, sess.ID)
	if err == nil && currentLease != nil {
		activeLeaseNodeID = currentLease.NodeID
		slog.Debug("[YunwuHypervisor] Found active session lease", "session_id", sess.ID, "lease_node_id", activeLeaseNodeID)
	}

	// 4. Load Yunwu nodes and runtime credits
	nodes, err := yh.getAllYunwuNodes(ctx)
	if err != nil {
		slog.Error("[YunwuHypervisor] Failed to fetch Yunwu nodes", "error", err)
		return nil, err
	}
	states := yh.getAllYunwuNodeStates(ctx, nodes)
	credits := yh.getAllYunwuNodeCredits(ctx, nodes)
	systemState := yh.hc.GetSystemState(ctx)

	slog.Debug("[YunwuHypervisor] Evaluated Yunwu pool status",
		"total_nodes", len(nodes),
		"system_state", systemState,
	)

	userPriority := 1
	if ua, err := yh.qm.GetUserAccess(ctx, ownerID); err == nil && ua != nil {
		userPriority = ua.Priority
	}

	// 5. Select Best Node via YunwuScheduler
	selectedNode, err := yh.sch.SelectNode(ctx, nodes, states, credits, userPriority, activeLeaseNodeID, systemState)
	if err != nil {
		slog.Error("[YunwuHypervisor] YunwuScheduler selection failed", "error", err, "total_nodes", len(nodes))
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
	slog.Debug("[YunwuHypervisor] Granted lease for node",
		"lease_id", newLease.LeaseID,
		"selected_node_id", selectedNode.NodeID,
		"account_name", selectedNode.AccountName,
	)

	// 7. Execute Runtime Request
	execStart := time.Now()
	meta, err := yh.rt.Execute(ctx, selectedNode.NodeID, reqPayload, stream)

	// 8. Rebinding & Fallback handling if error
	if err != nil {
		slog.Warn("[YunwuHypervisor] Runtime request failed on selected Yunwu node, attempting Rebinding",
			"failed_node_id", selectedNode.NodeID,
			"account_name", selectedNode.AccountName,
			"error", err,
		)
		reboundNode, rebindErr := yh.rm.HandleYunwuFailure(ctx, selectedNode.NodeID, err, nodes)
		if rebindErr == nil && reboundNode != nil {
			slog.Info("[YunwuHypervisor] Retrying execution on rebound node",
				"rebound_node_id", reboundNode.NodeID,
				"rebound_account_name", reboundNode.AccountName,
			)

			// Compile session context and inject into retry payload
			retryPayload := yh.compileAndInjectContext(sess, reqPayload)

			meta, err = yh.rt.Execute(ctx, reboundNode.NodeID, retryPayload, stream)
			if err == nil {
				selectedNode = reboundNode
				// Update session lease to rebound node
				expiresAt := time.Now().Add(30 * time.Minute)
				newLease := &lease.Lease{
					LeaseID:   fmt.Sprintf("lease-yunwu-%d", time.Now().UnixNano()),
					SessionID: sess.ID,
					NodeID:    reboundNode.NodeID,
					GrantedAt: time.Now(),
					ExpiresAt: &expiresAt,
				}
				_ = yh.lm.SaveLease(ctx, newLease)
			} else {
				slog.Error("[YunwuHypervisor] Execution failed on rebound node",
					"rebound_node_id", reboundNode.NodeID,
					"error", err,
				)
			}
		} else {
			slog.Error("[YunwuHypervisor] Rebinding failed or no alternative nodes", "rebind_error", rebindErr)
		}
	}

	if err != nil {
		slog.Error("[YunwuHypervisor] Final execution failed",
			"node_id", selectedNode.NodeID,
			"elapsed_ms", time.Since(execStart).Milliseconds(),
			"error", err,
		)
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
	var units int64 = 0
	if meta != nil {
		// Parse requested model
		var parsedReq struct {
			Model string `json:"model"`
		}
		_ = json.Unmarshal(reqPayload, &parsedReq)
		modelName := parsedReq.Model

		// Default pricing
		var inputPrice, outputPrice float64 = 0.002, 0.002

		if modelName != "" {
			rows, err := yh.store.DB().QueryContext(ctx, "SELECT input_price, output_price FROM ash_vendor_models WHERE model_name = $1 LIMIT 1", modelName)
			if err == nil {
				if rows.Next() {
					_ = rows.Scan(&inputPrice, &outputPrice)
				}
				rows.Close()
			} else {
				slog.Warn("[YunwuHypervisor] Failed to query model pricing", "model", modelName, "error", err)
			}
		}

		inputCost := (float64(meta.InputTokens) / 1000.0) * inputPrice
		outputCost := (float64(meta.OutputTokens) / 1000.0) * outputPrice
		totalCost := inputCost + outputCost
		units = int64(totalCost * 1000000.0)

		if units <= 0 {
			units = 1
		}
		_ = yh.qm.ConsumeQuota(ctx, ownerID, "yunwu", units)
	}

	slog.Info("[YunwuHypervisor] Request executed successfully",
		"owner_id", ownerID,
		"selected_node_id", selectedNode.NodeID,
		"account_name", selectedNode.AccountName,
		"latency_ms", latencyMs,
		"tokens_in", func() int64 { if meta != nil { return meta.InputTokens }; return 0 }(),
		"tokens_out", func() int64 { if meta != nil { return meta.OutputTokens }; return 0 }(),
		"total_units_consumed", units,
	)

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

	// Fallback to Redis nodes (only for unit tests where DB is empty)
	// This path is guarded by ASH_YUNWU_TEST_MODE to prevent silent production fallback.
	if len(list) == 0 && os.Getenv("ASH_YUNWU_TEST_MODE") == "true" {
		slog.Warn("[YunwuHypervisor] No nodes found in DB, falling back to Redis test nodes (ASH_YUNWU_TEST_MODE=true)")
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

// rawRequestPayload is a minimal structure to extract messages and system prompt
// from Anthropic/OpenAI-compatible JSON request payloads.
type rawRequestPayload struct {
	System   string `json:"system"`
	Messages []struct {
		Role    string `json:"role"`
		Content any    `json:"content"` // string or array of content blocks
	} `json:"messages"`
}

// extractAndStoreContext parses the request payload and stores extracted
// system prompt and recent messages into the session's ExecutionContext.
func (yh *YunwuHypervisor) extractAndStoreContext(sess *session.VirtualSession, reqPayload []byte) {
	var raw rawRequestPayload
	if err := json.Unmarshal(reqPayload, &raw); err != nil {
		slog.Debug("[YunwuHypervisor] Could not parse request payload for context extraction", "error", err)
		return
	}

	ctx := sess.Context
	if ctx == nil {
		ctx = &session.ExecutionContext{}
	}

	// Extract system prompt (keep latest)
	if raw.System != "" {
		ctx.SystemPrompt = raw.System
	}

	// Extract recent messages (keep last 20 to avoid unbounded growth)
	if len(raw.Messages) > 0 {
		recent := raw.Messages
		if len(recent) > 20 {
			recent = recent[len(recent)-20:]
		}
		msgs := make([]session.Message, 0, len(recent))
		for _, m := range recent {
			content := extractContentString(m.Content)
			if content != "" {
				msgs = append(msgs, session.Message{Role: m.Role, Content: content})
			}
		}
		ctx.RecentMessages = msgs
	}

	sess.Context = ctx
	slog.Debug("[YunwuHypervisor] Extracted session context",
		"session_id", sess.ID,
		"system_prompt_len", len(ctx.SystemPrompt),
		"recent_messages_count", len(ctx.RecentMessages),
	)
}

// compileAndInjectContext compiles the session's ExecutionContext into a preamble
// and injects it as a system message into the request payload for the retry.
// Returns the modified payload (or original if no context to inject).
func (yh *YunwuHypervisor) compileAndInjectContext(sess *session.VirtualSession, reqPayload []byte) []byte {
	if sess.Context == nil {
		return reqPayload
	}

	// Build compiler context from session
	compCtx := compiler.ExecutionContext{
		SystemPrompt:   sess.Context.SystemPrompt,
		Summary:        sess.Context.Summary,
		PinnedFacts:    sess.Context.PinnedFacts,
		TerminalState:  sess.Context.TerminalState,
		ToolState:      toolCallsToString(sess.Context.ToolState),
	}
	for _, m := range sess.Context.RecentMessages {
		compCtx.RecentMessages = append(compCtx.RecentMessages, compiler.Message{Role: m.Role, Content: m.Content})
	}

	compiled, err := yh.comp.Compile(compCtx)
	if err != nil || compiled == "" {
		slog.Debug("[YunwuHypervisor] Context compilation produced empty result, using original payload")
		return reqPayload
	}

	// Inject compiled context as a system message into the messages array
	slog.Debug("[YunwuHypervisor] Injecting compiled context into retry payload",
		"session_id", sess.ID,
		"compiled_length", len(compiled),
	)

	var raw rawRequestPayload
	if err := json.Unmarshal(reqPayload, &raw); err != nil {
		slog.Warn("[YunwuHypervisor] Could not parse request for context injection", "error", err)
		return reqPayload
	}

	// Prepend a system message with the compiled context
	contextMsg := struct {
		Role    string `json:"role"`
		Content any    `json:"content"`
	}{Role: "system", Content: compiled}

	// Insert at the beginning (before any existing system message or user messages)
	newMessages := make([]struct {
		Role    string `json:"role"`
		Content any    `json:"content"`
	}, 0, len(raw.Messages)+1)
	newMessages = append(newMessages, contextMsg)
	newMessages = append(newMessages, raw.Messages...)
	raw.Messages = newMessages

	modified, err := json.Marshal(raw)
	if err != nil {
		slog.Warn("[YunwuHypervisor] Could not marshal modified payload", "error", err)
		return reqPayload
	}

	return modified
}

// extractContentString converts a message content field (string or content-block array) to a string.
func extractContentString(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		// Content blocks array: [{"type":"text","text":"..."}, ...]
		var parts []string
		for _, block := range v {
			if blockMap, ok := block.(map[string]any); ok {
				if blockMap["type"] == "text" || blockMap["type"] == "input_text" {
					if text, ok := blockMap["text"].(string); ok {
						parts = append(parts, text)
					}
				}
			}
		}
		if len(parts) > 0 {
			result := ""
			for i, p := range parts {
				if i > 0 {
					result += "\n"
				}
				result += p
			}
			return result
		}
	}
	return ""
}

// toolCallsToString serializes tool calls to a compact string representation.
func toolCallsToString(toolCalls []session.ToolCall) string {
	if len(toolCalls) == 0 {
		return ""
	}
	var parts []string
	for _, tc := range toolCalls {
		parts = append(parts, fmt.Sprintf("[%s] %s: %s", tc.Type, tc.ID, tc.Function))
	}
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += ", "
		}
		result += p
	}
	return result
}

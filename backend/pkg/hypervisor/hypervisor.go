package hypervisor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/ent/account"
	"github.com/Wei-Shaw/sub2api/internal/credentials"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/backup"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/compiler"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/config"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/events"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/lease"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/observability"
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
	sw    *compiler.SummaryWorker
	hc    *health.HealthController
	yh    *yunwu.YunwuHypervisor
	cfg   *config.HypervisorConfig
	bm    *backup.BackupManager
}

// commitTrackingWriter records whether any response bytes have reached the
// client. Once committed, an upstream retry would risk a duplicated response.
type commitTrackingWriter struct {
	w io.Writer
	n int64
}

func (w *commitTrackingWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	w.n += int64(n)
	return n, err
}

func (w *commitTrackingWriter) Flush() {
	if f, ok := w.w.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *commitTrackingWriter) Committed() bool { return w.n > 0 }

func NewHypervisor(store *storage.Storage, rt runtime.Runtime, decryptors ...credentials.Cipher) *Hypervisor {
	cfg := config.LoadFromEnv()
	h := &Hypervisor{
		store: store,
		sm:    session.NewSessionManager(store),
		lm:    lease.NewLeaseManager(store),
		nm:    health.NewNodeManager(store),
		ew:    events.NewEventWriter(store),
		qm:    quota.NewQuotaManager(store, cfg),
		sch:   scheduler.NewScheduler(),
		rt:    rt,
		rm:    rebinding.NewRebindingManager(store, cfg),
		comp:  compiler.NewCompiler(),
		sw:    compiler.NewSummaryWorker(store),
		hc:    health.NewHealthController(store),
		yh:    yunwu.NewYunwuHypervisor(store, rt, decryptors...),
		cfg:   cfg,
		bm:    backup.NewBackupManager(store),
	}

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

func (h *Hypervisor) BackupEnabled() bool { return h.cfg.EnableBackup }

func (h *Hypervisor) BackupManager() *backup.BackupManager { return h.bm }

func (h *Hypervisor) HandleRequest(ctx context.Context, ownerID string, conversationID string, reqPayload []byte, stream io.Writer, platform string) (*runtime.ResponseMetadata, error) {
	startedAt := time.Now()
	res, err := h.handleRequest(ctx, ownerID, conversationID, reqPayload, stream, platform)
	observability.Default.ObserveRequest(time.Since(startedAt).Milliseconds())
	if err != nil {
		observability.Default.RequestFailed()
	}
	return res, err
}

func (h *Hypervisor) handleRequest(ctx context.Context, ownerID string, conversationID string, reqPayload []byte, stream io.Writer, platform string) (*runtime.ResponseMetadata, error) {
	requestID, _ := ctx.Value(ctxkey.RequestID).(string)
	// Extract model for logging
	var payloadMap map[string]interface{}
	var requestedModel string
	if err := json.Unmarshal(reqPayload, &payloadMap); err == nil {
		if m, ok := payloadMap["model"].(string); ok {
			requestedModel = m
		}
	}

	slog.Info("[Hypervisor] Handling request",
		"request_id", requestID,
		"hypervisor_mode", "standard",
		"owner_id", ownerID,
		"conversation_id", conversationID,
		"platform", platform,
		"model", requestedModel,
		"payload_size", len(reqPayload),
	)

	if platform == "yunwu" && h.yh != nil {
		slog.Info("[Hypervisor] Dispatching request to dedicated Yunwu Hypervisor Engine",
			"hypervisor_mode", "yunwu",
			"owner_id", ownerID,
			"conversation_id", conversationID,
			"platform", platform,
			"model", requestedModel,
		)
		return h.yh.HandleRequest(ctx, ownerID, conversationID, reqPayload, stream, platform)
	}

	// Register before the check so the first request participates in fair-share.
	userPriority := 1
	if ua, accessErr := h.qm.GetUserAccess(ctx, ownerID, "standard"); accessErr == nil && ua != nil {
		userPriority = ua.Priority
	}
	_ = h.qm.RegisterActiveUser(ctx, ownerID, "standard", userPriority)

	// 1. Check the shared standard tariff and provider-specific capacity with atomic token reservation.
	var reservationOpen bool
	scope := "standard:" + platform
	if h.cfg.EnableQuotaManager {
		estimatedTokens := estimateRequestTokens(reqPayload)
		if requestID == "" {
			requestID = fmt.Sprintf("req-std-%d", time.Now().UnixNano())
		}
		reserved, resErr := h.qm.ReserveQuotaTokens(ctx, ownerID, requestID, scope, estimatedTokens, 120*time.Second)
		if resErr != nil {
			slog.Error("[Hypervisor] Quota reservation failed with error", "owner_id", ownerID, "error", resErr)
			return nil, resErr
		}
		if !reserved {
			slog.Warn("[Hypervisor] User quota exceeded", "owner_id", ownerID)
			_ = h.logEvent(ctx, ownerID, nil, nil, "QuotaExceeded", map[string]any{"reason": "quota limit reached"})
			return nil, quota.ErrQuotaExceeded
		}
		reservationOpen = true
		defer func() {
			if reservationOpen {
				_ = h.qm.ReleaseQuotaTokens(context.Background(), ownerID, requestID, scope)
			}
		}()
	}

	// 1.5. Check Concurrent Slot
	if h.cfg.EnableQuotaManager {
		if err := h.qm.AcquireConcurrentSlot(ctx, ownerID, "standard"); err != nil {
			_ = h.logEvent(ctx, ownerID, nil, nil, "QuotaExceeded", map[string]any{"reason": "concurrent request limit exceeded"})
			return nil, err
		}
		defer h.qm.ReleaseConcurrentSlot(context.Background(), ownerID, "standard")
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
		slog.Info("[Hypervisor] Created new virtual session", "hypervisor_mode", "standard", "session_id", sessID, "owner_id", ownerID)
		_ = h.logEvent(ctx, ownerID, &sessID, nil, "SessionCreated", map[string]any{"conversation_id": conversationID})
	} else {
		slog.Debug("[Hypervisor] Resolved existing virtual session", "session_id", sessID, "version", sess.Version)
	}
	h.extractAndStoreContext(sess, reqPayload)
	if h.cfg.EnableSummary && sess.Context != nil {
		h.sw.TriggerAsyncSummary(ctx, ownerID, sess.ID, compilerContextFromSession(sess.Context))
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

	// Fetch system state from Redis
	systemState := h.hc.GetSystemState(ctx, platform)
	slog.Info("[Hypervisor] Loaded pool status",
		"hypervisor_mode", "standard",
		"total_nodes", len(nodes),
		"user_priority", userPriority,
		"system_state", systemState,
	)

	// Enforce degradation policies
	if systemState == "SURVIVAL" && !h.cfg.DisableSurvivalMode {
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
		"hypervisor_mode", "standard",
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

	var res *runtime.ResponseMetadata
	var execErr error
	var lastExecLatency time.Duration
	maxRetries := h.cfg.MaxRetry
	if !h.cfg.EnableRebinding {
		maxRetries = 0
	}
	responseWriter := &commitTrackingWriter{w: stream}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// 7. Execute request using Runtime
		slog.Info("[Hypervisor] Executing upstream request",
			"hypervisor_mode", "standard",
			"node_id", selectedNode.NodeID,
			"session_id", sess.ID,
			"attempt", attempt,
		)
		execStartedAt := time.Now()
		res, execErr = h.rt.Execute(ctx, selectedNode.NodeID, reqPayload, responseWriter)
		lastExecLatency = time.Since(execStartedAt)

		// 8. If successful, break out of the loop
		if execErr == nil {
			break
		}

		if ctx.Err() != nil {
			// Client cancellation is not evidence that the node is unhealthy.
			_ = h.logEvent(context.Background(), ownerID, &sess.ID, &selectedNode.NodeID, "RequestCancelled", nil)
			return nil, ctx.Err()
		}
		if responseWriter.Committed() {
			_ = h.logEvent(context.Background(), ownerID, &sess.ID, &selectedNode.NodeID, "RequestStreamInterrupted", map[string]any{"retry_suppressed": true})
			return nil, execErr
		}

		slog.Warn("[Hypervisor] Upstream request execution failed",
			"node_id", selectedNode.NodeID,
			"session_id", sess.ID,
			"attempt", attempt,
			"error", execErr,
		)
		state, _ := h.hc.NodeManager().GetNodeRuntimeState(ctx, selectedNode.NodeID)
		if state == nil {
			state = &health.NodeRuntimeState{NodeID: selectedNode.NodeID}
		}
		{
			previousRequests := state.TotalRequests
			state.TotalRequests++
			state.SuccessRate = float64(state.SuccessfulRequests) / float64(state.TotalRequests)
			state.AvgLatencyMs = rollingAverage(state.AvgLatencyMs, previousRequests, lastExecLatency.Milliseconds())
			nowErr := time.Now()
			state.LastErrorAt = &nowErr
			errorText := strings.ToLower(execErr.Error())
			if strings.Contains(errorText, "429") || strings.Contains(errorText, "too many requests") {
				state.Recent429++
				cooldownUntil := nowErr.Add(time.Duration(h.cfg.NodeCooldownSeconds) * time.Second)
				state.CooldownUntil = &cooldownUntil
				if node, nodeErr := h.hc.NodeManager().GetNode(ctx, selectedNode.NodeID); nodeErr == nil && node != nil {
					node.State = "cooldown"
					node.UpdatedAt = nowErr
					_ = h.hc.NodeManager().SaveNode(ctx, node)
				}
				_ = h.logEvent(ctx, "system", nil, &selectedNode.NodeID, "NodeCooldownStarted", map[string]any{
					"reason": "upstream_429", "cooldown_until": cooldownUntil,
				})
			}
			if errors.Is(execErr, context.DeadlineExceeded) || strings.Contains(errorText, "timeout") || strings.Contains(errorText, "deadline exceeded") {
				state.RecentTimeouts++
			}
			_ = h.hc.NodeManager().SaveNodeRuntimeState(ctx, state)
		}
		_ = h.logEvent(ctx, ownerID, &sess.ID, &selectedNode.NodeID, "RequestFailed", map[string]any{"error": execErr.Error()})
		if strings.Contains(strings.ToLower(execErr.Error()), "429") {
			observability.Default.Upstream429()
		}

		if attempt == maxRetries {
			slog.Error("[Hypervisor] Max retries reached, failing request", "session_id", sess.ID)
			return nil, execErr
		}

		// Trigger rebinding
		if h.cfg.EnableBackup {
			_ = h.bm.CreateSnapshot(ctx, sess.ID, snapshotFromSession(sess))
		}
		_ = h.logEvent(ctx, ownerID, &sess.ID, &selectedNode.NodeID, "RebindingStarted", map[string]any{"reason": "upstream request failed"})
		observability.Default.Rebinding()

		newSelectedNode, err := h.rm.RebindSession(
			ctx,
			sess,
			selectedNode.NodeID,
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
			observability.Default.RebindingFailed()
			return nil, execErr
		}

		slog.Info("[Hypervisor] Rebinding succeeded, retrying",
			"session_id", sess.ID,
			"old_node_id", selectedNode.NodeID,
			"new_node_id", newSelectedNode.NodeID,
		)
		_ = h.logEvent(ctx, ownerID, &sess.ID, &newSelectedNode.NodeID, "RebindingSucceeded", nil)
		selectedNode = newSelectedNode
	}

	// 9. Consume Quota on Success & Record Usage in Database
	if res != nil {
		totalTokens := res.InputTokens + res.OutputTokens
		costUnits := totalTokens
		if h.cfg.EnableQuotaManager {
			if recErr := h.qm.ReconcileQuotaTokens(ctx, ownerID, requestID, scope, totalTokens); recErr == nil {
				reservationOpen = false
			} else {
				slog.Error("[Hypervisor] ReconcileQuotaTokens failed", "owner_id", ownerID, "request_id", requestID, "error", recErr)
				_ = h.logEvent(ctx, ownerID, &sess.ID, &selectedNode.NodeID, "ReconcileFailed", map[string]any{"error": recErr.Error()})
			}
		}
		if totalTokens > 0 {
			if h.cfg.EnableUsageRecorder {
				_, _ = h.store.RecordUsage(ctx, ownerID, &sess.ID, &selectedNode.NodeID, res.InputTokens, res.OutputTokens, costUnits)
			}
			observability.Default.AddUsage(costUnits)

			state, _ := h.hc.NodeManager().GetNodeRuntimeState(ctx, selectedNode.NodeID)
			if state == nil {
				state = &health.NodeRuntimeState{NodeID: selectedNode.NodeID}
			}
			{
				state.UsedTokens += int64(totalTokens)
				previousRequests := state.TotalRequests
				state.TotalRequests++
				state.SuccessfulRequests++
				state.SuccessRate = float64(state.SuccessfulRequests) / float64(state.TotalRequests)
				state.AvgLatencyMs = rollingAverage(state.AvgLatencyMs, previousRequests, lastExecLatency.Milliseconds())
				nowSucc := time.Now()
				state.LastSuccessAt = &nowSucc
				_ = h.hc.NodeManager().SaveNodeRuntimeState(ctx, state)
			}

			slog.Info("[Hypervisor] Recorded request usage",
				"hypervisor_mode", "standard",
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

		// 10. Request Succeeded Comprehensive Log
		slog.Info("[Hypervisor] Request succeeded",
			"hypervisor_mode", "standard",
			"owner_id", ownerID,
			"session_id", sess.ID,
			"node_id", selectedNode.NodeID,
			"account_name", selectedNode.AccountName,
			"platform", platform,
			"model", requestedModel,
			"total_tokens", costUnits,
		)
		_ = h.logEvent(ctx, ownerID, &sess.ID, &selectedNode.NodeID, "RequestSucceeded", nil)
	}

	return res, nil
}

func rollingAverage(current int64, previousCount int64, sample int64) int64 {
	if previousCount <= 0 {
		return sample
	}
	return (current*previousCount + sample) / (previousCount + 1)
}

func (h *Hypervisor) extractAndStoreContext(sess *session.VirtualSession, payload []byte) {
	var raw struct {
		System   any `json:"system"`
		Messages []struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"messages"`
	}
	if json.Unmarshal(payload, &raw) != nil {
		return
	}
	if sess.Context == nil {
		sess.Context = &session.ExecutionContext{}
	}
	if system, ok := raw.System.(string); ok && system != "" {
		sess.Context.SystemPrompt = system
	}
	if len(raw.Messages) > 0 {
		messages := raw.Messages
		// Keep enough history to cross the async-summary threshold while bounding memory.
		if len(messages) > 40 {
			messages = messages[len(messages)-40:]
		}
		sess.Context.RecentMessages = sess.Context.RecentMessages[:0]
		for _, message := range messages {
			var content string
			switch value := message.Content.(type) {
			case string:
				content = value
			default:
				encoded, _ := json.Marshal(value)
				content = string(encoded)
			}
			if content != "" {
				sess.Context.RecentMessages = append(sess.Context.RecentMessages, session.Message{Role: message.Role, Content: content})
			}
		}
	}
}

func compilerContextFromSession(ctx *session.ExecutionContext) compiler.ExecutionContext {
	result := compiler.ExecutionContext{SystemPrompt: ctx.SystemPrompt, Summary: ctx.Summary, PinnedFacts: ctx.PinnedFacts, TerminalState: ctx.TerminalState}
	for _, message := range ctx.RecentMessages {
		result.RecentMessages = append(result.RecentMessages, compiler.Message{Role: message.Role, Content: message.Content})
	}
	return result
}

func snapshotFromSession(sess *session.VirtualSession) *backup.ProjectSnapshot {
	snapshot := &backup.ProjectSnapshot{SessionState: sess}
	if sess.Context != nil && sess.Context.ProjectState != nil {
		project := sess.Context.ProjectState
		snapshot.WorkspaceID, snapshot.GitBranch, snapshot.GitCommit = project.WorkspaceID, project.GitBranch, project.GitCommit
		snapshot.DirtyFiles, snapshot.OpenFiles = project.DirtyFiles, project.OpenFiles
		snapshot.LastCommand, snapshot.SnapshotRef = project.LastCommand, project.SnapshotRef
	}
	return snapshot
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
		// Redis-only nodes are supported by isolated tests below.
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
	if len(list) == 0 {
		return nil, fmt.Errorf("no healthy %s nodes", platform)
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
	if requestID, _ := ctx.Value(ctxkey.RequestID).(string); requestID != "" {
		payload["request_id"] = requestID
	}
	if retryCount, ok := ctx.Value(ctxkey.RetryCount).(int); ok {
		payload["retry_count"] = retryCount
	}
	_, err := h.ew.LogEvent(ctx, ownerID, sessID, nodeID, eventType, payload)
	return err
}

type tokenEstimatePayload struct {
	MaxTokens       int64 `json:"max_tokens"`
	MaxOutputTokens int64 `json:"max_output_tokens"`
}

func estimateRequestTokens(payload []byte) int64 {
	inputEst := int64(len(payload) / 4)
	if inputEst < 256 {
		inputEst = 256
	}

	outputEst := int64(4096)
	if len(payload) > 0 {
		var p tokenEstimatePayload
		if err := json.Unmarshal(payload, &p); err == nil {
			reqMax := p.MaxTokens
			if p.MaxOutputTokens > reqMax {
				reqMax = p.MaxOutputTokens
			}
			if reqMax > 0 {
				if reqMax > 128000 {
					reqMax = 128000
				}
				outputEst = reqMax
			}
		}
	}
	return inputEst + outputEst
}

package yunwu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/account"
	"github.com/Wei-Shaw/sub2api/internal/credentials"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/backup"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/compiler"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/config"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/events"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/lease"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/observability"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/quota"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/runtime"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/session"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
)

type YunwuHypervisor struct {
	store     *storage.Storage
	sm        *session.SessionManager
	lm        *lease.LeaseManager
	nm        *health.NodeManager
	ew        *events.EventWriter
	qm        *quota.QuotaManager
	sch       *YunwuScheduler
	rt        runtime.Runtime
	rm        *YunwuRebindingManager
	comp      *compiler.Compiler
	summary   *compiler.SummaryWorker
	bm        *backup.BackupManager
	hc        *YunwuHealthController
	sw        *StatsWorker
	cfg       *config.HypervisorConfig
	decryptor credentials.Cipher
}

type commitTrackingWriter struct {
	w io.Writer
	n int64
}

func (w *commitTrackingWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	w.n += int64(n)
	return n, err
}

func (w *commitTrackingWriter) Committed() bool { return w.n > 0 }

func (w *commitTrackingWriter) Flush() {
	if flusher, ok := w.w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func NewYunwuHypervisor(store *storage.Storage, rt runtime.Runtime, decryptors ...credentials.Cipher) *YunwuHypervisor {
	var decryptor credentials.Cipher
	if len(decryptors) > 0 {
		decryptor = decryptors[0]
	}
	cfg := config.LoadFromEnv()
	yh := &YunwuHypervisor{
		store:     store,
		sm:        session.NewSessionManager(store),
		lm:        lease.NewLeaseManager(store),
		nm:        health.NewNodeManager(store),
		ew:        events.NewEventWriter(store),
		qm:        quota.NewQuotaManager(store, cfg),
		sch:       NewYunwuScheduler(),
		rt:        rt,
		rm:        NewYunwuRebindingManager(store),
		comp:      compiler.NewCompiler(),
		summary:   compiler.NewSummaryWorker(store),
		bm:        backup.NewBackupManager(store),
		hc:        NewYunwuHealthController(store),
		sw:        NewStatsWorker(store, cfg),
		cfg:       cfg,
		decryptor: decryptor,
	}

	yh.hc.Start(context.Background(), 10*time.Second, func(ctx context.Context) []health.ExecutionNode {
		nodes, _ := yh.getAllYunwuNodes(ctx)
		return nodes
	})
	yh.startBalanceSync(context.Background())

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
	// Extract model for logging
	var payloadMap map[string]interface{}
	var requestedModel string
	if err := json.Unmarshal(reqPayload, &payloadMap); err == nil {
		if m, ok := payloadMap["model"].(string); ok {
			requestedModel = m
		}
	}

	slog.Info("[YunwuHypervisor] Handling Yunwu request",
		"hypervisor_mode", "yunwu",
		"owner_id", ownerID,
		"conversation_id", conversationID,
		"platform", platform,
		"model", requestedModel,
		"payload_size", len(reqPayload),
	)

	// 0. Resolve Priority and Register User
	userPriority := 1
	if ua, err := yh.qm.GetUserAccess(ctx, ownerID, "yunwu"); err == nil && ua != nil {
		userPriority = ua.Priority
	}
	_ = yh.qm.RegisterActiveUser(ctx, ownerID, "yunwu", userPriority)

	// 1. Quota Check
	allowed, err := yh.qm.CheckQuota(ctx, ownerID, "yunwu")
	if err != nil {
		slog.Error("[YunwuHypervisor] Quota check failed", "owner_id", ownerID, "error", err)
		return nil, err
	}
	if !allowed {
		slog.Warn("[YunwuHypervisor] User quota exceeded", "owner_id", ownerID)
		_, _ = yh.store.LogEvent(ctx, ownerID, nil, nil, "QuotaExceeded", map[string]any{"pool_type": "yunwu"})
		return nil, quota.ErrQuotaExceeded
	}

	// 1.5. Check Concurrent Slot
	if err := yh.qm.AcquireConcurrentSlot(ctx, ownerID, "yunwu"); err != nil {
		slog.Warn("[YunwuHypervisor] Concurrent limit exceeded", "owner_id", ownerID)
		return nil, err
	}
	defer yh.qm.ReleaseConcurrentSlot(context.Background(), ownerID, "yunwu")

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
		_, _ = yh.store.LogEvent(ctx, ownerID, &sessID, nil, "SessionCreated", map[string]any{"pool_type": "yunwu"})
	} else {
		slog.Debug("[YunwuHypervisor] Reusing existing virtual session", "session_id", sessID, "state", sess.State)
	}

	// 2.1 Extract and store session context from request payload
	yh.extractAndStoreContext(sess, reqPayload)
	_ = yh.sm.SaveSession(ctx, sess)
	if yh.cfg.EnableSummary && sess.Context != nil {
		yh.summary.TriggerAsyncSummary(ctx, ownerID, sess.ID, compilerContextFromYunwuSession(sess.Context))
	}

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

	slog.Info("[YunwuHypervisor] Evaluated Yunwu pool status",
		"hypervisor_mode", "yunwu",
		"total_nodes", len(nodes),
		"system_state", systemState,
	)

	// 5. Select Best Node via YunwuScheduler
	selectedNode, err := yh.sch.SelectNode(ctx, nodes, states, credits, userPriority, activeLeaseNodeID, systemState)
	if err != nil {
		slog.Error("[YunwuHypervisor] YunwuScheduler selection failed", "error", err, "total_nodes", len(nodes))
		return nil, err
	}
	inputPrice, outputPrice, fallbackPrice := yh.resolvePrice(ctx, selectedNode.NodeID, requestedModel)
	if fallbackPrice {
		observability.Default.YunwuFallbackPricing()
	}
	estimatedMicroUSD := estimateRequestMicroUSD(reqPayload, inputPrice, outputPrice)
	requestID := fmt.Sprintf("%s-%d", sess.ID, time.Now().UnixNano())
	reserved, reserveErr := yh.sw.Reserve(ctx, selectedNode.NodeID, requestID, estimatedMicroUSD, 10*time.Minute)
	if reserveErr != nil {
		return nil, reserveErr
	}
	if !reserved {
		return nil, quota.ErrQuotaExceeded
	}
	reservationNodeID := selectedNode.NodeID
	reservationOpen := true
	defer func() {
		if reservationOpen {
			_, _ = yh.sw.ReleaseReservation(context.Background(), reservationNodeID, requestID)
		}
	}()

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
	_, _ = yh.store.LogEvent(ctx, ownerID, &sess.ID, &selectedNode.NodeID, "RequestStarted", map[string]any{"pool_type": "yunwu"})
	slog.Info("[YunwuHypervisor] Granted lease for node",
		"hypervisor_mode", "yunwu",
		"lease_id", newLease.LeaseID,
		"selected_node_id", selectedNode.NodeID,
		"account_name", selectedNode.AccountName,
	)

	// 7. Execute Runtime Request
	execStart := time.Now()

	var meta *runtime.ResponseMetadata
	var execErr error
	maxRetries := yh.cfg.MaxRetry
	if !yh.cfg.EnableRebinding {
		maxRetries = 0
	}
	responseWriter := &commitTrackingWriter{w: stream}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		slog.Info("[YunwuHypervisor] Executing upstream request",
			"hypervisor_mode", "yunwu",
			"node_id", selectedNode.NodeID,
			"session_id", sess.ID,
			"attempt", attempt,
		)

		// Compile session context and inject into payload (for Yunwu-specifics if needed)
		if attempt > 0 {
			reqPayload = yh.compileAndInjectContext(sess, reqPayload)
		}

		meta, execErr = yh.rt.Execute(ctx, selectedNode.NodeID, reqPayload, responseWriter)

		// 8. If successful, break
		if execErr == nil {
			break
		}

		slog.Warn("[YunwuHypervisor] Runtime request failed on selected Yunwu node",
			"node_id", selectedNode.NodeID,
			"account_name", selectedNode.AccountName,
			"session_id", sess.ID,
			"attempt", attempt,
			"error", execErr,
		)
		_, _ = yh.store.LogEvent(ctx, ownerID, &sess.ID, &selectedNode.NodeID, "RequestFailed", map[string]any{"error": execErr.Error(), "attempt": attempt, "pool_type": "yunwu"})
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if responseWriter.Committed() {
			_, _ = yh.store.LogEvent(ctx, ownerID, &sess.ID, &selectedNode.NodeID, "RequestStreamInterrupted", map[string]any{"retry_suppressed": true, "pool_type": "yunwu"})
			return nil, execErr
		}

		if attempt == maxRetries {
			slog.Error("[YunwuHypervisor] Max retries reached, failing request", "session_id", sess.ID)
			break
		}

		// Rebind on failure
		if yh.cfg.EnableBackup {
			_ = yh.bm.CreateSnapshot(ctx, sess.ID, &backup.ProjectSnapshot{SessionState: sess})
		}
		_, _ = yh.store.LogEvent(ctx, ownerID, &sess.ID, &selectedNode.NodeID, "RebindingStarted", map[string]any{"reason": execErr.Error(), "pool_type": "yunwu"})
		observability.Default.Rebinding()
		reboundNode, rebindErr := yh.rm.HandleYunwuFailure(ctx, selectedNode.NodeID, execErr, nodes)
		if rebindErr == nil && reboundNode != nil {
			slog.Info("[YunwuHypervisor] Rebinding succeeded, retrying",
				"session_id", sess.ID,
				"rebound_node_id", reboundNode.NodeID,
				"rebound_account_name", reboundNode.AccountName,
			)
			selectedNode = reboundNode
			_, _ = yh.store.LogEvent(ctx, ownerID, &sess.ID, &reboundNode.NodeID, "RebindingSucceeded", map[string]any{"pool_type": "yunwu"})

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
			slog.Error("[YunwuHypervisor] Rebinding failed or no alternative nodes", "rebind_error", rebindErr)
			_, _ = yh.store.LogEvent(ctx, ownerID, &sess.ID, nil, "RebindingFailed", map[string]any{"reason": fmt.Sprint(rebindErr), "pool_type": "yunwu"})
			observability.Default.RebindingFailed()
			break // No point in retrying if no alternatives
		}
	}

	if execErr != nil {
		slog.Error("[YunwuHypervisor] Final execution failed",
			"node_id", selectedNode.NodeID,
			"elapsed_ms", time.Since(execStart).Milliseconds(),
			"error", execErr,
		)
		return nil, execErr
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
		var parsedReq struct {
			Model string `json:"model"`
		}
		_ = json.Unmarshal(reqPayload, &parsedReq)
		modelName := parsedReq.Model

		inputPrice, outputPrice, fallback := yh.resolvePrice(ctx, selectedNode.NodeID, modelName)
		if fallback {
			observability.Default.YunwuFallbackPricing()
		}

		totalTokens := meta.InputTokens + meta.OutputTokens
		costUnits := totalTokens
		if totalTokens > 0 {
			billedMicroUSD := CalculateBilledMicroUSD(meta.InputTokens, meta.OutputTokens, inputPrice, outputPrice)
			_, _ = yh.sw.ReconcileReservation(ctx, reservationNodeID, requestID, billedMicroUSD)
			reservationOpen = false
			_ = yh.qm.ConsumeQuota(ctx, ownerID, "yunwu", int64(costUnits))
			_ = yh.qm.ConsumeMicroUSD(ctx, ownerID, billedMicroUSD)
			_, _ = yh.store.RecordUsageDetails(ctx, ownerID, &sess.ID, &selectedNode.NodeID, meta.InputTokens, meta.OutputTokens, int64(costUnits), "yunwu", "yunwu", modelName, "micro_usd", billedMicroUSD)
			observability.Default.AddUsage(int64(costUnits))
			_, _ = yh.store.LogEvent(ctx, ownerID, &sess.ID, &selectedNode.NodeID, "UsageRecorded", map[string]any{"input_tokens": meta.InputTokens, "output_tokens": meta.OutputTokens, "cost_units": costUnits, "pool_type": "yunwu"})
			slog.Info("[YunwuHypervisor] Recorded Yunwu request usage",
				"hypervisor_mode", "yunwu",
				"owner_id", ownerID,
				"session_id", sess.ID,
				"node_id", selectedNode.NodeID,
				"input_tokens", meta.InputTokens,
				"output_tokens", meta.OutputTokens,
				"cost_units", costUnits,
			)
		}
	}

	slog.Info("[YunwuHypervisor] Request succeeded",
		"hypervisor_mode", "yunwu",
		"owner_id", ownerID,
		"session_id", sess.ID,
		"node_id", selectedNode.NodeID,
		"account_name", selectedNode.AccountName,
		"platform", platform,
		"model", requestedModel,
		"latency_ms", time.Since(execStart).Milliseconds(),
	)
	_, _ = yh.store.LogEvent(ctx, ownerID, &sess.ID, &selectedNode.NodeID, "RequestSucceeded", map[string]any{"latency_ms": time.Since(execStart).Milliseconds(), "pool_type": "yunwu"})

	return meta, nil
}

func CalculateBilledMicroUSD(inputTokens, outputTokens int64, inputPriceUSDPer1K, outputPriceUSDPer1K float64) int64 {
	return int64(math.Ceil(float64(inputTokens)*inputPriceUSDPer1K*1000 + float64(outputTokens)*outputPriceUSDPer1K*1000))
}

func estimateRequestMicroUSD(payload []byte, inputPriceUSDPer1K, outputPriceUSDPer1K float64) int64 {
	inputTokens := int64((len(payload) + 3) / 4)
	outputTokens := int64(4096)
	var request struct {
		MaxTokens       *int64 `json:"max_tokens"`
		MaxOutputTokens *int64 `json:"max_output_tokens"`
	}
	if json.Unmarshal(payload, &request) == nil {
		if request.MaxTokens != nil && *request.MaxTokens >= 0 {
			outputTokens = *request.MaxTokens
		}
		if request.MaxOutputTokens != nil && *request.MaxOutputTokens >= 0 {
			outputTokens = *request.MaxOutputTokens
		}
	}
	return CalculateBilledMicroUSD(inputTokens, outputTokens, inputPriceUSDPer1K, outputPriceUSDPer1K)
}

func (yh *YunwuHypervisor) getAllYunwuNodes(ctx context.Context) ([]health.ExecutionNode, error) {
	var dbAccounts []*ent.Account

	if yh.store != nil && yh.store.DB() != nil {
		dbAccounts, _ = yh.store.DB().Account.Query().
			Where(account.PlatformEqualFold("yunwu"), account.Status("active"), account.Schedulable(true)).
			All(ctx)
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
		snap, err := yh.sw.Snapshot(ctx, node.NodeID, time.Duration(yh.cfg.YunwuBalanceStaleSeconds)*time.Second)
		if err == nil && snap.Status == "fresh" {
			allocatableMicroUSD := snap.EstimatedAvailableMicroUSD - snap.ReserveMicroUSD
			allocatable := float64(allocatableMicroUSD) / 1_000_000
			if allocatable > 0 {
				credits[node.NodeID] = allocatable
			}
		} else if err == nil && snap.Status == "stale" {
			observability.Default.YunwuBalanceBecameStale()
			_, _ = yh.store.LogEvent(ctx, "system", nil, &node.NodeID, "YunwuBalanceStale", map[string]any{"synchronized_at": snap.SynchronizedAt})
		}
	}
	return credits
}

func (yh *YunwuHypervisor) startBalanceSync(ctx context.Context) {
	syncAll := func() {
		if yh.store == nil || yh.store.DB() == nil {
			return
		}
		accounts, err := yh.store.DB().Account.Query().Where(account.PlatformEqualFold("yunwu"), account.Status("active")).All(ctx)
		if err != nil {
			return
		}
		for _, acc := range accounts {
			creds, decErr := credentials.DecryptMap(acc.Credentials, yh.decryptor)
			if decErr != nil {
				observability.Default.YunwuBalanceSyncFailed()
				_, _ = yh.store.LogEvent(ctx, "system", nil, nil, "YunwuBalanceSyncFailed", map[string]any{"node_id": fmt.Sprint(acc.ID), "error": decErr.Error()})
				continue
			}
			apiKey, _ := creds["api_key"].(string)
			baseURL, _ := creds["base_url"].(string)
			if apiKey == "" {
				continue
			}
			node := &health.ExecutionNode{NodeID: fmt.Sprint(acc.ID), AccountName: acc.Name, Provider: "yunwu"}
			if _, syncErr := yh.sw.SyncNodeStats(ctx, node, apiKey, baseURL); syncErr != nil {
				observability.Default.YunwuBalanceSyncFailed()
				_, _ = yh.store.LogEvent(ctx, "system", nil, &node.NodeID, "YunwuBalanceSyncFailed", map[string]any{"error": syncErr.Error()})
			}
		}
	}
	go func() {
		syncAll()
		ticker := time.NewTicker(time.Duration(yh.cfg.YunwuBalanceSyncSeconds) * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				syncAll()
			}
		}
	}()
}

func (yh *YunwuHypervisor) resolvePrice(ctx context.Context, nodeID, model string) (float64, float64, bool) {
	in, out := yh.cfg.YunwuDefaultInputUSDPer1K, yh.cfg.YunwuDefaultOutputUSDPer1K
	if yh.store.DB() == nil || model == "" {
		return in, out, true
	}
	accID, _ := strconv.ParseInt(nodeID, 10, 64)
	acc, err := yh.store.DB().Account.Get(ctx, accID)
	if err != nil {
		return in, out, true
	}
	var vendorID string
	if acc.Extra != nil {
		vendorID, _ = acc.Extra["yunwu_vendor_id"].(string)
	}
	if vendorID != "" {
		if rows, err := yh.store.DB().QueryContext(ctx, `SELECT input_price,output_price FROM ash_vendor_models WHERE vendor_id=$1 AND model_name=$2 AND is_available=true`, vendorID, model); err == nil {
			defer rows.Close()
			if rows.Next() && rows.Scan(&in, &out) == nil {
				return in, out, false
			}
		}
	}
	creds, _ := credentials.DecryptMap(acc.Credentials, yh.decryptor)
	base, _ := creds["base_url"].(string)
	base = strings.TrimRight(base, "/")
	if base != "" {
		if rows, err := yh.store.DB().QueryContext(ctx, `SELECT m.input_price,m.output_price FROM ash_vendor_models m JOIN ash_vendors v ON v.id=m.vendor_id WHERE rtrim(v.url,'/')=$1 AND m.model_name=$2 AND m.is_available=true`, base, model); err == nil {
			defer rows.Close()
			if rows.Next() && rows.Scan(&in, &out) == nil {
				return in, out, false
			}
		}
	}
	return in, out, true
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
	if apiKey == "" {
		id, parseErr := strconv.ParseInt(nodeID, 10, 64)
		if parseErr != nil {
			return nil, parseErr
		}
		acc, getErr := yh.store.DB().Account.Get(ctx, id)
		if getErr != nil {
			return nil, getErr
		}
		decrypted, decErr := credentials.DecryptMap(acc.Credentials, yh.decryptor)
		if decErr != nil {
			return nil, decErr
		}
		apiKey, _ = decrypted["api_key"].(string)
		baseURL, _ = decrypted["base_url"].(string)
	}
	if apiKey == "" {
		return nil, fmt.Errorf("Yunwu API key is not configured")
	}
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

func (yh *YunwuHypervisor) BalanceSnapshot(ctx context.Context, nodeID string) (*BalanceSnapshot, error) {
	return yh.sw.Snapshot(ctx, nodeID, time.Duration(yh.cfg.YunwuBalanceStaleSeconds)*time.Second)
}
func (yh *YunwuHypervisor) BalanceReservePercent() int { return yh.cfg.YunwuBalanceReservePercent }

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

func compilerContextFromYunwuSession(ctx *session.ExecutionContext) compiler.ExecutionContext {
	result := compiler.ExecutionContext{
		SystemPrompt:  ctx.SystemPrompt,
		Summary:       ctx.Summary,
		PinnedFacts:   ctx.PinnedFacts,
		TerminalState: ctx.TerminalState,
		ToolState:     toolCallsToString(ctx.ToolState),
	}
	for _, message := range ctx.RecentMessages {
		result.RecentMessages = append(result.RecentMessages, compiler.Message{Role: message.Role, Content: message.Content})
	}
	return result
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
		SystemPrompt:  sess.Context.SystemPrompt,
		Summary:       sess.Context.Summary,
		PinnedFacts:   sess.Context.PinnedFacts,
		TerminalState: sess.Context.TerminalState,
		ToolState:     toolCallsToString(sess.Context.ToolState),
	}
	if sess.Context.ProjectState != nil {
		compCtx.ProjectState = &compiler.ProjectSnapshot{
			WorkspaceID: sess.Context.ProjectState.WorkspaceID,
			GitBranch:   sess.Context.ProjectState.GitBranch,
			GitCommit:   sess.Context.ProjectState.GitCommit,
			DirtyFiles:  sess.Context.ProjectState.DirtyFiles,
			OpenFiles:   sess.Context.ProjectState.OpenFiles,
			LastCommand: sess.Context.ProjectState.LastCommand,
			SnapshotRef: sess.Context.ProjectState.SnapshotRef,
		}
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

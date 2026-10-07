package yunwu

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/account"
	"github.com/Wei-Shaw/sub2api/ent/accountgroup"
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
	prober    *YunwuHealthProber
	cfg       *config.HypervisorConfig
	decryptor credentials.Cipher
	pm        PriceResolver
}

type commitTrackingWriter struct {
	w io.Writer
	n int64
}

type responseCaptureWriter struct {
	*commitTrackingWriter
	buf bytes.Buffer
}

func (w *responseCaptureWriter) Write(p []byte) (int, error) {
	if w.buf.Len() < 64*1024 {
		remaining := 64*1024 - w.buf.Len()
		if remaining > len(p) {
			remaining = len(p)
		}
		_, _ = w.buf.Write(p[:remaining])
	}
	return w.commitTrackingWriter.Write(p)
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

func (w *commitTrackingWriter) Header() http.Header {
	if hw, ok := w.w.(interface{ Header() http.Header }); ok {
		return hw.Header()
	}
	return make(http.Header)
}

func (w *commitTrackingWriter) WriteHeader(statusCode int) {
	if hw, ok := w.w.(interface{ WriteHeader(int) }); ok {
		hw.WriteHeader(statusCode)
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
		sch:       NewYunwuScheduler(cfg.YunwuRequireFreshBalance),
		rt:        rt,
		rm:        NewYunwuRebindingManager(store, cfg),
		comp:      compiler.NewCompiler(),
		summary:   compiler.NewSummaryWorker(store),
		bm:        backup.NewBackupManager(store),
		hc:        NewYunwuHealthController(store),
		sw:        NewStatsWorker(store, cfg),
		cfg:       cfg,
		decryptor: decryptor,
		pm:        NewProviderPriceManager(store, decryptor, nil),
	}

	if yh.sw != nil && yh.sw.Notifier() != nil {
		yh.hc.SetNotifier(yh.sw.Notifier())
		yh.rm.SetNotifier(yh.sw.Notifier())
	}
	yh.hc.Start(context.Background(), 10*time.Second, func(ctx context.Context) []health.ExecutionNode {
		nodes, _ := yh.getAllYunwuNodes(ctx)
		return nodes
	})
	yh.startBalanceSync(context.Background())
	if yh.sw != nil && yh.sw.Bot() != nil {
		yh.sw.Bot().Start(context.Background())
	}
	yh.prober = NewYunwuHealthProber(store, cfg, decryptor, yh.Notifier())
	yh.prober.Start(context.Background(), time.Duration(cfg.YunwuHealthProbeSeconds)*time.Second)

	slog.Info("[YunwuHypervisor] Initialized dedicated Yunwu Engine")
	return yh
}

// Notifier returns the active TelegramNotifier instance.
func (yh *YunwuHypervisor) Notifier() *TelegramNotifier {
	if yh != nil && yh.sw != nil && yh.sw.Notifier() != nil {
		return yh.sw.Notifier()
	}
	return GetDefaultNotifier()
}

// StatsWorker returns the underlying StatsWorker.
func (yh *YunwuHypervisor) StatsWorker() *StatsWorker {
	if yh != nil {
		return yh.sw
	}
	return nil
}

// SetPriceResolver allows injecting or replacing the pricing resolver
func (yh *YunwuHypervisor) SetPriceResolver(pr PriceResolver) {
	yh.pm = pr
}

// PriceManager returns the underlying ProviderPriceManager if configured
func (yh *YunwuHypervisor) PriceManager() *ProviderPriceManager {
	if ppm, ok := yh.pm.(*ProviderPriceManager); ok {
		return ppm
	}
	return nil
}

func (yh *YunwuHypervisor) resolveNodePrice(ctx context.Context, node *health.ExecutionNode, model string) (float64, float64, bool) {
	if node == nil {
		return 0, 0, false
	}
	if yh.pm != nil {
		return yh.pm.ResolveNodePrice(ctx, node, model)
	}
	return yh.resolvePrice(ctx, node.NodeID, model)
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
	if notifier := yh.Notifier(); notifier != nil && requestedModel != "" {
		notifier.RecordModelRequest(ctx, requestedModel)
	}
	if requestedModel == "" {
		requestedModel = modelNameFromRequest(ctx)
	}

	slog.Info("[YunwuHypervisor] Handling Yunwu request",
		"hypervisor_mode", "yunwu",
		"owner_id", ownerID,
		"conversation_id", conversationID,
		"platform", platform,
		"model", requestedModel,
		"payload_size", len(reqPayload),
	)

	// 0. Resolve Priority
	userPriority := 1
	if yh.qm != nil {
		if ua, err := yh.qm.GetUserAccess(ctx, ownerID, "yunwu"); err == nil && ua != nil {
			userPriority = ua.Priority
		}
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
	operationNodeID := ""
	if operation := operationNameFromRequest(ctx); operation != "" {
		if nodeID, lookupErr := yh.store.Redis().Get(ctx, yunwuOperationNodeKey(operation)).Result(); lookupErr == nil && nodeID != "" {
			activeLeaseNodeID = nodeID
			operationNodeID = nodeID
		}
	}
	if interactionID := interactionIDFromRequest(ctx); interactionID != "" {
		if nodeID, lookupErr := yh.store.Redis().Get(ctx, yunwuInteractionNodeKey(interactionID)).Result(); lookupErr == nil && nodeID != "" {
			activeLeaseNodeID = nodeID
			operationNodeID = nodeID
		}
	}
	if resourceName := statefulResourceNameFromRequest(ctx); resourceName != "" {
		if nodeID, lookupErr := yh.store.Redis().Get(ctx, yunwuResourceNodeKey(resourceName)).Result(); lookupErr == nil && nodeID != "" {
			activeLeaseNodeID = nodeID
			operationNodeID = nodeID
		}
	}

	// 4. Load Yunwu nodes and runtime credits
	nodes, err := yh.getAllYunwuNodes(ctx)
	if err != nil {
		slog.Error("[YunwuHypervisor] Failed to fetch Yunwu nodes", "error", err)
		return nil, err
	}

	// 4.1 Filter nodes by requested model family AND pricing availability
	requestedFamily := ClassifyModelFamily(requestedModel)
	var compatibleNodes []health.ExecutionNode
	familyMatchedNodes := 0
	nodePrices := make(map[string]float64)

	for _, node := range nodes {
		if !NodeSupportsFamily(node.Capabilities, requestedFamily) {
			continue
		}
		familyMatchedNodes++
		nodeCopy := node
		inPrice, outPrice, hasPrice := yh.resolveNodePrice(ctx, &nodeCopy, requestedModel)
		if !hasPrice {
			slog.Debug("[YunwuHypervisor] Candidate node skipped: no pricing found on provider",
				"node_id", node.NodeID,
				"account_name", node.AccountName,
				"provider", node.Provider,
				"model", requestedModel,
			)
			continue
		}
		compatibleNodes = append(compatibleNodes, node)
		nodePrices[node.NodeID] = inPrice + outPrice
	}

	if len(compatibleNodes) == 0 {
		if notifier := yh.Notifier(); notifier != nil {
			reason := "Нет доступных узлов с ценообразованием в пуле"
			if familyMatchedNodes == 0 {
				reason = "Семейство модели не поддерживается доступными узлами"
			}
			_, _ = notifier.NotifyBothProvidersUnavailable(ctx, requestedModel, "primary", "backup", reason)
		}
		if familyMatchedNodes == 0 {
			slog.Warn("[YunwuHypervisor] No candidate nodes support requested model family",
				"requested_model", requestedModel,
				"family", requestedFamily,
				"total_nodes", len(nodes),
			)
			return nil, ErrNoCompatibleNodes
		}
		slog.Warn("[YunwuHypervisor] No candidate nodes offer pricing for requested model",
			"requested_model", requestedModel,
			"family", requestedFamily,
			"family_nodes", familyMatchedNodes,
		)
		return nil, ErrNoPricingAvailable
	}
	nodes = compatibleNodes

	// 4.1 Assign Primary/Backup by price: the cheapest provider for the requested
	// model is Primary, the next cheapest is Backup. Nodes are ordered by price so
	// the scheduler (and failover/rebinding) always prefers the cheapest route.
	// An explicit administrator route (4.2) overrides this.
	sortNodesByPrice(nodes, nodePrices)
	primaryID, backupID := pricePrimaryBackup(nodes, nodePrices)
	if primaryID != "" {
		slog.Info("[YunwuHypervisor] Price-based Primary/Backup route",
			"model", requestedModel,
			"primary_node", primaryID,
			"primary_price", nodePrices[primaryID],
			"backup_node", backupID,
			"backup_price", nodePrices[backupID],
		)
	}

	// 4.2 Apply explicit Primary/Backup routing when the gateway provided an
	// ordered route for the requested model. This makes administrator
	// promotions effective for unofficial providers.
	routingApplied := false
	if routing := ModelRoutingFromContext(ctx); len(routing) > 0 {
		byID := make(map[string]health.ExecutionNode, len(nodes))
		for _, n := range nodes {
			byID[n.NodeID] = n
		}
		ordered := make([]health.ExecutionNode, 0, len(routing))
		matched := 0
		for _, id := range routing {
			if n, ok := byID[fmt.Sprintf("%d", id)]; ok {
				ordered = append(ordered, n)
				matched++
			}
		}
		if matched > 0 {
			nodes = ordered
			routingApplied = true
			slog.Info("[YunwuHypervisor] Applying explicit Primary/Backup model routing",
				"routing", describeRouting(routing),
				"matched_nodes", matched,
				"model", requestedModel,
			)
		} else {
			slog.Warn("[YunwuHypervisor] Explicit model routing configured but no candidate matched, falling back to pool",
				"routing", routing,
				"model", requestedModel,
			)
		}
	}

	if activeLeaseNodeID != "" {
		leaseValid := false
		for _, n := range nodes {
			if n.NodeID == activeLeaseNodeID {
				leaseValid = true
				break
			}
		}
		if !leaseValid {
			activeLeaseNodeID = ""
		}
	}

	if operationNodeID != "" {
		bound := nodes[:0]
		for _, node := range nodes {
			if node.NodeID == operationNodeID {
				bound = append(bound, node)
			}
		}
		nodes = bound
	}
	states := yh.getAllYunwuNodeStates(ctx, nodes)
	credits := yh.getAllYunwuNodeCredits(ctx, nodes)
	systemState := yh.hc.GetSystemState(ctx)

	slog.Info("[YunwuHypervisor] Evaluated Yunwu pool status",
		"hypervisor_mode", "yunwu",
		"total_nodes", len(nodes),
		"system_state", systemState,
	)

	// 5. Select the best node. When an explicit Primary/Backup route was provided,
	// honor its order strictly (Primary first, then Backup, ...), otherwise fall
	// back to cheapest-first pool scheduling.
	var selectedNode *health.ExecutionNode
	if routingApplied {
		for i := range nodes {
			n := &nodes[i]
			if n.State == "unavailable" || n.State == "cooldown" {
				continue
			}
			if st := states[n.NodeID]; st != nil && st.CooldownUntil != nil && time.Now().Before(*st.CooldownUntil) {
				continue
			}
			selectedNode = n
			break
		}
		if selectedNode == nil {
			slog.Warn("[YunwuHypervisor] All routed Primary/Backup nodes unavailable, falling back to pool scheduling",
				"model", requestedModel)
		}
	}
	if selectedNode == nil {
		selectedNode, err = yh.sch.SelectNode(ctx, nodes, states, credits, userPriority, activeLeaseNodeID, systemState, nodePrices)
		if err != nil {
			slog.Error("[YunwuHypervisor] YunwuScheduler selection failed", "error", err, "total_nodes", len(nodes))
			return nil, err
		}
	}
	media := classifyMediaRequest(ctx, requestedModel, reqPayload)
	admittedInputPrice, admittedOutputPrice, hasAdmittedPrice := yh.resolveNodePrice(ctx, selectedNode, requestedModel)
	if !hasAdmittedPrice {
		return nil, ErrNoPricingAvailable
	}
	estimatedMicroUSD := int64(0)
	if media.Kind != "" {
		estimatedMicroUSD, _ = yh.resolveMediaPrice(ctx, selectedNode.NodeID, requestedModel, media)
	} else {
		estimatedMicroUSD = estimateRequestMicroUSD(reqPayload, admittedInputPrice, admittedOutputPrice)
	}
	// Even non-billable control-plane calls atomically validate that the chosen
	// node still has allocatable balance. The 1 micro-USD hold is refunded.
	reservationEstimate := estimatedMicroUSD
	if reservationEstimate <= 0 {
		reservationEstimate = 1
	}
	requestID := fmt.Sprintf("%s-%d", sess.ID, time.Now().UnixNano())
	reserved := false
	if yh.cfg.YunwuRequireFreshBalance {
		var reserveErr error
		reserved, reserveErr = yh.sw.Reserve(ctx, selectedNode.NodeID, requestID, reservationEstimate, 10*time.Minute)
		if reserveErr != nil {
			return nil, reserveErr
		}
		if !reserved {
			return nil, ErrNodeBalanceExhausted
		}
	}
	reservationNodeID := selectedNode.NodeID
	reservationOpen := reserved
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

	modelName := requestedModel
	var parsedReq struct {
		Model string `json:"model"`
	}
	_ = json.Unmarshal(reqPayload, &parsedReq)
	if parsedReq.Model != "" {
		modelName = parsedReq.Model
	}

	var meta *runtime.ResponseMetadata
	var execErr error
	maxRetries := yh.cfg.MaxRetry
	if !yh.cfg.EnableRebinding {
		maxRetries = 0
	}
	responseWriter := &responseCaptureWriter{commitTrackingWriter: &commitTrackingWriter{w: stream}}

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

		effectivePayload := yh.prepareNodePayload(ctx, selectedNode, reqPayload)
		meta, execErr = yh.rt.Execute(ctx, selectedNode.NodeID, effectivePayload, responseWriter)

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

		if notifier := yh.Notifier(); notifier != nil {
			errStr := strings.ToLower(execErr.Error())
			// Event 2: Upstream API Key invalid/disabled
			if strings.Contains(errStr, "401") || strings.Contains(errStr, "unauthorized") ||
				strings.Contains(errStr, "invalid_api_key") || strings.Contains(errStr, "invalid api key") ||
				strings.Contains(errStr, "account_deactivated") || strings.Contains(errStr, "key disabled") ||
				strings.Contains(errStr, "authentication_error") {
				maskedKey := "***"
				if selectedNode != nil {
					maskedKey = maskKey(selectedNode.NodeID)
				}
				_, _ = notifier.NotifyUpstreamKeyInvalid(
					ctx,
					selectedNode.Provider,
					selectedNode.NodeID,
					selectedNode.AccountName,
					maskedKey,
					execErr.Error(),
				)
			}

			// Event 6: 429 / 5xx error spike
			code := 0
			if strings.Contains(errStr, "429") || strings.Contains(errStr, "too many requests") {
				code = 429
			} else if strings.Contains(errStr, "500") || strings.Contains(errStr, "502") ||
				strings.Contains(errStr, "503") || strings.Contains(errStr, "504") {
				code = 500
			}
			if code > 0 {
				notifier.RecordError(ctx, code, selectedNode.Provider)
				_, _ = notifier.CheckAndNotifyErrorSpike(ctx, selectedNode.Provider)
			}
		}

		// Update node runtime state for production error rate and metrics
		if errState, sErr := yh.nm.GetNodeRuntimeState(ctx, selectedNode.NodeID); sErr == nil && errState != nil {
			errState.TotalRequests++
			errState.SuccessRate = float64(errState.SuccessfulRequests) / float64(errState.TotalRequests)
			nowErr := time.Now()
			errState.LastErrorAt = &nowErr
			_ = yh.nm.SaveNodeRuntimeState(ctx, errState)
		}

		// If this failed attempt consumed tokens upstream, record it as a platform failover loss
		// (actual_cost = 0, so user is NOT charged, while upstream_cost is captured for margin analytics)
		if meta != nil && (meta.InputTokens > 0 || meta.OutputTokens > 0) {
			yh.recordUpstreamFailoverLoss(ctx, ownerID, sess.ID, *selectedNode, modelName, meta, attempt, stream != nil)
		}

		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if responseWriter.Committed() {
			_, _ = yh.store.LogEvent(ctx, ownerID, &sess.ID, &selectedNode.NodeID, "RequestStreamInterrupted", map[string]any{"retry_suppressed": true, "pool_type": "yunwu"})
			return nil, execErr
		}

		// Client-side request errors (malformed parameters, unsupported fields,
		// content policy, ...) must NOT trigger failover/rebinding: retrying the
		// same bad request on a Backup provider wastes upstream capacity, cools
		// down a healthy node and can cause uncontrolled spend. Fail closed with
		// the original upstream error.
		if isClientRequestError(execErr) {
			slog.Warn("[YunwuHypervisor] Client request error detected, suppressing failover",
				"node_id", selectedNode.NodeID,
				"model", requestedModel,
				"error", execErr,
			)
			_, _ = yh.store.LogEvent(ctx, ownerID, &sess.ID, &selectedNode.NodeID, "RequestClientError", map[string]any{
				"error":               execErr.Error(),
				"failover_suppressed": true,
				"pool_type":           "yunwu",
			})
			return nil, execErr
		}

		if attempt == maxRetries {
			slog.Error("[YunwuHypervisor] Max retries reached, failing request", "session_id", sess.ID)
			if notifier := yh.Notifier(); notifier != nil {
				_, _ = notifier.NotifyBothProvidersUnavailable(
					ctx,
					requestedModel,
					selectedNode.Provider,
					"backup_pool",
					fmt.Sprintf("Исчерпаны все попытки выполнения (%d): %v", maxRetries, execErr),
				)
			}
			break
		}

		// Event 5: Record failover and check failover rate
		if notifier := yh.Notifier(); notifier != nil && requestedModel != "" {
			notifier.RecordModelFailover(ctx, requestedModel)
			_, _ = notifier.CheckAndNotifyFailoverRate(ctx, requestedModel)
		}

		// Rebind on failure
		if yh.cfg.EnableBackup {
			_ = yh.bm.CreateSnapshot(ctx, sess.ID, &backup.ProjectSnapshot{SessionState: sess})
		}
		_, _ = yh.store.LogEvent(ctx, ownerID, &sess.ID, &selectedNode.NodeID, "RebindingStarted", map[string]any{"reason": execErr.Error(), "pool_type": "yunwu"})
		observability.Default.Rebinding()
		reboundNode, rebindErr := yh.rm.HandleYunwuFailure(ctx, sess.ID, selectedNode.NodeID, execErr, nodes, userPriority, credits, systemState, nodePrices)
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
			if notifier := yh.Notifier(); notifier != nil {
				_, _ = notifier.NotifyBothProvidersUnavailable(
					ctx,
					requestedModel,
					selectedNode.Provider,
					"backup_candidates",
					fmt.Sprintf("Ребиндинг не удался: %v", rebindErr),
				)
			}
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
	// Expose the executing node so the gateway layer can bill the end user
	// against the provider/account that actually served the request.
	if meta != nil {
		meta.NodeID = selectedNode.NodeID
		if media.Kind != "" && media.Kind != "media_control" && estimatedMicroUSD > 0 {
			meta.MediaUsageUnit = media.Kind
			meta.MediaUnits = media.Units
			meta.MediaCostMicroUSD = estimatedMicroUSD
		}
	}
	if operation := operationNameFromResponse(responseWriter.buf.Bytes()); operation != "" {
		_ = yh.store.Redis().Set(ctx, yunwuOperationNodeKey(operation), selectedNode.NodeID, 7*24*time.Hour).Err()
	}
	if interactionID := interactionIDFromResponse(ctx, responseWriter.buf.Bytes()); interactionID != "" {
		_ = yh.store.Redis().Set(ctx, yunwuInteractionNodeKey(interactionID), selectedNode.NodeID, 7*24*time.Hour).Err()
	}
	if token := ephemeralTokenFromResponse(ctx, responseWriter.buf.Bytes()); token != "" {
		_ = yh.store.Redis().Set(ctx, yunwuUpstreamTokenNodeKey(token), selectedNode.NodeID, 20*time.Hour).Err()
	}
	if resourceName := statefulResourceNameFromResponse(ctx, responseWriter.buf.Bytes()); resourceName != "" {
		_ = yh.store.Redis().Set(ctx, yunwuResourceNodeKey(resourceName), selectedNode.NodeID, 30*24*time.Hour).Err()
	}

	latencyMs := time.Since(execStart).Milliseconds()

	// Update node runtime metrics
	state := states[selectedNode.NodeID]
	if state == nil {
		state = &health.NodeRuntimeState{NodeID: selectedNode.NodeID}
	}
	state.TotalRequests++
	state.SuccessfulRequests++
	state.SuccessRate = float64(state.SuccessfulRequests) / float64(state.TotalRequests)
	state.AvgLatencyMs = (state.AvgLatencyMs + latencyMs) / 2
	if meta != nil {
		state.UsedTokens += int64(meta.InputTokens + meta.OutputTokens)
	}
	state.ConsecutiveSuccesses++
	nowSucc := time.Now()
	state.LastSuccessAt = &nowSucc
	_ = yh.nm.SaveNodeRuntimeState(ctx, state)

	// 9. Record usage
	if meta != nil {
		var parsedReq struct {
			Model string `json:"model"`
		}
		_ = json.Unmarshal(reqPayload, &parsedReq)
		modelName := parsedReq.Model
		if modelName == "" {
			modelName = requestedModel
		}

		inputPrice, outputPrice, hasPrice := yh.resolveNodePrice(ctx, selectedNode, modelName)
		if !hasPrice && hasAdmittedPrice {
			// Fallback to pre-admission pricing resolved when node was selected
			inputPrice, outputPrice, hasPrice = admittedInputPrice, admittedOutputPrice, true
		}
		if !hasPrice {
			slog.Warn("[YunwuHypervisor] Warning: price not found after execution, skipping user charge",
				"node_id", selectedNode.NodeID,
				"model", modelName,
			)
		} else {
			totalTokens := meta.InputTokens + meta.OutputTokens
			costUnits := totalTokens
			if media.Kind != "" {
				_, _ = yh.sw.ReconcileReservation(ctx, reservationNodeID, requestID, estimatedMicroUSD)
				reservationOpen = false
				if estimatedMicroUSD > 0 {
					_, _ = yh.store.RecordUsageDetails(ctx, ownerID, &sess.ID, &selectedNode.NodeID, 0, 0, media.Units, "yunwu", "yunwu", modelName, media.Kind, estimatedMicroUSD)
					observability.Default.AddUsage(media.Units)
					_, _ = yh.store.LogEvent(ctx, ownerID, &sess.ID, &selectedNode.NodeID, "UsageRecorded", map[string]any{"usage_unit": media.Kind, "units": media.Units, "billed_micro_usd": estimatedMicroUSD, "pool_type": "yunwu"})
				}
			} else if totalTokens > 0 {
				billedMicroUSD := CalculateBilledMicroUSD(meta.InputTokens, meta.OutputTokens, inputPrice, outputPrice)
				_, _ = yh.sw.ReconcileReservation(ctx, reservationNodeID, requestID, billedMicroUSD)
				reservationOpen = false
				_, _ = yh.store.RecordUsageDetails(ctx, ownerID, &sess.ID, &selectedNode.NodeID, meta.InputTokens, meta.OutputTokens, int64(costUnits), "yunwu", "yunwu", modelName, "micro_usd", billedMicroUSD)
				observability.Default.AddUsage(int64(costUnits))
				_, _ = yh.store.LogEvent(ctx, ownerID, &sess.ID, &selectedNode.NodeID, "UsageRecorded", map[string]any{"input_tokens": meta.InputTokens, "output_tokens": meta.OutputTokens, "cost_units": costUnits, "billed_micro_usd": billedMicroUSD, "pool_type": "yunwu"})
				slog.Info("[YunwuHypervisor] Recorded Yunwu request usage",
					"hypervisor_mode", "yunwu",
					"owner_id", ownerID,
					"session_id", sess.ID,
					"node_id", selectedNode.NodeID,
					"input_tokens", meta.InputTokens,
					"output_tokens", meta.OutputTokens,
					"cost_units", costUnits,
					"billed_micro_usd", billedMicroUSD,
				)
			} else {
				// Settle reservation without charging token overage
				_, _ = yh.sw.ReconcileReservation(ctx, reservationNodeID, requestID, estimatedMicroUSD)
				reservationOpen = false
				_, _ = yh.store.RecordUsageDetails(ctx, ownerID, &sess.ID, &selectedNode.NodeID, 0, 0, 0, "yunwu", "yunwu", modelName, "estimated_micro_usd", estimatedMicroUSD)
			}
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

func (yh *YunwuHypervisor) recordUpstreamFailoverLoss(
	ctx context.Context,
	ownerID string,
	sessionID string,
	node health.ExecutionNode,
	modelName string,
	meta *runtime.ResponseMetadata,
	attempt int,
	streaming bool,
) {
	if meta == nil || (meta.InputTokens <= 0 && meta.OutputTokens <= 0) {
		return
	}

	inPrice, outPrice, hasPrice := yh.resolveNodePrice(ctx, &node, modelName)
	if !hasPrice {
		inPrice, outPrice, hasPrice = yh.resolveNodePrice(ctx, &node, StripVendorPrefix(modelName))
	}
	if !hasPrice {
		return
	}

	inCostUSD := float64(meta.InputTokens) * (inPrice / 1000.0)
	outCostUSD := float64(meta.OutputTokens) * (outPrice / 1000.0)
	totalCostUSD := inCostUSD + outCostUSD

	var accountMapping map[string]string
	if yh.store != nil && yh.store.DB() != nil {
		if accID, err := strconv.ParseInt(node.NodeID, 10, 64); err == nil && accID > 0 {
			if dbAcc, err := yh.store.DB().Account.Get(ctx, accID); err == nil && dbAcc != nil {
				accountMapping = ParseAccountModelMapping(dbAcc.Extra)
			}
		}
	}
	upstreamModel := ResolveUpstreamModelID(node.Provider, modelName, accountMapping)

	lossReqID := fmt.Sprintf("%s-failover-loss-%d-%d", sessionID, attempt, time.Now().UnixNano())
	userID, _ := strconv.ParseInt(ownerID, 10, 64)
	accountID, _ := strconv.ParseInt(node.NodeID, 10, 64)

	if yh.store != nil {
		// 1. Record to usage_logs with actual_cost = 0 so it reflects as platform upstream loss in Gross Margin Analytics
		_, err := yh.store.RecordUpstreamFailoverLoss(
			ctx,
			userID,
			accountID,
			modelName,
			upstreamModel,
			int(meta.InputTokens),
			int(meta.OutputTokens),
			inCostUSD,
			outCostUSD,
			totalCostUSD,
			lossReqID,
			streaming,
		)
		if err != nil {
			slog.Warn("[YunwuHypervisor] Failed to record upstream failover loss to usage_logs", "error", err, "node_id", node.NodeID)
		}

		// 2. Record to ash_usages with billed_micro_usd = 0
		_, _ = yh.store.RecordUsageDetails(
			ctx,
			ownerID,
			&sessionID,
			&node.NodeID,
			meta.InputTokens,
			meta.OutputTokens,
			meta.InputTokens+meta.OutputTokens,
			"yunwu",
			node.Provider,
			modelName,
			"upstream_failover_loss",
			0,
		)

		// 3. Log event
		_, _ = yh.store.LogEvent(ctx, ownerID, &sessionID, &node.NodeID, "UpstreamFailoverLossRecorded", map[string]any{
			"attempt":        attempt,
			"input_tokens":   meta.InputTokens,
			"output_tokens":  meta.OutputTokens,
			"total_cost_usd": totalCostUSD,
			"provider":       node.Provider,
			"pool_type":      "yunwu",
		})
	}

	slog.Info("[YunwuHypervisor] Recorded upstream failover loss for failed attempt",
		"node_id", node.NodeID,
		"provider", node.Provider,
		"model", modelName,
		"input_tokens", meta.InputTokens,
		"output_tokens", meta.OutputTokens,
		"total_cost_usd", totalCostUSD,
		"attempt", attempt,
	)
}

// HandleLive proxies a bidirectional Gemini Live session through the isolated
// Yunwu pool. A bounded maximum duration makes node and user cost reservable
// before any realtime media is sent upstream.
func (yh *YunwuHypervisor) HandleLive(ctx context.Context, ownerID, conversationID, path string, headers http.Header, peer runtime.WebSocketPeer) error {
	return yh.handleLive(ctx, ownerID, conversationID, path, headers, peer, "")
}

// HandleEphemeralLive consumes a gateway-issued, short-lived token and proxies
// a constrained Live session with the corresponding vendor token on the exact
// Yunwu node that issued it.
func (yh *YunwuHypervisor) HandleEphemeralLive(ctx context.Context, gatewayToken, conversationID, path string, headers http.Header, peer runtime.WebSocketPeer) error {
	ownerID, upstreamToken, nodeID, err := yh.consumeEphemeralToken(ctx, gatewayToken)
	if err != nil {
		return err
	}
	u, err := url.Parse(path)
	if err != nil {
		return fmt.Errorf("invalid constrained Live path")
	}
	query := u.Query()
	query.Set("access_token", upstreamToken)
	u.RawQuery = query.Encode()
	return yh.handleLive(ctx, ownerID, conversationID, u.String(), headers, peer, nodeID)
}

func (yh *YunwuHypervisor) handleLive(ctx context.Context, ownerID, conversationID, path string, headers http.Header, peer runtime.WebSocketPeer, forcedNodeID string) error {
	userPriority := 1
	if yh.qm != nil {
		if access, err := yh.qm.GetUserAccess(ctx, ownerID, "yunwu"); err == nil && access != nil {
			userPriority = access.Priority
		}
	}

	nodes, err := yh.getAllYunwuNodes(ctx)
	if err != nil {
		return err
	}
	var geminiNodes []health.ExecutionNode
	for _, node := range nodes {
		if NodeSupportsFamily(node.Capabilities, FamilyGemini) {
			geminiNodes = append(geminiNodes, node)
		}
	}
	if len(geminiNodes) > 0 {
		nodes = geminiNodes
	}
	states := yh.getAllYunwuNodeStates(ctx, nodes)
	credits := yh.getAllYunwuNodeCredits(ctx, nodes)
	selected, err := yh.sch.SelectNode(ctx, nodes, states, credits, userPriority, forcedNodeID, yh.hc.GetSystemState(ctx))
	if err != nil {
		return err
	}
	if forcedNodeID != "" && selected.NodeID != forcedNodeID {
		return fmt.Errorf("ephemeral token node is unavailable")
	}
	maxMinutes := yh.liveMaxMinutes(ctx, selected.NodeID)
	liveModel, usageUnit := "gemini-live", "live_audio_minute"
	if strings.Contains(path, "BidiGenerateMusic") {
		liveModel, usageUnit = "lyria-realtime", "live_music_minute"
	}
	pricePerMinute, fallback := yh.resolveMediaPrice(ctx, selected.NodeID, liveModel, mediaRequest{Kind: usageUnit, Units: 1})
	if fallback {
		observability.Default.YunwuFallbackPricing()
	}
	estimated := pricePerMinute * maxMinutes
	requestID := fmt.Sprintf("live-%s-%d", ownerID, time.Now().UnixNano())
	reserved := false
	if yh.cfg.YunwuRequireFreshBalance {
		reserved, err = yh.sw.Reserve(ctx, selected.NodeID, requestID, max64(1, estimated), time.Duration(maxMinutes+2)*time.Minute)
		if err != nil {
			return err
		}
		if !reserved {
			return ErrNodeBalanceExhausted
		}
	}
	reservationOpen := reserved
	defer func() {
		if reservationOpen {
			_, _ = yh.sw.ReleaseReservation(context.Background(), selected.NodeID, requestID)
		}
	}()

	wsRuntime, ok := yh.rt.(runtime.WebSocketRuntime)
	if !ok {
		return fmt.Errorf("configured Yunwu runtime does not support websocket transport")
	}
	liveCtx, cancel := context.WithTimeout(ctx, time.Duration(maxMinutes)*time.Minute)
	started := time.Now()
	err = wsRuntime.ProxyWebSocket(liveCtx, selected.NodeID, path, headers, peer)
	cancel()
	duration := time.Since(started)
	minutes := int64(math.Ceil(duration.Minutes()))
	if minutes < 1 {
		minutes = 1
	}
	if minutes > maxMinutes {
		minutes = maxMinutes
	}
	billed := pricePerMinute * minutes
	// A successfully established session is billable even when either peer
	// closes it normally; a dial/setup failure is refunded by the defer above.
	if duration >= time.Second || err == nil {
		_, _ = yh.sw.ReconcileReservation(context.Background(), selected.NodeID, requestID, billed)
		reservationOpen = false
		if billed > 0 {
			_, _ = yh.store.RecordUsageDetails(context.Background(), ownerID, nil, &selected.NodeID, 0, 0, minutes, "yunwu", "yunwu", liveModel, usageUnit, billed)
			_, _ = yh.store.LogEvent(context.Background(), ownerID, nil, &selected.NodeID, "UsageRecorded", map[string]any{"usage_unit": usageUnit, "units": minutes, "billed_micro_usd": billed, "pool_type": "yunwu"})
		}
	}
	return err
}

type ephemeralTokenRecord struct {
	OwnerID       string `json:"owner_id"`
	UpstreamToken string `json:"upstream_token"`
	NodeID        string `json:"node_id"`
}

// ExchangeEphemeralToken replaces a vendor credential with an opaque token
// valid only at this gateway. The vendor credential never reaches the client.
func (yh *YunwuHypervisor) ExchangeEphemeralToken(ctx context.Context, ownerID, upstreamToken string, ttl time.Duration, uses int64) (string, error) {
	if ownerID == "" || upstreamToken == "" {
		return "", fmt.Errorf("invalid ephemeral token exchange")
	}
	if ttl <= 0 || ttl > 20*time.Hour {
		ttl = 30 * time.Minute
	}
	if uses < 1 || uses > 100 {
		uses = 1
	}
	nodeID, err := yh.store.Redis().Get(ctx, yunwuUpstreamTokenNodeKey(upstreamToken)).Result()
	if err != nil || nodeID == "" {
		return "", fmt.Errorf("ephemeral token node affinity unavailable")
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate ephemeral token: %w", err)
	}
	gatewayToken := fmt.Sprintf("ash_ephemeral_%x", random)
	record, _ := json.Marshal(ephemeralTokenRecord{OwnerID: ownerID, UpstreamToken: upstreamToken, NodeID: nodeID})
	key := yunwuGatewayTokenKey(gatewayToken)
	pipe := yh.store.Redis().TxPipeline()
	pipe.HSet(ctx, key, "record", record, "uses", uses)
	pipe.Expire(ctx, key, ttl)
	_, err = pipe.Exec(ctx)
	return gatewayToken, err
}

func (yh *YunwuHypervisor) consumeEphemeralToken(ctx context.Context, token string) (string, string, string, error) {
	if token == "" {
		return "", "", "", fmt.Errorf("missing ephemeral token")
	}
	const script = `
local uses = tonumber(redis.call('HGET', KEYS[1], 'uses'))
if not uses or uses <= 0 then return nil end
local record = redis.call('HGET', KEYS[1], 'record')
if not record then return nil end
if uses == 1 then redis.call('DEL', KEYS[1]) else redis.call('HINCRBY', KEYS[1], 'uses', -1) end
return record`
	value, err := yh.store.Redis().Eval(ctx, script, []string{yunwuGatewayTokenKey(token)}).Text()
	if err != nil || value == "" {
		return "", "", "", fmt.Errorf("invalid or expired ephemeral token")
	}
	var record ephemeralTokenRecord
	if json.Unmarshal([]byte(value), &record) != nil || record.OwnerID == "" || record.UpstreamToken == "" || record.NodeID == "" {
		return "", "", "", fmt.Errorf("invalid ephemeral token record")
	}
	return record.OwnerID, record.UpstreamToken, record.NodeID, nil
}

func (yh *YunwuHypervisor) liveMaxMinutes(ctx context.Context, nodeID string) int64 {
	const defaultMax = int64(15)
	if yh.store.DB() == nil {
		return defaultMax
	}
	id, err := strconv.ParseInt(nodeID, 10, 64)
	if err != nil {
		return defaultMax
	}
	acc, err := yh.store.DB().Account.Get(ctx, id)
	if err != nil || acc.Extra == nil {
		return defaultMax
	}
	value, ok := numericMicroUSD(acc.Extra["yunwu_live_max_minutes"])
	if !ok || value < 1 || value > 120 {
		return defaultMax
	}
	return value
}

func CalculateBilledMicroUSD(inputTokens, outputTokens int64, inputPriceUSDPer1K, outputPriceUSDPer1K float64) int64 {
	if inputTokens < 0 {
		inputTokens = 0
	}
	if outputTokens < 0 {
		outputTokens = 0
	}
	if inputPriceUSDPer1K < 0 || math.IsNaN(inputPriceUSDPer1K) || math.IsInf(inputPriceUSDPer1K, 0) {
		inputPriceUSDPer1K = 0
	}
	if outputPriceUSDPer1K < 0 || math.IsNaN(outputPriceUSDPer1K) || math.IsInf(outputPriceUSDPer1K, 0) {
		outputPriceUSDPer1K = 0
	}
	cost := float64(inputTokens)*inputPriceUSDPer1K*1000 + float64(outputTokens)*outputPriceUSDPer1K*1000
	if cost <= 0 || math.IsNaN(cost) || math.IsInf(cost, 0) {
		return 0
	}
	return int64(math.Ceil(cost))
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

type mediaRequest struct {
	Kind  string
	Units int64
}

func classifyMediaRequest(ctx context.Context, model string, payload []byte) mediaRequest {
	req, ok := runtime.UpstreamRequestFromContext(ctx)
	if !ok {
		return mediaRequest{}
	}
	method, path := strings.ToUpper(req.Method), strings.ToLower(req.Path)
	if method != http.MethodPost || isGeminiControlResource(path) || strings.Contains(path, "/files") || strings.Contains(path, "/operations/") || strings.Contains(path, ":counttokens") || strings.HasSuffix(strings.Split(path, "?")[0], "/cancel") {
		return mediaRequest{Kind: "media_control", Units: 0}
	}
	kind := ""
	switch {
	case strings.Contains(path, "/images/") || strings.HasPrefix(strings.ToLower(model), "imagen-"):
		kind = "images"
	case strings.Contains(path, ":generatecontent") && strings.HasPrefix(strings.ToLower(model), "gemini-") && strings.Contains(strings.ToLower(model), "image"):
		kind = "images"
	case strings.Contains(path, "/audio/speech") || strings.HasPrefix(strings.ToLower(model), "chirp-") || strings.HasPrefix(strings.ToLower(model), "lyria-"):
		kind = "audio"
	case strings.Contains(path, "/audio/transcriptions"), strings.Contains(path, "/audio/translations"):
		kind = "audio_input"
	case strings.Contains(path, "/videos"), strings.Contains(path, "/video/"), strings.HasPrefix(strings.ToLower(model), "veo-"):
		kind = "video"
	case strings.Contains(path, ":predictlongrunning"):
		kind = "media"
	default:
		return mediaRequest{}
	}
	units := int64(1)
	var body struct {
		N        int64 `json:"n"`
		Duration int64 `json:"duration"`
	}
	if json.Unmarshal(payload, &body) == nil {
		if body.N > 0 {
			units = body.N
		}
		if kind == "video" && body.Duration > 0 {
			units *= body.Duration
		}
	}
	return mediaRequest{Kind: kind, Units: units}
}

func isGeminiControlResource(path string) bool {
	for _, marker := range []string{"/auth_tokens", "/batches", "/cachedcontents", "/generatedfiles", "/filesearchstores", "/corpora", "/tunedmodels", "/dynamic"} {
		if strings.Contains(path, marker) {
			return true
		}
	}
	return false
}

// resolveMediaPrice returns a configured price per produced unit. Account
// extra supports yunwu_media_prices_micro_usd as {model: price, kind: price,
// default: price}; model wins over kind. Missing prices use the configurable
// per-request fallback and are surfaced through the fallback-pricing metric.
func (yh *YunwuHypervisor) resolveMediaPrice(ctx context.Context, nodeID, model string, media mediaRequest) (int64, bool) {
	if media.Kind == "media_control" || media.Units <= 0 {
		return 0, false
	}
	fallback := int64(math.Ceil(yh.cfg.YunwuDefaultOutputUSDPer1K * 1_000_000))
	if fallback < 1 {
		fallback = 1
	}
	if yh.store.DB() == nil {
		return fallback * media.Units, true
	}
	id, err := strconv.ParseInt(nodeID, 10, 64)
	if err != nil {
		return fallback * media.Units, true
	}
	acc, err := yh.store.DB().Account.Get(ctx, id)
	if err != nil || acc.Extra == nil {
		return fallback * media.Units, true
	}
	prices, ok := acc.Extra["yunwu_media_prices_micro_usd"].(map[string]any)
	if !ok {
		return fallback * media.Units, true
	}
	for _, key := range []string{model, media.Kind, "default"} {
		if price, ok := numericMicroUSD(prices[key]); ok && price >= 0 {
			return price * media.Units, false
		}
	}
	return fallback * media.Units, true
}

func numericMicroUSD(value any) (int64, bool) {
	switch n := value.(type) {
	case float64:
		return int64(math.Ceil(n)), true
	case int:
		return int64(n), true
	case int64:
		return n, true
	case json.Number:
		v, err := strconv.ParseFloat(string(n), 64)
		return int64(math.Ceil(v)), err == nil
	case string:
		v, err := strconv.ParseFloat(n, 64)
		return int64(math.Ceil(v)), err == nil
	default:
		return 0, false
	}
}

func operationNameFromRequest(ctx context.Context) string {
	req, ok := runtime.UpstreamRequestFromContext(ctx)
	if !ok {
		return ""
	}
	path := req.Path
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	i := strings.Index(path, "/operations/")
	if i < 0 {
		return ""
	}
	return strings.TrimPrefix(path[i+1:], "/")
}

func modelNameFromRequest(ctx context.Context) string {
	req, ok := runtime.UpstreamRequestFromContext(ctx)
	if !ok {
		return ""
	}
	path := req.Path
	start := strings.Index(path, "/models/")
	if start < 0 {
		return ""
	}
	modelAction := path[start+len("/models/"):]
	if end := strings.IndexAny(modelAction, ":?"); end >= 0 {
		modelAction = modelAction[:end]
	}
	decoded, err := url.PathUnescape(modelAction)
	if err != nil {
		return ""
	}
	return decoded
}

func operationNameFromResponse(body []byte) string {
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return ""
	}
	name, _ := payload["name"].(string)
	if strings.HasPrefix(name, "operations/") {
		return name
	}
	return ""
}

func yunwuOperationNodeKey(operation string) string {
	return "ash:yunwu:operation_node:" + operation
}

func interactionIDFromRequest(ctx context.Context) string {
	req, ok := runtime.UpstreamRequestFromContext(ctx)
	if !ok {
		return ""
	}
	path := req.Path
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	const prefix = "/interactions/"
	i := strings.Index(path, prefix)
	if i < 0 {
		return ""
	}
	id := path[i+len(prefix):]
	if slash := strings.IndexByte(id, '/'); slash >= 0 {
		id = id[:slash]
	}
	return id
}

func interactionIDFromResponse(ctx context.Context, body []byte) string {
	req, ok := runtime.UpstreamRequestFromContext(ctx)
	if !ok || !strings.Contains(req.Path, "/interactions") {
		return ""
	}
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return ""
	}
	id, _ := payload["id"].(string)
	return strings.TrimSpace(id)
}

func yunwuInteractionNodeKey(id string) string {
	return "ash:yunwu:interaction_node:" + id
}

func ephemeralTokenFromResponse(ctx context.Context, body []byte) string {
	req, ok := runtime.UpstreamRequestFromContext(ctx)
	if !ok || !strings.Contains(req.Path, "/auth_tokens") {
		return ""
	}
	var payload struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(body, &payload) != nil || !strings.HasPrefix(payload.Name, "auth_tokens/") {
		return ""
	}
	return payload.Name
}

func yunwuUpstreamTokenNodeKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("ash:yunwu:upstream_token_node:%x", sum[:])
}

func yunwuGatewayTokenKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("ash:yunwu:gateway_ephemeral:%x", sum[:])
}

func statefulResourceNameFromRequest(ctx context.Context) string {
	req, ok := runtime.UpstreamRequestFromContext(ctx)
	if !ok {
		return ""
	}
	path := strings.SplitN(req.Path, "?", 2)[0]
	path = strings.TrimPrefix(path, "/v1beta/")
	for _, root := range []string{"batches", "cachedContents", "generatedFiles", "fileSearchStores", "corpora", "tunedModels", "dynamic"} {
		if strings.HasPrefix(path, root+"/") {
			name := strings.SplitN(path, ":", 2)[0]
			return name
		}
	}
	return ""
}

func statefulResourceNameFromResponse(ctx context.Context, body []byte) string {
	req, ok := runtime.UpstreamRequestFromContext(ctx)
	if !ok || !strings.Contains(req.Path, "/v1beta/") {
		return ""
	}
	var payload struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(body, &payload) != nil {
		return ""
	}
	for _, root := range []string{"batches/", "cachedContents/", "generatedFiles/", "fileSearchStores/", "corpora/", "tunedModels/", "dynamic/"} {
		if strings.HasPrefix(payload.Name, root) {
			return payload.Name
		}
	}
	return ""
}

func yunwuResourceNodeKey(name string) string {
	sum := sha256.Sum256([]byte(name))
	return fmt.Sprintf("ash:yunwu:resource_node:%x", sum[:])
}

func (yh *YunwuHypervisor) getAllYunwuNodes(ctx context.Context) ([]health.ExecutionNode, error) {
	var dbAccounts []*ent.Account

	if yh.store != nil && yh.store.DB() != nil {
		dbAccounts, _ = yh.store.DB().Account.Query().
			Where(
				account.PlatformEqualFold("yunwu"),
				account.Status("active"),
				account.Schedulable(true),
				account.HasAccountGroups(),
			).
			All(ctx)
	}

	var list []health.ExecutionNode
	for _, acc := range dbAccounts {
		nodeID := fmt.Sprintf("%d", acc.ID)
		node, err := yh.nm.GetNode(ctx, nodeID)
		families := ParseNodeModelFamilies(acc.Extra)
		provider := ParseNodeProvider(acc.Extra)
		if err != nil || node == nil {
			node = &health.ExecutionNode{
				NodeID:       nodeID,
				AccountName:  acc.Name,
				Provider:     provider,
				State:        "healthy",
				Capabilities: families,
				Priority:     acc.Priority,
				UpdatedAt:    time.Now(),
			}
			_ = yh.nm.SaveNode(ctx, node)
		} else {
			node.AccountName = acc.Name
			node.Capabilities = families
			node.Provider = provider
			node.Priority = acc.Priority
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
			slog.Warn("[YunwuStatsWorker] Balance sync skipped: database is unavailable")
			return
		}
		accounts, err := yh.store.DB().Account.Query().
			Where(
				account.PlatformEqualFold("yunwu"),
				account.Status("active"),
			).All(ctx)
		if err != nil {
			slog.Warn("[YunwuStatsWorker] Failed to list Yunwu accounts for balance sync", "error", err)
			return
		}
		slog.Info("[YunwuStatsWorker] Starting Yunwu balance sync", "accounts", len(accounts))
		for _, acc := range accounts {
			creds, decErr := credentials.DecryptMap(acc.Credentials, yh.decryptor)
			if decErr != nil {
				observability.Default.YunwuBalanceSyncFailed()
				slog.Warn("[YunwuStatsWorker] Balance sync failed: cannot decrypt account credentials",
					"node_id", fmt.Sprint(acc.ID),
					"error", decErr,
				)
				_, _ = yh.store.LogEvent(ctx, "system", nil, nil, "YunwuBalanceSyncFailed", map[string]any{"node_id": fmt.Sprint(acc.ID), "error": decErr.Error()})
				continue
			}
			apiKey, _ := creds["api_key"].(string)
			baseURL, _ := creds["base_url"].(string)
			if baseURL == "" {
				baseURL = DefaultBaseURLForProvider(ParseNodeProvider(acc.Extra))
			}
			if apiKey == "" {
				observability.Default.YunwuBalanceSyncFailed()
				slog.Warn("[YunwuStatsWorker] Balance sync skipped: account API key is empty", "node_id", fmt.Sprint(acc.ID))
				_, _ = yh.store.LogEvent(ctx, "system", nil, nil, "YunwuBalanceSyncFailed", map[string]any{"node_id": fmt.Sprint(acc.ID), "error": "account API key is empty"})
				continue
			}
			node := &health.ExecutionNode{NodeID: fmt.Sprint(acc.ID), AccountName: acc.Name, Provider: ParseNodeProvider(acc.Extra)}
			if _, syncErr := yh.sw.SyncNodeStats(ctx, node, apiKey, baseURL); syncErr != nil {
				observability.Default.YunwuBalanceSyncFailed()
				slog.Warn("[YunwuStatsWorker] Yunwu balance sync failed",
					"node_id", node.NodeID,
					"error", syncErr,
				)
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
	node := health.ExecutionNode{NodeID: nodeID}
	if yh.nm != nil {
		if n, err := yh.nm.GetNode(ctx, nodeID); err == nil && n != nil {
			node = *n
		}
	}
	if yh.pm != nil {
		in, out, found := yh.pm.ResolveNodePrice(ctx, &node, model)
		if found {
			return in, out, false
		}
	}
	return yh.cfg.YunwuDefaultInputUSDPer1K, yh.cfg.YunwuDefaultOutputUSDPer1K, true
}

// GetHealthStatus returns the status summary of Yunwu Hypervisor Engine
func (yh *YunwuHypervisor) GetHealthStatus(ctx context.Context) map[string]any {
	nodes, _ := yh.getAllYunwuNodes(ctx)
	systemState := yh.hc.GetSystemState(ctx)
	healthy := 0
	cooldown := 0
	totalAvailableUSD := 0.0
	totalAllocatableUSD := 0.0

	for _, n := range nodes {
		if n.State == "healthy" {
			healthy++
		} else if n.State == "cooldown" {
			cooldown++
		}
		if yh.sw != nil {
			if snap, err := yh.sw.Snapshot(ctx, n.NodeID, 100*365*24*time.Hour); err == nil && snap != nil {
				totalAvailableUSD += snap.AvailableUSD
				allocatable := max64(0, snap.EstimatedAvailableMicroUSD-snap.ReserveMicroUSD)
				totalAllocatableUSD += float64(allocatable) / 1_000_000
			}
		}
	}

	rebinding24h, _ := yh.store.Redis().ZCard(ctx, "ash:yunwu:rebinding:24h").Result()

	return map[string]any{
		"status":                "ok",
		"system_state":          systemState,
		"platform":              "yunwu",
		"total_nodes":           len(nodes),
		"healthy_nodes":         healthy,
		"cooldown_nodes":        cooldown,
		"rebinding_24h_cnt":     rebinding24h,
		"total_available_usd":   totalAvailableUSD,
		"total_allocatable_usd": totalAllocatableUSD,
	}
}

// SyncNodeBalance triggers an immediate balance sync for a Yunwu node
func (yh *YunwuHypervisor) SyncNodeBalance(ctx context.Context, nodeID string, apiKey string, baseURL string) (*YunwuCreditGrantsResponse, error) {
	if apiKey == "" {
		id, parseErr := strconv.ParseInt(nodeID, 10, 64)
		if parseErr != nil {
			return nil, parseErr
		}
		if yh.store == nil || yh.store.DB() == nil {
			return nil, fmt.Errorf("database unavailable")
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
		if baseURL == "" {
			baseURL = DefaultBaseURLForProvider(ParseNodeProvider(acc.Extra))
		}
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

// NodeBalanceSyncResult describes the outcome of syncing an individual node's balance
type NodeBalanceSyncResult struct {
	NodeID          string  `json:"node_id"`
	AccountName     string  `json:"account_name"`
	Provider        string  `json:"provider"`
	AvailableUSD    float64 `json:"available_usd"`
	AllocatableUSD  float64 `json:"allocatable_usd"`
	TotalGrantedUSD float64 `json:"total_granted_usd"`
	TotalUsedUSD    float64 `json:"total_used_usd"`
	Status          string  `json:"status"`
	Source          string  `json:"source,omitempty"`
	Error           string  `json:"error,omitempty"`
}

// GroupBalanceSyncSummary summarizes batch balance sync for a group or all nodes
type GroupBalanceSyncSummary struct {
	GroupID             int64                   `json:"group_id,omitempty"`
	TotalNodes          int                     `json:"total_nodes"`
	SyncedNodes         int                     `json:"synced_nodes"`
	FailedNodes         int                     `json:"failed_nodes"`
	TotalAvailableUSD   float64                 `json:"total_available_usd"`
	TotalAllocatableUSD float64                 `json:"total_allocatable_usd"`
	Nodes               []NodeBalanceSyncResult `json:"nodes"`
}

// SyncGroupBalances syncs the balance for all active nodes belonging to a specific group
func (yh *YunwuHypervisor) SyncGroupBalances(ctx context.Context, groupID int64) (*GroupBalanceSyncSummary, error) {
	return yh.SyncAllBalances(ctx, groupID)
}

// SyncAllBalances syncs balance across all Yunwu accounts, optionally filtered by groupID
func (yh *YunwuHypervisor) SyncAllBalances(ctx context.Context, groupID ...int64) (*GroupBalanceSyncSummary, error) {
	if yh.store == nil || yh.store.DB() == nil {
		return nil, fmt.Errorf("database unavailable")
	}

	query := yh.store.DB().Account.Query().Where(
		account.PlatformEqualFold("yunwu"),
		account.Status("active"),
	)

	var targetGroupID int64
	if len(groupID) > 0 && groupID[0] > 0 {
		targetGroupID = groupID[0]
		query = query.Where(account.HasAccountGroupsWith(accountgroup.GroupIDEQ(targetGroupID)))
	}

	accounts, err := query.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query accounts: %w", err)
	}

	summary := &GroupBalanceSyncSummary{
		GroupID:    targetGroupID,
		TotalNodes: len(accounts),
		Nodes:      make([]NodeBalanceSyncResult, 0, len(accounts)),
	}

	reservePercent := yh.cfg.YunwuBalanceReservePercent

	for _, acc := range accounts {
		nodeID := fmt.Sprintf("%d", acc.ID)
		prov := ParseNodeProvider(acc.Extra)
		resItem := NodeBalanceSyncResult{
			NodeID:      nodeID,
			AccountName: acc.Name,
			Provider:    prov,
			Status:      "unknown",
		}

		creds, decErr := credentials.DecryptMap(acc.Credentials, yh.decryptor)
		if decErr != nil {
			observability.Default.YunwuBalanceSyncFailed()
			resItem.Status = "error"
			resItem.Error = decErr.Error()
			summary.FailedNodes++
			summary.Nodes = append(summary.Nodes, resItem)
			continue
		}

		apiKey, _ := creds["api_key"].(string)
		baseURL, _ := creds["base_url"].(string)
		if baseURL == "" {
			baseURL = DefaultBaseURLForProvider(prov)
		}

		if apiKey == "" {
			observability.Default.YunwuBalanceSyncFailed()
			resItem.Status = "error"
			resItem.Error = "API key is empty"
			summary.FailedNodes++
			summary.Nodes = append(summary.Nodes, resItem)
			continue
		}

		node := &health.ExecutionNode{
			NodeID:      nodeID,
			AccountName: acc.Name,
			Provider:    prov,
			State:       "healthy",
		}

		grants, syncErr := yh.sw.SyncNodeStats(ctx, node, apiKey, baseURL)
		if syncErr != nil {
			observability.Default.YunwuBalanceSyncFailed()
			resItem.Status = "error"
			resItem.Error = syncErr.Error()
			summary.FailedNodes++
			summary.Nodes = append(summary.Nodes, resItem)
			continue
		}

		availUSD := grants.TotalAvailable
		allocatableUSD := availUSD * (1.0 - float64(reservePercent)/100.0)
		if allocatableUSD < 0 {
			allocatableUSD = 0
		}

		resItem.Status = "fresh"
		resItem.Source = grants.Object
		resItem.AvailableUSD = availUSD
		resItem.AllocatableUSD = allocatableUSD
		resItem.TotalGrantedUSD = grants.TotalGranted
		resItem.TotalUsedUSD = grants.TotalUsed

		summary.SyncedNodes++
		summary.TotalAvailableUSD += availUSD
		summary.TotalAllocatableUSD += allocatableUSD
		summary.Nodes = append(summary.Nodes, resItem)
	}

	return summary, nil
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

// prepareNodePayload rewrites the model field in the request payload from canonical ID / alias
// to the upstream model ID appropriate for the selected execution node's provider.
func (yh *YunwuHypervisor) prepareNodePayload(ctx context.Context, node *health.ExecutionNode, payload []byte) []byte {
	if node == nil || len(payload) == 0 {
		return payload
	}
	var bodyMap map[string]any
	if err := json.Unmarshal(payload, &bodyMap); err != nil {
		return payload
	}
	modelVal, ok := bodyMap["model"]
	if !ok || modelVal == nil {
		return payload
	}
	modelStr, ok := modelVal.(string)
	if !ok || strings.TrimSpace(modelStr) == "" {
		return payload
	}

	var accountMapping map[string]string
	if yh.store != nil && yh.store.DB() != nil && node.NodeID != "" {
		if id, parseErr := strconv.ParseInt(node.NodeID, 10, 64); parseErr == nil {
			if acc, accErr := yh.store.DB().Account.Get(ctx, id); accErr == nil && acc != nil {
				accountMapping = ParseAccountModelMapping(acc.Extra)
			}
		}
	}

	upstreamModel := ResolveUpstreamModelID(node.Provider, modelStr, accountMapping)
	if upstreamModel != "" && upstreamModel != modelStr {
		bodyMap["model"] = upstreamModel
		if updated, err := json.Marshal(bodyMap); err == nil {
			slog.Debug("[YunwuHypervisor] Resolved canonical model to upstream model",
				"node_id", node.NodeID,
				"provider", node.Provider,
				"requested_model", modelStr,
				"upstream_model", upstreamModel,
			)
			return updated
		}
	}

	return payload
}

func maskKey(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "***"
	}
	if len(raw) <= 8 {
		return "***"
	}
	return raw[:4] + "..." + raw[len(raw)-4:]
}


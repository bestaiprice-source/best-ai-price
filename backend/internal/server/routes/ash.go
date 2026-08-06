package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/account"
	"github.com/Wei-Shaw/sub2api/ent/ashdeposit"
	"github.com/Wei-Shaw/sub2api/ent/ashevent"
	"github.com/Wei-Shaw/sub2api/ent/ashusage"
	"github.com/Wei-Shaw/sub2api/ent/usagelog"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	ashhypervisor "github.com/Wei-Shaw/sub2api/pkg/hypervisor"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/backup"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/lease"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/observability"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/quota"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/session"
	ashstats "github.com/Wei-Shaw/sub2api/pkg/hypervisor/stats"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/yunwu"

	"github.com/gin-gonic/gin"
)

// RegisterAshRoutes registers monitoring and administrative endpoints for the hypervisor
func RegisterAshRoutes(
	r *gin.Engine,
	h *handler.Handlers,
	jwtAuth middleware.JWTAuthMiddleware,
	adminAuth middleware.AdminAuthMiddleware,
) {
	setupRoutes := func(ash *gin.RouterGroup) {

		// 1. Public Health Check
		ash.GET("/health", func(c *gin.Context) {
			engine := h.Gateway.GetHypervisorEngine()
			if engine == nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "hypervisor engine not initialized"})
				return
			}

			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})

		// User endpoints (requires JWT Auth)
		userGroup := ash.Group("")
		userGroup.Use(gin.HandlerFunc(jwtAuth))
		{
			// GET /api/v1/ash/me/stats. The owner always comes from the JWT.
			userGroup.GET("/me/stats", func(c *gin.Context) {
				engine := h.Gateway.GetHypervisorEngine()
				if engine == nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "hypervisor engine not initialized"})
					return
				}
				subject, ok := middleware.GetAuthSubjectFromContext(c)
				if !ok {
					c.JSON(http.StatusUnauthorized, gin.H{"error": "authenticated user not found"})
					return
				}
				ownerID := strconv.FormatInt(subject.UserID, 10)

				poolType := c.Query("pool_type")
				if poolType == "" {
					poolType = "standard"
				}
				qm := quota.NewQuotaManager(engine.Storage())
				ua, err := qm.GetUserAccess(c.Request.Context(), ownerID, poolType)
				if err != nil {
					c.JSON(http.StatusNotFound, gin.H{"error": "user quota access not found"})
					return
				}

				// ASH usage is canonical for hypervisor requests. Fall back to the
				// legacy gateway table for installations not yet producing ASH rows.
				var sum []struct {
					Input  int64 `json:"input"`
					Output int64 `json:"output"`
				}
				_ = engine.Storage().DB().AshUsage.Query().
					Where(ashusage.OwnerID(ownerID)).
					Aggregate(
						ent.As(ent.Sum(ashusage.FieldInputTokens), "input"),
						ent.As(ent.Sum(ashusage.FieldOutputTokens), "output"),
					).
					Scan(c.Request.Context(), &sum)
				if len(sum) == 0 || (sum[0].Input == 0 && sum[0].Output == 0) {
					sum = nil
					_ = engine.Storage().DB().UsageLog.Query().Where(usagelog.UserID(subject.UserID)).Aggregate(
						ent.As(ent.Sum(usagelog.FieldInputTokens), "input"),
						ent.As(ent.Sum(usagelog.FieldOutputTokens), "output"),
					).Scan(c.Request.Context(), &sum)
				}

				var inputUsed, outputUsed int64
				if len(sum) > 0 {
					inputUsed = sum[0].Input
					outputUsed = sum[0].Output
				}
				activeSessions, rebindingCount, lastErrors := userOperationalStats(c.Request.Context(), engine, ownerID)
				remainingFairUse := int64(0)
				if ua.MonthlyLimit > ua.UsedUnits {
					remainingFairUse = ua.MonthlyLimit - ua.UsedUnits
				}

				c.JSON(http.StatusOK, gin.H{
					"owner_id":                ua.OwnerID,
					"plan_id":                 ua.PlanID,
					"monthly_limit":           ua.MonthlyLimit,
					"daily_burst":             ua.DailyBurst,
					"used_units":              ua.UsedUnits,
					"monthly_limit_micro_usd": ua.MonthlyLimitMicroUSD,
					"daily_burst_micro_usd":   ua.DailyBurstMicroUSD,
					"used_micro_usd":          ua.UsedMicroUSD,
					"remaining_micro_usd":     maxInt64(0, ua.MonthlyLimitMicroUSD-ua.UsedMicroUSD),
					"input_tokens":            inputUsed,
					"output_tokens":           outputUsed,
					"priority":                ua.Priority,
					"deposit_state":           ua.DepositState,
					"is_active":               ua.IsActive,
					"remaining_fair_use":      remainingFairUse,
					"active_sessions":         activeSessions,
					"rebinding_count":         rebindingCount,
					"last_errors":             lastErrors,
				})
			})

			// GET /api/v1/ash/me/sessions/{session_id}.
			userGroup.GET("/me/sessions/:session_id", func(c *gin.Context) {
				engine := h.Gateway.GetHypervisorEngine()
				if engine == nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "hypervisor engine not initialized"})
					return
				}
				sessionID := c.Param("session_id")
				subject, ok := middleware.GetAuthSubjectFromContext(c)
				if !ok {
					c.JSON(http.StatusUnauthorized, gin.H{"error": "authenticated user not found"})
					return
				}

				sm := session.NewSessionManager(engine.Storage())
				sess, err := sm.GetSession(c.Request.Context(), sessionID)
				if err != nil || sess == nil {
					c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
					return
				}
				if sess.OwnerID != strconv.FormatInt(subject.UserID, 10) {
					// Do not disclose whether another user's session exists.
					c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
					return
				}

				lm := lease.NewLeaseManager(engine.Storage())
				currLease, _ := lm.GetLease(c.Request.Context(), sessionID)

				c.JSON(http.StatusOK, gin.H{
					"session_id":      sess.ID,
					"owner_id":        sess.OwnerID,
					"conversation_id": sess.ConversationID,
					"state":           sess.State,
					"version":         sess.Version,
					"created_at":      sess.CreatedAt,
					"updated_at":      sess.UpdatedAt,
					"lease":           currLease,
				})
			})

			// POST /api/v1/ash/me/sessions/{session_id}/snapshot.
			userGroup.POST("/me/sessions/:session_id/snapshot", func(c *gin.Context) {
				engine := h.Gateway.GetHypervisorEngine()
				if engine == nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "hypervisor engine not initialized"})
					return
				}
				if !engine.BackupEnabled() {
					c.JSON(http.StatusNotImplemented, gin.H{"error": "backup is disabled"})
					return
				}
				subject, ok := middleware.GetAuthSubjectFromContext(c)
				if !ok {
					c.JSON(http.StatusUnauthorized, gin.H{"error": "authenticated user not found"})
					return
				}
				sessionID := c.Param("session_id")
				sess, err := session.NewSessionManager(engine.Storage()).GetSession(c.Request.Context(), sessionID)
				if err != nil || sess == nil || sess.OwnerID != strconv.FormatInt(subject.UserID, 10) {
					c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
					return
				}
				var snap backup.ProjectSnapshot
				if c.Request.ContentLength > 0 {
					if err := c.ShouldBindJSON(&snap); err != nil {
						c.JSON(http.StatusBadRequest, gin.H{"error": "invalid snapshot payload"})
						return
					}
				}
				snap.SessionState = sess
				if err := engine.BackupManager().CreateSnapshot(c.Request.Context(), sessionID, &snap); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create snapshot"})
					return
				}
				c.JSON(http.StatusCreated, snap)
			})

			// POST /api/v1/ash/me/sessions/{session_id}/restore.
			userGroup.POST("/me/sessions/:session_id/restore", func(c *gin.Context) {
				engine := h.Gateway.GetHypervisorEngine()
				if engine == nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "hypervisor engine not initialized"})
					return
				}
				if !engine.BackupEnabled() {
					c.JSON(http.StatusNotImplemented, gin.H{"error": "backup is disabled"})
					return
				}
				subject, ok := middleware.GetAuthSubjectFromContext(c)
				if !ok {
					c.JSON(http.StatusUnauthorized, gin.H{"error": "authenticated user not found"})
					return
				}
				sessionID := c.Param("session_id")
				snap, err := engine.BackupManager().GetLatestSnapshot(c.Request.Context(), sessionID)
				ownerID := strconv.FormatInt(subject.UserID, 10)
				if err != nil || snap.SessionState == nil || snap.SessionState.OwnerID != ownerID {
					c.JSON(http.StatusNotFound, gin.H{"error": "snapshot not found"})
					return
				}
				restored, err := engine.BackupManager().RestoreSessionFromSnapshot(c.Request.Context(), sessionID)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to restore snapshot"})
					return
				}
				_, _ = engine.Storage().LogEvent(c.Request.Context(), ownerID, &sessionID, nil, "SnapshotRestored", nil)
				c.JSON(http.StatusOK, restored)
			})
		}

		// Admin endpoints (requires Admin Auth)
		adminGroup := ash.Group("")
		adminGroup.Use(gin.HandlerFunc(adminAuth))
		{
			// GET /ash/stats/users/{owner_id} - complete admin view of one user.
			adminGroup.GET("/stats/users/:owner_id", func(c *gin.Context) {
				engine := h.Gateway.GetHypervisorEngine()
				if engine == nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "hypervisor engine not initialized"})
					return
				}
				ownerID := c.Param("owner_id")
				poolType := c.DefaultQuery("pool_type", "standard")
				userStat, err := ashstats.NewStatsCollector(engine.Storage()).GetUserStats(c.Request.Context(), ownerID, poolType)
				if err != nil {
					c.JSON(http.StatusNotFound, gin.H{"error": "user quota access not found"})
					return
				}
				activeSessions, rebindingCount, lastErrors := userOperationalStats(c.Request.Context(), engine, ownerID)
				remainingFairUse := int64(0)
				if userStat.MonthlyLimit > userStat.UsedUnits {
					remainingFairUse = userStat.MonthlyLimit - userStat.UsedUnits
				}
				c.JSON(http.StatusOK, gin.H{
					"owner_id": userStat.OwnerID, "plan_id": userStat.PlanID,
					"used_units": userStat.UsedUnits, "remaining_fair_use": remainingFairUse,
					"active_sessions": activeSessions, "rebinding_count": rebindingCount,
					"last_errors": lastErrors, "deposit_state": userStat.DepositState,
					"input_tokens": userStat.InputTokens, "output_tokens": userStat.OutputTokens,
				})
			})

			// GET /ash/stats - Global Stats Summary
			adminGroup.GET("/stats", func(c *gin.Context) {
				engine := h.Gateway.GetHypervisorEngine()
				if engine == nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "hypervisor engine not initialized"})
					return
				}

				// Aggregate global stats from ash_usages
				var sum []struct {
					Input  int64 `json:"input"`
					Output int64 `json:"output"`
					Cost   int64 `json:"cost"`
				}
				_ = engine.Storage().DB().AshUsage.Query().
					Aggregate(
						ent.As(ent.Sum(ashusage.FieldInputTokens), "input"),
						ent.As(ent.Sum(ashusage.FieldOutputTokens), "output"),
						ent.As(ent.Sum(ashusage.FieldCostUnits), "cost"),
					).
					Scan(c.Request.Context(), &sum)

				var input, output, cost int64
				if len(sum) > 0 {
					input = sum[0].Input
					output = sum[0].Output
					cost = sum[0].Cost
				}

				// Fall back to standard UsageLog table if AshUsage has no entries yet
				if input == 0 && output == 0 {
					var legacySum []struct {
						Input  int64 `json:"input"`
						Output int64 `json:"output"`
					}
					_ = engine.Storage().DB().UsageLog.Query().
						Aggregate(
							ent.As(ent.Sum(usagelog.FieldInputTokens), "input"),
							ent.As(ent.Sum(usagelog.FieldOutputTokens), "output"),
						).
						Scan(c.Request.Context(), &legacySum)
					if len(legacySum) > 0 {
						input = legacySum[0].Input
						output = legacySum[0].Output
						cost = input + output
					}
				}

				// Count active sessions
				sessionKeys, _ := engine.Storage().Redis().Keys(c.Request.Context(), "ash:session:*").Result()
				accounts, _ := engine.Storage().DB().Account.Query().All(c.Request.Context())
				nodeStates := map[string]int{}
				queueDepth := 0
				totalConcurrency := 0
				for _, acc := range accounts {
					nodeID := strconv.FormatInt(acc.ID, 10)
					node, _ := health.NewNodeManager(engine.Storage()).GetNode(c.Request.Context(), nodeID)
					state := "healthy"
					if node != nil && node.State != "" {
						state = node.State
					}
					nodeStates[state]++
					runtimeState, _ := health.NewNodeManager(engine.Storage()).GetNodeRuntimeState(c.Request.Context(), nodeID)
					if runtimeState != nil {
						queueDepth += runtimeState.QueueDepth
					}
					if acc.Concurrency > 0 {
						totalConcurrency += acc.Concurrency
					}
				}
				liveMetrics := observability.Default.Current()
				liveMetrics.NodeStateCount = nodeStates
				liveMetrics.QueueDepth = queueDepth
				liveMetrics.ActiveSessions = len(sessionKeys)
				if totalConcurrency > 0 {
					liveMetrics.PoolUtilization = float64(queueDepth) / float64(totalConcurrency) * 100
				}

				c.JSON(http.StatusOK, gin.H{
					"total_input_tokens":    input,
					"total_output_tokens":   output,
					"total_cost_units":      cost,
					"active_sessions_count": len(sessionKeys),
					"metrics":               liveMetrics,
				})
			})

			// GET /ash/stats/nodes - Node metrics details
			adminGroup.GET("/stats/nodes", func(c *gin.Context) {
				engine := h.Gateway.GetHypervisorEngine()
				if engine == nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "hypervisor engine not initialized"})
					return
				}

				dbAccounts, _ := engine.Storage().DB().Account.Query().
					Where(account.StatusIn("active", "error")).
					All(c.Request.Context())

				var list []gin.H

				for _, acc := range dbAccounts {
					nodeID := fmt.Sprintf("%d", acc.ID)
					node, _ := engine.Storage().Redis().HGetAll(c.Request.Context(), "ash:node:"+nodeID).Result()

					accName := acc.Name
					if accName == "" {
						accName = fmt.Sprintf("Node #%d", acc.ID)
					}
					provider := acc.Platform
					if provider == "" {
						provider = "anthropic"
					}
					nodeState := "healthy"
					if node != nil && node["State"] != "" {
						nodeState = node["State"]
					}

					stateKey := "ash:runtime_state:" + nodeID
					stateMap, _ := engine.Storage().Redis().HGetAll(c.Request.Context(), stateKey).Result()

					successRate := 1.0
					var avgLatency int64
					var recent429, recentTimeouts int
					var cooldownUntil string

					if len(stateMap) > 0 {
						if sr, parseErr := strconv.ParseFloat(stateMap["SuccessRate"], 64); parseErr == nil {
							successRate = sr
						}
						if al, parseErr := strconv.ParseInt(stateMap["AvgLatencyMs"], 10, 64); parseErr == nil {
							avgLatency = al
						}
						if r429, parseErr := strconv.Atoi(stateMap["Recent429"]); parseErr == nil {
							recent429 = r429
						}
						if rt, parseErr := strconv.Atoi(stateMap["RecentTimeouts"]); parseErr == nil {
							recentTimeouts = rt
						}
						cooldownUntil = stateMap["CooldownUntil"]
					}

					list = append(list, gin.H{
						"node_id":         nodeID,
						"account_name":    accName,
						"provider":        provider,
						"state":           nodeState,
						"capabilities":    []string{"messages", "stream"},
						"success_rate":    successRate,
						"avg_latency_ms":  avgLatency,
						"recent_429":      recent429,
						"recent_timeouts": recentTimeouts,
						"cooldown_until":  cooldownUntil,
					})
				}

				c.JSON(http.StatusOK, list)
			})

			// GET /ash/events?owner_id=...
			adminGroup.GET("/events", func(c *gin.Context) {
				engine := h.Gateway.GetHypervisorEngine()
				if engine == nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "hypervisor engine not initialized"})
					return
				}

				query := engine.Storage().DB().AshEvent.Query()
				if ownerID := c.Query("owner_id"); ownerID != "" {
					query = query.Where(ashevent.OwnerID(ownerID))
				}

				events, err := query.Order(ent.Desc(ashevent.FieldCreatedAt)).Limit(100).All(c.Request.Context())
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, events)
			})

			// GET /ash/usage?owner_id=...
			adminGroup.GET("/usage", func(c *gin.Context) {
				engine := h.Gateway.GetHypervisorEngine()
				if engine == nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "hypervisor engine not initialized"})
					return
				}

				query := engine.Storage().DB().AshUsage.Query()
				if ownerID := c.Query("owner_id"); ownerID != "" {
					query = query.Where(ashusage.OwnerID(ownerID))
				}

				usages, err := query.Order(ent.Desc(ashusage.FieldCreatedAt)).Limit(100).All(c.Request.Context())
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, usages)
			})

			// POST /ash/admin/nodes/{node_id}/cooldown
			adminGroup.POST("/admin/nodes/:node_id/cooldown", func(c *gin.Context) {
				engine := h.Gateway.GetHypervisorEngine()
				if engine == nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "hypervisor engine not initialized"})
					return
				}
				nodeID := c.Param("node_id")

				nm := health.NewNodeManager(engine.Storage())
				node, err := nm.GetNode(c.Request.Context(), nodeID)
				if err != nil || node == nil {
					c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
					return
				}
				if strings.EqualFold(node.Provider, "yunwu") && !strings.EqualFold(c.Query("pool"), "yunwu") {
					c.JSON(http.StatusBadRequest, gin.H{"error": "node belongs to yunwu pool; pass pool=yunwu query parameter to modify yunwu nodes"})
					return
				}

				cooldownUntil := time.Now().Add(10 * time.Minute)
				node.State = "cooldown"
				_ = nm.SaveNode(c.Request.Context(), node)

				state, err := nm.GetNodeRuntimeState(c.Request.Context(), nodeID)
				if err != nil || state == nil {
					state = &health.NodeRuntimeState{NodeID: nodeID}
				}
				state.CooldownUntil = &cooldownUntil
				_ = nm.SaveNodeRuntimeState(c.Request.Context(), state)

				_, _ = engine.Storage().LogEvent(c.Request.Context(), "admin", nil, &nodeID, "NodeCooldownStarted", map[string]any{
					"duration": "10m",
					"actor":    "admin",
				})
				slog.Info("[AshAdmin] Admin triggered node cooldown", "node_id", nodeID, "cooldown_until", cooldownUntil)

				c.JSON(http.StatusOK, gin.H{"status": "ok", "cooldown_until": cooldownUntil})
			})

			// POST /ash/admin/nodes/{node_id}/disable
			adminGroup.POST("/admin/nodes/:node_id/disable", func(c *gin.Context) {
				engine := h.Gateway.GetHypervisorEngine()
				if engine == nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "hypervisor engine not initialized"})
					return
				}
				nodeID := c.Param("node_id")

				nm := health.NewNodeManager(engine.Storage())
				node, err := nm.GetNode(c.Request.Context(), nodeID)
				if err != nil || node == nil {
					c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
					return
				}
				if strings.EqualFold(node.Provider, "yunwu") && !strings.EqualFold(c.Query("pool"), "yunwu") {
					c.JSON(http.StatusBadRequest, gin.H{"error": "node belongs to yunwu pool; pass pool=yunwu query parameter to modify yunwu nodes"})
					return
				}

				node.State = "unavailable"
				_ = nm.SaveNode(c.Request.Context(), node)

				_, _ = engine.Storage().LogEvent(c.Request.Context(), "admin", nil, &nodeID, "NodeUnavailable", map[string]any{
					"actor": "admin",
				})
				slog.Info("[AshAdmin] Admin disabled node", "node_id", nodeID)

				c.JSON(http.StatusOK, gin.H{"status": "ok", "node_state": "unavailable"})
			})

			// POST /ash/admin/nodes/{node_id}/enable
			adminGroup.POST("/admin/nodes/:node_id/enable", func(c *gin.Context) {
				engine := h.Gateway.GetHypervisorEngine()
				if engine == nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "hypervisor engine not initialized"})
					return
				}
				nodeID := c.Param("node_id")

				nm := health.NewNodeManager(engine.Storage())
				node, err := nm.GetNode(c.Request.Context(), nodeID)
				if err != nil || node == nil {
					c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
					return
				}
				if strings.EqualFold(node.Provider, "yunwu") && !strings.EqualFold(c.Query("pool"), "yunwu") {
					c.JSON(http.StatusBadRequest, gin.H{"error": "node belongs to yunwu pool; pass pool=yunwu query parameter to modify yunwu nodes"})
					return
				}

				node.State = "healthy"
				_ = nm.SaveNode(c.Request.Context(), node)

				state, err := nm.GetNodeRuntimeState(c.Request.Context(), nodeID)
				if err == nil && state != nil {
					state.CooldownUntil = nil
					state.Recent429 = 0
					state.RecentTimeouts = 0
					_ = nm.SaveNodeRuntimeState(c.Request.Context(), state)
				}

				_, _ = engine.Storage().LogEvent(c.Request.Context(), "admin", nil, &nodeID, "NodeRecovered", map[string]any{
					"actor": "admin",
				})
				slog.Info("[AshAdmin] Admin enabled node", "node_id", nodeID)

				c.JSON(http.StatusOK, gin.H{"status": "ok", "node_state": "healthy"})
			})

			// POST /ash/admin/users/{owner_id}/plan
			adminGroup.POST("/admin/users/:owner_id/plan", func(c *gin.Context) {
				engine := h.Gateway.GetHypervisorEngine()
				if engine == nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "hypervisor engine not initialized"})
					return
				}
				ownerID := c.Param("owner_id")

				var body struct {
					PlanID string `json:"plan_id"`
				}
				if err := c.ShouldBindJSON(&body); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
					return
				}

				poolType := c.Query("pool_type")
				if poolType == "" {
					poolType = "standard"
				}
				qm := quota.NewQuotaManager(engine.Storage())
				ua, err := qm.GetUserAccess(c.Request.Context(), ownerID, poolType)
				if err != nil {
					ua = &quota.UserAccess{
						OwnerID:      ownerID,
						PlanID:       "default",
						MonthlyLimit: 5000000,
						DailyBurst:   500000,
						Priority:     1,
						IsActive:     true,
					}
				}

				ua.PlanID = body.PlanID

				// Save back to Redis via QuotaManager
				_ = qm.SaveUserAccess(c.Request.Context(), ua, poolType)

				_, _ = engine.Storage().LogEvent(c.Request.Context(), "admin", nil, nil, "PlanAssigned", map[string]any{
					"owner_id": ownerID,
					"plan_id":  body.PlanID,
				})

				c.JSON(http.StatusOK, ua)
			})

			// POST /ash/admin/users/{owner_id}/quota
			adminGroup.POST("/admin/users/:owner_id/quota", func(c *gin.Context) {
				engine := h.Gateway.GetHypervisorEngine()
				if engine == nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "hypervisor engine not initialized"})
					return
				}
				ownerID := c.Param("owner_id")

				var body struct {
					MonthlyLimit         *int64 `json:"monthly_limit"`
					DailyBurst           *int64 `json:"daily_burst"`
					MonthlyLimitMicroUSD *int64 `json:"monthly_limit_micro_usd"`
					DailyBurstMicroUSD   *int64 `json:"daily_burst_micro_usd"`
					Priority             *int   `json:"priority"`
					IsActive             *bool  `json:"is_active"`
				}
				if err := c.ShouldBindJSON(&body); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
					return
				}

				poolType := c.Query("pool_type")
				if poolType == "" {
					poolType = "standard"
				}
				if poolType != "standard" && poolType != "yunwu" {
					c.JSON(http.StatusBadRequest, gin.H{"error": "pool_type must be standard or yunwu"})
					return
				}
				if poolType == "standard" && (body.MonthlyLimitMicroUSD != nil || body.DailyBurstMicroUSD != nil) {
					c.JSON(http.StatusBadRequest, gin.H{"error": "micro-USD limits are only valid for pool_type=yunwu"})
					return
				}
				if poolType == "yunwu" && (body.MonthlyLimit != nil || body.DailyBurst != nil) {
					c.JSON(http.StatusBadRequest, gin.H{"error": "token limits are only valid for pool_type=standard"})
					return
				}
				qm := quota.NewQuotaManager(engine.Storage())
				ua, err := qm.GetUserAccess(c.Request.Context(), ownerID, poolType)
				if err != nil {
					ua = &quota.UserAccess{
						OwnerID:      ownerID,
						PlanID:       "default",
						MonthlyLimit: 5000000,
						DailyBurst:   500000,
						Priority:     1,
						IsActive:     true,
					}
				}

				if body.MonthlyLimit != nil {
					ua.MonthlyLimit = *body.MonthlyLimit
				}
				if body.DailyBurst != nil {
					ua.DailyBurst = *body.DailyBurst
				}
				if body.MonthlyLimitMicroUSD != nil {
					ua.MonthlyLimitMicroUSD = *body.MonthlyLimitMicroUSD
				}
				if body.DailyBurstMicroUSD != nil {
					ua.DailyBurstMicroUSD = *body.DailyBurstMicroUSD
				}
				if body.Priority != nil {
					ua.Priority = *body.Priority
				}
				if body.IsActive != nil {
					ua.IsActive = *body.IsActive
				}

				if err := qm.SaveUserAccess(c.Request.Context(), ua, poolType); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				_, _ = engine.Storage().LogEvent(c.Request.Context(), "admin", nil, nil, "UserQuotaUpdated", map[string]any{
					"owner_id":      ownerID,
					"pool_type":     poolType,
					"monthly_limit": ua.MonthlyLimit,
					"daily_burst":   ua.DailyBurst,
					"priority":      ua.Priority,
					"is_active":     ua.IsActive,
				})

				c.JSON(http.StatusOK, ua)
			})

			// GET /ash/stats/analytics
			adminGroup.GET("/stats/analytics", func(c *gin.Context) {
				engine := h.Gateway.GetHypervisorEngine()
				if engine == nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "hypervisor engine not initialized"})
					return
				}

				ctx := c.Request.Context()
				since24h := time.Now().Add(-24 * time.Hour)

				rebindingCount, _ := engine.Storage().DB().AshEvent.Query().
					Where(
						ashevent.EventType("RebindingSucceeded"),
						ashevent.CreatedAtGTE(since24h),
					).Count(ctx)

				totalEventsCount, _ := engine.Storage().DB().AshEvent.Query().
					Where(ashevent.CreatedAtGTE(since24h)).Count(ctx)
				var usageByOwner []struct {
					OwnerID string `json:"owner_id"`
					Used    int64  `json:"used"`
				}
				_ = engine.Storage().DB().AshUsage.Query().Where(ashusage.CreatedAtGTE(since24h)).
					GroupBy(ashusage.FieldOwnerID).
					Aggregate(ent.As(ent.Sum(ashusage.FieldCostUnits), "used")).
					Scan(ctx, &usageByOwner)
				sort.Slice(usageByOwner, func(i, j int) bool { return usageByOwner[i].Used > usageByOwner[j].Used })
				var totalUsed int64
				for _, item := range usageByOwner {
					totalUsed += item.Used
				}
				heavyUsers := make([]any, 0)
				if len(usageByOwner) > 0 {
					average := totalUsed / int64(len(usageByOwner))
					for _, item := range usageByOwner {
						if item.Used >= average && item.Used > 0 {
							heavyUsers = append(heavyUsers, item)
						}
					}
				}
				if len(usageByOwner) > 10 {
					usageByOwner = usageByOwner[:10]
				}
				var depositTotals []struct {
					Amount int64 `json:"amount"`
				}
				_ = engine.Storage().DB().AshDeposit.Query().Aggregate(ent.As(ent.Sum(ashdeposit.FieldAmountCents), "amount")).Scan(ctx, &depositTotals)
				revenueEstimateCents := int64(0)
				if len(depositTotals) > 0 {
					revenueEstimateCents = depositTotals[0].Amount
				}

				sessionKeys, _ := engine.Storage().Redis().Keys(ctx, "ash:session:*").Result()
				totalNodes, _ := engine.Storage().DB().Account.Query().Where(account.Status("active")).Count(ctx)
				utilization := 0.0
				if totalNodes > 0 {
					utilization = (float64(len(sessionKeys)) / float64(totalNodes)) * 100.0
					if utilization > 100.0 {
						utilization = 100.0
					}
				}
				unusedCapacity := int64(totalNodes)*quota.NodeCapacity - totalUsed
				if unusedCapacity < 0 {
					unusedCapacity = 0
				}
				liveMetrics := observability.Default.Current()
				upstream429Rate := float64(0)
				if liveMetrics.RequestsTotal > 0 {
					upstream429Rate = float64(liveMetrics.Upstream429Total) / float64(liveMetrics.RequestsTotal)
				}

				c.JSON(http.StatusOK, gin.H{
					"status":                   "ok",
					"rebinding_24h_count":      rebindingCount,
					"total_events_24h":         totalEventsCount,
					"pool_utilization_percent": utilization,
					"top_users":                usageByOwner,
					"heavy_users":              heavyUsers,
					"unused_capacity_units":    unusedCapacity,
					"upstream_429_rate":        upstream429Rate,
					"revenue_estimate_cents":   revenueEstimateCents,
				})
			})

			// GET /ash/admin/deposits
			adminGroup.GET("/admin/deposits", func(c *gin.Context) {
				engine := h.Gateway.GetHypervisorEngine()
				if engine == nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "hypervisor engine not initialized"})
					return
				}

				query := engine.Storage().DB().AshDeposit.Query()
				if ownerID := c.Query("owner_id"); ownerID != "" {
					query = query.Where(ashdeposit.OwnerID(ownerID))
				}

				deposits, err := query.Order(ent.Desc(ashdeposit.FieldCreatedAt)).All(c.Request.Context())
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, deposits)
			})

			// POST /ash/admin/deposits
			adminGroup.POST("/admin/deposits", func(c *gin.Context) {
				engine := h.Gateway.GetHypervisorEngine()
				if engine == nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "hypervisor engine not initialized"})
					return
				}

				var req struct {
					OwnerID     string `json:"owner_id" binding:"required"`
					AmountCents int64  `json:"amount_cents" binding:"required"`
					Currency    string `json:"currency"`
					NoticeDays  int    `json:"notice_days"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload"})
					return
				}
				if req.Currency == "" {
					req.Currency = "USD"
				}
				if req.NoticeDays <= 0 {
					req.NoticeDays = 10
				}

				dep, err := engine.Storage().DB().AshDeposit.Create().
					SetOwnerID(req.OwnerID).
					SetAmountCents(req.AmountCents).
					SetCurrency(req.Currency).
					SetNoticeDays(req.NoticeDays).
					SetState("held").
					Save(c.Request.Context())

				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}

				_, _ = engine.Storage().LogEvent(c.Request.Context(), req.OwnerID, nil, nil, "DepositCreated", map[string]any{
					"deposit_id":   dep.ID,
					"amount_cents": dep.AmountCents,
					"currency":     dep.Currency,
				})
				slog.Info("[AshAdmin] Created security deposit", "owner_id", req.OwnerID, "amount_cents", req.AmountCents)

				c.JSON(http.StatusOK, dep)
			})

			// POST /ash/admin/deposits/{deposit_id}/release
			adminGroup.POST("/admin/deposits/:deposit_id/release", func(c *gin.Context) {
				engine := h.Gateway.GetHypervisorEngine()
				if engine == nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "hypervisor engine not initialized"})
					return
				}
				depositIDStr := c.Param("deposit_id")
				depositID, parseErr := strconv.ParseInt(depositIDStr, 10, 64)

				now := time.Now()
				if parseErr == nil {
					_ = engine.Storage().DB().AshDeposit.UpdateOneID(depositID).
						SetState("released").
						SetReleasedAt(now).
						Exec(c.Request.Context())
				}

				ownerID := c.Query("owner_id")
				if ownerID != "" {
					poolType := c.Query("pool_type")
					if poolType == "" {
						poolType = "standard"
					}
					qm := quota.NewQuotaManager(engine.Storage())
					if ua, err := qm.GetUserAccess(c.Request.Context(), ownerID, poolType); err == nil && ua != nil {
						ua.DepositState = "released"
						data, _ := json.Marshal(ua)
						_ = engine.Storage().Redis().Set(c.Request.Context(), fmt.Sprintf("ash:user_access:%s:%s", ownerID, poolType), data, 0).Err()
					}
				}

				_, _ = engine.Storage().LogEvent(c.Request.Context(), "admin", nil, nil, "DepositReleased", map[string]any{
					"deposit_id": depositIDStr,
				})
				slog.Info("[AshAdmin] Released deposit", "deposit_id", depositIDStr)

				c.JSON(http.StatusOK, gin.H{"status": "ok", "deposit_state": "released"})
			})

			// =========================================================================
			// DEDICATED YUNWU HYPERVISOR ENGINE ROUTES
			// =========================================================================

			// GET /ash/admin/yunwu/health
			adminGroup.GET("/admin/yunwu/health", func(c *gin.Context) {
				slog.Info("[YunwuAdminAPI] Fetching Yunwu system health")
				engine := h.Gateway.GetHypervisorEngine()
				if engine == nil {
					slog.Error("[YunwuAdminAPI] Hypervisor engine not initialized")
					c.JSON(http.StatusInternalServerError, gin.H{"error": "hypervisor engine not initialized"})
					return
				}

				yh := engine.YunwuEngine()
				if yh == nil {
					yh = yunwu.NewYunwuHypervisor(engine.Storage(), nil)
				}
				c.JSON(http.StatusOK, yh.GetHealthStatus(c.Request.Context()))
			})

			// GET /ash/admin/yunwu/nodes
			adminGroup.GET("/admin/yunwu/nodes", func(c *gin.Context) {
				slog.Info("[YunwuAdminAPI] Listing Yunwu execution nodes")
				engine := h.Gateway.GetHypervisorEngine()
				if engine == nil {
					slog.Error("[YunwuAdminAPI] Hypervisor engine not initialized")
					c.JSON(http.StatusInternalServerError, gin.H{"error": "hypervisor engine not initialized"})
					return
				}

				dbAccounts, _ := engine.Storage().DB().Account.Query().
					Where(account.PlatformEqualFold("yunwu")).
					All(c.Request.Context())

				if len(dbAccounts) == 0 {
					dbAccounts, _ = engine.Storage().DB().Account.Query().
						Where(account.NameContainsFold("yunwu")).
						All(c.Request.Context())
				}

				var list []gin.H
				for _, acc := range dbAccounts {
					nodeID := fmt.Sprintf("%d", acc.ID)
					snapshot, _ := engine.YunwuEngine().BalanceSnapshot(c.Request.Context(), nodeID)
					availableUSD, allocatableUSD, balanceStatus, balanceSource := 0.0, 0.0, "balance_unknown", ""
					var synchronizedAt, staleAt any
					reservePercent := engine.YunwuEngine().BalanceReservePercent()
					if snapshot != nil {
						availableUSD = snapshot.AvailableUSD
						allocatableUSD = float64(maxInt64(0, snapshot.EstimatedAvailableMicroUSD-snapshot.ReserveMicroUSD)) / 1_000_000
						balanceStatus = snapshot.Status
						balanceSource = snapshot.Source
						synchronizedAt = snapshot.SynchronizedAt
						staleAt = snapshot.StaleAt
					}

					nodeState := "healthy"
					if state, err := health.NewNodeManager(engine.Storage()).GetNodeRuntimeState(c.Request.Context(), nodeID); err == nil && state != nil {
						if state.CooldownUntil != nil && time.Now().Before(*state.CooldownUntil) {
							nodeState = "cooldown"
						}
					}

					baseURL := "https://api.yunwu.ai/v1"
					if acc.Credentials != nil {
						if urlStr, ok := acc.Credentials["base_url"].(string); ok && urlStr != "" {
							baseURL = urlStr
						}
					}

					list = append(list, gin.H{
						"node_id":                 nodeID,
						"account_name":            acc.Name,
						"platform":                "yunwu",
						"type":                    acc.Type,
						"state":                   nodeState,
						"total_available_usd":     availableUSD,
						"allocatable_usd":         allocatableUSD,
						"balance_status":          balanceStatus,
						"balance_source":          balanceSource,
						"balance_synchronized_at": synchronizedAt,
						"balance_stale_at":        staleAt,
						"reserve_percent":         reservePercent,
						"base_url":                baseURL,
						"status":                  acc.Status,
					})
				}

				slog.Info("[YunwuAdminAPI] Returned Yunwu nodes list", "count", len(list))
				c.JSON(http.StatusOK, list)
			})

			// POST /ash/admin/yunwu/nodes/:id/sync - Manual balance sync with Yunwu API
			adminGroup.POST("/admin/yunwu/nodes/:id/sync", func(c *gin.Context) {
				nodeID := c.Param("id")
				slog.Info("[YunwuAdminAPI] Manual balance sync requested for node", "node_id", nodeID)

				engine := h.Gateway.GetHypervisorEngine()
				if engine == nil {
					slog.Error("[YunwuAdminAPI] Hypervisor engine not initialized")
					c.JSON(http.StatusInternalServerError, gin.H{"error": "hypervisor engine not initialized"})
					return
				}

				yh := engine.YunwuEngine()
				if yh == nil {
					yh = yunwu.NewYunwuHypervisor(engine.Storage(), nil)
				}
				grants, err := yh.SyncNodeBalance(c.Request.Context(), nodeID, "", "")
				if err != nil {
					slog.Error("[YunwuAdminAPI] Sync node balance failed", "node_id", nodeID, "error", err)
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				_, _ = engine.Storage().LogEvent(c.Request.Context(), "admin", nil, &nodeID, "YunwuBalanceSynced", map[string]any{
					"total_available": grants.TotalAvailable,
				})

				slog.Info("[YunwuAdminAPI] Sync node balance succeeded", "node_id", nodeID, "total_available", grants.TotalAvailable)
				c.JSON(http.StatusOK, grants)
			})
		}
	}

	legacy := r.Group("/ash")
	legacy.Use(func(c *gin.Context) {
		c.Header("Deprecation", "true")
		c.Header("Sunset", "Wed, 05 Aug 2027 00:00:00 GMT")
		c.Header("Link", `</api/v1/ash>; rel="successor-version"`)
		c.Next()
	})
	setupRoutes(legacy)
	setupRoutes(r.Group("/api/v1/ash"))
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func userOperationalStats(ctx context.Context, engine *ashhypervisor.Hypervisor, ownerID string) (int, int, []gin.H) {
	sessionKeys, _ := engine.Storage().Redis().Keys(ctx, "ash:session:*").Result()
	sm := session.NewSessionManager(engine.Storage())
	activeSessions := 0
	for _, key := range sessionKeys {
		sessionID := strings.TrimPrefix(key, "ash:session:")
		if sess, err := sm.GetSession(ctx, sessionID); err == nil && sess != nil && sess.OwnerID == ownerID {
			activeSessions++
		}
	}
	rebindingCount, _ := engine.Storage().DB().AshEvent.Query().Where(
		ashevent.OwnerID(ownerID),
		ashevent.EventType("RebindingSucceeded"),
	).Count(ctx)
	errorEvents, _ := engine.Storage().DB().AshEvent.Query().Where(
		ashevent.OwnerID(ownerID),
		ashevent.EventTypeIn("RequestFailed", "RebindingFailed"),
	).Order(ent.Desc(ashevent.FieldCreatedAt)).Limit(5).All(ctx)
	lastErrors := make([]gin.H, 0, len(errorEvents))
	for _, event := range errorEvents {
		lastErrors = append(lastErrors, gin.H{
			"event_type": event.EventType,
			"session_id": event.SessionID,
			"node_id":    event.NodeID,
			"payload":    event.Payload,
			"created_at": event.CreatedAt,
		})
	}
	return activeSessions, rebindingCount, lastErrors
}

package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/account"
	"github.com/Wei-Shaw/sub2api/ent/ashevent"
	"github.com/Wei-Shaw/sub2api/ent/ashusage"
	"github.com/Wei-Shaw/sub2api/ent/usagelog"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/lease"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/quota"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/session"

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

		plat := c.Query("platform")
		if plat == "" {
			plat = "anthropic"
		}

		state := engine.HealthController().GetSystemState(c.Request.Context(), plat)

		dbAccounts, _ := engine.Storage().DB().Account.Query().
			Where(account.StatusIn("active", "error")).
			All(c.Request.Context())

		healthyCount := 0
		cooldownCount := 0
		totalCount := len(dbAccounts)

		for _, acc := range dbAccounts {
			nodeID := fmt.Sprintf("%d", acc.ID)
			node, err := engine.Storage().Redis().HGetAll(c.Request.Context(), "ash:node:"+nodeID).Result()
			st := "healthy"
			if err == nil && len(node) > 0 && node["State"] != "" {
				st = node["State"]
			}
			if st == "cooldown" {
				cooldownCount++
			} else if st == "healthy" || st == "busy" {
				healthyCount++
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"status":         "ok",
			"system_state":   state,
			"platform":       plat,
			"total_nodes":    totalCount,
			"healthy_nodes":  healthyCount,
			"cooldown_nodes": cooldownCount,
		})
	})

	// User endpoints (requires JWT Auth)
	userGroup := ash.Group("")
	userGroup.Use(gin.HandlerFunc(jwtAuth))
	{
		// GET /ash/stats/users/{owner_id}
		userGroup.GET("/stats/users/:owner_id", func(c *gin.Context) {
			engine := h.Gateway.GetHypervisorEngine()
			if engine == nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "hypervisor engine not initialized"})
				return
			}
			ownerID := c.Param("owner_id")

			qm := quota.NewQuotaManager(engine.Storage())
			ua, err := qm.GetUserAccess(c.Request.Context(), ownerID)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "user quota access not found"})
				return
			}

			// Sum user usage tokens
			var sum []struct {
				Input  int64 `json:"input"`
				Output int64 `json:"output"`
			}
			_ = engine.Storage().DB().AshUsage.Query().
				Where(ashusage.OwnerID(ownerID)).
				Aggregate(
					ent.Sum(ashusage.FieldInputTokens),
					ent.Sum(ashusage.FieldOutputTokens),
				).
				Scan(c.Request.Context(), &sum)

			var inputUsed, outputUsed int64
			if len(sum) > 0 {
				inputUsed = sum[0].Input
				outputUsed = sum[0].Output
			}

			c.JSON(http.StatusOK, gin.H{
				"owner_id":       ua.OwnerID,
				"plan_id":        ua.PlanID,
				"monthly_limit":  ua.MonthlyLimit,
				"daily_burst":    ua.DailyBurst,
				"used_units":     ua.UsedUnits,
				"input_tokens":   inputUsed,
				"output_tokens":  outputUsed,
				"priority":       ua.Priority,
				"deposit_state":  ua.DepositState,
				"is_active":      ua.IsActive,
			})
		})

		// GET /ash/sessions/{session_id}
		userGroup.GET("/sessions/:session_id", func(c *gin.Context) {
			engine := h.Gateway.GetHypervisorEngine()
			if engine == nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "hypervisor engine not initialized"})
				return
			}
			sessionID := c.Param("session_id")

			sm := session.NewSessionManager(engine.Storage())
			sess, err := sm.GetSession(c.Request.Context(), sessionID)
			if err != nil || sess == nil {
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
	}

	// Admin endpoints (requires Admin Auth)
	adminGroup := ash.Group("")
	adminGroup.Use(gin.HandlerFunc(adminAuth))
	{
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
					ent.Sum(ashusage.FieldInputTokens),
					ent.Sum(ashusage.FieldOutputTokens),
					ent.Sum(ashusage.FieldCostUnits),
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
						ent.Sum(usagelog.FieldInputTokens),
						ent.Sum(usagelog.FieldOutputTokens),
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

			c.JSON(http.StatusOK, gin.H{
				"total_input_tokens":   input,
				"total_output_tokens":  output,
				"total_cost_units":     cost,
				"active_sessions_count": len(sessionKeys),
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

			node.State = "unavailable"
			_ = nm.SaveNode(c.Request.Context(), node)

			_, _ = engine.Storage().LogEvent(c.Request.Context(), "admin", nil, &nodeID, "NodeUnavailable", map[string]any{
				"actor": "admin",
			})

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

			node.State = "healthy"
			_ = nm.SaveNode(c.Request.Context(), node)

			state, err := nm.GetNodeRuntimeState(c.Request.Context(), nodeID)
			if err != nil || state == nil {
				state = &health.NodeRuntimeState{NodeID: nodeID}
			}
			state.CooldownUntil = nil
			state.Recent429 = 0
			state.RecentTimeouts = 0
			_ = nm.SaveNodeRuntimeState(c.Request.Context(), state)

			_, _ = engine.Storage().LogEvent(c.Request.Context(), "admin", nil, &nodeID, "NodeRecovered", map[string]any{
				"actor": "admin",
			})

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

			qm := quota.NewQuotaManager(engine.Storage())
			ua, err := qm.GetUserAccess(c.Request.Context(), ownerID)
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
			
			// Save back to Redis
			data, _ := json.Marshal(ua)
			_ = engine.Storage().Redis().Set(c.Request.Context(), "ash:user_access:"+ownerID, data, 0).Err()

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
				MonthlyLimit *int64 `json:"monthly_limit"`
				DailyBurst   *int64 `json:"daily_burst"`
				Priority     *int   `json:"priority"`
				IsActive     *bool  `json:"is_active"`
			}
			if err := c.ShouldBindJSON(&body); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
				return
			}

			qm := quota.NewQuotaManager(engine.Storage())
			ua, err := qm.GetUserAccess(c.Request.Context(), ownerID)
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
			if body.Priority != nil {
				ua.Priority = *body.Priority
			}
			if body.IsActive != nil {
				ua.IsActive = *body.IsActive
			}

			data, _ := json.Marshal(ua)
			_ = engine.Storage().Redis().Set(c.Request.Context(), "ash:user_access:"+ownerID, data, 0).Err()

			c.JSON(http.StatusOK, ua)
		})

		// POST /ash/admin/deposits/{deposit_id}/release
		adminGroup.POST("/admin/deposits/:deposit_id/release", func(c *gin.Context) {
			engine := h.Gateway.GetHypervisorEngine()
			if engine == nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "hypervisor engine not initialized"})
				return
			}
			depositID := c.Param("deposit_id")

			ownerID := c.Query("owner_id")
			if ownerID != "" {
				qm := quota.NewQuotaManager(engine.Storage())
				if ua, err := qm.GetUserAccess(c.Request.Context(), ownerID); err == nil && ua != nil {
					ua.DepositState = "released"
					data, _ := json.Marshal(ua)
					_ = engine.Storage().Redis().Set(c.Request.Context(), "ash:user_access:"+ownerID, data, 0).Err()
				}
			}

			_, _ = engine.Storage().LogEvent(c.Request.Context(), "admin", nil, nil, "DepositReleased", map[string]any{
				"deposit_id": depositID,
			})

			c.JSON(http.StatusOK, gin.H{"status": "ok", "deposit_state": "released"})
		})
	}
	}

	setupRoutes(r.Group("/ash"))
	setupRoutes(r.Group("/api/v1/ash"))
}

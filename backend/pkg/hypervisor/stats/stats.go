package stats

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/account"
	"github.com/Wei-Shaw/sub2api/ent/ashevent"
	"github.com/Wei-Shaw/sub2api/ent/ashusage"
	"github.com/Wei-Shaw/sub2api/ent/usagelog"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/quota"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
)

type GlobalStats struct {
	TotalInputTokens   int64 `json:"total_input_tokens"`
	TotalOutputTokens  int64 `json:"total_output_tokens"`
	TotalCostUnits     int64 `json:"total_cost_units"`
	ActiveSessions     int   `json:"active_sessions_count"`
	LegacyInputTokens  int64 `json:"legacy_input_tokens"`
	LegacyOutputTokens int64 `json:"legacy_output_tokens"`
	LegacyCostUnits    int64 `json:"legacy_cost_units"`
}

type NodeStat struct {
	NodeID         string   `json:"node_id"`
	AccountName    string   `json:"account_name"`
	Provider       string   `json:"provider"`
	State          string   `json:"state"`
	Capabilities   []string `json:"capabilities"`
	SuccessRate    float64  `json:"success_rate"`
	AvgLatencyMs   int64    `json:"avg_latency_ms"`
	Recent429      int      `json:"recent_429"`
	RecentTimeouts int      `json:"recent_timeouts"`
	CooldownUntil  string   `json:"cooldown_until,omitempty"`
}

type UserStat struct {
	OwnerID              string `json:"owner_id"`
	PlanID               string `json:"plan_id"`
	MonthlyLimit         int64  `json:"monthly_limit"`
	DailyBurst           int64  `json:"daily_burst"`
	UsedUnits            int64  `json:"used_units"`
	MonthlyLimitMicroUSD int64  `json:"monthly_limit_micro_usd"`
	DailyBurstMicroUSD   int64  `json:"daily_burst_micro_usd"`
	UsedMicroUSD         int64  `json:"used_micro_usd"`
	RemainingMicroUSD    int64  `json:"remaining_micro_usd"`
	InputTokens          int64  `json:"input_tokens"`
	OutputTokens         int64  `json:"output_tokens"`
	Priority             int    `json:"priority"`
	DepositState         string `json:"deposit_state"`
	IsActive             bool   `json:"is_active"`
}

type AnalyticsSummary struct {
	Status                 string  `json:"status"`
	Rebinding24hCount      int     `json:"rebinding_24h_count"`
	TotalEvents24h         int     `json:"total_events_24h"`
	PoolUtilizationPercent float64 `json:"pool_utilization_percent"`
}

type StatsCollector struct {
	store *storage.Storage
	qm    *quota.QuotaManager
	nm    *health.NodeManager
}

func NewStatsCollector(store *storage.Storage) *StatsCollector {
	return &StatsCollector{
		store: store,
		qm:    quota.NewQuotaManager(store),
		nm:    health.NewNodeManager(store),
	}
}

func (sc *StatsCollector) GetGlobalStats(ctx context.Context) (*GlobalStats, error) {
	var sum []struct {
		Input  int64 `json:"input"`
		Output int64 `json:"output"`
		Cost   int64 `json:"cost"`
	}
	// usage_logs is canonical. ash_usages is reported separately for the
	// transition period and is never mixed into quota or billing totals.
	_ = sc.store.DB().UsageLog.Query().
		Aggregate(
			ent.As(ent.Sum(usagelog.FieldInputTokens), "input"),
			ent.As(ent.Sum(usagelog.FieldOutputTokens), "output"),
		).
		Scan(ctx, &sum)

	var input, output, cost int64
	if len(sum) > 0 {
		input = sum[0].Input
		output = sum[0].Output
		cost = input + output
	}

	var legacy []struct {
		Input  int64 `json:"input"`
		Output int64 `json:"output"`
		Cost   int64 `json:"cost"`
	}
	_ = sc.store.DB().AshUsage.Query().Aggregate(
		ent.As(ent.Sum(ashusage.FieldInputTokens), "input"),
		ent.As(ent.Sum(ashusage.FieldOutputTokens), "output"),
		ent.As(ent.Sum(ashusage.FieldCostUnits), "cost"),
	).Scan(ctx, &legacy)

	activeSessions := sc.countKeys(ctx, "ash:session:*")

	return &GlobalStats{
		TotalInputTokens:  input,
		TotalOutputTokens: output,
		TotalCostUnits:    cost,
		ActiveSessions:    activeSessions,
		LegacyInputTokens: legacyValue(legacy, func(v struct {
			Input  int64 `json:"input"`
			Output int64 `json:"output"`
			Cost   int64 `json:"cost"`
		}) int64 {
			return v.Input
		}),
		LegacyOutputTokens: legacyValue(legacy, func(v struct {
			Input  int64 `json:"input"`
			Output int64 `json:"output"`
			Cost   int64 `json:"cost"`
		}) int64 {
			return v.Output
		}),
		LegacyCostUnits: legacyValue(legacy, func(v struct {
			Input  int64 `json:"input"`
			Output int64 `json:"output"`
			Cost   int64 `json:"cost"`
		}) int64 {
			return v.Cost
		}),
	}, nil
}

func (sc *StatsCollector) GetUserStats(ctx context.Context, ownerID string, poolType string) (*UserStat, error) {
	if poolType == "" {
		poolType = "standard"
	}
	ua, err := sc.qm.GetUserAccess(ctx, ownerID, poolType)
	if err != nil {
		return nil, err
	}

	var sum []struct {
		Input  int64 `json:"input"`
		Output int64 `json:"output"`
	}
	_ = sc.store.DB().AshUsage.Query().Where(ashusage.OwnerID(ownerID)).Aggregate(
		ent.As(ent.Sum(ashusage.FieldInputTokens), "input"),
		ent.As(ent.Sum(ashusage.FieldOutputTokens), "output"),
	).Scan(ctx, &sum)
	if len(sum) == 0 || (sum[0].Input == 0 && sum[0].Output == 0) {
		sum = nil
		owner, parseErr := strconv.ParseInt(ownerID, 10, 64)
		if parseErr == nil {
			_ = sc.store.DB().UsageLog.Query().Where(usagelog.UserID(owner)).Aggregate(
				ent.As(ent.Sum(usagelog.FieldInputTokens), "input"),
				ent.As(ent.Sum(usagelog.FieldOutputTokens), "output"),
			).Scan(ctx, &sum)
		}
	}

	var inputUsed, outputUsed int64
	if len(sum) > 0 {
		inputUsed = sum[0].Input
		outputUsed = sum[0].Output
	}

	return &UserStat{
		OwnerID:              ua.OwnerID,
		PlanID:               ua.PlanID,
		MonthlyLimit:         ua.MonthlyLimit,
		DailyBurst:           ua.DailyBurst,
		UsedUnits:            ua.UsedUnits,
		MonthlyLimitMicroUSD: ua.MonthlyLimitMicroUSD,
		DailyBurstMicroUSD:   ua.DailyBurstMicroUSD,
		UsedMicroUSD:         ua.UsedMicroUSD,
		RemainingMicroUSD:    nonnegative(ua.MonthlyLimitMicroUSD - ua.UsedMicroUSD),
		InputTokens:          inputUsed,
		OutputTokens:         outputUsed,
		Priority:             ua.Priority,
		DepositState:         ua.DepositState,
		IsActive:             ua.IsActive,
	}, nil
}

func nonnegative(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}

func (sc *StatsCollector) GetNodeStats(ctx context.Context) ([]NodeStat, error) {
	dbAccounts, _ := sc.store.DB().Account.Query().
		Where(account.StatusIn("active", "error")).
		All(ctx)

	var list []NodeStat
	for _, acc := range dbAccounts {
		nodeID := fmt.Sprintf("%d", acc.ID)
		node, _ := sc.nm.GetNode(ctx, nodeID)

		accName := acc.Name
		if accName == "" {
			accName = fmt.Sprintf("Node #%d", acc.ID)
		}
		provider := acc.Platform
		if provider == "" {
			provider = "anthropic"
		}
		nodeState := "healthy"
		if node != nil && node.State != "" {
			nodeState = node.State
		}

		state, _ := sc.nm.GetNodeRuntimeState(ctx, nodeID)
		successRate := 1.0
		var avgLatency int64
		var recent429, recentTimeouts int
		var cooldownUntil string

		if state != nil {
			if state.TotalRequests > 0 {
				successRate = state.SuccessRate
			}
			avgLatency = state.AvgLatencyMs
			recent429 = state.Recent429
			recentTimeouts = state.RecentTimeouts
			if state.CooldownUntil != nil {
				cooldownUntil = state.CooldownUntil.Format(time.RFC3339)
			}
		}

		list = append(list, NodeStat{
			NodeID:         nodeID,
			AccountName:    accName,
			Provider:       provider,
			State:          nodeState,
			Capabilities:   []string{"messages", "stream"},
			SuccessRate:    successRate,
			AvgLatencyMs:   avgLatency,
			Recent429:      recent429,
			RecentTimeouts: recentTimeouts,
			CooldownUntil:  cooldownUntil,
		})
	}
	return list, nil
}

func (sc *StatsCollector) GetAnalytics(ctx context.Context) (*AnalyticsSummary, error) {
	since24h := time.Now().Add(-24 * time.Hour)

	rebindingCount, _ := sc.store.DB().AshEvent.Query().
		Where(
			ashevent.EventType("RebindingSucceeded"),
			ashevent.CreatedAtGTE(since24h),
		).Count(ctx)

	totalEventsCount, _ := sc.store.DB().AshEvent.Query().
		Where(ashevent.CreatedAtGTE(since24h)).Count(ctx)

	activeSessions := sc.countKeys(ctx, "ash:session:*")
	totalNodes, _ := sc.store.DB().Account.Query().Where(account.Status("active")).Count(ctx)
	utilization := 0.0
	if totalNodes > 0 {
		utilization = (float64(activeSessions) / float64(totalNodes)) * 100.0
		if utilization > 100.0 {
			utilization = 100.0
		}
	}

	return &AnalyticsSummary{
		Status:                 "ok",
		Rebinding24hCount:      rebindingCount,
		TotalEvents24h:         totalEventsCount,
		PoolUtilizationPercent: utilization,
	}, nil
}

func (sc *StatsCollector) countKeys(ctx context.Context, pattern string) int {
	var cursor uint64
	total := 0
	for {
		keys, next, err := sc.store.Redis().Scan(ctx, cursor, pattern, 500).Result()
		if err != nil {
			return total
		}
		total += len(keys)
		cursor = next
		if cursor == 0 {
			return total
		}
	}
}

func legacyValue[T any](values []T, pick func(T) int64) int64 {
	if len(values) == 0 {
		return 0
	}
	return pick(values[0])
}

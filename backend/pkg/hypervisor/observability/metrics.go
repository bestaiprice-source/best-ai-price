package observability

import "sync/atomic"

// Metrics contains the mandatory process-local hypervisor counters. Durable
// usage and events remain in PostgreSQL; these counters serve live operations.
type Metrics struct {
	requestsTotal           atomic.Int64
	requestsFailedTotal     atomic.Int64
	upstream429Total        atomic.Int64
	rebindingTotal          atomic.Int64
	rebindingFailed         atomic.Int64
	latencyMsTotal          atomic.Int64
	usageUnitsTotal         atomic.Int64
	summaryJobsTotal        atomic.Int64
	snapshotTotal           atomic.Int64
	yunwuBalanceSyncSuccess atomic.Int64
	yunwuBalanceSyncFailure atomic.Int64
	yunwuBalanceStale       atomic.Int64
	yunwuFallbackPricing    atomic.Int64
}

type Snapshot struct {
	RequestsTotal             int64          `json:"requests_total"`
	RequestsFailedTotal       int64          `json:"requests_failed_total"`
	Upstream429Total          int64          `json:"upstream_429_total"`
	RebindingTotal            int64          `json:"rebinding_total"`
	RebindingFailedTotal      int64          `json:"rebinding_failed_total"`
	LatencyMs                 float64        `json:"latency_ms"`
	UsageUnitsTotal           int64          `json:"usage_units_total"`
	SummaryJobsTotal          int64          `json:"summary_jobs_total"`
	SnapshotTotal             int64          `json:"snapshot_total"`
	NodeStateCount            map[string]int `json:"node_state_count"`
	QueueDepth                int            `json:"queue_depth"`
	ActiveSessions            int            `json:"active_sessions"`
	PoolUtilization           float64        `json:"pool_utilization"`
	YunwuBalanceSyncSuccess   int64          `json:"yunwu_balance_sync_success_total"`
	YunwuBalanceSyncFailure   int64          `json:"yunwu_balance_sync_failure_total"`
	YunwuBalanceStale         int64          `json:"yunwu_balance_stale_total"`
	YunwuFallbackPricingTotal int64          `json:"yunwu_fallback_pricing_total"`
}

var Default Metrics

func (m *Metrics) RequestFailed()           { m.requestsFailedTotal.Add(1) }
func (m *Metrics) Upstream429()             { m.upstream429Total.Add(1) }
func (m *Metrics) Rebinding()               { m.rebindingTotal.Add(1) }
func (m *Metrics) RebindingFailed()         { m.rebindingFailed.Add(1) }
func (m *Metrics) AddUsage(units int64)     { m.usageUnitsTotal.Add(units) }
func (m *Metrics) SummaryJob()              { m.summaryJobsTotal.Add(1) }
func (m *Metrics) Snapshot()                { m.snapshotTotal.Add(1) }
func (m *Metrics) YunwuBalanceSynced()      { m.yunwuBalanceSyncSuccess.Add(1) }
func (m *Metrics) YunwuBalanceSyncFailed()  { m.yunwuBalanceSyncFailure.Add(1) }
func (m *Metrics) YunwuBalanceBecameStale() { m.yunwuBalanceStale.Add(1) }
func (m *Metrics) YunwuFallbackPricing()    { m.yunwuFallbackPricing.Add(1) }

func (m *Metrics) ObserveRequest(latencyMs int64) {
	m.requestsTotal.Add(1)
	m.latencyMsTotal.Add(latencyMs)
}

func (m *Metrics) Current() Snapshot {
	requests := m.requestsTotal.Load()
	avgLatency := float64(0)
	if requests > 0 {
		avgLatency = float64(m.latencyMsTotal.Load()) / float64(requests)
	}
	return Snapshot{
		RequestsTotal:             requests,
		RequestsFailedTotal:       m.requestsFailedTotal.Load(),
		Upstream429Total:          m.upstream429Total.Load(),
		RebindingTotal:            m.rebindingTotal.Load(),
		RebindingFailedTotal:      m.rebindingFailed.Load(),
		LatencyMs:                 avgLatency,
		UsageUnitsTotal:           m.usageUnitsTotal.Load(),
		SummaryJobsTotal:          m.summaryJobsTotal.Load(),
		SnapshotTotal:             m.snapshotTotal.Load(),
		YunwuBalanceSyncSuccess:   m.yunwuBalanceSyncSuccess.Load(),
		YunwuBalanceSyncFailure:   m.yunwuBalanceSyncFailure.Load(),
		YunwuBalanceStale:         m.yunwuBalanceStale.Load(),
		YunwuFallbackPricingTotal: m.yunwuFallbackPricing.Load(),
	}
}

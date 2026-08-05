package yunwu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/config"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/observability"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/runtime"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
	"github.com/redis/go-redis/v9"
)

type YunwuCreditGrantsResponse struct {
	Object         string  `json:"object"`
	TotalGranted   float64 `json:"total_granted"`
	TotalUsed      float64 `json:"total_used"`
	TotalAvailable float64 `json:"total_available"`
}

type BalanceSnapshot struct {
	GrantedUSD                 float64   `json:"granted_usd"`
	UsedUSD                    float64   `json:"used_usd"`
	AvailableUSD               float64   `json:"available_usd"`
	AvailableMicroUSD          int64     `json:"available_micro_usd"`
	APIAvailableMicroUSD       int64     `json:"api_available_micro_usd"`
	EstimatedAvailableMicroUSD int64     `json:"estimated_available_micro_usd"`
	ReserveMicroUSD            int64     `json:"reserve_micro_usd"`
	StaleAt                    time.Time `json:"stale_at"`
	Source                     string    `json:"source"`
	SynchronizedAt             time.Time `json:"synchronized_at"`
	Status                     string    `json:"status"`
}

type YunwuUserSelfResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Quota     float64 `json:"quota"`
		UsedQuota float64 `json:"used_quota"`
		Balance   float64 `json:"balance"`
	} `json:"data"`
}

type StatsWorker struct {
	store      *storage.Storage
	nm         *health.NodeManager
	httpClient *http.Client
	cfg        *config.HypervisorConfig
}

func NewStatsWorker(store *storage.Storage, configs ...*config.HypervisorConfig) *StatsWorker {
	cfg := config.LoadFromEnv()
	if len(configs) > 0 && configs[0] != nil {
		cfg = configs[0]
	}
	return &StatsWorker{
		store: store,
		nm:    health.NewNodeManager(store),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		cfg: cfg,
	}
}

func balanceKey(nodeID string) string { return "ash:yunwu:balance:" + nodeID }

func (w *StatsWorker) Snapshot(ctx context.Context, nodeID string, staleAfter time.Duration) (*BalanceSnapshot, error) {
	data, err := w.store.Redis().Get(ctx, balanceKey(nodeID)).Bytes()
	if err != nil {
		return nil, err
	}
	var snap BalanceSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, err
	}
	if time.Since(snap.SynchronizedAt) > staleAfter {
		snap.Status = "stale"
	}
	reserved, _ := w.store.Redis().Get(ctx, reservedKey(nodeID)).Int64()
	snap.EstimatedAvailableMicroUSD = snap.APIAvailableMicroUSD - reserved
	if snap.EstimatedAvailableMicroUSD < 0 {
		snap.EstimatedAvailableMicroUSD = 0
	}
	return &snap, nil
}

func reservedKey(nodeID string) string       { return "ash:yunwu:reserved:" + nodeID }
func reservationKey(requestID string) string { return "ash:yunwu:reservation:" + requestID }

var reserveScript = redis.NewScript(`
local raw = redis.call('GET', KEYS[1])
if not raw or redis.call('EXISTS', KEYS[3]) == 1 then return 0 end
local snap = cjson.decode(raw)
if snap.status ~= 'fresh' then return 0 end
local reserved = tonumber(redis.call('GET', KEYS[2]) or '0')
local available = tonumber(snap.api_available_micro_usd or snap.available_micro_usd or 0)
local reserve = tonumber(snap.reserve_micro_usd or 0)
local amount = tonumber(ARGV[1])
if available - reserved - amount < reserve then return 0 end
redis.call('INCRBY', KEYS[2], amount)
redis.call('SET', KEYS[3], cjson.encode({node_id=ARGV[2], amount=amount, created_at=ARGV[4]}), 'EX', ARGV[3])
return 1
`)

var releaseScript = redis.NewScript(`
local raw = redis.call('GET', KEYS[1])
if not raw then return 0 end
local reservation = cjson.decode(raw)
local amount = tonumber(reservation.amount or 0)
redis.call('DECRBY', KEYS[2], amount)
if tonumber(redis.call('GET', KEYS[2]) or '0') < 0 then redis.call('SET', KEYS[2], 0) end
redis.call('DEL', KEYS[1])
return amount
`)

var reconcileScript = redis.NewScript(`
local reservationRaw = redis.call('GET', KEYS[1])
if not reservationRaw then return 0 end
local reservation = cjson.decode(reservationRaw)
local reserved = tonumber(reservation.amount or 0)
redis.call('DECRBY', KEYS[2], reserved)
if tonumber(redis.call('GET', KEYS[2]) or '0') < 0 then redis.call('SET', KEYS[2], 0) end
local snapshotRaw = redis.call('GET', KEYS[3])
if snapshotRaw then
  local snapshot = cjson.decode(snapshotRaw)
  local api = tonumber(snapshot.api_available_micro_usd or snapshot.available_micro_usd or 0)
  api = math.max(0, api - tonumber(ARGV[1]))
  snapshot.api_available_micro_usd = api
  snapshot.available_micro_usd = api
  snapshot.available_usd = api / 1000000
  snapshot.estimated_available_micro_usd = math.max(0, api - tonumber(redis.call('GET', KEYS[2]) or '0'))
  redis.call('SET', KEYS[3], cjson.encode(snapshot))
end
redis.call('DEL', KEYS[1])
return reserved
`)

func (w *StatsWorker) Reserve(ctx context.Context, nodeID, requestID string, amount int64, ttl time.Duration) (bool, error) {
	if amount <= 0 {
		return false, fmt.Errorf("reservation amount must be positive")
	}
	result, err := reserveScript.Run(ctx, w.store.Redis(), []string{balanceKey(nodeID), reservedKey(nodeID), reservationKey(requestID)}, amount, nodeID, int64(ttl/time.Second), time.Now().UTC().Format(time.RFC3339Nano)).Int()
	return result == 1, err
}

func (w *StatsWorker) ReleaseReservation(ctx context.Context, nodeID, requestID string) (int64, error) {
	return releaseScript.Run(ctx, w.store.Redis(), []string{reservationKey(requestID), reservedKey(nodeID)}).Int64()
}

func (w *StatsWorker) ReconcileReservation(ctx context.Context, nodeID, requestID string, billedMicroUSD int64) (int64, error) {
	return reconcileScript.Run(ctx, w.store.Redis(), []string{reservationKey(requestID), reservedKey(nodeID), balanceKey(nodeID)}, billedMicroUSD).Int64()
}

func (w *StatsWorker) saveSnapshot(ctx context.Context, node *health.ExecutionNode, grants *YunwuCreditGrantsResponse, source string) error {
	now := time.Now().UTC()
	previous, _ := w.Snapshot(ctx, node.NodeID, 100*365*24*time.Hour)
	apiAvailable := int64(grants.TotalAvailable * 1_000_000)
	reserve := apiAvailable * int64(w.cfg.YunwuBalanceReservePercent) / 100
	reserved, _ := w.store.Redis().Get(ctx, reservedKey(node.NodeID)).Int64()
	snap := BalanceSnapshot{GrantedUSD: grants.TotalGranted, UsedUSD: grants.TotalUsed, AvailableUSD: grants.TotalAvailable, AvailableMicroUSD: apiAvailable, APIAvailableMicroUSD: apiAvailable, EstimatedAvailableMicroUSD: max64(0, apiAvailable-reserved), ReserveMicroUSD: reserve, StaleAt: now.Add(time.Duration(w.cfg.YunwuBalanceStaleSeconds) * time.Second), Source: source, SynchronizedAt: now, Status: "fresh"}
	data, _ := json.Marshal(snap)
	pipe := w.store.Redis().TxPipeline()
	pipe.Set(ctx, balanceKey(node.NodeID), data, 0)
	pipe.Del(ctx, "ash:quota_limit_cache:yunwu")
	_, err := pipe.Exec(ctx)
	if err == nil && previous != nil && previous.AvailableMicroUSD != snap.AvailableMicroUSD {
		_, _ = w.store.LogEvent(ctx, "system", nil, &node.NodeID, "YunwuBalanceReconciled", map[string]any{"previous_available_micro_usd": previous.AvailableMicroUSD, "available_micro_usd": snap.AvailableMicroUSD})
	}
	return err
}

// SyncNodeStats fetches the latest credit balance and usage stats from Yunwu API for a specific node
func (w *StatsWorker) SyncNodeStats(ctx context.Context, node *health.ExecutionNode, apiKey string, baseURL string) (*YunwuCreditGrantsResponse, error) {
	origin, apiPrefix, err := normalizeBillingBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	billingURL := origin + apiPrefix + "/dashboard/billing/credit_grants"

	req, err := http.NewRequestWithContext(ctx, "GET", billingURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := w.httpClient.Do(req)
	if err != nil {
		slog.Warn("[YunwuStatsWorker] Failed to query credit_grants endpoint, trying user/self fallback",
			"node_id", node.NodeID,
			"error", err,
		)
		return w.syncNodeStatsFallback(ctx, node, apiKey, origin)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		slog.Warn("[YunwuStatsWorker] Credit grants returned non-200, attempting user/self fallback",
			"node_id", node.NodeID,
			"status_code", resp.StatusCode,
			"upstream_request_id", yunwuRequestID(resp.Header),
			"response_body", runtime.SafeResponseBody(body),
		)
		return w.syncNodeStatsFallback(ctx, node, apiKey, origin)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body failed: %w", err)
	}

	var grants YunwuCreditGrantsResponse
	if err := json.Unmarshal(body, &grants); err != nil {
		return w.syncNodeStatsFallback(ctx, node, apiKey, origin)
	}

	// Update node runtime state in Redis
	state, _ := w.nm.GetNodeRuntimeState(ctx, node.NodeID)
	if state == nil {
		state = &health.NodeRuntimeState{
			NodeID:      node.NodeID,
			SuccessRate: 1.0,
		}
	}
	_ = w.nm.SaveNodeRuntimeState(ctx, state)

	// Save the authoritative API balance snapshot; no compatibility key is written.
	_ = w.saveSnapshot(ctx, node, &grants, "credit_grants")
	observability.Default.YunwuBalanceSynced()

	slog.Info("[YunwuStatsWorker] Synced Yunwu node credit balance",
		"node_id", node.NodeID,
		"account_name", node.AccountName,
		"total_available_usd", grants.TotalAvailable,
		"total_used_usd", grants.TotalUsed,
	)

	return &grants, nil
}

func normalizeBillingBaseURL(raw string) (string, string, error) {
	if strings.TrimSpace(raw) == "" {
		raw = "https://yunwu.ai/v1"
	}
	u, err := url.Parse(strings.TrimRight(raw, "/"))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", "", fmt.Errorf("invalid Yunwu base URL")
	}
	prefix := strings.TrimRight(u.EscapedPath(), "/")
	// Account URLs commonly point at the OpenAI-compatible /v1 API. Billing is
	// hosted at the same origin and may retain any reverse-proxy prefix before /v1.
	if strings.HasSuffix(prefix, "/v1") {
		prefix = strings.TrimSuffix(prefix, "/v1")
	}
	return u.Scheme + "://" + u.Host, prefix, nil
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func (w *StatsWorker) syncNodeStatsFallback(ctx context.Context, node *health.ExecutionNode, apiKey string, baseURL string) (*YunwuCreditGrantsResponse, error) {
	if baseURL == "" {
		baseURL = "https://yunwu.ai"
	}
	url := fmt.Sprintf("%s/api/user/self", baseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		responseBody := runtime.SafeResponseBody(body)
		slog.Warn("[YunwuStatsWorker] User/self returned non-200",
			"node_id", node.NodeID,
			"status_code", resp.StatusCode,
			"upstream_request_id", yunwuRequestID(resp.Header),
			"response_body", responseBody,
		)
		return nil, fmt.Errorf("yunwu user/self API returned status %d: %s", resp.StatusCode, responseBody)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var userSelf YunwuUserSelfResponse
	if err := json.Unmarshal(body, &userSelf); err != nil {
		return nil, err
	}

	availableUSD := userSelf.Data.Balance
	if availableUSD == 0 && userSelf.Data.Quota > 0 {
		if userSelf.Data.Quota > 1000 {
			// New API / One API standard: 500,000 units = $1 USD
			availableUSD = userSelf.Data.Quota / 500000.0
		} else {
			availableUSD = userSelf.Data.Quota
		}
	}

	grants := &YunwuCreditGrantsResponse{
		Object:         "credit_grants",
		TotalGranted:   userSelf.Data.Quota,
		TotalUsed:      userSelf.Data.UsedQuota,
		TotalAvailable: availableUSD,
	}

	_ = w.saveSnapshot(ctx, node, grants, "user_self")
	observability.Default.YunwuBalanceSynced()
	return grants, nil
}

func yunwuRequestID(headers http.Header) string {
	for _, name := range []string{"X-Request-ID", "Request-ID", "OpenAI-Request-ID"} {
		if value := headers.Get(name); value != "" {
			return value
		}
	}
	return ""
}

// StartSyncLoop runs a background ticker synchronizing Yunwu node stats periodically
func (w *StatsWorker) StartSyncLoop(ctx context.Context, interval time.Duration, getYunwuNodes func(ctx context.Context) ([]health.ExecutionNode, map[string]string, map[string]string)) {
	ticker := time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				nodes, apiKeys, baseURLs := getYunwuNodes(ctx)
				for _, node := range nodes {
					apiKey := apiKeys[node.NodeID]
					baseURL := baseURLs[node.NodeID]
					if apiKey != "" {
						_, _ = w.SyncNodeStats(ctx, &node, apiKey, baseURL)
					}
				}
			}
		}
	}()
}

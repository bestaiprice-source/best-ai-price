package yunwu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/config"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/observability"
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

	// Persist the synced balance to the Account entity in DB so it is reflected across all views
	if w.store != nil && w.store.DB() != nil {
		if accID, parseErr := strconv.ParseInt(node.NodeID, 10, 64); parseErr == nil && accID > 0 {
			if dbAcc, getErr := w.store.DB().Account.Get(ctx, accID); getErr == nil && dbAcc != nil {
				extra := dbAcc.Extra
				if extra == nil {
					extra = make(map[string]any)
				}
				extra["balance_usd"] = grants.TotalAvailable
				extra["balance_updated_at"] = now.Format(time.RFC3339)
				extra["balance_source"] = source
				_ = w.store.DB().Account.UpdateOneID(accID).SetExtra(extra).Exec(ctx)
			}
		}
	}

	return err
}

// SyncNodeStats fetches the latest credit balance and usage stats from Yunwu API for a specific node
func (w *StatsWorker) SyncNodeStats(ctx context.Context, node *health.ExecutionNode, apiKey string, baseURL string) (*YunwuCreditGrantsResponse, error) {
	origin, apiPrefix, err := normalizeBillingBaseURL(baseURL)
	if err != nil {
		return nil, err
	}

	provider := ""
	if node != nil {
		provider = CanonicalProvider(node.Provider)
	}

	// Strategy Priority:
	// For Portdan / sub2api architecture, prioritize sub2api usage endpoint (/v1/usage)
	if provider == ProviderPortdan || strings.Contains(strings.ToLower(baseURL), "portdan") {
		sub2Grants, errSub2 := w.syncSub2APIUsage(ctx, origin, apiPrefix, apiKey)
		if errSub2 == nil && sub2Grants != nil {
			w.recordSyncedSnapshot(ctx, node, sub2Grants, "sub2api_usage")
			return sub2Grants, nil
		}
	}

	// Strategy 1: Primary OpenAI / DeepSeek credit_grants endpoint
	grants, errGrants := w.syncCreditGrants(ctx, origin, apiPrefix, apiKey)
	if errGrants == nil && grants != nil && (grants.TotalAvailable > 0 || grants.TotalGranted > 0) {
		w.recordSyncedSnapshot(ctx, node, grants, "credit_grants")
		return grants, nil
	}

	// Strategy 2: Direct subscription available balance (e.g. OpenLux / OpenAI with explicit total_available)
	subGrants, errSub := w.syncSubscriptionUsage(ctx, origin, apiPrefix, apiKey)
	if errSub == nil && subGrants != nil && subGrants.Object == "subscription_direct" && subGrants.TotalAvailable > 0 {
		w.recordSyncedSnapshot(ctx, node, subGrants, "subscription")
		return subGrants, nil
	}

	// Strategy 3: New API / One API native token usage endpoint (/api/usage/token) ⚡
	// This directly returns the exact token balance and available quota without subtraction heuristics.
	// If the token has unlimited_quota and no specific remaining quota, it cleanly falls through.
	tokenGrants, errToken := w.syncNewAPITokenUsage(ctx, origin, apiPrefix, apiKey)
	if errToken == nil && tokenGrants != nil && (tokenGrants.TotalAvailable > 0 || tokenGrants.TotalGranted > 0) {
		w.recordSyncedSnapshot(ctx, node, tokenGrants, "token_usage")
		return tokenGrants, nil
	}

	// Strategy 4: User profile & wallet balance (/api/user/self, /v1/user/self, /api/user/balance) ⚡
	// This returns the authoritative user account balance (e.g. $3.00 for api top / new-api)
	// before relying on unverified hard limits.
	userGrants, errUser := w.syncUserSelf(ctx, origin, apiPrefix, apiKey)
	if errUser == nil && userGrants != nil && (userGrants.TotalAvailable > 0 || userGrants.TotalGranted > 0) {
		w.recordSyncedSnapshot(ctx, node, userGrants, "user_self")
		return userGrants, nil
	}

	// Strategy 5: Subscription with usage subtraction calculation (when total_available wasn't direct)
	if errSub == nil && subGrants != nil && (subGrants.TotalAvailable > 0 || subGrants.TotalGranted > 0) {
		w.recordSyncedSnapshot(ctx, node, subGrants, "subscription")
		return subGrants, nil
	}

	// Strategy 6: Gateway usage endpoint (/v1/usage, /usage)
	sub2Grants, errSub2 := w.syncSub2APIUsage(ctx, origin, apiPrefix, apiKey)
	if errSub2 == nil && sub2Grants != nil && (sub2Grants.TotalAvailable > 0 || sub2Grants.TotalGranted > 0) {
		w.recordSyncedSnapshot(ctx, node, sub2Grants, "sub2api_usage")
		return sub2Grants, nil
	}

	// Strategy 7: Fallback endpoint scanner (token list, simple balance, etc.)
	fallbackGrants, errFallback := w.syncNodeStatsFallback(ctx, node, apiKey, origin)
	if errFallback == nil && fallbackGrants != nil && (fallbackGrants.TotalAvailable > 0 || fallbackGrants.TotalGranted > 0) {
		w.recordSyncedSnapshot(ctx, node, fallbackGrants, "fallback")
		return fallbackGrants, nil
	}

	// If we got a valid response even if 0 available balance
	if tokenGrants != nil {
		w.recordSyncedSnapshot(ctx, node, tokenGrants, "token_usage_zero")
		return tokenGrants, nil
	}
	if userGrants != nil {
		w.recordSyncedSnapshot(ctx, node, userGrants, "user_self_zero")
		return userGrants, nil
	}
	if sub2Grants != nil {
		w.recordSyncedSnapshot(ctx, node, sub2Grants, "sub2api_usage_zero")
		return sub2Grants, nil
	}
	if fallbackGrants != nil {
		w.recordSyncedSnapshot(ctx, node, fallbackGrants, "fallback_zero")
		return fallbackGrants, nil
	}
	if subGrants != nil {
		w.recordSyncedSnapshot(ctx, node, subGrants, "subscription_zero")
		return subGrants, nil
	}
	if grants != nil {
		w.recordSyncedSnapshot(ctx, node, grants, "credit_grants_zero")
		return grants, nil
	}

	slog.Warn("[YunwuStatsWorker] All balance sync strategies failed",
		"node_id", node.NodeID,
		"provider", provider,
		"err_token", errToken,
		"err_user", errUser,
		"err_sub", errSub,
		"err_sub2", errSub2,
		"err_grants", errGrants,
		"err_fallback", errFallback,
	)

	return nil, fmt.Errorf("failed to sync Yunwu balance across all endpoints (token: %v, user/self: %v, subscription: %v, credit_grants: %v, sub2api: %v, fallback: %v)", errToken, errUser, errSub, errGrants, errSub2, errFallback)
}

func normalizeQuotaToUSD(val float64) float64 {
	if val <= 0 {
		return 0
	}
	// In One API / New API / Yunwu, quota units are integer-based where 500,000 units = $1.00 USD.
	// If a returned quota/balance exceeds 1,000, it is in raw quota units and must be divided by 500,000.
	if val > 1000 {
		return val / 500000.0
	}
	return val
}

func (w *StatsWorker) syncSub2APIUsage(ctx context.Context, origin, apiPrefix, apiKey string) (*YunwuCreditGrantsResponse, error) {
	urls := []string{
		origin + apiPrefix + "/usage",
		origin + "/v1/usage",
		origin + "/usage",
		origin + apiPrefix + "/v1/usage",
	}

	seen := make(map[string]bool)
	var uniqueURLs []string
	for _, u := range urls {
		if !seen[u] {
			seen[u] = true
			uniqueURLs = append(uniqueURLs, u)
		}
	}

	var lastErr error
	for _, u := range uniqueURLs {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("Accept", "application/json")

		resp, err := w.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d from %s", resp.StatusCode, u)
			continue
		}

		contentType := resp.Header.Get("Content-Type")
		if contentType != "" && !strings.Contains(contentType, "json") {
			resp.Body.Close()
			lastErr = fmt.Errorf("non-JSON content-type (%s) from %s", contentType, u)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		trimmed := strings.TrimSpace(string(body))
		if strings.HasPrefix(trimmed, "<") {
			lastErr = fmt.Errorf("HTML response from %s", u)
			continue
		}

		var sub2Resp struct {
			Mode      string   `json:"mode"`
			IsValid   bool     `json:"isValid"`
			PlanName  string   `json:"planName"`
			Balance   *float64 `json:"balance"`
			Remaining *float64 `json:"remaining"`
			Unit      string   `json:"unit"`
			Quota     *struct {
				Limit     float64 `json:"limit"`
				Used      float64 `json:"used"`
				Remaining float64 `json:"remaining"`
				Unit      string  `json:"unit"`
			} `json:"quota"`
			Usage *struct {
				TotalCost float64 `json:"total_cost"`
				TodayCost float64 `json:"today_cost"`
			} `json:"usage"`
			Data *struct {
				Balance   *float64 `json:"balance"`
				Quota     *float64 `json:"quota"`
				Remaining *float64 `json:"remaining"`
			} `json:"data"`
		}

		if err := json.Unmarshal(body, &sub2Resp); err != nil {
			lastErr = err
			continue
		}

		var totalAvail, totalGranted, totalUsed float64
		found := false

		if sub2Resp.Quota != nil {
			totalAvail = sub2Resp.Quota.Remaining
			totalGranted = sub2Resp.Quota.Limit
			totalUsed = sub2Resp.Quota.Used
			found = true
		} else if sub2Resp.Remaining != nil {
			totalAvail = *sub2Resp.Remaining
			found = true
			if sub2Resp.Balance != nil {
				totalGranted = *sub2Resp.Balance
				if totalGranted >= totalAvail {
					totalUsed = totalGranted - totalAvail
				}
			} else if sub2Resp.Usage != nil {
				totalUsed = sub2Resp.Usage.TotalCost
				totalGranted = totalAvail + totalUsed
			} else {
				totalGranted = totalAvail
			}
		} else if sub2Resp.Balance != nil {
			totalAvail = *sub2Resp.Balance
			found = true
			if sub2Resp.Usage != nil {
				totalUsed = sub2Resp.Usage.TotalCost
			}
			totalGranted = totalAvail + totalUsed
		} else if sub2Resp.Data != nil {
			if sub2Resp.Data.Balance != nil {
				totalAvail = normalizeQuotaToUSD(*sub2Resp.Data.Balance)
				totalGranted = totalAvail
				found = true
			} else if sub2Resp.Data.Remaining != nil {
				totalAvail = normalizeQuotaToUSD(*sub2Resp.Data.Remaining)
				totalGranted = totalAvail
				found = true
			} else if sub2Resp.Data.Quota != nil {
				totalAvail = normalizeQuotaToUSD(*sub2Resp.Data.Quota)
				totalGranted = totalAvail
				found = true
			}
		}

		if found {
			return &YunwuCreditGrantsResponse{
				Object:         "sub2api_usage",
				TotalGranted:   totalGranted,
				TotalUsed:      totalUsed,
				TotalAvailable: totalAvail,
			}, nil
		}
	}
	return nil, lastErr
}

func (w *StatsWorker) syncCreditGrants(ctx context.Context, origin, apiPrefix, apiKey string) (*YunwuCreditGrantsResponse, error) {
	urls := []string{
		origin + apiPrefix + "/dashboard/billing/credit_grants",
		origin + "/v1/dashboard/billing/credit_grants",
		origin + "/dashboard/billing/credit_grants",
	}

	seen := make(map[string]bool)
	var uniqueURLs []string
	for _, u := range urls {
		if !seen[u] {
			seen[u] = true
			uniqueURLs = append(uniqueURLs, u)
		}
	}

	var lastErr error
	for _, billingURL := range uniqueURLs {
		req, err := http.NewRequestWithContext(ctx, "GET", billingURL, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Accept", "application/json")

		resp, err := w.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d from %s", resp.StatusCode, billingURL)
			continue
		}

		contentType := resp.Header.Get("Content-Type")
		if contentType != "" && !strings.Contains(contentType, "json") {
			resp.Body.Close()
			lastErr = fmt.Errorf("non-JSON content-type (%s) from %s", contentType, billingURL)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		trimmed := strings.TrimSpace(string(body))
		if strings.HasPrefix(trimmed, "<") {
			lastErr = fmt.Errorf("HTML response from %s", billingURL)
			continue
		}

		var grants struct {
			Object         string  `json:"object"`
			TotalGranted   float64 `json:"total_granted"`
			TotalUsed      float64 `json:"total_used"`
			TotalAvailable float64 `json:"total_available"`
			Grants         struct {
				Data []struct {
					GrantAmount float64 `json:"grant_amount"`
					UsedAmount  float64 `json:"used_amount"`
				} `json:"data"`
			} `json:"grants"`
		}
		if err := json.Unmarshal(body, &grants); err != nil {
			lastErr = err
			continue
		}

		totalAvail := normalizeQuotaToUSD(grants.TotalAvailable)
		totalGranted := normalizeQuotaToUSD(grants.TotalGranted)
		totalUsed := normalizeQuotaToUSD(grants.TotalUsed)

		if totalAvail == 0 && len(grants.Grants.Data) > 0 {
			for _, g := range grants.Grants.Data {
				rem := g.GrantAmount - g.UsedAmount
				if rem > 0 {
					totalAvail += normalizeQuotaToUSD(rem)
				}
			}
		}

		objName := grants.Object
		if objName == "" {
			objName = "credit_summary"
		}

		return &YunwuCreditGrantsResponse{
			Object:         objName,
			TotalGranted:   totalGranted,
			TotalUsed:      totalUsed,
			TotalAvailable: totalAvail,
		}, nil
	}
	return nil, lastErr
}

func (w *StatsWorker) syncNewAPITokenUsage(ctx context.Context, origin, apiPrefix, apiKey string) (*YunwuCreditGrantsResponse, error) {
	urls := []string{
		origin + "/api/usage/token",
		origin + apiPrefix + "/usage/token",
		origin + "/v1/api/usage/token",
		origin + apiPrefix + "/v1/usage/token",
	}

	seen := make(map[string]bool)
	var uniqueURLs []string
	for _, u := range urls {
		if !seen[u] {
			seen[u] = true
			uniqueURLs = append(uniqueURLs, u)
		}
	}

	var lastErr error
	for _, u := range uniqueURLs {
		req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Accept", "application/json")

		resp, err := w.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d from %s", resp.StatusCode, u)
			continue
		}

		contentType := resp.Header.Get("Content-Type")
		if contentType != "" && !strings.Contains(contentType, "json") {
			resp.Body.Close()
			lastErr = fmt.Errorf("non-JSON content-type (%s) from %s", contentType, u)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		trimmed := strings.TrimSpace(string(body))
		if strings.HasPrefix(trimmed, "<") {
			lastErr = fmt.Errorf("HTML response from %s", u)
			continue
		}

		type tokenData struct {
			Object         string  `json:"object"`
			Name           string  `json:"name"`
			TotalGranted   float64 `json:"total_granted"`
			TotalUsed      float64 `json:"total_used"`
			TotalAvailable float64 `json:"total_available"`
			RemainQuota    float64 `json:"remain_quota"`
			UnlimitedQuota bool    `json:"unlimited_quota"`
		}

		var envelope struct {
			Data   *tokenData `json:"data"`
			Object string     `json:"object"`
			tokenData
		}

		if err := json.Unmarshal(body, &envelope); err != nil {
			lastErr = err
			continue
		}

		td := envelope.Data
		if td == nil && (envelope.TotalAvailable > 0 || envelope.TotalGranted > 0 || envelope.RemainQuota > 0 || envelope.UnlimitedQuota || envelope.Object == "token_usage") {
			td = &envelope.tokenData
		}

		if td == nil {
			lastErr = fmt.Errorf("no token usage data from %s", u)
			continue
		}

		avail := td.TotalAvailable
		if avail == 0 && td.RemainQuota > 0 {
			avail = td.RemainQuota
		}

		// When unlimited_quota is true and no specific remaining quota is defined on the token,
		// the token has no token-level quota restriction, so the true balance resides in the user's
		// account wallet (/api/user/self). Do not return a fake 999999.0 balance; fall through to syncUserSelf!
		if td.UnlimitedQuota && avail <= 0 {
			lastErr = fmt.Errorf("token has unlimited_quota with no specific quota, falling through to user wallet balance")
			continue
		}

		availUSD := normalizeQuotaToUSD(avail)
		grantedUSD := normalizeQuotaToUSD(td.TotalGranted)
		usedUSD := normalizeQuotaToUSD(td.TotalUsed)

		if grantedUSD == 0 && (availUSD > 0 || usedUSD > 0) {
			grantedUSD = availUSD + usedUSD
		}

		return &YunwuCreditGrantsResponse{
			Object:         "token_usage",
			TotalGranted:   grantedUSD,
			TotalUsed:      usedUSD,
			TotalAvailable: availUSD,
		}, nil
	}
	return nil, lastErr
}

func (w *StatsWorker) syncUserSelf(ctx context.Context, origin, apiPrefix, apiKey string) (*YunwuCreditGrantsResponse, error) {
	urls := []string{
		origin + "/api/user/self",
		origin + apiPrefix + "/user/self",
		origin + "/v1/user/self",
		origin + apiPrefix + "/v1/user/self",
		origin + "/user/self",
		origin + "/api/user/balance",
		origin + "/v1/user/balance",
	}

	seen := make(map[string]bool)
	var uniqueURLs []string
	for _, u := range urls {
		if !seen[u] {
			seen[u] = true
			uniqueURLs = append(uniqueURLs, u)
		}
	}

	var lastErr error
	for _, u := range uniqueURLs {
		req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("Accept", "application/json")

		resp, err := w.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d from %s", resp.StatusCode, u)
			continue
		}

		contentType := resp.Header.Get("Content-Type")
		if contentType != "" && !strings.Contains(contentType, "json") {
			resp.Body.Close()
			lastErr = fmt.Errorf("non-JSON content-type (%s) from %s", contentType, u)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		trimmed := strings.TrimSpace(string(body))
		if strings.HasPrefix(trimmed, "<") {
			lastErr = fmt.Errorf("HTML response from %s", u)
			continue
		}

		type userData struct {
			Quota       float64 `json:"quota"`
			UsedQuota   float64 `json:"used_quota"`
			Balance     float64 `json:"balance"`
			RemainQuota float64 `json:"remain_quota"`
			TotalQuota  float64 `json:"total_quota"`
		}

		var envelope struct {
			Success bool      `json:"success"`
			Code    any       `json:"code"`
			Message string    `json:"message"`
			Data    *userData `json:"data"`
			userData
		}

		if err := json.Unmarshal(body, &envelope); err != nil {
			lastErr = err
			continue
		}

		ud := envelope.Data
		if ud == nil && (envelope.Quota > 0 || envelope.Balance > 0 || envelope.RemainQuota > 0 || envelope.UsedQuota > 0) {
			ud = &envelope.userData
		}

		if ud == nil {
			lastErr = fmt.Errorf("no user quota data from %s", u)
			continue
		}

		avail := ud.Balance
		if avail == 0 && ud.RemainQuota > 0 {
			avail = ud.RemainQuota
		}
		if avail == 0 && ud.Quota > 0 {
			avail = ud.Quota
		}

		availUSD := normalizeQuotaToUSD(avail)
		usedUSD := normalizeQuotaToUSD(ud.UsedQuota)
		totalGrantedUSD := normalizeQuotaToUSD(ud.TotalQuota)
		if totalGrantedUSD == 0 {
			totalGrantedUSD = availUSD + usedUSD
		}

		return &YunwuCreditGrantsResponse{
			Object:         "user_self",
			TotalGranted:   totalGrantedUSD,
			TotalUsed:      usedUSD,
			TotalAvailable: availUSD,
		}, nil
	}
	return nil, lastErr
}

func (w *StatsWorker) syncSubscriptionUsage(ctx context.Context, origin, apiPrefix, apiKey string) (*YunwuCreditGrantsResponse, error) {
	urls := []string{
		origin + apiPrefix + "/dashboard/billing/subscription",
		origin + "/v1/dashboard/billing/subscription",
		origin + "/dashboard/billing/subscription",
	}

	seen := make(map[string]bool)
	var uniqueURLs []string
	for _, u := range urls {
		if !seen[u] {
			seen[u] = true
			uniqueURLs = append(uniqueURLs, u)
		}
	}

	var lastErr error

	for _, u := range uniqueURLs {
		req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Accept", "application/json")

		resp, err := w.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d from %s", resp.StatusCode, u)
			continue
		}

		contentType := resp.Header.Get("Content-Type")
		if contentType != "" && !strings.Contains(contentType, "json") {
			resp.Body.Close()
			lastErr = fmt.Errorf("non-JSON content-type (%s) from %s", contentType, u)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		trimmed := strings.TrimSpace(string(body))
		if strings.HasPrefix(trimmed, "<") {
			lastErr = fmt.Errorf("HTML response from %s", u)
			continue
		}

		var sub struct {
			HardLimitUSD    float64 `json:"hard_limit_usd"`
			SystemHardLimit float64 `json:"system_hard_limit_usd"`
			HardLimit       float64 `json:"hard_limit"`
			TotalGranted    float64 `json:"total_granted"`
			TotalAvailable  float64 `json:"total_available"`
		}
		if err := json.Unmarshal(body, &sub); err != nil {
			lastErr = err
			continue
		}

		subURLUsed := u
		hardLimit := sub.HardLimitUSD
		if hardLimit == 0 && sub.SystemHardLimit > 0 {
			hardLimit = sub.SystemHardLimit
		}
		if hardLimit == 0 && sub.HardLimit > 0 {
			hardLimit = sub.HardLimit
		}
		if hardLimit == 0 && sub.TotalGranted > 0 {
			hardLimit = sub.TotalGranted
		}

		hardLimitUSD := normalizeQuotaToUSD(hardLimit)

		// If the subscription endpoint already provides total_available directly
		if sub.TotalAvailable > 0 {
			totalAvailUSD := normalizeQuotaToUSD(sub.TotalAvailable)
			return &YunwuCreditGrantsResponse{
				Object:         "subscription_direct",
				TotalGranted:   hardLimitUSD,
				TotalUsed:      maxFloat64(0, hardLimitUSD-totalAvailUSD),
				TotalAvailable: totalAvailUSD,
			}, nil
		}

		// In New API / One API, if hardLimitUSD is 0, this is a legitimate 0 available balance
		if hardLimitUSD <= 0 && sub.TotalAvailable <= 0 {
			return &YunwuCreditGrantsResponse{
				Object:         "subscription_zero",
				TotalGranted:   0,
				TotalUsed:      0,
				TotalAvailable: 0,
			}, nil
		}

		// Fetch usage to calculate remaining available USD
		// Use historical start date to ensure all usage against this token/account is counted
		now := time.Now()
		startDate := "2020-01-01"
		endDate := now.AddDate(0, 0, 2).Format("2006-01-02")

		usageBase := strings.TrimSuffix(subURLUsed, "/subscription")
		usageURL := fmt.Sprintf("%s/usage?start_date=%s&end_date=%s", usageBase, startDate, endDate)

		totalUsageUSD := 0.0
		if reqUsage, err := http.NewRequestWithContext(ctx, "GET", usageURL, nil); err == nil {
			reqUsage.Header.Set("Authorization", "Bearer "+apiKey)
			reqUsage.Header.Set("Accept", "application/json")
			if respUsage, err := w.httpClient.Do(reqUsage); err == nil {
				defer respUsage.Body.Close()
				if respUsage.StatusCode == http.StatusOK {
					if usageBody, err := io.ReadAll(respUsage.Body); err == nil {
						var u struct {
							TotalUsage float64 `json:"total_usage"` // Cents or Quota
						}
						if json.Unmarshal(usageBody, &u) == nil && u.TotalUsage > 0 {
							if u.TotalUsage > 100000 {
								totalUsageUSD = u.TotalUsage / 500000.0
							} else {
								totalUsageUSD = u.TotalUsage / 100.0
							}
						}
					}
				}
			}
		}

		// Calculate available balance: Remaining = HardLimit - TotalUsage
		availableUSD := maxFloat64(0, hardLimitUSD-totalUsageUSD)
		totalGrantedUSD := hardLimitUSD
		if totalGrantedUSD < availableUSD+totalUsageUSD {
			totalGrantedUSD = availableUSD + totalUsageUSD
		}

		return &YunwuCreditGrantsResponse{
			Object:         "subscription",
			TotalGranted:   totalGrantedUSD,
			TotalUsed:      totalUsageUSD,
			TotalAvailable: availableUSD,
		}, nil
	}
	return nil, lastErr
}

func maxFloat64(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func (w *StatsWorker) recordSyncedSnapshot(ctx context.Context, node *health.ExecutionNode, grants *YunwuCreditGrantsResponse, source string) {
	// Update node runtime state in Redis
	state, _ := w.nm.GetNodeRuntimeState(ctx, node.NodeID)
	if state == nil {
		state = &health.NodeRuntimeState{
			NodeID:      node.NodeID,
			SuccessRate: 1.0,
		}
	}
	_ = w.nm.SaveNodeRuntimeState(ctx, state)

	// Save the authoritative API balance snapshot
	_ = w.saveSnapshot(ctx, node, grants, source)
	observability.Default.YunwuBalanceSynced()

	slog.Info("[YunwuStatsWorker] Synced Yunwu node credit balance",
		"node_id", node.NodeID,
		"account_name", node.AccountName,
		"source", source,
		"total_available_usd", grants.TotalAvailable,
		"total_used_usd", grants.TotalUsed,
	)
}

func normalizeBillingBaseURL(raw string) (string, string, error) {
	if strings.TrimSpace(raw) == "" {
		raw = "https://api.openlux.ai/v1"
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
		baseURL = "https://api.openlux.ai"
	}

	urls := []string{
		baseURL + "/api/usage/token",
		baseURL + "/v1/api/usage/token",
		baseURL + "/api/user/self",
		baseURL + "/v1/user/self",
		baseURL + "/user/self",
		baseURL + "/api/token/",
		baseURL + "/v1/token/",
		baseURL + "/api/user/balance",
		baseURL + "/v1/user/balance",
	}

	var lastErr error
	for _, u := range urls {
		req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Accept", "application/json")

		resp, err := w.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d from %s", resp.StatusCode, u)
			continue
		}

		contentType := resp.Header.Get("Content-Type")
		if contentType != "" && !strings.Contains(contentType, "json") {
			resp.Body.Close()
			lastErr = fmt.Errorf("non-JSON content-type (%s) from %s", contentType, u)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		trimmed := strings.TrimSpace(string(body))
		if strings.HasPrefix(trimmed, "<") {
			lastErr = fmt.Errorf("HTML response from %s", u)
			continue
		}

		// Try token_usage format: {"data": {"total_available": ...}} or {"total_available": ...}
		var tokenUsage struct {
			Data struct {
				TotalAvailable float64 `json:"total_available"`
				TotalGranted   float64 `json:"total_granted"`
				TotalUsed      float64 `json:"total_used"`
				RemainQuota    float64 `json:"remain_quota"`
			} `json:"data"`
			TotalAvailable float64 `json:"total_available"`
			TotalGranted   float64 `json:"total_granted"`
			TotalUsed      float64 `json:"total_used"`
			RemainQuota    float64 `json:"remain_quota"`
		}
		if json.Unmarshal(body, &tokenUsage) == nil {
			avail := tokenUsage.Data.TotalAvailable
			if avail == 0 && tokenUsage.Data.RemainQuota > 0 {
				avail = tokenUsage.Data.RemainQuota
			}
			if avail == 0 && tokenUsage.TotalAvailable > 0 {
				avail = tokenUsage.TotalAvailable
			}
			if avail == 0 && tokenUsage.RemainQuota > 0 {
				avail = tokenUsage.RemainQuota
			}
			if avail > 0 {
				availUSD := normalizeQuotaToUSD(avail)
				granted := tokenUsage.Data.TotalGranted
				if granted == 0 {
					granted = tokenUsage.TotalGranted
				}
				used := tokenUsage.Data.TotalUsed
				if used == 0 {
					used = tokenUsage.TotalUsed
				}
				grantedUSD := normalizeQuotaToUSD(granted)
				usedUSD := normalizeQuotaToUSD(used)
				if grantedUSD == 0 {
					grantedUSD = availUSD + usedUSD
				}
				return &YunwuCreditGrantsResponse{
					Object:         "token_usage",
					TotalGranted:   grantedUSD,
					TotalUsed:      usedUSD,
					TotalAvailable: availUSD,
				}, nil
			}
		}

		// Try user profile format
		var userSelf struct {
			Success bool `json:"success"`
			Data    struct {
				Quota       float64 `json:"quota"`
				UsedQuota   float64 `json:"used_quota"`
				Balance     float64 `json:"balance"`
				RemainQuota float64 `json:"remain_quota"`
				TotalQuota  float64 `json:"total_quota"`
			} `json:"data"`
		}
		if json.Unmarshal(body, &userSelf) == nil && (userSelf.Success || userSelf.Data.Quota > 0 || userSelf.Data.Balance > 0 || userSelf.Data.RemainQuota > 0) {
			availableUSD := normalizeQuotaToUSD(userSelf.Data.Balance)
			if availableUSD == 0 && userSelf.Data.RemainQuota > 0 {
				availableUSD = normalizeQuotaToUSD(userSelf.Data.RemainQuota)
			} else if availableUSD == 0 && userSelf.Data.Quota > 0 {
				availableUSD = normalizeQuotaToUSD(userSelf.Data.Quota)
			}

			totalGranted := normalizeQuotaToUSD(userSelf.Data.TotalQuota)
			if totalGranted == 0 {
				totalGranted = availableUSD + normalizeQuotaToUSD(userSelf.Data.UsedQuota)
			}

			return &YunwuCreditGrantsResponse{
				Object:         "user_self",
				TotalGranted:   totalGranted,
				TotalUsed:      normalizeQuotaToUSD(userSelf.Data.UsedQuota),
				TotalAvailable: availableUSD,
			}, nil
		}

		// Try token list/detail format: {"data": [{"remain_quota": 50000000, ...}]}
		var tokenList struct {
			Success bool `json:"success"`
			Data    []struct {
				RemainQuota float64 `json:"remain_quota"`
				UsedQuota   float64 `json:"used_quota"`
				Unlimited   bool    `json:"unlimited_quota"`
			} `json:"data"`
		}
		if json.Unmarshal(body, &tokenList) == nil && len(tokenList.Data) > 0 {
			totalRemain := 0.0
			for _, item := range tokenList.Data {
				if item.RemainQuota > 0 {
					totalRemain += normalizeQuotaToUSD(item.RemainQuota)
				}
			}
			return &YunwuCreditGrantsResponse{
				Object:         "token_quota",
				TotalGranted:   totalRemain,
				TotalUsed:      0,
				TotalAvailable: totalRemain,
			}, nil
		}

		// Try simple balance format: {"balance": 10.5} or {"data": {"balance": 10.5}}
		var simpleBalance struct {
			Balance   *float64 `json:"balance"`
			Remaining *float64 `json:"remaining"`
			Data      *struct {
				Balance   *float64 `json:"balance"`
				Quota     *float64 `json:"quota"`
				Remaining *float64 `json:"remaining"`
			} `json:"data"`
		}
		if json.Unmarshal(body, &simpleBalance) == nil {
			if simpleBalance.Balance != nil {
				avail := normalizeQuotaToUSD(*simpleBalance.Balance)
				return &YunwuCreditGrantsResponse{
					Object:         "user_balance",
					TotalGranted:   avail,
					TotalUsed:      0,
					TotalAvailable: avail,
				}, nil
			}
			if simpleBalance.Remaining != nil {
				avail := normalizeQuotaToUSD(*simpleBalance.Remaining)
				return &YunwuCreditGrantsResponse{
					Object:         "user_balance",
					TotalGranted:   avail,
					TotalUsed:      0,
					TotalAvailable: avail,
				}, nil
			}
			if simpleBalance.Data != nil {
				if simpleBalance.Data.Balance != nil {
					avail := normalizeQuotaToUSD(*simpleBalance.Data.Balance)
					return &YunwuCreditGrantsResponse{
						Object:         "user_balance",
						TotalGranted:   avail,
						TotalUsed:      0,
						TotalAvailable: avail,
					}, nil
				}
				if simpleBalance.Data.Remaining != nil {
					avail := normalizeQuotaToUSD(*simpleBalance.Data.Remaining)
					return &YunwuCreditGrantsResponse{
						Object:         "user_balance",
						TotalGranted:   avail,
						TotalUsed:      0,
						TotalAvailable: avail,
					}, nil
				}
				if simpleBalance.Data.Quota != nil {
					avail := normalizeQuotaToUSD(*simpleBalance.Data.Quota)
					return &YunwuCreditGrantsResponse{
						Object:         "user_balance",
						TotalGranted:   avail,
						TotalUsed:      0,
						TotalAvailable: avail,
					}, nil
				}
			}
		}
	}

	return nil, lastErr
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

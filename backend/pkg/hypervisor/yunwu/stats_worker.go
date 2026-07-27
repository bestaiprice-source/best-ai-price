package yunwu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
)

type YunwuCreditGrantsResponse struct {
	Object         string  `json:"object"`
	TotalGranted   float64 `json:"total_granted"`
	TotalUsed      float64 `json:"total_used"`
	TotalAvailable float64 `json:"total_available"`
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
}

func NewStatsWorker(store *storage.Storage) *StatsWorker {
	return &StatsWorker{
		store: store,
		nm:    health.NewNodeManager(store),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SyncNodeStats fetches the latest credit balance and usage stats from Yunwu API for a specific node
func (w *StatsWorker) SyncNodeStats(ctx context.Context, node *health.ExecutionNode, apiKey string, baseURL string) (*YunwuCreditGrantsResponse, error) {
	if baseURL == "" {
		baseURL = "https://yunwu.ai/v1"
	}
	url := fmt.Sprintf("%s/dashboard/billing/credit_grants", baseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
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
		return w.syncNodeStatsFallback(ctx, node, apiKey, baseURL)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Warn("[YunwuStatsWorker] Credit grants returned non-200, attempting user/self fallback",
			"node_id", node.NodeID,
			"status_code", resp.StatusCode,
		)
		return w.syncNodeStatsFallback(ctx, node, apiKey, baseURL)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body failed: %w", err)
	}

	var grants YunwuCreditGrantsResponse
	if err := json.Unmarshal(body, &grants); err != nil {
		return w.syncNodeStatsFallback(ctx, node, apiKey, baseURL)
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

	// Save available credit balance to Redis key ash:yunwu:credit:<node_id>
	_ = w.store.Redis().Set(ctx, fmt.Sprintf("ash:yunwu:credit:%s", node.NodeID), grants.TotalAvailable, 24*time.Hour).Err()

	slog.Info("[YunwuStatsWorker] Synced Yunwu node credit balance",
		"node_id", node.NodeID,
		"account_name", node.AccountName,
		"total_available_usd", grants.TotalAvailable,
		"total_used_usd", grants.TotalUsed,
	)

	return &grants, nil
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
		return nil, fmt.Errorf("yunwu user/self API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var userSelf YunwuUserSelfResponse
	if err := json.Unmarshal(body, &userSelf); err != nil {
		return nil, err
	}

	grants := &YunwuCreditGrantsResponse{
		Object:         "credit_grants",
		TotalGranted:   userSelf.Data.Quota,
		TotalUsed:      userSelf.Data.UsedQuota,
		TotalAvailable: userSelf.Data.Balance,
	}

	_ = w.store.Redis().Set(ctx, fmt.Sprintf("ash:yunwu:credit:%s", node.NodeID), grants.TotalAvailable, 24*time.Hour).Err()
	return grants, nil
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

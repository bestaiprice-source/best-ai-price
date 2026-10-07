package yunwu

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/ent/account"
	"github.com/Wei-Shaw/sub2api/internal/credentials"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/config"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
)

// probeOutcome classifies the result of an active health probe against an
// upstream Unofficial-provider endpoint.
type probeOutcome int

const (
	probeHealthy probeOutcome = iota
	probeAuthInvalid
	probeRateLimited
	probeServerError
	probeUnreachable
)

// classifyProbeStatus maps an HTTP status / transport error to a probe outcome.
// A reachable 4xx (other than 401/403/429) is treated as healthy because it
// proves the endpoint and credentials work even if the specific diagnostic path
// is unsupported by that aggregator.
func classifyProbeStatus(status int, err error) probeOutcome {
	if err != nil {
		return probeUnreachable
	}
	switch {
	case status >= 200 && status < 300:
		return probeHealthy
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return probeAuthInvalid
	case status == http.StatusTooManyRequests:
		return probeRateLimited
	case status >= 500:
		return probeServerError
	default:
		return probeHealthy
	}
}

// YunwuHealthProber runs a periodic, cheap (free-endpoint) liveness/auth probe
// against each active Unofficial-provider node. It feeds latency and error
// signals into the node runtime state and immediately takes a node with an
// invalid/disabled upstream key out of scheduling (recovering it automatically
// once the key works again). It intentionally avoids paid test generations.
type YunwuHealthProber struct {
	store     *storage.Storage
	nm        *health.NodeManager
	cfg       *config.HypervisorConfig
	decryptor credentials.Cipher
	client    *http.Client
	notifier  *TelegramNotifier
}

// NewYunwuHealthProber constructs a health prober.
func NewYunwuHealthProber(store *storage.Storage, cfg *config.HypervisorConfig, decryptor credentials.Cipher, notifier *TelegramNotifier) *YunwuHealthProber {
	timeout := 10 * time.Second
	if cfg != nil && cfg.YunwuHealthProbeTimeoutMs > 0 {
		timeout = time.Duration(cfg.YunwuHealthProbeTimeoutMs) * time.Millisecond
	}
	p := &YunwuHealthProber{
		store:     store,
		nm:        health.NewNodeManager(store),
		cfg:       cfg,
		decryptor: decryptor,
		client:    &http.Client{Timeout: timeout},
		notifier:  notifier,
	}
	if p.notifier == nil {
		p.notifier = GetDefaultNotifier()
	}
	return p
}

func probeUnavailableKey(nodeID string) string { return "ash:yunwu:probe:unavailable:" + nodeID }

// Start begins periodic probing. A non-positive interval disables the prober.
func (p *YunwuHealthProber) Start(ctx context.Context, interval time.Duration) {
	if p == nil || p.store == nil || p.store.DB() == nil || interval <= 0 {
		return
	}
	go func() {
		// Small initial delay so startup traffic settles first.
		select {
		case <-ctx.Done():
			return
		case <-time.After(30 * time.Second):
		}
		p.probeAll(ctx)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.probeAll(ctx)
			}
		}
	}()
}

func (p *YunwuHealthProber) probeAll(ctx context.Context) {
	accounts, err := p.store.DB().Account.Query().
		Where(
			account.PlatformEqualFold("yunwu"),
			account.Status("active"),
		).All(ctx)
	if err != nil {
		slog.Warn("[YunwuHealthProber] Failed to list Yunwu accounts", "error", err)
		return
	}
	for _, acc := range accounts {
		if ctx.Err() != nil {
			return
		}
		p.probeAccount(ctx, acc.ID, acc.Name, ParseNodeProvider(acc.Extra), acc.Credentials)
	}
}

func (p *YunwuHealthProber) probeAccount(ctx context.Context, accID int64, accName, provider string, encCreds map[string]any) {
	nodeID := strconv.FormatInt(accID, 10)
	creds, decErr := credentials.DecryptMap(encCreds, p.decryptor)
	if decErr != nil {
		slog.Debug("[YunwuHealthProber] Cannot decrypt credentials", "node_id", nodeID, "error", decErr)
		return
	}
	apiKey, _ := creds["api_key"].(string)
	baseURL, _ := creds["base_url"].(string)
	if baseURL == "" {
		baseURL = DefaultBaseURLForProvider(provider)
	}
	if strings.TrimSpace(apiKey) == "" || strings.TrimSpace(baseURL) == "" {
		return
	}

	outcome, latencyMs := p.probeEndpoint(ctx, baseURL, apiKey)

	state, err := p.nm.GetNodeRuntimeState(ctx, nodeID)
	if err != nil || state == nil {
		state = &health.NodeRuntimeState{NodeID: nodeID}
	}
	now := time.Now()

	switch outcome {
	case probeHealthy:
		if latencyMs > 0 {
			if state.AvgLatencyMs <= 0 {
				state.AvgLatencyMs = latencyMs
			} else {
				state.AvgLatencyMs = (state.AvgLatencyMs + latencyMs) / 2
			}
		}
		state.LastSuccessAt = &now
		state.ConsecutiveSuccesses++
		p.recoverNode(ctx, nodeID, state)
	case probeAuthInvalid:
		state.LastErrorAt = &now
		state.ConsecutiveSuccesses = 0
		_ = p.store.Redis().Set(ctx, probeUnavailableKey(nodeID), "1", 24*time.Hour).Err()
		p.markUnavailable(ctx, nodeID, state, accName, provider)
	case probeRateLimited:
		state.Recent429++
		state.LastErrorAt = &now
		state.ConsecutiveSuccesses = 0
	case probeServerError:
		state.RecentTimeouts++
		state.LastErrorAt = &now
		state.ConsecutiveSuccesses = 0
	case probeUnreachable:
		state.RecentTimeouts++
		state.LastErrorAt = &now
		state.ConsecutiveSuccesses = 0
	}
	_ = p.nm.SaveNodeRuntimeState(ctx, state)
}

// probeEndpoint performs a lightweight authenticated GET against the provider's
// model catalog endpoint (a free diagnostic, never a paid generation).
func (p *YunwuHealthProber) probeEndpoint(ctx context.Context, baseURL, apiKey string) (probeOutcome, int64) {
	endpoint := strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/models"
	probeCtx, cancel := context.WithTimeout(ctx, p.client.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return probeUnreachable, 0
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("User-Agent", "BestAIPrice-Gateway-HealthProbe/1.0")

	start := time.Now()
	resp, err := p.client.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return probeUnreachable, latency
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return classifyProbeStatus(resp.StatusCode, nil), latency
}

func (p *YunwuHealthProber) markUnavailable(ctx context.Context, nodeID string, state *health.NodeRuntimeState, accName, provider string) {
	node, err := p.nm.GetNode(ctx, nodeID)
	if err != nil || node == nil {
		node = &health.ExecutionNode{NodeID: nodeID, AccountName: accName, Provider: provider}
	}
	if node.State != "unavailable" {
		node.State = "unavailable"
		_ = p.nm.SaveNode(ctx, node)
		slog.Warn("[YunwuHealthProber] Upstream key invalid/disabled, node marked unavailable",
			"node_id", nodeID, "provider", provider)
		if p.notifier != nil {
			_, _ = p.notifier.NotifyUpstreamKeyInvalid(ctx, provider, nodeID, accName, maskKey(nodeID), "health probe returned 401/403")
		}
	}
}

func (p *YunwuHealthProber) recoverNode(ctx context.Context, nodeID string, state *health.NodeRuntimeState) {
	flagged, _ := p.store.Redis().Exists(ctx, probeUnavailableKey(nodeID)).Result()
	if flagged == 0 {
		return
	}
	_ = p.store.Redis().Del(ctx, probeUnavailableKey(nodeID)).Err()
	node, err := p.nm.GetNode(ctx, nodeID)
	if err != nil || node == nil {
		return
	}
	if node.State == "unavailable" {
		node.State = "healthy"
		state.CooldownUntil = nil
		state.Recent429 = 0
		state.RecentTimeouts = 0
		_ = p.nm.SaveNode(ctx, node)
		slog.Info("[YunwuHealthProber] Upstream key recovered, node restored to healthy", "node_id", nodeID)
	}
}

//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/config"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/yunwu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The catalog↔charge parity guard must stay silent when the price we deduct
// equals the price the catalog publishes, and must alert exactly once when the
// charged per-unit price drifts away from the published basis.
func TestYunwuCatalogPriceMismatchAlert(t *testing.T) {
	ctx := context.Background()
	resetYunwuProviderScopeCache()
	t.Cleanup(resetYunwuProviderScopeCache)

	yw := newMultiProviderPricing(map[string]map[string]*YunwuPricingRawItem{
		"openlux": {"claude-sonnet-5": {ModelRatio: 1.0, CompletionRatio: 5.0, Available: true}},
	})
	bs := NewBillingService(nil, nil, yw)

	published, err := bs.GetYunwuPrimaryModelPricing(ctx, "claude-sonnet-5")
	require.NoError(t, err)
	require.NotNil(t, published)
	require.Greater(t, published.InputPricePerToken, 0.0)

	var mu sync.Mutex
	sent := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		sent++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer server.Close()

	prev := yunwu.GetDefaultNotifier()
	notifier := yunwu.NewTelegramNotifier(nil, &config.HypervisorConfig{
		TelegramBotToken:      "test-token",
		TelegramChatID:        "-100123456789",
		TelegramAlertsEnabled: true,
	})
	notifier.SetBaseURL(server.URL)
	notifier.SetHTTPClient(server.Client())
	t.Cleanup(func() { yunwu.SetDefaultNotifier(prev) })

	sentCount := func() int {
		mu.Lock()
		defer mu.Unlock()
		return sent
	}

	const inTokens = 1000
	tokens := UsageTokens{InputTokens: inTokens}

	// 1. Charged basis == published basis -> no alert.
	aligned := &CostBreakdown{InputCost: float64(inTokens) * published.InputPricePerToken}
	alertIfYunwuCatalogPriceMismatch(ctx, bs, "openlux", "claude-sonnet-5", tokens, aligned, 1.5)
	assert.Zero(t, sentCount(), "an aligned charge must not alert")

	// 2. A divergence within the tolerance must stay silent.
	tol := &CostBreakdown{InputCost: float64(inTokens) * published.InputPricePerToken * 1.01}
	alertIfYunwuCatalogPriceMismatch(ctx, bs, "openlux", "claude-sonnet-5", tokens, tol, 1.5)
	assert.Zero(t, sentCount(), "a within-tolerance charge must not alert")

	// 3. Charged 30% above the published basis -> alert.
	diverged := &CostBreakdown{InputCost: float64(inTokens) * published.InputPricePerToken * 1.3}
	alertIfYunwuCatalogPriceMismatch(ctx, bs, "openlux", "claude-sonnet-5", tokens, diverged, 1.5)
	assert.Equal(t, 1, sentCount(), "a diverged charge must alert exactly once")

	// 4. Missing inputs never panic and never alert.
	alertIfYunwuCatalogPriceMismatch(ctx, bs, "openlux", "", tokens, diverged, 1.5)
	alertIfYunwuCatalogPriceMismatch(ctx, bs, "openlux", "claude-sonnet-5", tokens, nil, 1.5)
	alertIfYunwuCatalogPriceMismatch(ctx, nil, "openlux", "claude-sonnet-5", tokens, diverged, 1.5)
	assert.Equal(t, 1, sentCount())
}

// An unknown model (no published pricing) must never produce a parity alert.
func TestYunwuCatalogPriceMismatchSkipsUnpricedModel(t *testing.T) {
	ctx := context.Background()
	yw := newMultiProviderPricing(map[string]map[string]*YunwuPricingRawItem{
		"openlux": {"claude-sonnet-5": {ModelRatio: 1.0, CompletionRatio: 5.0, Available: true}},
	})
	bs := NewBillingService(nil, nil, yw)

	prev := yunwu.GetDefaultNotifier()
	yunwu.SetDefaultNotifier(nil)
	t.Cleanup(func() { yunwu.SetDefaultNotifier(prev) })

	cost := &CostBreakdown{InputCost: 1.0}
	alertIfYunwuCatalogPriceMismatch(ctx, bs, "openlux", "no-such-model", UsageTokens{InputTokens: 1000}, cost, 1.5)
	alertIfYunwuCatalogPriceMismatch(ctx, bs, "openlux", "claude-sonnet-5", UsageTokens{}, cost, 1.5)
}

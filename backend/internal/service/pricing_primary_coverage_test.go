//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newMultiProviderPricing builds a YunwuPricingService backed by in-memory
// per-provider price tables (no HTTP), mirroring the real provider isolation.
func newMultiProviderPricing(provs map[string]map[string]*YunwuPricingRawItem) *YunwuPricingService {
	yw := NewYunwuPricingService()
	urls := map[string]string{}
	updated := map[string]time.Time{}
	for p := range provs {
		urls[p] = "mock-" + p
		updated[p] = time.Now()
	}
	yw.providerURLs = urls
	yw.providerPricing = provs
	yw.providerUpdated = updated
	return yw
}

// A token-priced provider must win over a (cheaper) fixed per-request provider:
// the primary lookup prefers token pricing whenever it exists.
func TestPrimaryPrefersTokenOverFixed(t *testing.T) {
	resetYunwuProviderScopeCache()
	t.Cleanup(resetYunwuProviderScopeCache)

	yw := newMultiProviderPricing(map[string]map[string]*YunwuPricingRawItem{
		"openlux": {"m": {ModelPrice: 0.01}},                      // cheaper, but FIXED per request
		"portdan": {"m": {ModelRatio: 5.0, CompletionRatio: 1.0}}, // token: 10e-6 / token
	})

	p, ok := yw.GetModelPricingPrimaryAcrossProviders(context.Background(), "m")
	require.True(t, ok)
	require.NotNil(t, p)
	assert.Zero(t, p.FixedPricePerRequest, "a token price must win over a cheaper fixed price")
	assert.InDelta(t, 10e-6, p.InputPricePerToken, 1e-12)
	assert.InDelta(t, 10e-6, p.OutputPricePerToken, 1e-12)
}

// Selling at primary × markup means a serving provider that is more expensive
// than the primary is STILL profitable while its cost stays under the charged
// price — it is only a loss (alert) once it exceeds primary × markup.
func TestComputeYunwuMargin_WithinMarkupIsProfitable(t *testing.T) {
	primary := &ModelPricing{InputPricePerToken: 2e-6, OutputPricePerToken: 10e-6}
	// charged per 1M: input $2.2, output $11.0 (markup 1.10)

	// Slightly pricier than the primary, but still below the charged price.
	slightlyPricier := &ModelPricing{InputPricePerToken: 2.1e-6, OutputPricePerToken: 10e-6}
	assert.False(t, computeYunwuMargin(slightlyPricier, primary, 1.10).Unprofitable)

	// Exactly at the charged price -> still not a loss (must strictly exceed).
	atCharged := &ModelPricing{InputPricePerToken: 2.2e-6, OutputPricePerToken: 10e-6}
	assert.False(t, computeYunwuMargin(atCharged, primary, 1.10).Unprofitable)

	// Above the charged price -> loss on the input leg.
	aboveCharged := &ModelPricing{InputPricePerToken: 2.3e-6, OutputPricePerToken: 10e-6}
	m := computeYunwuMargin(aboveCharged, primary, 1.10)
	require.True(t, m.Unprofitable)
	assert.Equal(t, "input", m.Leg)
	assert.InDelta(t, 2.2, m.ChargedUSD, 1e-6)
	assert.InDelta(t, 2.3, m.CostUSD, 1e-6)

	// Without any markup, any provider pricier than the primary is already a loss.
	noMarkup := &ModelPricing{InputPricePerToken: 2.0001e-6, OutputPricePerToken: 10e-6}
	assert.True(t, computeYunwuMargin(noMarkup, primary, 1.0).Unprofitable)
}

// End-to-end: the published retail price and the actual charge must both equal
// the CHEAPEST provider's price × markup — across several providers.
func TestRetailChargeEqualsCheapestProviderTimesMarkup(t *testing.T) {
	resetYunwuProviderScopeCache()
	t.Cleanup(resetYunwuProviderScopeCache)

	yw := newMultiProviderPricing(map[string]map[string]*YunwuPricingRawItem{
		// in+out per token: openlux 12e-6, portdan 6e-6 (cheapest), x-llm 24e-6
		"openlux": {"claude-sonnet-5": {ModelRatio: 1.0, CompletionRatio: 5.0}},
		"portdan": {"claude-sonnet-5": {ModelRatio: 0.5, CompletionRatio: 5.0}},
		"x-llm":   {"claude-sonnet-5": {ModelRatio: 2.0, CompletionRatio: 5.0}},
	})
	bs := NewBillingService(nil, nil, yw)
	svc := &GatewayService{billingService: bs}

	ctx := ContextWithYunwuPrimaryPricing(context.Background())
	var item *PublicModelPricingItem
	for _, it := range svc.GetPublicModelsPricing(ctx) {
		if it.CanonicalModelID == "anthropic/claude-sonnet-5" {
			c := it
			item = &c
		}
	}
	require.NotNil(t, item, "claude-sonnet-5 must be published")
	require.NotNil(t, item.InputPricePer1M)
	require.NotNil(t, item.OutputPricePer1M)

	// portdan is the cheapest provider -> catalog = portdan × 1.10.
	assert.InDelta(t, 1e-6*1e6*1.10, *item.InputPricePer1M, 1e-4)
	assert.InDelta(t, 5e-6*1e6*1.10, *item.OutputPricePer1M, 1e-4)

	// Billing must charge exactly the same price (show == sell).
	cost, err := bs.CalculateCostWithContext(ctx, "anthropic/claude-sonnet-5",
		UsageTokens{InputTokens: 1_000_000, OutputTokens: 1_000_000}, 1.10, "")
	require.NoError(t, err)
	assert.InDelta(t, *item.InputPricePer1M, cost.InputCost*1.10, 1e-4)
	assert.InDelta(t, *item.OutputPricePer1M, cost.OutputCost*1.10, 1e-4)
	assert.InDelta(t, *item.InputPricePer1M+*item.OutputPricePer1M, cost.ActualCost, 1e-4)
}

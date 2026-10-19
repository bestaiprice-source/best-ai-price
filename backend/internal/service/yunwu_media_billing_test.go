//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestYunwuMediaTotal(t *testing.T) {
	// The upstream per-unit price is authoritative (same basis as the catalog).
	total, ok := yunwuMediaTotal(0.40, 1, 2000)
	require.True(t, ok)
	assert.InDelta(t, 0.40, total, 1e-9)

	// Units multiply the per-unit price.
	total, ok = yunwuMediaTotal(0.40, 3, 2000)
	require.True(t, ok)
	assert.InDelta(t, 1.20, total, 1e-9)

	// Fallback to the engine-provided micro-USD cost when no upstream price.
	total, ok = yunwuMediaTotal(0, 1, 2000)
	require.True(t, ok)
	assert.InDelta(t, 0.002, total, 1e-9)

	// Nothing to bill at all.
	_, ok = yunwuMediaTotal(0, 1, 0)
	assert.False(t, ok)

	// A non-positive unit count is treated as one unit.
	total, ok = yunwuMediaTotal(0.40, 0, 0)
	require.True(t, ok)
	assert.InDelta(t, 0.40, total, 1e-9)
}

// The per-unit price the gateway charges for an image/media model must be the
// exact value the public catalog advertises (max across upstream providers),
// so "what we show" equals "what we sell at" (both carry the same markup).
func TestYunwuMediaUnitPriceMatchesCatalog(t *testing.T) {
	yw := NewYunwuPricingService()
	yw.providerURLs = map[string]string{"openlux": "mock-openlux", "x-llm": "mock-xllm"}
	yw.providerPricing = map[string]map[string]*YunwuPricingRawItem{
		"openlux": {"gemini-3-pro-image-preview": {ModelPrice: 0.33, Available: true}},
		"x-llm":   {"gemini-3-pro-image-preview": {ModelPrice: 0.40, Available: true}},
	}
	yw.providerUpdated = map[string]time.Time{
		"openlux": time.Now(),
		"x-llm":   time.Now(),
	}
	billingSvc := NewBillingService(nil, nil, yw)

	ctx := context.Background()

	// Catalog publishes the PRIMARY (cheapest) provider price * 1.10 markup:
	// min(0.33, 0.40) = 0.33 -> $0.363 native per request.
	svc := &GatewayService{billingService: billingSvc}
	items := svc.GetPublicModelsPricing(ctx)
	var found bool
	var catalogPrice float64
	for _, it := range items {
		if it.CanonicalModelID == "google/gemini-3-pro-image-preview" {
			found = true
			require.NotNil(t, it.NativePriceUSD)
			require.NotNil(t, it.NativePriceUnit)
			catalogPrice = *it.NativePriceUSD
			assert.Equal(t, "request", *it.NativePriceUnit)
			assert.InDelta(t, 0.363, catalogPrice, 1e-3)
		}
	}
	require.True(t, found, "image model must appear in the public catalog")

	// Billing resolves the same per-unit upstream price for this model.
	unit := resolveYunwuMediaUnitPrice(ctx, billingSvc, "gemini-3-pro-image-preview")
	require.Greater(t, unit, 0.0)

	// Advertised price == charged price for one unit (× the same 1.10 markup).
	total, ok := yunwuMediaTotal(unit, 1, 0)
	require.True(t, ok)
	billed := total * 1.10
	assert.InDelta(t, catalogPrice, billed, 1e-6,
		"catalog image price must equal the charged price for one unit")

	// An unknown media model resolves to 0 (falls back to engine cost, never free).
	assert.Equal(t, 0.0, resolveYunwuMediaUnitPrice(ctx, billingSvc, "totally-unknown-image-model"))
}

// End-to-end check across every pricing type: for each priced model in the
// public catalog, the price we publish must equal what the real billing path
// charges, carrying the same (group) multiplier. Covers token, fixed-per-request
// (image/video) and multi-provider max-cost resolution.
func TestVerifyCatalogMatchesBillingAllPricingTypes(t *testing.T) {
	yw := NewYunwuPricingService()
	yw.providerURLs = map[string]string{"openlux": "mock-openlux", "x-llm": "mock-xllm"}
	yw.providerPricing = map[string]map[string]*YunwuPricingRawItem{
		"openlux": {
			"claude-sonnet-5":            {ModelRatio: 1.5, CompletionRatio: 5.0, Available: true},
			"deepseek-v3":                {ModelRatio: 0.14495, CompletionRatio: 4.0, Available: true},
			"gemini-3-pro-image-preview": {ModelPrice: 0.33, Available: true},
			"sora-video":                 {ModelPrice: 0.5, Available: true},
		},
		"x-llm": {
			// More expensive than openlux for the token model -> max() must win.
			"claude-sonnet-5":            {ModelRatio: 2.0, CompletionRatio: 5.0, Available: true},
			"gemini-3-pro-image-preview": {ModelPrice: 0.40, Available: true},
		},
	}
	yw.providerUpdated = map[string]time.Time{"openlux": time.Now(), "x-llm": time.Now()}
	billingSvc := NewBillingService(nil, nil, yw)

	const multiplier = 1.25
	groupRepo := &mockGroupRepoForPricing{
		activeGroups: []Group{{Name: "yunwu", Platform: PlatformYunwu, RateMultiplier: multiplier}},
	}
	svc := &GatewayService{billingService: billingSvc, groupRepo: groupRepo}

	ctx := context.Background()
	items := svc.GetPublicModelsPricing(ctx)

	tokenChecked, mediaChecked := 0, 0
	for _, item := range items {
		if item.InputPricePer1M != nil {
			tokenChecked++
			// The gateway billing path sets ContextWithYunwuPrimaryPricing; replicate it.
			billedCtx := ContextWithYunwuPrimaryPricing(ctx)
			cost, err := billingSvc.CalculateCostWithContext(billedCtx, item.CanonicalModelID,
				UsageTokens{InputTokens: 10_000, OutputTokens: 10_000}, multiplier, "")
			require.NoError(t, err, "billing must succeed for %s", item.CanonicalModelID)
			billedIn1M := cost.InputCost / 10_000 * 1_000_000 * multiplier
			billedOut1M := cost.OutputCost / 10_000 * 1_000_000 * multiplier
			assert.InDeltaf(t, *item.InputPricePer1M, billedIn1M, 1e-4,
				"catalog input rate for %s must equal the charged rate", item.CanonicalModelID)
			assert.InDeltaf(t, *item.OutputPricePer1M, billedOut1M, 1e-4,
				"catalog output rate for %s must equal the charged rate", item.CanonicalModelID)
		}
		if item.NativePriceUSD != nil {
			mediaChecked++
			// The media billing path charges unit × units × multiplier for a unit.
			unit := resolveYunwuMediaUnitPrice(ctx, billingSvc, item.CanonicalModelID)
			require.Greaterf(t, unit, 0.0, "media model %s must resolve an upstream unit price", item.CanonicalModelID)
			total, ok := yunwuMediaTotal(unit, 1, 0)
			require.True(t, ok)
			billed := total * multiplier
			assert.InDeltaf(t, *item.NativePriceUSD, billed, 1e-6,
				"catalog media price for %s must equal the charged price", item.CanonicalModelID)
		}
	}
	assert.GreaterOrEqual(t, tokenChecked, 2, "expected token-priced models to be verified")
	assert.GreaterOrEqual(t, mediaChecked, 2, "expected media-priced models to be verified")

	// Sanity: the PRIMARY (cheapest) provider basis wins (openlux's cheaper token
	// model), not the most expensive one.
	var sonnetFound bool
	for _, it := range items {
		if it.CanonicalModelID == "anthropic/claude-sonnet-5" {
			sonnetFound = true
			require.NotNil(t, it.InputPricePer1M)
			// min(openlux 1.5, x-llm 2.0) * 2e-6 * 1e6 * 1.25 = $3.75/1M
			assert.InDelta(t, 3.75, *it.InputPricePer1M, 1e-4)
		}
	}
	require.True(t, sonnetFound, "claude-sonnet-5 must be listed")
}

// The loss guard must flag a request only when the SERVING provider costs more
// than the charged retail price (primary provider price × markup).
func TestComputeYunwuMargin(t *testing.T) {
	// Primary provider is cheaper;
	//   charged (per 1M) = 2e-6 * 1e6 * 1.10 = $2.2 (input)
	primary := &ModelPricing{InputPricePerToken: 2e-6, OutputPricePerToken: 10e-6}

	// Serving on the primary provider itself -> profit, no alert.
	m := computeYunwuMargin(primary, primary, 1.10)
	assert.False(t, m.Unprofitable)

	// Serving on a cheaper backup -> still profit.
	cheaper := &ModelPricing{InputPricePerToken: 1e-6, OutputPricePerToken: 5e-6}
	assert.False(t, computeYunwuMargin(cheaper, primary, 1.10).Unprofitable)

	// Serving on a more expensive provider: input 4e-6 > charged 2.2/1M.
	pricier := &ModelPricing{InputPricePerToken: 4e-6, OutputPricePerToken: 10e-6}
	m = computeYunwuMargin(pricier, primary, 1.10)
	require.True(t, m.Unprofitable)
	assert.Equal(t, "input", m.Leg)
	assert.InDelta(t, 2.2, m.ChargedUSD, 1e-6)
	assert.InDelta(t, 4.0, m.CostUSD, 1e-6)

	// Output leg loss.
	pricierOut := &ModelPricing{InputPricePerToken: 2e-6, OutputPricePerToken: 20e-6}
	m = computeYunwuMargin(pricierOut, primary, 1.10)
	require.True(t, m.Unprofitable)
	assert.Equal(t, "output", m.Leg)

	// Fixed / per-request basis.
	fixedPrimary := &ModelPricing{FixedPricePerRequest: 0.20}
	m = computeYunwuMargin(&ModelPricing{FixedPricePerRequest: 0.40}, fixedPrimary, 1.10)
	require.True(t, m.Unprofitable)
	assert.Equal(t, "request", m.Leg)
	assert.InDelta(t, 0.22, m.ChargedUSD, 1e-9)
	assert.InDelta(t, 0.40, m.CostUSD, 1e-9)
	assert.False(t, computeYunwuMargin(fixedPrimary, fixedPrimary, 1.10).Unprofitable)

	// Mixed pricing kinds cannot be compared -> never flagged.
	assert.False(t, computeYunwuMargin(pricier, fixedPrimary, 1.10).Unprofitable)
	assert.False(t, computeYunwuMargin(fixedPrimary, pricier, 1.10).Unprofitable)
}

// Provider scope: the retail "primary" provider must be the cheapest provider
// WE hold accounts for, never a cheaper configured aggregator we cannot serve from.
func TestYunwuPrimaryRespectsProviderScope(t *testing.T) {
	yw := NewYunwuPricingService()
	yw.providerURLs = map[string]string{"openlux": "m1", "x-llm": "m2"}
	yw.providerPricing = map[string]map[string]*YunwuPricingRawItem{
		"openlux": {"claude-sonnet-5": {ModelRatio: 1.5, CompletionRatio: 5.0, Available: true}},
		"x-llm":   {"claude-sonnet-5": {ModelRatio: 2.0, CompletionRatio: 5.0, Available: true}},
	}
	yw.providerUpdated = map[string]time.Time{"openlux": time.Now(), "x-llm": time.Now()}
	billingSvc := NewBillingService(nil, nil, yw)
	ctx := context.Background()

	// Unrestricted: cheapest = openlux (1.5).
	p, err := billingSvc.GetYunwuPrimaryModelPricing(ctx, "claude-sonnet-5")
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.InDelta(t, 1.5*2e-6, p.InputPricePerToken, 1e-12)

	// Scoped to x-llm only: primary must be x-llm's (2.0) even though openlux is
	// cheaper but we hold no account there.
	scoped := ContextWithYunwuProviders(ctx, []string{"x-llm"})
	p, err = billingSvc.GetYunwuPrimaryModelPricing(scoped, "claude-sonnet-5")
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.InDelta(t, 2.0*2e-6, p.InputPricePerToken, 1e-12)

	// Provider canonicalization ("X-LLM" == "x-llm").
	scopedCase := ContextWithYunwuProviders(ctx, []string{"X-LLM"})
	p, _ = billingSvc.GetYunwuPrimaryModelPricing(scopedCase, "claude-sonnet-5")
	require.NotNil(t, p)
	assert.InDelta(t, 2.0*2e-6, p.InputPricePerToken, 1e-12)
}

// End-to-end: with a schedulable yunwu account on a pricier provider, the catalog
// publishes THAT provider's price (the one we can serve), not the cheapest
// configured aggregator.
func TestCatalogUsesAccountScopedPrimary(t *testing.T) {
	resetYunwuProviderScopeCache()
	t.Cleanup(resetYunwuProviderScopeCache)

	yw := NewYunwuPricingService()
	yw.providerURLs = map[string]string{"openlux": "m1", "portdan": "m2"}
	yw.providerPricing = map[string]map[string]*YunwuPricingRawItem{
		"openlux": {"claude-sonnet-5": {ModelRatio: 1.0, CompletionRatio: 5.0, Available: true}},
		"portdan": {"claude-sonnet-5": {ModelRatio: 2.0, CompletionRatio: 5.0, Available: true}},
	}
	yw.providerUpdated = map[string]time.Time{"openlux": time.Now(), "portdan": time.Now()}
	billingSvc := NewBillingService(nil, nil, yw)

	repo := &mockAccountRepoForPricing{
		schedulable: []Account{
			{Platform: PlatformYunwu, Extra: map[string]any{"provider": "portdan"}},
		},
	}
	svc := &GatewayService{billingService: billingSvc, accountRepo: repo}

	items := svc.GetPublicModelsPricing(context.Background())
	var found bool
	for _, it := range items {
		if it.CanonicalModelID == "anthropic/claude-sonnet-5" {
			found = true
			require.NotNil(t, it.InputPricePer1M)
			// portdan (2.0) is the cheapest we can serve from -> 2.0 * 2e-6 * 1e6 * 1.10 = $4.4
			assert.InDelta(t, 4.4, *it.InputPricePer1M, 1e-4)
		}
	}
	require.True(t, found, "claude-sonnet-5 must be listed")
}

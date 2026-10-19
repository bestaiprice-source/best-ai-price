//go:build unit

package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToCanonicalModelID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"claude-3-7-sonnet", "anthropic/claude-3-7-sonnet"},
		{"claude-sonnet-5", "anthropic/claude-sonnet-5"},
		{"claude-opus-4-6", "anthropic/claude-opus-4-6"},
		{"gpt-4o", "openai/gpt-4o"},
		{"gpt-5.5", "openai/gpt-5.5"},
		{"o1-mini", "openai/o1-mini"},
		{"o3-mini", "openai/o3-mini"},
		{"sora", "openai/sora"},
		{"sora-turbo", "openai/sora-turbo"},
		{"dall-e-3", "openai/dall-e-3"},
		{"gemini-2.5-flash", "google/gemini-2.5-flash"},
		{"gemini-2-5-flash", "google/gemini-2-5-flash"},
		{"deepseek-v3", "deepseek/deepseek-v3"},
		{"deepseek-r1", "deepseek/deepseek-r1"},
		{"kimi-k3", "moonshot/kimi-k3"},
		{"moonshot-v1-8k", "moonshot/moonshot-v1-8k"},
		{"glm-5-3", "zhipu/glm-5-3"},
		{"chatglm-turbo", "zhipu/chatglm-turbo"},
		{"qwen-2.5-72b", "qwen/qwen-2.5-72b"},
		{"grok-2", "xai/grok-2"},
		{"minimax-text-01", "minimax/minimax-text-01"},
		{"mistral-large", "mistral/mistral-large"},
		{"codestral-2501", "mistral/codestral-2501"},
		{"llama-3.3-70b", "meta/llama-3.3-70b"},
		{"kling-v1-6", "kuaishou/kling-v1-6"},
		{"seedance-v1", "bytedance/seedance-v1"},
		{"anthropic/claude-3-5-sonnet", "anthropic/claude-3-5-sonnet"},
		{"google/gemini-1.5-pro", "google/gemini-1.5-pro"},
		{"publishers/google/models/gemini-1.5-pro", "google/gemini-1.5-pro"},
		{"models/claude-3-5-haiku", "anthropic/claude-3-5-haiku"},
		{"unknown-vendor-model", "other/unknown-vendor-model"},
		{"", ""},
		{"   ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			actual := ToCanonicalModelID(tt.input)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestGetPublicModelsPricing_UnpricedWhenNoYunwuUpstream(t *testing.T) {
	// Without upstream Yunwu pricing, models have no price (nil/null) and status is unavailable
	svc := &GatewayService{}
	items := svc.GetPublicModelsPricing(context.Background())

	require.NotEmpty(t, items)
	assert.GreaterOrEqual(t, len(items), len(knownCanonicalModelIDs))

	for _, item := range items {
		require.NotEmpty(t, item.CanonicalModelID)
		assert.Nil(t, item.InputPricePer1M)
		assert.Nil(t, item.OutputPricePer1M)
		assert.Nil(t, item.NativePriceUSD)
		assert.False(t, item.Available)
		assert.Equal(t, "unavailable", item.Status)
	}
}

func helperNewBillingWithYunwuPricing(raw map[string]*YunwuPricingRawItem) *BillingService {
	yw := NewYunwuPricingService()
	yw.providerURLs = map[string]string{"openlux": "mock"}
	yw.providerPricing = map[string]map[string]*YunwuPricingRawItem{
		"openlux": raw,
	}
	yw.providerUpdated = map[string]time.Time{
		"openlux": time.Now(),
	}
	return NewBillingService(nil, nil, yw)
}

func TestGetPublicModelsPricing_WithYunwuUpstreamPricing(t *testing.T) {
	// Base token calculation in One API standard: basePerToken = 0.002 / 1000 = 2e-6
	// For claude-sonnet-5: ModelRatio 1.5 -> $3.0/1M tokens; CompletionRatio 5.0 -> $15.0/1M tokens
	// For sora: ModelPrice 0.10 -> $0.10 fixed/second
	raw := map[string]*YunwuPricingRawItem{
		"claude-sonnet-5": {
			ModelRatio:      1.5,
			CompletionRatio: 5.0,
			Available:       true,
		},
		"sora": {
			ModelPrice: 0.10,
			Available:  true,
		},
	}

	billingSvc := helperNewBillingWithYunwuPricing(raw)
	svc := &GatewayService{
		billingService: billingSvc,
	}

	items := svc.GetPublicModelsPricing(context.Background())
	require.NotEmpty(t, items)

	byID := make(map[string]PublicModelPricingItem)
	for _, item := range items {
		byID[item.CanonicalModelID] = item
	}

	// 1. anthropic/claude-sonnet-5 has upstream pricing with 10% base markup (1.10)
	sonnet5, exists := byID["anthropic/claude-sonnet-5"]
	require.True(t, exists)
	require.NotNil(t, sonnet5.InputPricePer1M)
	require.NotNil(t, sonnet5.OutputPricePer1M)
	assert.Equal(t, 3.3, *sonnet5.InputPricePer1M)
	assert.Equal(t, 16.5, *sonnet5.OutputPricePer1M)

	// 2. openai/sora has native fixed price with 10% base markup (1.10)
	sora, exists := byID["openai/sora"]
	require.True(t, exists)
	require.NotNil(t, sora.NativePriceUSD)
	require.NotNil(t, sora.NativePriceUnit)
	assert.Equal(t, 0.11, *sora.NativePriceUSD)
	assert.Equal(t, "request", *sora.NativePriceUnit)

	// 3. Unpriced model has nil prices
	gpt4o, exists := byID["openai/gpt-4o"]
	require.True(t, exists)
	assert.Nil(t, gpt4o.InputPricePer1M)
	assert.Nil(t, gpt4o.OutputPricePer1M)
	assert.False(t, gpt4o.Available)
	assert.Equal(t, "unavailable", gpt4o.Status)
}

func TestRoundFloat(t *testing.T) {
	assert.Equal(t, 1.2345, roundFloat(1.23446, 4))
	assert.Equal(t, 1.23, roundFloat(1.23446, 2))
}

func TestRawModelWithoutVersion(t *testing.T) {
	assert.Equal(t, "claude-3-5-sonnet", rawModelWithoutVersion("claude-3-5-sonnet-20241022"))
	assert.Equal(t, "claude-3-7-sonnet", rawModelWithoutVersion("claude-3-7-sonnet-20250219"))
	assert.Equal(t, "gpt-4o", rawModelWithoutVersion("gpt-4o"))
	assert.Equal(t, "unknown-model-1234", rawModelWithoutVersion("unknown-model-1234"))
	assert.Equal(t, "short-1234567a", rawModelWithoutVersion("short-1234567a"))
}

type mockAccountRepoForPricing struct {
	AccountRepository
	schedulable []Account
}

func (m *mockAccountRepoForPricing) ListSchedulable(ctx context.Context) ([]Account, error) {
	return m.schedulable, nil
}

func TestGetPublicModelsPricing_WithAccountsAndBilling(t *testing.T) {
	repo := &mockAccountRepoForPricing{
		schedulable: []Account{
			{
				Platform: "anthropic",
				Extra: map[string]any{
					AccountExtraFetchedModelsKey: []any{"claude-3-5-sonnet", "claude-sonnet-5"},
				},
				Credentials: map[string]any{
					"model_mapping": map[string]any{
						"custom-claude": "claude-3-5-sonnet",
					},
				},
			},
		},
	}

	raw := map[string]*YunwuPricingRawItem{
		"claude-3-5-sonnet": {
			ModelRatio:      1.5,
			CompletionRatio: 5.0,
			Available:       true,
		},
		"custom-claude": {
			ModelRatio:      1.5,
			CompletionRatio: 5.0,
			Available:       true,
		},
	}

	billingSvc := helperNewBillingWithYunwuPricing(raw)
	svc := &GatewayService{
		accountRepo:    repo,
		billingService: billingSvc,
	}

	items := svc.GetPublicModelsPricing(context.Background())
	require.NotEmpty(t, items)

	byID := make(map[string]PublicModelPricingItem)
	for _, item := range items {
		byID[item.CanonicalModelID] = item
	}

	// claude-3-5-sonnet should now be available and have price
	sonnet, exists := byID["anthropic/claude-3-5-sonnet"]
	require.True(t, exists)
	assert.True(t, sonnet.Available)
	assert.Equal(t, "available", sonnet.Status)
	assert.NotNil(t, sonnet.InputPricePer1M)

	// custom-claude should also be listed and available
	custom, exists := byID["other/custom-claude"]
	require.True(t, exists)
	assert.True(t, custom.Available)
	assert.NotNil(t, custom.InputPricePer1M)
}

func TestGetPublicModelsPricing_NilServiceSafe(t *testing.T) {
	var svc *GatewayService
	items := svc.GetPublicModelsPricing(context.Background())
	assert.NotEmpty(t, items)
}

func TestGetPublicModelsPricing_PlatformFallbackAndPricingTypes(t *testing.T) {
	repo := &mockAccountRepoForPricing{
		schedulable: []Account{
			{Platform: "anthropic"},
			{Platform: "openai"},
			{Platform: "google"},
			{Platform: "yunwu"},
		},
	}

	raw := map[string]*YunwuPricingRawItem{
		"claude-3-5-sonnet": {
			ModelRatio:      1.5,
			CompletionRatio: 5.0,
			Available:       true,
		},
		"gpt-4o": {
			ModelRatio:      1.25,
			CompletionRatio: 4.0,
			Available:       true,
		},
	}

	billingSvc := helperNewBillingWithYunwuPricing(raw)
	svc := &GatewayService{
		accountRepo:    repo,
		billingService: billingSvc,
	}

	items := svc.GetPublicModelsPricing(context.Background())
	require.NotEmpty(t, items)

	var hasAnthropicAvailable, hasOpenAIAvailable bool
	for _, item := range items {
		if strings.HasPrefix(item.CanonicalModelID, "anthropic/") && item.Available {
			hasAnthropicAvailable = true
		}
		if strings.HasPrefix(item.CanonicalModelID, "openai/") && item.Available {
			hasOpenAIAvailable = true
		}
	}
	assert.True(t, hasAnthropicAvailable)
	assert.True(t, hasOpenAIAvailable)
}

type mockGroupRepoForPricing struct {
	GroupRepository
	groupsByPlatform map[string][]Group
	activeGroups     []Group
}

func (m *mockGroupRepoForPricing) ListActiveByPlatform(ctx context.Context, platform string) ([]Group, error) {
	return m.groupsByPlatform[platform], nil
}

func (m *mockGroupRepoForPricing) ListActive(ctx context.Context) ([]Group, error) {
	return m.activeGroups, nil
}

func TestGetPublicModelsPricing_WithYunwuGroupMarkup(t *testing.T) {
	groupRepo := &mockGroupRepoForPricing{
		groupsByPlatform: map[string][]Group{
			PlatformYunwu: {
				{
					Name:           "Yunwu Public",
					Platform:       PlatformYunwu,
					RateMultiplier: 1.25, // 25% markup
				},
			},
		},
	}

	raw := map[string]*YunwuPricingRawItem{
		"claude-sonnet-5": {
			ModelRatio:      1.5,
			CompletionRatio: 5.0,
			Available:       true,
		},
		"sora": {
			ModelPrice: 0.10,
			Available:  true,
		},
	}
	billingSvc := helperNewBillingWithYunwuPricing(raw)

	svc := &GatewayService{
		groupRepo:      groupRepo,
		billingService: billingSvc,
	}

	assert.Equal(t, 1.25, svc.ResolveYunwuRateMultiplier(context.Background()))

	items := svc.GetPublicModelsPricing(context.Background())
	require.NotEmpty(t, items)

	byID := make(map[string]PublicModelPricingItem)
	for _, item := range items {
		byID[item.CanonicalModelID] = item
	}

	// anthropic/claude-sonnet-5: base was $3.0 input, $15.0 output
	// with 1.25 markup: $3.0 * 1.25 = $3.75, $15.0 * 1.25 = $18.75
	sonnet5, exists := byID["anthropic/claude-sonnet-5"]
	require.True(t, exists)
	require.NotNil(t, sonnet5.InputPricePer1M)
	require.NotNil(t, sonnet5.OutputPricePer1M)
	assert.Equal(t, 3.75, *sonnet5.InputPricePer1M)
	assert.Equal(t, 18.75, *sonnet5.OutputPricePer1M)

	// openai/sora: base was $0.10
	// with 1.25 markup: $0.10 * 1.25 = $0.125
	sora, exists := byID["openai/sora"]
	require.True(t, exists)
	require.NotNil(t, sora.NativePriceUSD)
	assert.Equal(t, 0.125, *sora.NativePriceUSD)
}

func TestGetPublicModelsPricing_GeminiAndDeepSeekPricing(t *testing.T) {
	raw := map[string]*YunwuPricingRawItem{
		"gemini-2.5-flash": {ModelRatio: 0.15, CompletionRatio: 8.34, Available: true},
		"gemini-3.5-flash": {ModelRatio: 0.75, CompletionRatio: 6.0, Available: true},
		"deepseek-v3":      {ModelRatio: 0.14495, CompletionRatio: 4.0, Available: true},
		"deepseek-r1":      {ModelRatio: 0.2899, CompletionRatio: 4.0, Available: true},
	}
	billingSvc := helperNewBillingWithYunwuPricing(raw)
	svc := &GatewayService{billingService: billingSvc}
	items := svc.GetPublicModelsPricing(context.Background())

	byID := make(map[string]PublicModelPricingItem)
	for _, item := range items {
		byID[item.CanonicalModelID] = item
	}

	// 1. Check gemini-2.5-flash
	g25, exists := byID["google/gemini-2.5-flash"]
	require.True(t, exists)
	require.NotNil(t, g25.InputPricePer1M)
	require.NotNil(t, g25.OutputPricePer1M)

	// 2. Check gemini-3.5-flash and dashed gemini-3-5-flash
	g35, exists := byID["google/gemini-3.5-flash"]
	require.True(t, exists)
	require.NotNil(t, g35.InputPricePer1M)

	g35Dash, exists := byID["google/gemini-3-5-flash"]
	require.True(t, exists)
	require.NotNil(t, g35Dash.InputPricePer1M)

	// 3. Check deepseek-v3 and aliased deepseek-chat
	d3, exists := byID["deepseek/deepseek-v3"]
	require.True(t, exists)
	require.NotNil(t, d3.InputPricePer1M)

	dChat, exists := byID["deepseek/deepseek-chat"]
	require.True(t, exists)
	require.NotNil(t, dChat.InputPricePer1M)

	// 4. Check deepseek-r1 and aliased deepseek-reasoner
	dr1, exists := byID["deepseek/deepseek-r1"]
	require.True(t, exists)
	require.NotNil(t, dr1.InputPricePer1M)

	dReasoner, exists := byID["deepseek/deepseek-reasoner"]
	require.True(t, exists)
	require.NotNil(t, dReasoner.InputPricePer1M)

	// Test nil safety
	var nilBilling *BillingService
	assert.Nil(t, nilBilling.GetYunwuKnownModelNames(context.Background()))
	emptyBilling := &BillingService{}
	assert.Nil(t, emptyBilling.GetYunwuKnownModelNames(context.Background()))
}

func TestResolveYunwuRateMultiplier_Fallbacks(t *testing.T) {
	// 1. Matched by active group name "yunwu"
	groupRepoByName := &mockGroupRepoForPricing{
		activeGroups: []Group{
			{Name: "General Yunwu Group", RateMultiplier: 1.30},
		},
	}
	svc1 := &GatewayService{groupRepo: groupRepoByName}
	assert.Equal(t, 1.30, svc1.ResolveYunwuRateMultiplier(context.Background()))

	// 2. Fallback to schedulable account RateMultiplier
	rate := 1.20
	accRepo := &mockAccountRepoForPricing{
		schedulable: []Account{
			{Platform: "yunwu", RateMultiplier: &rate},
		},
	}
	svc2 := &GatewayService{accountRepo: accRepo}
	assert.Equal(t, 1.20, svc2.ResolveYunwuRateMultiplier(context.Background()))

	// 3. Fallback to config.Default.RateMultiplier
	// (cfg.Default.RateMultiplier)
	cfg := &config.Config{}
	cfg.Default.RateMultiplier = 1.15
	svc3 := &GatewayService{cfg: cfg}
	assert.Equal(t, 1.15, svc3.ResolveYunwuRateMultiplier(context.Background()))

	// 4. Default fallback to 1.10 (10% base markup) when nothing configured
	svc4 := &GatewayService{}
	assert.Equal(t, 1.10, svc4.ResolveYunwuRateMultiplier(context.Background()))

	// 5. Explicit multiplier in context overrides repository/config
	ctxWithRate := ContextWithRateMultiplier(context.Background(), 1.55)
	assert.Equal(t, 1.55, svc1.ResolveYunwuRateMultiplier(ctxWithRate))
}

func TestVerifyCatalogMatchesBillingExact(t *testing.T) {
	raw := map[string]*YunwuPricingRawItem{
		"claude-3-5-sonnet": {ModelRatio: 1.5, CompletionRatio: 5.0, Available: true},
		"gemini-2.5-flash":  {ModelRatio: 0.15, CompletionRatio: 8.34, Available: true},
		"gemini-3.5-flash":  {ModelRatio: 0.75, CompletionRatio: 6.0, Available: true},
		"deepseek-v3":       {ModelRatio: 0.14495, CompletionRatio: 4.0, Available: true},
		"deepseek-r1":       {ModelRatio: 0.2899, CompletionRatio: 4.0, Available: true},
		"midjourney":        {ModelPrice: 0.05, Available: true},
	}
	billingSvc := helperNewBillingWithYunwuPricing(raw)

	groupRepo := &mockGroupRepoForPricing{
		activeGroups: []Group{
			{Name: "yunwu", Platform: PlatformYunwu, RateMultiplier: 1.25},
		},
	}
	svc := &GatewayService{
		billingService: billingSvc,
		groupRepo:      groupRepo,
	}

	ctx := context.Background()
	items := svc.GetPublicModelsPricing(ctx)
	require.NotEmpty(t, items)

	for _, item := range items {
		if item.InputPricePer1M == nil && item.NativePriceUSD == nil {
			continue
		}

		if item.InputPricePer1M != nil {
			// Simulate 10,000 input & output tokens billing request to verify unit token rate parity
			cost, err := billingSvc.CalculateCostWithContext(ctx, item.CanonicalModelID, UsageTokens{
				InputTokens:  10_000,
				OutputTokens: 10_000,
			}, 1.25, "")
			require.NoError(t, err, "billing should succeed for %s", item.CanonicalModelID)

			billedInput1M := (cost.InputCost / 10_000.0) * 1_000_000 * 1.25
			billedOutput1M := (cost.OutputCost / 10_000.0) * 1_000_000 * 1.25

			assert.InDeltaf(t, *item.InputPricePer1M, billedInput1M, 1e-4,
				"Catalog input price for %s (%f) must exactly match billing deduction (%f)",
				item.CanonicalModelID, *item.InputPricePer1M, billedInput1M)
			assert.InDeltaf(t, *item.OutputPricePer1M, billedOutput1M, 1e-4,
				"Catalog output price for %s (%f) must exactly match billing deduction (%f)",
				item.CanonicalModelID, *item.OutputPricePer1M, billedOutput1M)
		}

		if item.NativePriceUSD != nil {
			cost, err := billingSvc.CalculateCostWithContext(ctx, item.CanonicalModelID, UsageTokens{}, 1.25, "")
			require.NoError(t, err, "billing should succeed for native price model %s", item.CanonicalModelID)
			assert.InDeltaf(t, *item.NativePriceUSD, cost.ActualCost, 1e-4,
				"Catalog native price for %s (%f) must exactly match billing deduction (%f)",
				item.CanonicalModelID, *item.NativePriceUSD, cost.ActualCost)
		}
	}

	// Verify alias and prefix normalization yields identical billing results
	// 1. Google Gemini
	costCanonical, err := billingSvc.CalculateCostWithContext(ctx, "google/gemini-2.5-flash", UsageTokens{InputTokens: 1000}, 1.25, "")
	require.NoError(t, err)
	costSlug, err := billingSvc.CalculateCostWithContext(ctx, "gemini-2.5-flash", UsageTokens{InputTokens: 1000}, 1.25, "")
	require.NoError(t, err)
	costDashed, err := billingSvc.CalculateCostWithContext(ctx, "gemini-2-5-flash", UsageTokens{InputTokens: 1000}, 1.25, "")
	require.NoError(t, err)
	assert.Equal(t, costCanonical.ActualCost, costSlug.ActualCost)
	assert.Equal(t, costCanonical.ActualCost, costDashed.ActualCost)

	// 2. DeepSeek
	costD3, err := billingSvc.CalculateCostWithContext(ctx, "deepseek/deepseek-v3", UsageTokens{InputTokens: 1000}, 1.25, "")
	require.NoError(t, err)
	costChat, err := billingSvc.CalculateCostWithContext(ctx, "deepseek-chat", UsageTokens{InputTokens: 1000}, 1.25, "")
	require.NoError(t, err)
	costChatWithVendor, err := billingSvc.CalculateCostWithContext(ctx, "deepseek/deepseek-chat", UsageTokens{InputTokens: 1000}, 1.25, "")
	require.NoError(t, err)
	assert.Equal(t, costD3.ActualCost, costChat.ActualCost)
	assert.Equal(t, costD3.ActualCost, costChatWithVendor.ActualCost)

	// 3. Anthropic with date snapshot
	costSonnet, err := billingSvc.CalculateCostWithContext(ctx, "anthropic/claude-3-5-sonnet", UsageTokens{InputTokens: 1000}, 1.25, "")
	require.NoError(t, err)
	costSonnetSnapshot, err := billingSvc.CalculateCostWithContext(ctx, "claude-3-5-sonnet-20241022", UsageTokens{InputTokens: 1000}, 1.25, "")
	require.NoError(t, err)
	assert.Equal(t, costSonnet.ActualCost, costSonnetSnapshot.ActualCost)
}

// A model that is "available" (listed by a schedulable account) but has no
// price from any connected upstream-yunwu provider must remain unpriced (null)
// and unavailable. Falling back to LiteLLM / static tables here would advertise
// a price the yunwu billing path never charges (it fails safe instead), i.e.
// catalog/billing divergence.
func TestGetPublicModelsPricing_AvailableButUnpricedStaysNull(t *testing.T) {
	billingSvc := &BillingService{
		fallbackPrices: map[string]*ModelPricing{
			// A static fallback exists for this model, but it must NOT leak into
			// the catalog because it is not an upstream-yunwu price.
			"custom-unpriced-model": {InputPricePerToken: 3e-6, OutputPricePerToken: 15e-6},
		},
	}
	repo := &mockAccountRepoForPricing{
		schedulable: []Account{
			{
				Platform: "custom",
				Extra: map[string]any{
					AccountExtraFetchedModelsKey: []any{"custom-unpriced-model"},
				},
			},
		},
	}
	svc := &GatewayService{
		accountRepo:    repo,
		billingService: billingSvc,
	}

	items := svc.GetPublicModelsPricing(context.Background())
	var found bool
	for _, it := range items {
		if strings.Contains(it.CanonicalModelID, "custom-unpriced-model") {
			found = true
			assert.Nil(t, it.InputPricePer1M, "unpriced-upstream model must expose no input price")
			assert.Nil(t, it.OutputPricePer1M, "unpriced-upstream model must expose no output price")
			assert.Nil(t, it.NativePriceUSD)
			assert.False(t, it.Available)
			assert.Equal(t, "unavailable", it.Status)
		}
	}
	assert.True(t, found)
}

// A fixed per-unit price must preserve its native billing unit (e.g. "second"
// for video) instead of always being labelled "request".
func TestGetPublicModelsPricing_FixedPriceUnit(t *testing.T) {
	raw := map[string]*YunwuPricingRawItem{
		"video-model": {ModelPrice: 0.10, FixedPriceUnit: "second", Available: true},
	}
	billingSvc := helperNewBillingWithYunwuPricing(raw)
	svc := &GatewayService{billingService: billingSvc}

	items := svc.GetPublicModelsPricing(context.Background())
	var found bool
	for _, it := range items {
		if strings.HasSuffix(it.CanonicalModelID, "video-model") {
			found = true
			require.NotNil(t, it.NativePriceUSD)
			require.NotNil(t, it.NativePriceUnit)
			// Base default multiplier 1.10 -> $0.10 * 1.10 = $0.11
			assert.InDelta(t, 0.11, *it.NativePriceUSD, 1e-3)
			assert.Equal(t, "second", *it.NativePriceUnit)
			assert.Nil(t, it.InputPricePer1M)
		}
	}
	assert.True(t, found)
}

// A schedulable yunwu account must make the core families (claude/gpt/gemini)
// available, not just the moonshot/zhipu/deepseek/qwen vendors — a yunwu provider
// serves all of them.
func TestGetPublicModelsPricing_YunwuAccountMarksCoreFamiliesAvailable(t *testing.T) {
	resetYunwuProviderScopeCache()
	t.Cleanup(resetYunwuProviderScopeCache)

	yw := NewYunwuPricingService()
	yw.providerURLs = map[string]string{"openlux": "m1"}
	yw.providerPricing = map[string]map[string]*YunwuPricingRawItem{
		"openlux": {
			"claude-sonnet-5":  {ModelRatio: 1.5, CompletionRatio: 5.0, Available: true},
			"gpt-5.4":          {ModelRatio: 1.0, CompletionRatio: 5.0, Available: true},
			"gemini-2.5-flash": {ModelRatio: 0.15, CompletionRatio: 8.0, Available: true},
		},
	}
	yw.providerUpdated = map[string]time.Time{"openlux": time.Now()}
	billingSvc := NewBillingService(nil, nil, yw)
	repo := &mockAccountRepoForPricing{schedulable: []Account{
		{Platform: PlatformYunwu, Extra: map[string]any{"provider": "openlux"}},
	}}
	svc := &GatewayService{billingService: billingSvc, accountRepo: repo}

	byID := map[string]PublicModelPricingItem{}
	for _, it := range svc.GetPublicModelsPricing(context.Background()) {
		byID[it.CanonicalModelID] = it
	}
	for _, id := range []string{"anthropic/claude-sonnet-5", "openai/gpt-5.4", "google/gemini-2.5-flash"} {
		it, ok := byID[id]
		require.True(t, ok, "%s must be listed", id)
		require.NotNilf(t, it.InputPricePer1M, "%s must be priced", id)
		assert.Truef(t, it.Available, "%s must be available via the yunwu pool", id)
	}
}

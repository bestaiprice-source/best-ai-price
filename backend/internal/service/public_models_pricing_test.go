package service

import (
	"context"
	"strings"
	"testing"

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

func TestGetPublicModelsPricing_FallbackAndDefaults(t *testing.T) {
	svc := &GatewayService{}
	items := svc.GetPublicModelsPricing(context.Background())

	require.NotEmpty(t, items)
	assert.GreaterOrEqual(t, len(items), len(knownCanonicalModelIDs))

	byID := make(map[string]PublicModelPricingItem)
	for _, item := range items {
		require.NotEmpty(t, item.CanonicalModelID)
		byID[item.CanonicalModelID] = item
	}

	// 1. Check anthropic/claude-sonnet-5 (from user prompt)
	sonnet5, exists := byID["anthropic/claude-sonnet-5"]
	require.True(t, exists, "anthropic/claude-sonnet-5 should exist in items")
	require.NotNil(t, sonnet5.InputPricePer1M)
	require.NotNil(t, sonnet5.OutputPricePer1M)
	assert.Equal(t, 3.0, *sonnet5.InputPricePer1M)
	assert.Equal(t, 15.0, *sonnet5.OutputPricePer1M)
	assert.Equal(t, "unavailable", sonnet5.Status)
	assert.False(t, sonnet5.Available)

	// 2. Check openai/gpt-5.5
	gpt55, exists := byID["openai/gpt-5.5"]
	require.True(t, exists)
	require.NotNil(t, gpt55.InputPricePer1M)
	assert.Equal(t, 2.5, *gpt55.InputPricePer1M)
	assert.Equal(t, 10.0, *gpt55.OutputPricePer1M)

	// 3. Check flat-rate native price (image & video)
	gptImage2, exists := byID["openai/gpt-image-2"]
	require.True(t, exists)
	require.NotNil(t, gptImage2.NativePriceUSD)
	require.NotNil(t, gptImage2.NativePriceUnit)
	assert.Equal(t, 0.04, *gptImage2.NativePriceUSD)
	assert.Equal(t, "image", *gptImage2.NativePriceUnit)
	assert.Nil(t, gptImage2.InputPricePer1M)

	sora, exists := byID["openai/sora"]
	require.True(t, exists)
	require.NotNil(t, sora.NativePriceUSD)
	require.NotNil(t, sora.NativePriceUnit)
	assert.Equal(t, 0.10, *sora.NativePriceUSD)
	assert.Equal(t, "second", *sora.NativePriceUnit)

	// 4. Check moonshot/kimi-k3
	kimiK3, exists := byID["moonshot/kimi-k3"]
	require.True(t, exists)
	require.NotNil(t, kimiK3.InputPricePer1M)
	assert.Equal(t, 2.76, *kimiK3.InputPricePer1M)
	assert.Equal(t, 13.8, *kimiK3.OutputPricePer1M)

	// 5. Check zhipu/glm-5-3
	glm53, exists := byID["zhipu/glm-5-3"]
	require.True(t, exists)
	require.NotNil(t, glm53.InputPricePer1M)
	assert.Equal(t, 1.104, *glm53.InputPricePer1M)
	assert.Equal(t, 3.864, *glm53.OutputPricePer1M)
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

	billingSvc := &BillingService{}
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

	// claude-3-5-sonnet should now be available
	sonnet, exists := byID["anthropic/claude-3-5-sonnet"]
	require.True(t, exists)
	assert.True(t, sonnet.Available)
	assert.Equal(t, "available", sonnet.Status)

	// custom-claude should also be listed
	custom, exists := byID["other/custom-claude"]
	require.True(t, exists)
	assert.True(t, custom.Available)
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

	billingSvc := NewBillingService(nil, nil, nil)
	billingSvc.fallbackPrices["fixed-model"] = &ModelPricing{
		FixedPricePerRequest: 0.05,
	}
	billingSvc.fallbackPrices["image-model"] = &ModelPricing{
		ImageOutputPricePerToken: 0.02,
	}

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



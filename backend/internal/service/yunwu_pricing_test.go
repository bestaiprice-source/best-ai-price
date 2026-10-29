//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/runtime"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/yunwu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleYunwuPricingPayload() YunwuPricingResponse {
	return YunwuPricingResponse{
		Data: []YunwuPricingRawItem{
			{
				ModelName:       "gemini-3.8-flash",
				QuotaType:       0,
				ModelRatio:      0.375,
				ModelPrice:      0,
				CompletionRatio: 5,
				CacheRatio:      0.1,
				Available:       true,
			},
			{
				ModelName:       "gemini-3.5-flash",
				QuotaType:       0,
				ModelRatio:      0.75,
				ModelPrice:      0,
				CompletionRatio: 6,
				CacheRatio:      0,
				Available:       true,
			},
			{
				ModelName:       "gemini-3-pro-preview",
				QuotaType:       0,
				ModelRatio:      1.0,
				ModelPrice:      0,
				CompletionRatio: 6,
				CacheRatio:      0.2,
				Available:       true,
			},
			{
				ModelName:       "gemini-3.1-flash-image-preview",
				QuotaType:       1,
				ModelRatio:      0,
				ModelPrice:      0.1655,
				CompletionRatio: 0,
				CacheRatio:      0,
				Available:       true,
			},
		},
	}
}

func TestYunwuPricingService_FetchPricing_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/pricing", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sampleYunwuPricingPayload())
	}))
	defer ts.Close()

	svc := NewYunwuPricingServiceWithClient(ts.URL+"/api/pricing", ts.Client())
	data, err := svc.FetchPricing(context.Background())
	require.NoError(t, err)
	require.Len(t, data, 4)
	require.Contains(t, data, "gemini-3.8-flash")
	require.Equal(t, 0.375, data["gemini-3.8-flash"].ModelRatio)
	require.Equal(t, 5.0, data["gemini-3.8-flash"].CompletionRatio)
}

func TestYunwuPricingService_GetModelPricing_Calculations(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sampleYunwuPricingPayload())
	}))
	defer ts.Close()

	svc := NewYunwuPricingServiceWithClient(ts.URL+"/api/pricing", ts.Client())

	// 1. Test gemini-3.8-flash token pricing
	p, ok := svc.GetModelPricing(context.Background(), "gemini-3.8-flash")
	require.True(t, ok)
	require.NotNil(t, p)

	// basePerToken = 0.002 / 1000 = 2e-6
	expectedInput := 0.375 * 2e-6
	expectedOutput := 0.375 * 5.0 * 2e-6
	expectedCacheRead := 0.375 * 0.1 * 2e-6
	expectedCacheCreation := expectedInput * 1.25

	require.InDelta(t, expectedInput, p.InputPricePerToken, 1e-12)
	require.InDelta(t, expectedOutput, p.OutputPricePerToken, 1e-12)
	require.InDelta(t, expectedCacheRead, p.CacheReadPricePerToken, 1e-12)
	require.InDelta(t, expectedCacheCreation, p.CacheCreationPricePerToken, 1e-12)
	require.Equal(t, 0.0, p.FixedPricePerRequest)

	// 2. Test alias gemini-3-pro -> gemini-3-pro-preview
	pAlias, ok := svc.GetModelPricing(context.Background(), "gemini-3-pro")
	require.True(t, ok)
	require.NotNil(t, pAlias)
	require.InDelta(t, 1.0*2e-6, pAlias.InputPricePerToken, 1e-12)
	require.InDelta(t, 1.0*6.0*2e-6, pAlias.OutputPricePerToken, 1e-12)

	// 3. Test fixed price model
	pFixed, ok := svc.GetModelPricing(context.Background(), "gemini-3.1-flash-image-preview")
	require.True(t, ok)
	require.NotNil(t, pFixed)
	require.InDelta(t, 0.1655, pFixed.FixedPricePerRequest, 1e-8)
}

func TestYunwuPricingService_FallbackOnNetworkFailure(t *testing.T) {
	var shouldFail atomic.Bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if shouldFail.Load() {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sampleYunwuPricingPayload())
	}))
	defer ts.Close()

	svc := NewYunwuPricingServiceWithClient(ts.URL+"/api/pricing", ts.Client())

	// First fetch succeeds and populates cache snapshot
	p1, ok := svc.GetModelPricing(context.Background(), "gemini-3.8-flash")
	require.True(t, ok)
	require.NotNil(t, p1)

	// Upstream starts failing (500)
	shouldFail.Store(true)

	// Second fetch gracefully uses cached snapshot
	p2, ok := svc.GetModelPricing(context.Background(), "gemini-3.8-flash")
	require.True(t, ok)
	require.NotNil(t, p2)
	require.Equal(t, p1.InputPricePerToken, p2.InputPricePerToken)
}

func TestYunwuPricingService_SingleflightDeduplication(t *testing.T) {
	var requestCount atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		time.Sleep(50 * time.Millisecond) // simulate remote latency
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sampleYunwuPricingPayload())
	}))
	defer ts.Close()

	svc := NewYunwuPricingServiceWithClient(ts.URL+"/api/pricing", ts.Client())

	var wg sync.WaitGroup
	concurrentRequests := 15
	wg.Add(concurrentRequests)

	for i := 0; i < concurrentRequests; i++ {
		go func() {
			defer wg.Done()
			_, _ = svc.FetchPricing(context.Background())
		}()
	}

	wg.Wait()

	// Singleflight should have merged simultaneous calls into 1 HTTP request
	require.Equal(t, int32(1), requestCount.Load(), "singleflight should coalesce concurrent fetches into 1 request")
}

func TestBillingService_WithYunwuPricing_CalculateCost(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sampleYunwuPricingPayload())
	}))
	defer ts.Close()

	ywSvc := NewYunwuPricingServiceWithClient(ts.URL+"/api/pricing", ts.Client())
	billingSvc := NewBillingService(&config.Config{}, nil, ywSvc)

	// 1. CalculateCost for gemini-3.8-flash
	tokens := UsageTokens{
		InputTokens:         1000,
		OutputTokens:        500,
		CacheReadTokens:     200,
		CacheCreationTokens: 100,
	}

	multiplier := 1.5
	cost, err := billingSvc.CalculateCost("gemini-3.8-flash", tokens, multiplier)
	require.NoError(t, err)
	require.NotNil(t, cost)

	// Input: 1000 * (0.375 * 2e-6) = 0.00075
	// Output: 500 * (0.375 * 5 * 2e-6) = 0.001875
	// CacheRead: 200 * (0.375 * 0.1 * 2e-6) = 0.000015
	// CacheCreation: 100 * (0.375 * 2e-6 * 1.25) = 0.00009375
	expectedInputCost := 1000 * (0.375 * 2e-6)
	expectedOutputCost := 500 * (0.375 * 5 * 2e-6)
	expectedTotalCost := cost.InputCost + cost.OutputCost + cost.CacheCreationCost + cost.CacheReadCost

	require.InDelta(t, expectedInputCost, cost.InputCost, 1e-10)
	require.InDelta(t, expectedOutputCost, cost.OutputCost, 1e-10)
	require.InDelta(t, expectedTotalCost, cost.TotalCost, 1e-10)
	require.InDelta(t, expectedTotalCost*multiplier, cost.ActualCost, 1e-10)
	require.True(t, cost.ActualCost > 0, "actual cost must be deducted and > 0")

	// 2. Fixed price model gemini-3.1-flash-image-preview
	fixedCost, err := billingSvc.CalculateCost("gemini-3.1-flash-image-preview", UsageTokens{}, 2.0)
	require.NoError(t, err)
	require.NotNil(t, fixedCost)
	require.InDelta(t, 0.1655, fixedCost.TotalCost, 1e-8)
	require.InDelta(t, 0.1655*2.0, fixedCost.ActualCost, 1e-8)
}

func TestYunwuPricingService_ConstructorsAndEnv(t *testing.T) {
	// Default constructor
	s1 := NewYunwuPricingService()
	require.Equal(t, DefaultYunwuPricingURL, s1.pricingURL)

	// Env override YUNWU_PRICING_URL
	t.Setenv("YUNWU_PRICING_URL", "https://custom-yunwu.com/api/pricing")
	s2 := NewYunwuPricingService()
	require.Equal(t, "https://custom-yunwu.com/api/pricing", s2.pricingURL)

	// Env override OPENLUX_PRICING_URL
	t.Setenv("YUNWU_PRICING_URL", "")
	t.Setenv("OPENLUX_PRICING_URL", "https://custom-openlux.com/api/pricing")
	s3 := NewYunwuPricingService()
	require.Equal(t, "https://custom-openlux.com/api/pricing", s3.pricingURL)

	// NewYunwuPricingServiceWithClient defaults
	s4 := NewYunwuPricingServiceWithClient("", nil)
	require.Equal(t, DefaultYunwuPricingURL, s4.pricingURL)
	require.NotNil(t, s4.httpClient)
}

func TestYunwuPricingService_EdgeCases(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sampleYunwuPricingPayload())
	}))
	defer ts.Close()

	svc := NewYunwuPricingServiceWithClient(ts.URL+"/api/pricing", ts.Client())

	// Empty model name
	pEmpty, ok := svc.GetModelPricing(context.Background(), "")
	require.False(t, ok)
	require.Nil(t, pEmpty)

	// Unknown model
	pUnknown, ok := svc.GetModelPricing(context.Background(), "unknown-nonexistent-model")
	require.False(t, ok)
	require.Nil(t, pUnknown)

	// Prefix models/
	pPrefixed, ok := svc.GetModelPricing(context.Background(), "models/gemini-3.8-flash")
	require.True(t, ok)
	require.NotNil(t, pPrefixed)
	require.InDelta(t, 0.375*2e-6, pPrefixed.InputPricePerToken, 1e-12)

	// Model with cache_ratio == 0 (gemini-3.5-flash in sample has cache_ratio 0)
	pNoCacheRatio, ok := svc.GetModelPricing(context.Background(), "gemini-3.5-flash")
	require.True(t, ok)
	require.NotNil(t, pNoCacheRatio)
	expectedInput := 0.75 * 2e-6
	require.InDelta(t, expectedInput, pNoCacheRatio.InputPricePerToken, 1e-12)
	require.InDelta(t, expectedInput*0.5, pNoCacheRatio.CacheReadPricePerToken, 1e-12)

	// Initial fetch network failure with empty cache
	badSvc := NewYunwuPricingServiceWithClient("http://127.0.0.1:0/unreachable", nil)
	pBad, ok := badSvc.GetModelPricing(context.Background(), "gemini-3.8-flash")
	require.False(t, ok)
	require.Nil(t, pBad)
}

func TestOpenAIGatewayService_CheckModelPricingAvailable(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sampleYunwuPricingPayload())
	}))
	defer ts.Close()

	ywSvc := NewYunwuPricingServiceWithClient(ts.URL+"/api/pricing", ts.Client())
	billingSvc := NewBillingService(&config.Config{}, nil, ywSvc)
	gw := &OpenAIGatewayService{billingService: billingSvc}

	// Model in Yunwu pricing
	err := gw.CheckModelPricingAvailable(context.Background(), "gemini-3.8-flash")
	require.NoError(t, err)

	// Model alias
	err = gw.CheckModelPricingAvailable(context.Background(), "gemini-3-pro")
	require.NoError(t, err)

	// Fallback model in BillingService (claude-sonnet-4)
	err = gw.CheckModelPricingAvailable(context.Background(), "claude-sonnet-4")
	require.NoError(t, err)

	// Unknown model not found in Yunwu, LiteLLM or fallbacks -> returns error
	err = gw.CheckModelPricingAvailable(context.Background(), "totally-unknown-model-xyz")
	require.Error(t, err)
	require.Contains(t, err.Error(), "pricing not found for model")
}

func TestYunwuPricingService_ProviderIsolation(t *testing.T) {
	// Endpoint 1: OpenLux-like
	tsOpenLux := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(YunwuPricingResponse{
			Data: []YunwuPricingRawItem{
				{ModelName: "gemini-3.8-flash", ModelRatio: 0.375, CompletionRatio: 5},
				{ModelName: "gpt-4o", ModelRatio: 1.0, CompletionRatio: 3},
			},
		})
	}))
	defer tsOpenLux.Close()

	// Endpoint 2: Top-API-like (different price for gpt-4o, and exclusive claude-haiku-4-5)
	tsTopAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(YunwuPricingResponse{
			Data: []YunwuPricingRawItem{
				{ModelName: "claude-haiku-4-5", ModelRatio: 1.25, CompletionRatio: 4},
				{ModelName: "gpt-4o", ModelRatio: 2.0, CompletionRatio: 3},
			},
		})
	}))
	defer tsTopAPI.Close()

	// Endpoint 3: X-LLM-like (exclusive kimi-k2.6, different ratio for gpt-4o)
	tsXLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(YunwuPricingResponse{
			Data: []YunwuPricingRawItem{
				{ModelName: "kimi-k2.6", ModelRatio: 3.25, CompletionRatio: 2},
				{ModelName: "gpt-4o", ModelRatio: 4.0, CompletionRatio: 3},
			},
		})
	}))
	defer tsXLLM.Close()

	svc := NewYunwuPricingServiceWithClient(tsOpenLux.URL, tsOpenLux.Client())
	svc.SetProviderURL("api-top", tsTopAPI.URL)
	svc.SetProviderURL("x-llm", tsXLLM.URL)

	ctx := context.Background()

	// 1. OpenLux fetch contains only OpenLux models
	openluxPrices, err := svc.FetchPricingForProvider(ctx, "openlux")
	require.NoError(t, err)
	require.Len(t, openluxPrices, 2)
	require.Contains(t, openluxPrices, "gemini-3.8-flash")
	require.Contains(t, openluxPrices, "gpt-4o")
	require.NotContains(t, openluxPrices, "claude-haiku-4-5", "OpenLux must not leak models from Top-API")
	require.NotContains(t, openluxPrices, "kimi-k2.6", "OpenLux must not leak models from X-LLM")
	require.Equal(t, 1.0, openluxPrices["gpt-4o"].ModelRatio)

	// 2. Top-API fetch contains only Top-API models
	topPrices, err := svc.FetchPricingForProvider(ctx, "api-top")
	require.NoError(t, err)
	require.Len(t, topPrices, 2)
	require.Contains(t, topPrices, "claude-haiku-4-5")
	require.Contains(t, topPrices, "gpt-4o")
	require.NotContains(t, topPrices, "gemini-3.8-flash", "Top-API must not leak models from OpenLux")
	require.NotContains(t, topPrices, "kimi-k2.6", "Top-API must not leak models from X-LLM")
	require.Equal(t, 2.0, topPrices["gpt-4o"].ModelRatio)

	// 3. X-LLM fetch contains only X-LLM models
	xllmPrices, err := svc.FetchPricingForProvider(ctx, "x-llm")
	require.NoError(t, err)
	require.Len(t, xllmPrices, 2)
	require.Contains(t, xllmPrices, "kimi-k2.6")
	require.Contains(t, xllmPrices, "gpt-4o")
	require.NotContains(t, xllmPrices, "gemini-3.8-flash", "X-LLM must not leak models from OpenLux")
	require.NotContains(t, xllmPrices, "claude-haiku-4-5", "X-LLM must not leak models from Top-API")
	require.Equal(t, 4.0, xllmPrices["gpt-4o"].ModelRatio)

	// 4. Pricing lookup for OpenLux returns OpenLux pricing
	pOpenLux, ok := svc.GetModelPricingForProvider(ctx, "openlux", "gpt-4o")
	require.True(t, ok)
	require.InDelta(t, 2e-6*1.0, pOpenLux.InputPricePerToken, 1e-9)

	// 5. Model not in OpenLux cannot be resolved through OpenLux
	_, ok = svc.GetModelPricingForProvider(ctx, "openlux", "claude-haiku-4-5")
	require.False(t, ok, "Model absent in OpenLux must not resolve via shared pool")
	_, ok = svc.GetModelPricingForProvider(ctx, "openlux", "kimi-k2.6")
	require.False(t, ok, "Model absent in OpenLux must not resolve via shared pool")

	// 6. Pricing lookup for Top-API returns Top-API pricing
	pTop, ok := svc.GetModelPricingForProvider(ctx, "api-top", "gpt-4o")
	require.True(t, ok)
	require.InDelta(t, 2e-6*2.0, pTop.InputPricePerToken, 1e-9)

	// 7. Pricing lookup for X-LLM returns X-LLM pricing
	pXLLM, ok := svc.GetModelPricingForProvider(ctx, "x-llm", "gpt-4o")
	require.True(t, ok)
	require.InDelta(t, 2e-6*4.0, pXLLM.InputPricePerToken, 1e-9)

	pKimi, ok := svc.GetModelPricingForProvider(ctx, "x-llm", "kimi-k2.6")
	require.True(t, ok)
	require.InDelta(t, 2e-6*3.25, pKimi.InputPricePerToken, 1e-9)

	// 8. Context-aware provider routing
	ctxWithTop := ContextWithPricingProvider(ctx, "api-top")
	pCtxTop, ok := svc.GetModelPricing(ctxWithTop, "gpt-4o")
	require.True(t, ok)
	require.InDelta(t, 2e-6*2.0, pCtxTop.InputPricePerToken, 1e-9)

	ctxWithXLLM := ContextWithPricingProvider(ctx, "x-llm")
	pCtxXLLM, ok := svc.GetModelPricing(ctxWithXLLM, "gpt-4o")
	require.True(t, ok)
	require.InDelta(t, 2e-6*4.0, pCtxXLLM.InputPricePerToken, 1e-9)

	pCtxKimi, ok := svc.GetModelPricing(ctxWithXLLM, "kimi-k2.6")
	require.True(t, ok)
	require.InDelta(t, 2e-6*3.25, pCtxKimi.InputPricePerToken, 1e-9)

	// 9. Default context falls back strictly to OpenLux, not a shared pool
	pDefault, ok := svc.GetModelPricing(ctx, "gpt-4o")
	require.True(t, ok)
	require.InDelta(t, 2e-6*1.0, pDefault.InputPricePerToken, 1e-9)

	// 8. CanonicalProvider helper normalizes variations
	require.Equal(t, "openlux", CanonicalPricingProvider("OpenLux"))
	require.Equal(t, "openlux", CanonicalPricingProvider("api.openlux.ai"))
	require.Equal(t, "api-top", CanonicalPricingProvider("top-api"))
	require.Equal(t, "api-top", CanonicalPricingProvider("API-TOP"))
	require.Equal(t, "yuanqiaoyun", CanonicalPricingProvider("yuanqiao"))
	require.Equal(t, "x-llm", CanonicalPricingProvider("x-llm"))
	require.Equal(t, "x-llm", CanonicalPricingProvider("x-llm.net"))
	require.Equal(t, "x-llm", CanonicalPricingProvider("xllm"))
	require.Equal(t, "portdan", CanonicalPricingProvider("portdan.com"))
}

func TestYunwuPricingService_FetchPricingForProvider_PortdanAndMultiFormat(t *testing.T) {
	// Mock BestAIPrice format for Portdan
	bestAIPriceJSON := `[
		{
			"provider_domain": "portdan.com",
			"provider_name": "Portdan AI",
			"model_name": "claude-3-7-sonnet",
			"canonical_model_id": "anthropic/claude-3-7-sonnet",
			"input_price_usd_per_1m": 3.0,
			"output_price_usd_per_1m": 15.0,
			"available": true
		},
		{
			"provider_domain": "other.com",
			"provider_name": "Other AI",
			"model_name": "other-model",
			"canonical_model_id": "other/other-model",
			"input_price_usd_per_1m": 5.0,
			"output_price_usd_per_1m": 25.0,
			"available": true
		}
	]`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(bestAIPriceJSON))
	}))
	defer ts.Close()

	svc := NewYunwuPricingServiceWithClient(ts.URL, ts.Client())
	svc.SetProviderPricingURL("portdan", ts.URL)

	ctx := context.Background()
	prices, err := svc.FetchPricingForProvider(ctx, "portdan")
	require.NoError(t, err)
	require.NotEmpty(t, prices)

	// claude-3-7-sonnet should be parsed
	item, ok := prices["claude-3-7-sonnet"]
	require.True(t, ok)
	require.NotNil(t, item)
	// Base token = 2e-6. 3.0 / 1M = 3e-6 per token. ModelRatio = 3e-6 / 2e-6 = 1.5
	assert.InDelta(t, 1.5, item.ModelRatio, 1e-4)
	assert.InDelta(t, 5.0, item.CompletionRatio, 1e-4)

	// other-model should be filtered out because provider is not portdan
	_, hasOther := prices["other-model"]
	assert.False(t, hasOther)
}

func TestEffectiveGroupRatio(t *testing.T) {
	gr := map[string]float64{"a": 0.5, "b": 0.25, "c": 1.0}
	assert.InDelta(t, 0.25, effectiveGroupRatio([]string{"a", "b"}, gr), 1e-9)
	assert.InDelta(t, 0.5, effectiveGroupRatio([]string{"a", "unknown"}, gr), 1e-9)
	assert.InDelta(t, 1.0, effectiveGroupRatio(nil, gr), 1e-9, "no enabled groups -> no discount")
	assert.InDelta(t, 1.0, effectiveGroupRatio([]string{"a"}, nil), 1e-9, "no group_ratio map -> no discount")
	assert.InDelta(t, 1.0, effectiveGroupRatio([]string{"unknown"}, gr), 1e-9, "unknown group -> no discount")
}

func TestYunwuPricingService_AppliesGroupRatio(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"model_name": "claude-sonnet-5", "model_ratio": 1.0, "completion_ratio": 5.0,
					"enable_groups": []string{"Kiro-Claude-1", "Claude-Code-1"}},
				{"model_name": "no-groups-model", "model_ratio": 2.0, "completion_ratio": 1.0},
			},
			"group_ratio": map[string]float64{
				"Kiro-Claude-1": 0.08824,
				"Claude-Code-1": 0.17647,
				"default":       1.0,
			},
		})
	}))
	defer ts.Close()

	svc := NewYunwuPricingServiceWithClient(ts.URL, ts.Client())
	ctx := context.Background()
	raw, err := svc.FetchPricingForProvider(ctx, "openlux")
	require.NoError(t, err)
	require.NotNil(t, raw["claude-sonnet-5"])
	assert.InDelta(t, 0.08824, raw["claude-sonnet-5"].GroupRatio, 1e-6)

	// Nothing is pinned, so the request is priced for the provider's `default`
	// group: it IS listed here (1.0), so the official rate applies.
	p, ok := svc.GetModelPricingForProvider(ctx, "openlux", "claude-sonnet-5")
	require.True(t, ok)
	require.NotNil(t, p)
	assert.InDelta(t, 2.0, p.InputPricePerToken*1e6, 1e-4)
	assert.InDelta(t, 10.0, p.OutputPricePerToken*1e6, 1e-4)

	// With the key pinned to a discounted group, that group's rate is charged.
	pinned := ContextWithYunwuTierGroups(ctx, map[string]yunwu.TierGroupNames{
		"openlux": {Smart: "Kiro-Claude-1"},
	})
	p3, ok := svc.GetModelPricingForProvider(pinned, "openlux", "claude-sonnet-5")
	require.True(t, ok)
	require.NotNil(t, p3)
	assert.InDelta(t, 0.17648, p3.InputPricePerToken*1e6, 1e-4)
	assert.InDelta(t, 0.8824, p3.OutputPricePerToken*1e6, 1e-4)

	// A model without enable_groups cannot be discounted either: it bills at the
	// official rate (model_ratio x base).
	p2, ok := svc.GetModelPricingForProvider(ctx, "openlux", "no-groups-model")
	require.True(t, ok)
	require.NotNil(t, p2)
	assert.InDelta(t, 4.0, p2.InputPricePerToken*1e6, 1e-4)
}

// Tier defaults and the reconciliation floor, end-to-end through the pricing path
// the catalog AND billing both use:
//
//	smart with an unknown group  -> official rate (1.0)
//	fast  with an unknown group  -> official rate + safety margin (1.1)
//	observed ratio               -> a FLOOR: it lifts a price, never lowers it
//	a pinned discounted group    -> still discounted
func TestGetModelPricingForProvider_TierDefaultsAndObservedFloor(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"model_name": "claude-sonnet-5", "model_ratio": 1.5, "completion_ratio": 5.0,
					"enable_groups": []string{"cheap"}},
			},
			"group_ratio": map[string]float64{"cheap": 0.1},
		})
	}))
	defer ts.Close()

	svc := NewYunwuPricingServiceWithClient(ts.URL, ts.Client())
	ctx := context.Background()
	per1M := func(ctx context.Context) float64 {
		p, ok := svc.GetModelPricingForProvider(ctx, "openlux", "claude-sonnet-5")
		require.True(t, ok)
		require.NotNil(t, p)
		return p.InputPricePerToken * 1e6
	}

	// model_ratio 1.5 -> official $3/1M
	assert.InDelta(t, 3.0, per1M(ctx), 1e-4, "smart default = official rate")

	fastCtx := runtime.ContextWithTier(ctx, runtime.TierFast)
	assert.InDelta(t, 3.3, per1M(fastCtx), 1e-4, "fast default = official rate + safety margin")

	// The provider charged 1.25x the official rate: both tiers are floored there.
	floored := ContextWithYunwuPricingScope(ctx, YunwuPricingScope{
		Observed: map[string]map[string]float64{"openlux": {"smart": 1.25, "fast": 1.25}},
	})
	assert.InDelta(t, 3.75, per1M(floored), 1e-4, "the observed ratio lifts the smart price")
	assert.InDelta(t, 4.125, per1M(runtime.ContextWithTier(floored, runtime.TierFast)), 1e-4,
		"the correction multiplies the fast default")

	// A pinned group is used as-is while the observation is BELOW it: a cheaper
	// reading never lowers a price.
	pinnedCheap := ContextWithYunwuPricingScope(ctx, YunwuPricingScope{
		TierGroups: map[string]yunwu.TierGroupNames{"openlux": {Smart: "cheap"}},
		Observed:   map[string]map[string]float64{"openlux": {"smart": 0.05}},
	})
	assert.InDelta(t, 0.3, per1M(pinnedCheap), 1e-4, "the pinned discounted group stands")

	// A correction ≥ 1 likewise multiplies a pinned group's own rate, so a pin that
	// turned out too optimistic is lifted instead of trusted.
	pinnedCheapCostlier := ContextWithYunwuPricingScope(ctx, YunwuPricingScope{
		TierGroups: map[string]yunwu.TierGroupNames{"openlux": {Smart: "cheap"}},
		Observed:   map[string]map[string]float64{"openlux": {"smart": 2.0}},
	})
	assert.InDelta(t, 0.6, per1M(pinnedCheapCostlier), 1e-4,
		"a measured 2x over the pinned group doubles its rate")
}

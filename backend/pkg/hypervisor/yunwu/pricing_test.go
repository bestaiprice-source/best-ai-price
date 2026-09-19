package yunwu

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/quota"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPricingURLForProvider(t *testing.T) {
	tests := []struct {
		provider string
		baseURL  string
		expected string
	}{
		{
			provider: "openlux",
			baseURL:  "",
			expected: "https://api.openlux.ai/api/pricing",
		},
		{
			provider: "api-top",
			baseURL:  "",
			expected: "https://api-top.com/api/pricing",
		},
		{
			provider: "yuanqiaoyun",
			baseURL:  "",
			expected: "https://yuanqiaoyun.com/api/pricing",
		},
		{
			provider: "portdan",
			baseURL:  "",
			expected: "https://portdan.com/api/v1/channels/available",
		},
		{
			provider: "portdan",
			baseURL:  "https://custom-portdan.com/v1",
			expected: "https://custom-portdan.com/api/v1/channels/available",
		},
		{
			provider: "custom_vendor",
			baseURL:  "http://vendor.example.com/api/v1",
			expected: "http://vendor.example.com/api/pricing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			got := PricingURLForProvider(tt.provider, tt.baseURL)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestProviderPriceManagerFetchAndCache(t *testing.T) {
	var statusCode int32 = http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pricing" {
			http.NotFound(w, r)
			return
		}

		code := int(atomic.LoadInt32(&statusCode))
		if code != http.StatusOK {
			http.Error(w, "server error", code)
			return
		}

		payload := map[string]any{
			"success": true,
			"data": []map[string]any{
				{
					"model_name":       "claude-3-7-sonnet-20250219",
					"quota_type":       0,
					"model_ratio":      15.0,
					"completion_ratio": 5.0,
					"available":        true,
				},
				{
					"model_name":       "gpt-4o",
					"quota_type":       1,
					"model_price":      0.005,
					"completion_ratio": 1.0,
					"available":        true,
				},
				{
					"model_name":       "default-ratio-model",
					"quota_type":       0,
					"model_ratio":      5.0,
					"completion_ratio": 0.0,
					"available":        true,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer server.Close()

	ppm := NewProviderPriceManager(nil, nil, server.Client())
	ctx := context.Background()

	// Edge cases
	in, out, found := ppm.ResolveNodePrice(ctx, nil, "model")
	assert.False(t, found)
	assert.Equal(t, 0.0, in)
	assert.Equal(t, 0.0, out)

	node := &health.ExecutionNode{
		NodeID:   "test-node-1",
		Provider: "mock_provider",
	}

	in, out, found = ppm.ResolveNodePrice(ctx, node, "")
	assert.False(t, found)

	// Fetch directly from server URL
	fetched, err := ppm.fetchProviderPrices(ctx, server.URL+"/api/pricing")
	require.NoError(t, err)
	assert.Len(t, fetched, 3)

	// Verify model ratio calculation: 15.0 * 0.002 = 0.03
	claudePrice, found := ppm.lookupModelInPrices(fetched, "claude-3-7-sonnet-20250219")
	assert.True(t, found)
	assert.InDelta(t, 0.03, claudePrice.InputPricePer1k, 0.0001)
	assert.InDelta(t, 0.15, claudePrice.OutputPricePer1k, 0.0001)

	// Verify prefix models/ lookup
	prefixPrice, prefixFound := ppm.lookupModelInPrices(fetched, "models/claude-3-7-sonnet-20250219")
	assert.True(t, prefixFound)
	assert.Equal(t, claudePrice.InputPricePer1k, prefixPrice.InputPricePer1k)

	// Verify alias lookup: "claude-3-7-sonnet" -> "claude-3-7-sonnet-20250219"
	aliasPrice, aliasFound := ppm.lookupModelInPrices(fetched, "claude-3-7-sonnet")
	assert.True(t, aliasFound)
	assert.Equal(t, claudePrice.InputPricePer1k, aliasPrice.InputPricePer1k)

	// Model price type quota_type=1 (fixed model_price)
	gptPrice, foundGPT := ppm.lookupModelInPrices(fetched, "gpt-4o")
	assert.True(t, foundGPT)
	assert.Equal(t, 0.005, gptPrice.InputPricePer1k)
	assert.Equal(t, 0.005, gptPrice.OutputPricePer1k)

	// Default completion ratio (compRatio <= 0 -> 1.0)
	defaultRatioPrice, foundDefRatio := ppm.lookupModelInPrices(fetched, "default-ratio-model")
	assert.True(t, foundDefRatio)
	assert.InDelta(t, 0.01, defaultRatioPrice.InputPricePer1k, 0.0001)
	assert.InDelta(t, 0.01, defaultRatioPrice.OutputPricePer1k, 0.0001)

	// Fetch error on non-200 HTTP code
	atomic.StoreInt32(&statusCode, http.StatusBadGateway)
	_, errFetchErr := ppm.fetchProviderPrices(ctx, server.URL+"/api/pricing")
	assert.Error(t, errFetchErr)
	atomic.StoreInt32(&statusCode, http.StatusOK)

	// Node ID cached lookup
	ppm.SetProviderPrices("test-node-id-direct", fetched)
	inNodeID, outNodeID, foundNodeID := ppm.ResolveNodePrice(ctx, &health.ExecutionNode{NodeID: "test-node-id-direct"}, "gpt-4o")
	assert.True(t, foundNodeID)
	assert.Equal(t, 0.005, inNodeID)
	assert.Equal(t, 0.005, outNodeID)

	// Test ResolveNodePrice in non-test mode with custom endpoint
	t.Setenv("ASH_YUNWU_TEST_MODE", "false")
	KnownProviderPricingURLs["mock_provider"] = server.URL + "/api/pricing"
	defer delete(KnownProviderPricingURLs, "mock_provider")

	in, out, foundLive := ppm.ResolveNodePrice(ctx, node, "claude-3-7-sonnet")
	assert.True(t, foundLive)
	assert.InDelta(t, 0.03, in, 0.0001)
	assert.InDelta(t, 0.15, out, 0.0001)

	// Second request is served from 30-minute cache
	inLive2, outLive2, foundLive2 := ppm.ResolveNodePrice(ctx, node, "claude-3-7-sonnet")
	assert.True(t, foundLive2)
	assert.Equal(t, in, inLive2)
	assert.Equal(t, out, outLive2)

	// Unlisted model returns false from cached pricing table
	_, _, foundUnlisted := ppm.ResolveNodePrice(ctx, node, "completely-unlisted")
	assert.False(t, foundUnlisted)

	// Node without pricing URL in non-test mode
	noURLNode := &health.ExecutionNode{NodeID: "no-url-node", Provider: "unknown_prov"}
	_, _, foundNoURL := ppm.ResolveNodePrice(ctx, noURLNode, "claude-3-7-sonnet")
	assert.False(t, foundNoURL)

	// Test explicit manual SetNoPriceCooldown / IsNoPriceCooldown / ClearNoPriceCooldown
	ppm.SetNoPriceCooldown(node.NodeID, "claude-3-7-sonnet", 10*time.Minute)
	assert.True(t, ppm.IsNoPriceCooldown(node.NodeID, "claude-3-7-sonnet"))
	_, _, foundManualCooldown := ppm.ResolveNodePrice(ctx, node, "claude-3-7-sonnet")
	assert.False(t, foundManualCooldown)

	ppm.ClearNoPriceCooldown(node.NodeID, "claude-3-7-sonnet")
	assert.False(t, ppm.IsNoPriceCooldown(node.NodeID, "claude-3-7-sonnet"))
	inRecovered, outRecovered, foundRecovered := ppm.ResolveNodePrice(ctx, node, "claude-3-7-sonnet")
	assert.True(t, foundRecovered)
	assert.InDelta(t, 0.03, inRecovered, 0.0001)
	assert.InDelta(t, 0.15, outRecovered, 0.0001)

	// Network errors (500) do NOT set negative cooldown
	ppmFresh := NewProviderPriceManager(nil, nil, nil)
	atomic.StoreInt32(&statusCode, http.StatusInternalServerError)
	_, _, foundFailed := ppmFresh.ResolveNodePrice(ctx, node, "claude-3-7-sonnet")
	assert.False(t, foundFailed)
	assert.False(t, ppmFresh.IsNoPriceCooldown(node.NodeID, "claude-3-7-sonnet"))

	// Cache injection & ResolveNodePrice
	ppm.SetProviderPrices("test-node-1", fetched)
	in, out, foundNode := ppm.ResolveNodePrice(ctx, node, "claude-3-7-sonnet")
	assert.True(t, foundNode)
	assert.InDelta(t, 0.03, in, 0.0001)
	assert.InDelta(t, 0.15, out, 0.0001)

	// Check missing model on node
	_, _, foundNodeMissing := ppm.ResolveNodePrice(ctx, node, "unpriced-model")
	assert.False(t, foundNodeMissing)
}

func TestResolvePriceFallbackAndHelpers(t *testing.T) {
	store := setupTestYunwuStore(t)
	rt := &mockYunwuRuntime{}
	yh := NewYunwuHypervisor(store, rt)
	ctx := context.Background()

	// Direct resolvePrice on engine
	in, out, fallback := yh.resolvePrice(ctx, "non-existent-node", "claude-3-7-sonnet")
	assert.True(t, fallback)
	assert.Greater(t, in, 0.0)
	assert.Greater(t, out, 0.0)

	// Set custom price resolver
	mockPR := &mockPriceResolver{in: 0.01, out: 0.02, found: true}
	yh.SetPriceResolver(mockPR)
	inPR, outPR, hasPR := yh.resolveNodePrice(ctx, &health.ExecutionNode{NodeID: "test"}, "claude")
	assert.True(t, hasPR)
	assert.Equal(t, 0.01, inPR)
	assert.Equal(t, 0.02, outPR)

	// Nil node
	_, _, hasNil := yh.resolveNodePrice(ctx, nil, "claude")
	assert.False(t, hasNil)

	// Reserve and ReleaseReservation helpers
	sw := NewStatsWorker(store, nil, nil, nil)
	require.NotNil(t, sw)

	_, _ = sw.Reserve(ctx, "test-node", "req-1", 500, time.Minute)
	_, _ = sw.ReleaseReservation(ctx, "test-node", "req-1")

	// Helper functions
	assert.Equal(t, 10.0, maxFloat64(5.0, 10.0))
	assert.Equal(t, 8.0, maxFloat64(8.0, 3.0))
	assert.Contains(t, yunwuResourceNodeKey("res-1"), "ash:yunwu:resource_node:")
	h := http.Header{}
	h.Set("X-Request-ID", "req-test-123")
	assert.Equal(t, "req-test-123", yunwuRequestID(h))
	assert.Empty(t, yunwuRequestID(http.Header{}))
}

func TestWritersAndControlHelpers(t *testing.T) {
	// 1. Commit and capture writers
	rec := httptest.NewRecorder()
	cw := &commitTrackingWriter{w: rec}
	assert.False(t, cw.Committed())
	n, err := cw.Write([]byte("hello "))
	require.NoError(t, err)
	assert.Equal(t, 6, n)
	assert.True(t, cw.Committed())
	cw.Flush()
	cw.WriteHeader(http.StatusOK)
	assert.NotNil(t, cw.Header())

	cwBuf := &commitTrackingWriter{w: &bytes.Buffer{}}
	cwBuf.WriteHeader(http.StatusOK)
	assert.NotNil(t, cwBuf.Header())

	rcw := &responseCaptureWriter{
		commitTrackingWriter: cw,
		buf:                  bytes.Buffer{},
	}
	n, err = rcw.Write([]byte("world"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "world", rcw.buf.String())

	// 2. Control and media helpers
	assert.True(t, isGeminiControlResource("/v1beta/auth_tokens"))
	assert.True(t, isGeminiControlResource("/batches/list"))
	assert.False(t, isGeminiControlResource("/v1beta/models/gemini-2.5-flash:generateContent"))

	val, ok := numericMicroUSD(12.5)
	assert.True(t, ok)
	assert.Equal(t, int64(13), val)
	val, ok = numericMicroUSD(10)
	assert.True(t, ok)
	assert.Equal(t, int64(10), val)
	val, ok = numericMicroUSD(int64(25))
	assert.True(t, ok)
	assert.Equal(t, int64(25), val)
	val, ok = numericMicroUSD(json.Number("42.1"))
	assert.True(t, ok)
	assert.Equal(t, int64(43), val)
	val, ok = numericMicroUSD("100.5")
	assert.True(t, ok)
	assert.Equal(t, int64(101), val)
	_, ok = numericMicroUSD(struct{}{})
	assert.False(t, ok)

	// Keys
	assert.NotEmpty(t, yunwuOperationNodeKey("op-1"))
	assert.NotEmpty(t, yunwuInteractionNodeKey("int-1"))

	// 3. Engine balance and price manager nil check
	store := setupTestYunwuStore(t)
	rt := &mockYunwuRuntime{}
	yh := NewYunwuHypervisor(store, rt)
	assert.NotNil(t, yh.PriceManager())
	yh.SetPriceResolver(nil)
	assert.Nil(t, yh.PriceManager())

	assert.Equal(t, 10, yh.BalanceReservePercent())
	ctx := context.Background()
	_, _ = yh.BalanceSnapshot(ctx, "test-node")
}

func TestBillingAndStatsWorkerHelpers(t *testing.T) {
	// normalizeQuotaToUSD
	assert.Equal(t, 0.0, normalizeQuotaToUSD(-5))
	assert.Equal(t, 0.0, normalizeQuotaToUSD(0))
	assert.Equal(t, 50.0, normalizeQuotaToUSD(50))
	assert.Equal(t, 1.0, normalizeQuotaToUSD(500000))

	// normalizeBillingBaseURL
	origin, prefix, err := normalizeBillingBaseURL("")
	require.NoError(t, err)
	assert.Equal(t, "https://api.openlux.ai", origin)
	assert.Empty(t, prefix)

	origin, prefix, err = normalizeBillingBaseURL("https://api-top.com/v1")
	require.NoError(t, err)
	assert.Equal(t, "https://api-top.com", origin)
	assert.Empty(t, prefix)

	origin, prefix, err = normalizeBillingBaseURL("https://example.com/proxy/v1")
	require.NoError(t, err)
	assert.Equal(t, "https://example.com", origin)
	assert.Equal(t, "/proxy", prefix)

	_, _, err = normalizeBillingBaseURL("::invalid-url")
	assert.Error(t, err)

	// max64
	assert.Equal(t, int64(50), max64(50, 20))
	assert.Equal(t, int64(100), max64(10, 100))
}

func TestResourceNameAndContextHelpers(t *testing.T) {
	t.Setenv("ASH_YUNWU_TEST_MODE", "true")
	ctx := context.Background()

	// Empty context
	assert.Empty(t, statefulResourceNameFromRequest(ctx))
	assert.Empty(t, statefulResourceNameFromResponse(ctx, []byte(`{}`)))
	assert.Empty(t, operationNameFromRequest(ctx))
	assert.Empty(t, interactionIDFromRequest(ctx))

	// Request context
	reqCtx := runtime.WithUpstreamRequest(ctx, http.MethodPost, "/v1beta/batches/batch-123:cancel", "application/json")
	assert.Equal(t, "batches/batch-123", statefulResourceNameFromRequest(reqCtx))

	reqCtx2 := runtime.WithUpstreamRequest(ctx, http.MethodGet, "/v1beta/cachedContents/cache-xyz?query=1", "application/json")
	assert.Equal(t, "cachedContents/cache-xyz", statefulResourceNameFromRequest(reqCtx2))

	reqCtx3 := runtime.WithUpstreamRequest(ctx, http.MethodGet, "/v1beta/unrelated/path", "application/json")
	assert.Empty(t, statefulResourceNameFromRequest(reqCtx3))

	// Response context
	respCtx := runtime.WithUpstreamRequest(ctx, http.MethodPost, "/v1beta/models/gemini-2.5-flash", "application/json")
	assert.Equal(t, "batches/batch-999", statefulResourceNameFromResponse(respCtx, []byte(`{"name":"batches/batch-999"}`)))
	assert.Empty(t, statefulResourceNameFromResponse(respCtx, []byte(`{"name":"invalid"}`)))
	assert.Empty(t, statefulResourceNameFromResponse(respCtx, []byte(`not-json`)))

	// Interaction ID
	intCtx := runtime.WithUpstreamRequest(ctx, http.MethodGet, "/v1beta/interactions/interaction-abc/status", "application/json")
	assert.Equal(t, "interaction-abc", interactionIDFromRequest(intCtx))

	// Interaction ID from response
	intRespCtx := runtime.WithUpstreamRequest(ctx, http.MethodPost, "/v1beta/interactions", "application/json")
	assert.Equal(t, "interaction-resp-1", interactionIDFromResponse(intRespCtx, []byte(`{"id":"interaction-resp-1"}`)))
	assert.Empty(t, interactionIDFromResponse(intRespCtx, []byte(`not-json`)))
	assert.Empty(t, interactionIDFromResponse(ctx, []byte(`{"id":"interaction-resp-1"}`)))

	// Ephemeral token
	authCtx := runtime.WithUpstreamRequest(ctx, http.MethodPost, "/v1beta/auth_tokens", "application/json")
	assert.Equal(t, "auth_tokens/tok-ephem-1", ephemeralTokenFromResponse(authCtx, []byte(`{"name":"auth_tokens/tok-ephem-1"}`)))
	assert.Empty(t, ephemeralTokenFromResponse(authCtx, []byte(`{"name":"invalid"}`)))
	assert.Empty(t, ephemeralTokenFromResponse(ctx, []byte(`{"name":"auth_tokens/tok-ephem-1"}`)))
	assert.NotEmpty(t, yunwuUpstreamTokenNodeKey("tok-1"))

	// liveMaxMinutes with nil DB
	store := setupTestYunwuStore(t)
	rt := &mockYunwuLiveRuntime{}
	yh := NewYunwuHypervisor(store, rt)
	assert.Equal(t, int64(15), yh.liveMaxMinutes(ctx, "node-1"))
	assert.Equal(t, int64(15), yh.liveMaxMinutes(ctx, "not-a-number"))

	// resolveMediaPrice
	price, fb := yh.resolveMediaPrice(ctx, "node-1", "gemini-2.5-flash", mediaRequest{Kind: "media_control", Units: 1})
	assert.Equal(t, int64(0), price)
	assert.False(t, fb)

	price, fb = yh.resolveMediaPrice(ctx, "node-1", "gemini-2.5-flash", mediaRequest{Kind: "images", Units: 2})
	assert.Greater(t, price, int64(0))
	assert.True(t, fb)

	price, fb = yh.resolveMediaPrice(ctx, "not-a-number", "gemini-2.5-flash", mediaRequest{Kind: "images", Units: 2})
	assert.Greater(t, price, int64(0))
	assert.True(t, fb)

	// estimateRequestMicroUSD
	est1 := estimateRequestMicroUSD([]byte(`{"max_tokens":100}`), 0.003, 0.015)
	assert.Greater(t, est1, int64(0))
	est2 := estimateRequestMicroUSD([]byte(`{"max_output_tokens":200}`), 0.003, 0.015)
	assert.Greater(t, est2, int64(0))

	// modelNameFromRequest
	noModelCtx := runtime.WithUpstreamRequest(ctx, http.MethodGet, "/v1beta/unrelated", "application/json")
	assert.Empty(t, modelNameFromRequest(noModelCtx))
	unescapedCtx := runtime.WithUpstreamRequest(ctx, http.MethodPost, "/v1beta/models/tunedModels%2Fcustom-1:generateContent", "application/json")
	assert.Equal(t, "tunedModels/custom-1", modelNameFromRequest(unescapedCtx))

	// operationNameFromResponse
	assert.Equal(t, "operations/op-123", operationNameFromResponse([]byte(`{"name":"operations/op-123"}`)))
	assert.Empty(t, operationNameFromResponse([]byte(`{"name":"other/123"}`)))
	assert.Empty(t, operationNameFromResponse([]byte(`not-json`)))

	// SyncNodeBalance edge cases
	_, errSync := yh.SyncNodeBalance(ctx, "not-a-number", "", "")
	assert.Error(t, errSync)
	_, errSync = yh.SyncNodeBalance(ctx, "999", "", "")
	assert.Error(t, errSync)

	// Ephemeral token exchange and consumption
	_, err := yh.ExchangeEphemeralToken(ctx, "", "", 0, 0)
	assert.Error(t, err)
	_, err = yh.ExchangeEphemeralToken(ctx, "user-1", "tok-upstream-1", 0, 0)
	assert.Error(t, err)

	_ = yh.store.Redis().Set(ctx, yunwuUpstreamTokenNodeKey("tok-upstream-1"), "node-1", 0).Err()
	gwToken, err := yh.ExchangeEphemeralToken(ctx, "user-1", "tok-upstream-1", time.Hour, 2)
	require.NoError(t, err)
	assert.NotEmpty(t, gwToken)

	owner, upToken, nodeID, err := yh.consumeEphemeralToken(ctx, gwToken)
	require.NoError(t, err)
	assert.Equal(t, "user-1", owner)
	assert.Equal(t, "tok-upstream-1", upToken)
	assert.Equal(t, "node-1", nodeID)

	_, _, _, err = yh.consumeEphemeralToken(ctx, "")
	assert.Error(t, err)
	_, _, _, err = yh.consumeEphemeralToken(ctx, "ash_ephemeral_invalid")
	assert.Error(t, err)

	// HandleEphemeralLive test
	nm := health.NewNodeManager(store)
	_ = nm.SaveNode(ctx, &health.ExecutionNode{NodeID: "node-1", Provider: "yunwu", State: "healthy"})
	_ = store.Redis().SAdd(ctx, "ash:yunwu:nodes", "node-1").Err()
	setTestBalance(t, store, "node-1", 50_000_000)

	_ = yh.qm.SaveUserAccess(ctx, &quota.UserAccess{OwnerID: "user-1", MonthlyLimitMicroUSD: 50_000_000, Priority: 1, IsActive: true}, "yunwu")

	_ = store.Redis().Set(ctx, yunwuUpstreamTokenNodeKey("tok-upstream-live"), "node-1", 0).Err()
	gwTokenLive, err := yh.ExchangeEphemeralToken(ctx, "user-1", "tok-upstream-live", time.Hour, 1)
	require.NoError(t, err)

	err = yh.HandleEphemeralLive(ctx, gwTokenLive, "conv-live-e2e", "/ws/BidiGenerateMusic", nil, nil)
	require.NoError(t, err)

	err = yh.HandleEphemeralLive(ctx, "ash_ephemeral_invalid", "conv-live-e2e", "/ws/BidiGenerateMusic", nil, nil)
	assert.Error(t, err)
}

func TestStatsWorkerSyncEndpoints(t *testing.T) {
	var disableGrants, disableSub bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/dashboard/billing/credit_grants":
			if disableGrants {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"total_granted":   100.0,
				"total_used":      20.0,
				"total_available": 80.0,
			})
		case "/dashboard/billing/subscription":
			if disableSub {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"hard_limit_usd":  50.0,
				"total_available": 40.0,
			})
		case "/api/user/self":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"quota":      500000.0,
					"used_quota": 100000.0,
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	store := setupTestYunwuStore(t)
	sw := NewStatsWorker(store)
	sw.httpClient = server.Client()
	ctx := context.Background()

	// 1. syncCreditGrants
	cg, err := sw.syncCreditGrants(ctx, server.URL, "", "test-key")
	require.NoError(t, err)
	assert.Equal(t, 80.0, cg.TotalAvailable)

	// 2. syncSubscriptionUsage
	sub, err := sw.syncSubscriptionUsage(ctx, server.URL, "", "test-key")
	require.NoError(t, err)
	assert.Equal(t, 40.0, sub.TotalAvailable)

	// 3. syncNodeStatsFallback
	node := &health.ExecutionNode{NodeID: "sync-node-1"}
	fb, err := sw.syncNodeStatsFallback(ctx, node, "test-key", server.URL)
	require.NoError(t, err)
	assert.Equal(t, 1.0, fb.TotalAvailable)

	// 4. SyncNodeStats Strategy 1 (credit_grants)
	stats, err := sw.SyncNodeStats(ctx, node, "test-key", server.URL)
	require.NoError(t, err)
	assert.NotNil(t, stats)

	// Strategy 2 (subscription)
	disableGrants = true
	statsSub, err := sw.SyncNodeStats(ctx, node, "test-key", server.URL)
	require.NoError(t, err)
	assert.Equal(t, 40.0, statsSub.TotalAvailable)

	// Strategy 3 (user_self fallback)
	disableSub = true
	statsUser, err := sw.SyncNodeStats(ctx, node, "test-key", server.URL)
	require.NoError(t, err)
	assert.Equal(t, 1.0, statsUser.TotalAvailable)

	// Strategy failure on invalid origin
	_, err = sw.SyncNodeStats(ctx, node, "test-key", ":::bad-url")
	assert.Error(t, err)
	assert.NotNil(t, stats)

	// 5. PricingURLForProvider edge cases
	assert.Empty(t, PricingURLForProvider("unknown-vendor", ""))
	assert.Empty(t, PricingURLForProvider("unknown-vendor", ":::not-a-url"))

	// 6. fetchProviderPrices empty URL
	ppm := NewProviderPriceManager(nil, nil, nil)
	_, err = ppm.fetchProviderPrices(ctx, "")
	assert.Error(t, err)

	// 7. SyncNodeBalance on YunwuHypervisor
	rt := &mockYunwuRuntime{}
	yh := NewYunwuHypervisor(store, rt)
	yh.sw.httpClient = server.Client()
	syncResp, err := yh.SyncNodeBalance(ctx, "sync-node-1", "test-key", server.URL)
	require.NoError(t, err)
	assert.NotNil(t, syncResp)

	// 8. getAllYunwuNodeCredits with stale snapshot
	staleTime := time.Now().Add(-1 * time.Hour)
	staleSnap := BalanceSnapshot{
		Status:         "stale",
		SynchronizedAt: staleTime,
	}
	staleBytes, _ := json.Marshal(staleSnap)
	_ = store.Redis().Set(ctx, balanceKey("stale-node"), staleBytes, 0).Err()
	credits := yh.getAllYunwuNodeCredits(ctx, []health.ExecutionNode{{NodeID: "stale-node"}})
	assert.Empty(t, credits)
}

type mockPriceResolver struct {
	in, out float64
	found   bool
}

func (m *mockPriceResolver) ResolveNodePrice(ctx context.Context, node *health.ExecutionNode, model string) (float64, float64, bool) {
	return m.in, m.out, m.found
}

func TestProviderPriceManager_Sub2APIChannelsAvailableFormat(t *testing.T) {
	var capturedAuthHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuthHeader = r.Header.Get("Authorization")
		if r.URL.Path != "/api/v1/channels/available" {
			http.NotFound(w, r)
			return
		}

		// Return sub2api /channels/available JSON response structure
		payload := []map[string]any{
			{
				"id":   1,
				"name": "Standard Group",
				"platforms": []map[string]any{
					{
						"id":   1,
						"name": "Anthropic",
						"supported_models": []map[string]any{
							{
								"name": "claude-3-7-sonnet-20250219",
								"pricing": map[string]any{
									"input_price":  0.003,
									"output_price": 0.015,
								},
							},
							{
								"name": "claude-3-5-haiku-20241022",
								"pricing": map[string]any{
									"input_price":  0.001,
									"output_price": 0.005,
								},
							},
							{
								"name": "free-tier-model",
								"pricing": map[string]any{
									"input_price":  0.0,
									"output_price": 0.0,
								},
							},
							{
								"name": "claude-3-opus-per-token",
								"pricing": map[string]any{
									"input_price":  0.000015,
									"output_price": 0.000075,
								},
							},
						},
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer server.Close()

	ppm := NewProviderPriceManager(nil, nil, server.Client())
	ctx := context.Background()

	// 1. Fetch with token
	prices, err := ppm.fetchProviderPrices(ctx, server.URL+"/api/v1/channels/available", "secret-user-token")
	require.NoError(t, err)
	assert.Equal(t, "Bearer secret-user-token", capturedAuthHeader)
	assert.Len(t, prices, 4)

	// 2. Check parsed prices
	sonnet, found := ppm.lookupModelInPrices(prices, "claude-3-7-sonnet-20250219")
	assert.True(t, found)
	assert.Equal(t, 0.003, sonnet.InputPricePer1k)
	assert.Equal(t, 0.015, sonnet.OutputPricePer1k)
	assert.True(t, sonnet.Available)

	haiku, found := ppm.lookupModelInPrices(prices, "claude-3-5-haiku-20241022")
	assert.True(t, found)
	assert.Equal(t, 0.001, haiku.InputPricePer1k)
	assert.Equal(t, 0.005, haiku.OutputPricePer1k)

	// Per-token normalization (0.000015 -> 0.015)
	opus, found := ppm.lookupModelInPrices(prices, "claude-3-opus-per-token")
	assert.True(t, found)
	assert.InDelta(t, 0.015, opus.InputPricePer1k, 0.000001)
	assert.InDelta(t, 0.075, opus.OutputPricePer1k, 0.000001)

	// Free tier model (pricing 0,0)
	freeMod, found := ppm.lookupModelInPrices(prices, "free-tier-model")
	assert.True(t, freeMod.Available)
	assert.True(t, found)
	assert.Equal(t, 0.0, freeMod.InputPricePer1k)
}

func TestProviderPriceManager_PortdanEndpointSoleSource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/channels/available" {
			http.NotFound(w, r)
			return
		}
		payload := []map[string]any{
			{
				"id": 1,
				"platforms": []map[string]any{
					{
						"supported_models": []map[string]any{
							{
								"name": "claude-3-7-sonnet-20250219",
								"pricing": map[string]any{
									"input_price":  0.003,
									"output_price": 0.015,
								},
							},
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer server.Close()

	ctx := context.Background()
	ppm := NewProviderPriceManager(nil, nil, server.Client())

	node := &health.ExecutionNode{
		NodeID:   "portdan-node-1",
		Provider: "portdan",
	}

	// 1. In test mode, Portdan does NOT get default benchmark fallback ($0.003/$0.015)
	t.Setenv("ASH_YUNWU_TEST_MODE", "true")
	in, out, found := ppm.ResolveNodePrice(ctx, node, "claude-3-7-sonnet")
	assert.False(t, found, "Portdan must not receive generic benchmark price fallback")
	assert.Equal(t, 0.0, in)
	assert.Equal(t, 0.0, out)

	// 2. In live mode with Portdan pricing URL overridden to test server
	t.Setenv("ASH_YUNWU_TEST_MODE", "false")
	KnownProviderPricingURLs[ProviderPortdan] = server.URL + "/api/v1/channels/available"
	defer func() {
		KnownProviderPricingURLs[ProviderPortdan] = "https://portdan.com/api/v1/channels/available"
	}()

	in, out, found = ppm.ResolveNodePrice(ctx, node, "claude-3-7-sonnet")
	assert.True(t, found, "Portdan endpoint /api/v1/channels/available must resolve pricing")
	assert.Equal(t, 0.003, in)
	assert.Equal(t, 0.015, out)

	// 3. Unlisted model on Portdan returns false (sole source)
	_, _, unlistedFound := ppm.ResolveNodePrice(ctx, node, "unsupported-model")
	assert.False(t, unlistedFound, "Models not in Portdan /channels/available must not resolve")
}

func TestProviderPriceManager_CacheIsolationBetweenProviders(t *testing.T) {
	t.Setenv("ASH_YUNWU_TEST_MODE", "false")
	ctx := context.Background()

	// Provider A server: gpt-4o at $0.005 / $0.015
	serverA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{
					"model_name":       "gpt-4o",
					"model_ratio":      2.5,
					"completion_ratio": 3.0,
				},
			},
		})
	}))
	defer serverA.Close()

	// Provider B server: gpt-4o at $0.001 / $0.003
	serverB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{
					"model_name":       "gpt-4o",
					"model_ratio":      0.5,
					"completion_ratio": 3.0,
				},
			},
		})
	}))
	defer serverB.Close()

	oldAPITop := KnownProviderPricingURLs[ProviderAPITop]
	oldYuanqiaoyun := KnownProviderPricingURLs[ProviderYuanqiaoyun]
	KnownProviderPricingURLs[ProviderAPITop] = serverA.URL + "/api/pricing"
	KnownProviderPricingURLs[ProviderYuanqiaoyun] = serverB.URL + "/api/pricing"
	defer func() {
		KnownProviderPricingURLs[ProviderAPITop] = oldAPITop
		KnownProviderPricingURLs[ProviderYuanqiaoyun] = oldYuanqiaoyun
	}()

	nodeA := &health.ExecutionNode{
		NodeID:   "node-server-a",
		Provider: "api-top",
	}
	nodeB := &health.ExecutionNode{
		NodeID:   "node-server-b",
		Provider: "yuanqiaoyun",
	}

	ppm := NewProviderPriceManager(nil, nil, http.DefaultClient)

	// 1. Resolve price on Node A -> fetches from serverA
	inA, outA, foundA := ppm.ResolveNodePrice(ctx, nodeA, "gpt-4o")
	require.True(t, foundA)
	assert.InDelta(t, 0.005, inA, 0.0001)
	assert.InDelta(t, 0.015, outA, 0.0001)

	// 2. Resolve price on Node B -> fetches from serverB (NOT polluted by Node A's cache!)
	inB, outB, foundB := ppm.ResolveNodePrice(ctx, nodeB, "gpt-4o")
	require.True(t, foundB)
	assert.InDelta(t, 0.001, inB, 0.0001)
	assert.InDelta(t, 0.003, outB, 0.0001)

	// 3. Query Node A again -> served from its own isolated cache
	inA2, outA2, foundA2 := ppm.ResolveNodePrice(ctx, nodeA, "gpt-4o")
	require.True(t, foundA2)
	assert.InDelta(t, 0.005, inA2, 0.0001)
	assert.InDelta(t, 0.015, outA2, 0.0001)

	// 4. Query Node B again -> served from its own isolated cache
	inB2, outB2, foundB2 := ppm.ResolveNodePrice(ctx, nodeB, "gpt-4o")
	require.True(t, foundB2)
	assert.InDelta(t, 0.001, inB2, 0.0001)
	assert.InDelta(t, 0.003, outB2, 0.0001)
}




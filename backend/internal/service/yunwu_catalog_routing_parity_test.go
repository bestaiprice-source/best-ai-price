package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/yunwu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The catalog (service) and the routing node (hypervisor) must price the SAME
// models from the SAME table. When they disagreed, ~43% of the advertised
// catalog answered HTTP 400 "model pricing is not available" on the first call
// (`xai/grok-4`, `google/gemini-2-5-flash`, …): the node lookup carried its own,
// smaller normalization. This test drives both sides against one served table
// and fails if they can drift apart again.
func TestYunwuCatalogAndNodePricingAgree(t *testing.T) {
	// BestAIPrice providers.json shape (the format the portdan scope uses), where
	// both implementations filter strictly on the portdan records.
	table := []map[string]any{
		{"provider_name": "Portdan", "provider_domain": "portdan.com", "model_name": "gemini-2.5-flash",
			"canonical_model_id": "google/gemini-2.5-flash", "input_price_usd_per_1m": 2.0, "output_price_usd_per_1m": 8.0},
		{"provider_name": "Portdan", "provider_domain": "portdan.com", "model_name": "grok-4",
			"canonical_model_id": "xai/grok-4", "input_price_usd_per_1m": 3.0, "output_price_usd_per_1m": 15.0},
		{"provider_name": "Portdan", "provider_domain": "portdan.com", "model_name": "claude-3-7-sonnet-20250219",
			"canonical_model_id": "anthropic/claude-3-7-sonnet", "input_price_usd_per_1m": 3.0, "output_price_usd_per_1m": 15.0},
		// Another vendor's records must never leak into the portdan scope.
		{"provider_name": "Other", "provider_domain": "elsewhere.example", "model_name": "gemini-2.5-flash",
			"input_price_usd_per_1m": 0.0001, "output_price_usd_per_1m": 0.0001},
	}
	body, err := json.Marshal(table)
	require.NoError(t, err)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	t.Setenv("PORTDAN_PRICING_URL", srv.URL)

	// Catalog / billing side.
	pricingSvc := NewYunwuPricingService()
	pricingSvc.SetProviderPricingURL("portdan", srv.URL)

	// Routing side — the hypervisor's per-node price manager, same endpoint.
	ppm := yunwu.NewProviderPriceManager(nil, nil, srv.Client())
	node := &health.ExecutionNode{NodeID: "portdan-node", Provider: "portdan"}
	ctx := context.Background()

	cases := []struct {
		model     string
		wantInput float64 // $/1k tokens from the portdan record
	}{
		{"google/gemini-2.5-flash", 0.002},
		{"google/gemini-2-5-flash", 0.002}, // catalog's dash spelling
		{"xai/grok-4", 0.003},              // vendor prefix
		{"anthropic/claude-3-7-sonnet", 0.003},
		{"claude-3-7-sonnet-20250219", 0.003}, // dated upstream id
	}

	for _, tc := range cases {
		svcPricing, ok := pricingSvc.GetModelPricingForProvider(ctx, "portdan", tc.model)
		require.Truef(t, ok, "catalog: %s is not priced (would be advertised without a price)", tc.model)
		assert.InDeltaf(t, tc.wantInput, svcPricing.InputPricePerToken*1000, 1e-9, "catalog price for %s", tc.model)

		in, out, matched, nodeOK := ppm.ResolveNodePriceMatch(ctx, node, tc.model)
		require.Truef(t, nodeOK, "routing: %s is priced in the catalog but NOT on the node (HTTP 400)", tc.model)
		assert.InDeltaf(t, svcPricing.InputPricePerToken*1000, in, 1e-9, "input price parity for %s", tc.model)
		assert.InDeltaf(t, svcPricing.OutputPricePerToken*1000, out, 1e-9, "output price parity for %s", tc.model)
		assert.NotEmptyf(t, matched, "the provider-side model name for %s must be known", tc.model)
	}

	// A model priced by nobody is unpriced on BOTH sides (fail closed) — the
	// catalog marks it unavailable and the engine refuses it.
	_, ok := pricingSvc.GetModelPricingForProvider(ctx, "portdan", "grok-9-not-listed")
	assert.False(t, ok)
	_, _, _, nodeOK := ppm.ResolveNodePriceMatch(ctx, node, "grok-9-not-listed")
	assert.False(t, nodeOK)
}

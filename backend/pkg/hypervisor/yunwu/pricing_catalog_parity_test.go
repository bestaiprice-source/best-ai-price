package yunwu

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/quota"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// capturingYunwuRuntime records the payload that would go upstream.
type capturingYunwuRuntime struct {
	lastNodeID string
	lastModel  string
}

func (m *capturingYunwuRuntime) Execute(_ context.Context, nodeID string, reqPayload []byte, _ io.Writer) (*runtime.ResponseMetadata, error) {
	m.lastNodeID = nodeID
	var body map[string]any
	if err := json.Unmarshal(reqPayload, &body); err == nil {
		if s, ok := body["model"].(string); ok {
			m.lastModel = s
		}
	}
	return &runtime.ResponseMetadata{StatusCode: 200, InputTokens: 10, OutputTokens: 20}, nil
}

// A model whose catalog price is only reachable through normalization (vendor
// prefix, dot/dash spelling, alias) must be BOTH priceable on the node and
// dispatched under the name the provider actually lists — otherwise the catalog
// advertises a model that fails with 400 (unpriced) or model_not_found.
func TestResolveNodePriceMatchReturnsProviderModelName(t *testing.T) {
	ppm := NewProviderPriceManager(nil, nil, nil)
	ppm.SetProviderPrices("node-x", map[string]ProviderModelPrice{
		"gemini-2.5-flash": {InputPricePer1k: 0.001, OutputPricePer1k: 0.004, Available: true},
		"grok-4":           {InputPricePer1k: 0.002, OutputPricePer1k: 0.010, Available: true},
		"claude-sonnet-5":  {InputPricePer1k: 0.003, OutputPricePer1k: 0.015, Available: true},
	})
	node := &health.ExecutionNode{NodeID: "node-x", Provider: "openlux"}

	cases := []struct {
		requested string
		wantModel string
	}{
		// The two models from the live bug report.
		{"google/gemini-2-5-flash", "gemini-2.5-flash"},
		{"xai/grok-4", "grok-4"},
		{"google/gemini-2.0-flash", "gemini-2.5-flash"}, // alias → provider name
		{"claude-sonnet-5", "claude-sonnet-5"},          // exact match stays exact
	}
	for _, tc := range cases {
		in, out, matched, ok := ppm.ResolveNodePriceMatch(context.Background(), node, tc.requested)
		require.Truef(t, ok, "expected %s to be priced on the node", tc.requested)
		assert.Greater(t, in, 0.0)
		assert.Greater(t, out, 0.0)
		assert.Equalf(t, tc.wantModel, matched, "matched provider model for %s", tc.requested)

		// ResolveNodePrice / MatchNodeModelPrice keep the previous contract.
		_, _, ok = ppm.ResolveNodePrice(context.Background(), node, tc.requested)
		assert.True(t, ok)
		m, ok := ppm.MatchNodeModelPrice(context.Background(), node, tc.requested)
		assert.True(t, ok)
		assert.Equal(t, tc.wantModel, m)
	}

	// Genuinely unpriced models stay unpriced (fail closed, never a guess).
	_, _, _, ok := ppm.ResolveNodePriceMatch(context.Background(), node, "totally-unknown-model")
	assert.False(t, ok)
	_, ok = ppm.MatchNodeModelPrice(context.Background(), node, "totally-unknown-model")
	assert.False(t, ok)
	_, ok = ppm.MatchNodeModelPrice(context.Background(), nil, "gemini-2.5-flash")
	assert.False(t, ok)
}

// End-to-end through the hypervisor: the request admitted for a normalized model
// name reaches the upstream with the provider's own model id.
func TestHandleRequestDispatchesProviderModelName(t *testing.T) {
	t.Setenv("ASH_YUNWU_TEST_MODE", "true")

	store := setupTestYunwuStore(t)
	rt := &capturingYunwuRuntime{}
	yh := NewYunwuHypervisor(store, rt)

	nm := health.NewNodeManager(store)
	ctx := context.Background()
	require.NoError(t, nm.SaveNode(ctx, &health.ExecutionNode{
		NodeID:       "node-dispatch",
		AccountName:  "Dispatch Node",
		Provider:     "openlux",
		State:        "healthy",
		Capabilities: DefaultModelFamilies(),
	}))
	require.NoError(t, store.Redis().SAdd(ctx, "ash:yunwu:nodes", "node-dispatch").Err())
	setTestBalance(t, store, "node-dispatch", 10_000_000)

	ppm := yh.PriceManager()
	require.NotNil(t, ppm)
	ppm.SetProviderPrices("node-dispatch", map[string]ProviderModelPrice{
		"gemini-2.5-flash": {InputPricePer1k: 0.001, OutputPricePer1k: 0.004, Available: true},
		"grok-4":           {InputPricePer1k: 0.002, OutputPricePer1k: 0.010, Available: true},
	})

	require.NoError(t, yh.qm.SaveUserAccess(ctx, &quota.UserAccess{
		OwnerID:      "user-dispatch",
		PlanID:       "basic",
		MonthlyLimit: 1_000_000,
		UsedUnits:    0,
		Priority:     1,
		IsActive:     true,
	}, "yunwu"))

	// Dash spelling of a dot-named model, and a prefixed vendor model.
	for _, tc := range []struct{ requested, want string }{
		{"google/gemini-2-5-flash", "gemini-2.5-flash"},
		{"xai/grok-4", "grok-4"},
	} {
		payload := []byte(`{"model": "` + tc.requested + `", "messages": [{"role": "user", "content": "hi"}]}`)
		_, err := yh.HandleRequest(ctx, "user-dispatch", "conv-"+tc.requested, payload, io.Discard, "yunwu")
		require.NoErrorf(t, err, "request for %s", tc.requested)
		assert.Equal(t, "node-dispatch", rt.lastNodeID)
		assert.Equalf(t, tc.want, rt.lastModel, "upstream model for %s", tc.requested)
	}

	// A model the pool cannot price is still rejected before any upstream call.
	rt.lastModel = ""
	payload := []byte(`{"model": "xai/grok-9-unpriced", "messages": [{"role": "user", "content": "hi"}]}`)
	_, err := yh.HandleRequest(ctx, "user-dispatch", "conv-unpriced", payload, io.Discard, "yunwu")
	require.ErrorIs(t, err, ErrNoPricingAvailable)
	assert.Empty(t, rt.lastModel)
}

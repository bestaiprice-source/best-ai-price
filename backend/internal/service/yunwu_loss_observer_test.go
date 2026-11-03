//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestObserveYunwuRequestCost_LiftsToServingProviderCost: a request served by a
// provider more expensive than the published (primary) basis must be re-priced at
// the serving provider's own rate — never sold below cost — and the row must be
// marked so it is visible in the usage history.
func TestObserveYunwuRequestCost_LiftsToServingProviderCost(t *testing.T) {
	resetYunwuProviderScopeCache()
	t.Cleanup(resetYunwuProviderScopeCache)

	yw := newMultiProviderPricing(map[string]map[string]*YunwuPricingRawItem{
		"portdan": {"claude-sonnet-5": {ModelRatio: 0.5}}, // cheapest => what we publish
		"x-llm":   {"claude-sonnet-5": {ModelRatio: 2.0}}, // what actually served: 4x dearer
	})
	bs := NewBillingService(nil, nil, yw)

	// The caller supplies the SAME billing method the original charge used; the
	// observer only swaps in the serving provider's rate.
	recost := func(context.Context) (*CostBreakdown, error) {
		return &CostBreakdown{InputCost: 4.0, TotalCost: 4.0, ActualCost: 4.4}, nil
	}

	cost := &CostBreakdown{InputCost: 1.0, TotalCost: 1.0, ActualCost: 1.1}
	lifted := observeYunwuRequestCost(context.Background(), bs, "x-llm", "claude-sonnet-5", 1.1, cost, recost)
	require.True(t, lifted)
	assert.InDelta(t, 4.0, cost.TotalCost, 1e-9, "the charge must cover the serving provider's cost")
	assert.InDelta(t, 4.4, cost.ActualCost, 1e-9)
	assert.True(t, cost.SoldAtCost, "the row must be marked as billed at cost")
}

// TestObserveYunwuRequestCost_LeavesProfitableAndUnverifiableRequestsAlone: a
// request that already covers its cost keeps its charge, and one whose cost cannot
// be compared (no price for the model on the serving provider) is left untouched
// rather than assumed profitable.
func TestObserveYunwuRequestCost_LeavesProfitableAndUnverifiableRequestsAlone(t *testing.T) {
	resetYunwuProviderScopeCache()
	t.Cleanup(resetYunwuProviderScopeCache)

	yw := newMultiProviderPricing(map[string]map[string]*YunwuPricingRawItem{
		"portdan": {"claude-sonnet-5": {ModelRatio: 0.5}},
		"x-llm":   {"claude-sonnet-5": {ModelRatio: 2.0}},
	})
	bs := NewBillingService(nil, nil, yw)
	ctx := context.Background()
	recost := func(context.Context) (*CostBreakdown, error) {
		return &CostBreakdown{InputCost: 4.0, TotalCost: 4.0, ActualCost: 4.4}, nil
	}

	// Already above the serving provider's cost ⇒ nothing to correct.
	profitable := &CostBreakdown{InputCost: 10.0, TotalCost: 10.0, ActualCost: 11.0}
	assert.False(t, observeYunwuRequestCost(ctx, bs, "x-llm", "claude-sonnet-5", 1.1, profitable, recost))
	assert.InDelta(t, 10.0, profitable.TotalCost, 1e-9)
	assert.False(t, profitable.SoldAtCost)

	// The serving provider has no price for this model ⇒ the check cannot run and
	// the charge is left exactly as it was (never silently treated as profitable).
	unverifiable := &CostBreakdown{InputCost: 1.0, TotalCost: 1.0, ActualCost: 1.1}
	assert.False(t, observeYunwuRequestCost(ctx, bs, "x-llm", "model-without-price", 1.1, unverifiable, recost))
	assert.InDelta(t, 1.0, unverifiable.TotalCost, 1e-9)
	assert.False(t, unverifiable.SoldAtCost)

	// A nil re-price function or a missing service must be a no-op, never a panic.
	noop := &CostBreakdown{InputCost: 1.0, TotalCost: 1.0}
	assert.False(t, observeYunwuRequestCost(ctx, bs, "x-llm", "claude-sonnet-5", 1.1, noop, nil))
	assert.False(t, observeYunwuRequestCost(ctx, nil, "x-llm", "claude-sonnet-5", 1.1, noop, recost))
}

package yunwu

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/quota"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/runtime"
)

// ---------------------------------------------------------------------------
// The provider billing group a key lives in
// (account.Extra smart_group_name / fast_group_name / detected_group_name)
// ---------------------------------------------------------------------------

func TestParseTierGroupNames(t *testing.T) {
	// The group is no longer pinned by hand: only the value the provider itself
	// reported (detected_group_name) is read, and legacy pins are ignored so a
	// stale value cannot keep driving a price.
	groups := ParseTierGroupNames(map[string]any{
		ExtraSmartGroupName:    "  default ",
		ExtraFastGroupName:     "Azure-Gpt-6",
		ExtraDetectedGroupName: "Azure-Gpt-1",
	})
	assert.Empty(t, groups.Smart)
	assert.Empty(t, groups.Fast)
	assert.Equal(t, "Azure-Gpt-1", groups.Detected)
	assert.Equal(t, "Azure-Gpt-1", groups.For(runtime.TierSmart))
	assert.Equal(t, "Azure-Gpt-1", groups.For(runtime.TierFast))

	// Pins alone no longer resolve to a group: without a detected value the
	// official rate applies.
	pinnedOnly := ParseTierGroupNames(map[string]any{ExtraSmartGroupName: "kiro", ExtraFastGroupName: "Azure-Gpt-6"})
	assert.True(t, pinnedOnly.IsZero())
	assert.Equal(t, DefaultProviderGroupName, pinnedOnly.For(runtime.TierSmart))

	empty := ParseTierGroupNames(nil)
	assert.True(t, empty.IsZero())

	// A non-string value is ignored rather than panicking.
	ignored := ParseTierGroupNames(map[string]any{ExtraSmartGroupName: 42, ExtraFastGroupName: nil})
	assert.True(t, ignored.IsZero())

	// A single-key node keeps no per-tier pin: one key serves both tiers, so the
	// group is whatever the provider reports (detected), or the official rate.
	single := ParseTierGroupNames(map[string]any{
		ExtraSmartGroupName:    "kiro",
		ExtraFastGroupName:     "Azure-Gpt-6",
		"tiering_disabled":     true,
		ExtraDetectedGroupName: "Claude max企业",
	})
	assert.Empty(t, single.Smart)
	assert.Empty(t, single.Fast)
	assert.Equal(t, "Claude max企业", single.Detected)
	assert.Equal(t, "Claude max企业", single.For(runtime.TierSmart))
	assert.Equal(t, "Claude max企业", single.For(runtime.TierFast))

	// Without a detected group it stays on the official rate.
	singleNoDetect := ParseTierGroupNames(map[string]any{"tiering_disabled": true, ExtraSmartGroupName: "kiro"})
	assert.Equal(t, DefaultProviderGroupName, singleNoDetect.For(runtime.TierSmart))
}

func TestTierGroupNamesForAndMerge(t *testing.T) {
	// Both keys are commonly issued inside the same group: configuring the smart
	// group is enough for fast too.
	same := TierGroupNames{Smart: "Azure-Gpt-1"}
	assert.Equal(t, "Azure-Gpt-1", same.For(runtime.TierSmart))
	assert.Equal(t, "Azure-Gpt-1", same.For(runtime.TierFast))

	both := TierGroupNames{Smart: "Azure-Gpt-1", Fast: "Azure-Gpt-6"}
	assert.Equal(t, "Azure-Gpt-1", both.For(runtime.TierSmart))
	assert.Equal(t, "Azure-Gpt-6", both.For(runtime.TierFast))

	merged := TierGroupNames{Fast: "f"}.Merge(TierGroupNames{Smart: "s", Fast: "other"})
	assert.Equal(t, "s", merged.Smart)
	assert.Equal(t, "f", merged.Fast, "the first configured value wins")
}

func TestNodeTierGroupsAreAlwaysResolved(t *testing.T) {
	// Nothing configured: both tiers fall back to the provider default group,
	// which One-API/New-API bill at the official rate.
	plain := NodeTierGroups(nil)
	assert.Equal(t, map[string]string{"smart": "default", "fast": "default"}, plain)
	assert.Equal(t, "default", GroupNameForTier(plain, runtime.TierSmart))
	assert.Equal(t, "default", GroupNameForTier(plain, runtime.TierFast))
	assert.Equal(t, "default", GroupNameForTier(nil, runtime.TierFast))

	// A detected group is used until an admin pins the real one.
	detected := NodeTierGroups(map[string]any{ExtraDetectedGroupName: "Azure-Gpt-6"})
	assert.Equal(t, "Azure-Gpt-6", GroupNameForTier(detected, runtime.TierSmart))

	// A legacy pin is ignored: the group the provider reported is the one used.
	legacyPin := NodeTierGroups(map[string]any{
		ExtraSmartGroupName:    "Azure-Gpt-1",
		ExtraDetectedGroupName: "Azure-Gpt-6",
	})
	assert.Equal(t, "Azure-Gpt-6", GroupNameForTier(legacyPin, runtime.TierSmart))
	assert.Equal(t, "Azure-Gpt-6", GroupNameForTier(legacyPin, runtime.TierFast),
		"both tiers follow the detected group")
}

// ---------------------------------------------------------------------------
// The engine prices a candidate node with the node's OWN group
// ---------------------------------------------------------------------------

const tierGroupTestModel = "gpt-4o"

// providerGroupRatios is OpenLux-shaped: the provider publishes discounts for its
// named groups, and the `default` group our keys live in is not listed at all.
var providerGroupRatios = map[string]float64{"Azure-Gpt-1": 0.044, "Azure-Gpt-6": 0.9}

func seedTierGroupTestPool(t *testing.T, node *health.ExecutionNode, ownerID string) (*YunwuHypervisor, *mockYunwuRuntime) {
	t.Helper()
	t.Setenv("ASH_YUNWU_TEST_MODE", "true")

	store := setupTestYunwuStore(t)
	rt := &mockYunwuRuntime{}
	yh := NewYunwuHypervisor(store, rt)
	nm := health.NewNodeManager(store)
	ctx := context.Background()

	copied := *node
	if len(copied.Tiers) == 0 {
		copied.Tiers = []string{runtime.TierSmart, runtime.TierFast}
	}
	require.NoError(t, nm.SaveNode(ctx, &copied))
	require.NoError(t, store.Redis().SAdd(ctx, "ash:yunwu:nodes", copied.NodeID).Err())
	setTestBalance(t, store, copied.NodeID, 50_000_000)
	require.NoError(t, nm.SaveNodeRuntimeState(ctx, &health.NodeRuntimeState{NodeID: copied.NodeID, SuccessRate: 1.0}))

	require.NoError(t, yh.qm.SaveUserAccess(ctx, &quota.UserAccess{
		OwnerID: ownerID, MonthlyLimit: 1_000_000, Priority: 1, IsActive: true,
	}, "yunwu"))

	require.NotNil(t, yh.PriceManager())
	yh.PriceManager().SetProviderPrices(copied.NodeID, map[string]ProviderModelPrice{
		tierGroupTestModel: {
			InputPricePer1k:      0.0025,
			OutputPricePer1k:     0.01,
			BaseInputPricePer1k:  0.0025,
			BaseOutputPricePer1k: 0.01,
			CheapestGroupRatio:   0.044,
			GroupRatios:          providerGroupRatios,
			Available:            true,
		},
	})
	return yh, rt
}

// TestNodeWithoutGroupUsesTheCheapestModelGroup: when the provider reports no
// group for the key, the model is priced at the cheapest group it is ENABLED for
// (a model lives in several groups, so this is a per-model decision) — for both
// tiers. The measured correction then moves that assumption to what the provider
// really charges.
func TestNodeWithoutGroupUsesTheCheapestModelGroup(t *testing.T) {
	owner := "tier-group-owner-default"
	node := &health.ExecutionNode{
		NodeID:     "110",
		Provider:   "openlux",
		State:      "healthy",
		TierGroups: NodeTierGroups(nil),
	}
	yh, rt := seedTierGroupTestPool(t, node, owner)
	ctx := context.Background()

	in, _, ok := yh.PriceManager().ResolveNodePrice(ctx, node, tierGroupTestModel)
	require.True(t, ok)
	assert.InDelta(t, 0.0025*0.044, in, 1e-9,
		"smart falls back to the cheapest group the model is enabled for")

	fastCtx := runtime.ContextWithTier(ctx, runtime.TierFast)
	in, _, ok = yh.PriceManager().ResolveNodePrice(fastCtx, node, tierGroupTestModel)
	require.True(t, ok)
	assert.InDelta(t, 0.0025*0.044, in, 1e-9,
		"fast uses the same per-model basis")

	payload := []byte(`{"model":"` + tierGroupTestModel + `"}`)
	_, err := yh.HandleRequest(fastCtx, owner, "conv-default-fast", payload, io.Discard, "openai")
	require.NoError(t, err)
	assert.Equal(t, "110", rt.lastNodeID)
}

// TestNodeObservedRatioCorrectsThePrice: the measured correction moves the node's
// price to what the provider is known to charge — here upwards, so an assumption
// that turned out too optimistic cannot sell below cost.
func TestNodeObservedRatioCorrectsThePrice(t *testing.T) {
	owner := "tier-group-owner-observed"
	node := &health.ExecutionNode{
		NodeID:             "113",
		Provider:           "openlux",
		State:              "healthy",
		TierGroups:         NodeTierGroups(nil),
		ObservedGroupRatio: 1.4,
	}
	yh, _ := seedTierGroupTestPool(t, node, owner)
	ctx := context.Background()

	in, _, ok := yh.PriceManager().ResolveNodePrice(ctx, node, tierGroupTestModel)
	require.True(t, ok)
	assert.InDelta(t, 0.0025*0.044*1.4, in, 1e-9, "the measured correction must raise the assumed rate")

	fastCtx := runtime.ContextWithTier(ctx, runtime.TierFast)
	// The measurement is PER KEY: the smart correction must not touch the fast price,
	// whose own key has its own provider counter.
	in, _, ok = yh.PriceManager().ResolveNodePrice(fastCtx, node, tierGroupTestModel)
	require.True(t, ok)
	assert.InDelta(t, 0.0025*0.044, in, 1e-9,
		"the fast tier is corrected by the FAST key's own measurement only")

	// With the fast key's own measurement the fast price is corrected as well.
	node.ObservedGroupRatioFast = 1.4
	in, _, ok = yh.PriceManager().ResolveNodePrice(fastCtx, node, tierGroupTestModel)
	require.True(t, ok)
	assert.InDelta(t, 0.0025*0.044*1.4, in, 1e-9,
		"the fast correction multiplies the fast basis")
}

// A cheaper reading now LOWERS the rate: the correction tracks what the provider
// really charges in both directions, instead of only ever lifting a price.
func TestNodeObservedRatioBelowAssumptionLowersThePrice(t *testing.T) {
	owner := "tier-group-owner-observed-low"
	node := &health.ExecutionNode{
		NodeID:             "114",
		Provider:           "openlux",
		State:              "healthy",
		TierGroups:         NodeTierGroups(map[string]any{ExtraDetectedGroupName: "Azure-Gpt-6"}),
		ObservedGroupRatio: 0.5,
	}
	yh, _ := seedTierGroupTestPool(t, node, owner)
	ctx := context.Background()

	in, _, ok := yh.PriceManager().ResolveNodePrice(ctx, node, tierGroupTestModel)
	require.True(t, ok)
	assert.InDelta(t, 0.0025*0.9*0.5, in, 1e-9, "a cheaper reading must lower the rate")
}

// TestNodeDetectedGroupPricesInsteadOfTheCheapestAdvertisedGroup: the group the
// provider API reported for the key prices the request — NOT the cheapest group
// the provider advertises for the model (pricing by the cheapest advertised group
// is what used to sell far below the provider's real charge).
func TestNodeDetectedGroupPricesInsteadOfTheCheapestAdvertisedGroup(t *testing.T) {
	owner := "tier-group-owner-pinned"
	node := &health.ExecutionNode{
		NodeID:     "111",
		Provider:   "openlux",
		State:      "healthy",
		TierGroups: NodeTierGroups(map[string]any{ExtraDetectedGroupName: "Azure-Gpt-6"}),
	}
	yh, rt := seedTierGroupTestPool(t, node, owner)
	ctx := context.Background()

	in, _, ok := yh.PriceManager().ResolveNodePrice(ctx, node, tierGroupTestModel)
	require.True(t, ok)
	assert.InDelta(t, 0.0025*0.9, in, 1e-9, "a pinned group must use that group's ratio")

	fastCtx := runtime.ContextWithTier(ctx, runtime.TierFast)
	in, _, ok = yh.PriceManager().ResolveNodePrice(fastCtx, node, tierGroupTestModel)
	require.True(t, ok)
	assert.InDelta(t, 0.0025*0.9, in, 1e-9)

	payload := []byte(`{"model":"` + tierGroupTestModel + `"}`)
	_, err := yh.HandleRequest(fastCtx, owner, "conv-pinned-fast", payload, io.Discard, "openai")
	require.NoError(t, err)
	assert.Equal(t, "111", rt.lastNodeID)
}

// TestDetectedGroupPricesTheRequest: the group discovered from the provider API
// (balance sync) is the pricing basis.
func TestDetectedGroupPricesTheRequest(t *testing.T) {
	owner := "tier-group-owner-detected"
	detected := NodeTierGroups(map[string]any{ExtraDetectedGroupName: "Azure-Gpt-6"})
	node := &health.ExecutionNode{
		NodeID:     "112",
		Provider:   "openlux",
		State:      "healthy",
		TierGroups: detected,
	}
	yh, _ := seedTierGroupTestPool(t, node, owner)
	ctx := context.Background()

	in, _, ok := yh.PriceManager().ResolveNodePrice(ctx, node, tierGroupTestModel)
	require.True(t, ok)
	assert.InDelta(t, 0.0025*0.9, in, 1e-9, "the detected group must be honoured")
}

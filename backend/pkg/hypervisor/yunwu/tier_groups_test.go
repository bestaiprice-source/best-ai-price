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
// Pinned provider billing groups (account.Extra smart_group_name / fast_group_name)
// ---------------------------------------------------------------------------

func TestParseTierGroupNames(t *testing.T) {
	groups := ParseTierGroupNames(map[string]any{
		ExtraSmartGroupName: "  default ",
		ExtraFastGroupName:  "Azure-Gpt-6",
	})
	assert.Equal(t, "default", groups.Smart)
	assert.Equal(t, "Azure-Gpt-6", groups.Fast)
	assert.False(t, groups.IsZero())

	empty := ParseTierGroupNames(nil)
	assert.True(t, empty.IsZero())

	// A non-string value is ignored rather than panicking.
	ignored := ParseTierGroupNames(map[string]any{ExtraSmartGroupName: 42, ExtraFastGroupName: nil})
	assert.True(t, ignored.IsZero())
}

func TestTierGroupNamesForAndMerge(t *testing.T) {
	same := TierGroupNames{Smart: "default"}
	// Both keys are commonly issued inside the same group: configuring the smart
	// group is enough for fast too.
	assert.Equal(t, "default", same.For(runtime.TierSmart))
	assert.Equal(t, "default", same.For(runtime.TierFast))

	both := TierGroupNames{Smart: "Azure-Gpt-1", Fast: "Azure-Gpt-6"}
	assert.Equal(t, "Azure-Gpt-1", both.For(runtime.TierSmart))
	assert.Equal(t, "Azure-Gpt-6", both.For(runtime.TierFast))
	assert.Equal(t, "", TierGroupNames{}.For(runtime.TierFast))

	merged := TierGroupNames{Fast: "f"}.Merge(TierGroupNames{Smart: "s", Fast: "other"})
	assert.Equal(t, "s", merged.Smart)
	assert.Equal(t, "f", merged.Fast, "the first configured value wins")
}

func TestTierGroupMapRoundTrip(t *testing.T) {
	groups := TierGroupNames{Smart: "default", Fast: "Azure-Gpt-6"}
	mapped := groups.Map()
	assert.Equal(t, map[string]string{"smart": "default", "fast": "Azure-Gpt-6"}, mapped)
	assert.True(t, TierGroupNames{}.Map() == nil)

	assert.Equal(t, groups, ParseTierGroupMap(mapped))
	assert.Equal(t, "default", GroupNameForTier(mapped, runtime.TierSmart))
	assert.Equal(t, "", GroupNameForTier(nil, runtime.TierFast))
	// The fast→smart fallback also applies to the node-side map.
	assert.Equal(t, "default", GroupNameForTier(map[string]string{"smart": "default"}, runtime.TierFast))
}

func TestResolveTierGroupRatioForGroup(t *testing.T) {
	ratios := map[string]float64{"Azure-Gpt-1": 0.044, "Azure-Gpt-6": 0.9}

	// A pinned group that the provider publishes wins.
	r, ok := ResolveTierGroupRatioForGroup(ratios, 0.044, runtime.TierFast, "Azure-Gpt-6")
	require.True(t, ok)
	assert.InDelta(t, 0.9, r, 1e-9)

	// The pinned group is not published (e.g. `default`, which providers bill at
	// the official rate): the request is priced at the official rate instead of
	// failing closed or borrowing the cheapest group's rate.
	r, ok = ResolveTierGroupRatioForGroup(ratios, 0.044, runtime.TierSmart, "default")
	require.True(t, ok)
	assert.InDelta(t, 1.0, r, 1e-9)

	// A provider that does not price per group keeps its single rate.
	r, ok = ResolveTierGroupRatioForGroup(nil, 0.25, runtime.TierFast, "whatever")
	require.True(t, ok)
	assert.InDelta(t, 0.25, r, 1e-9)

	// With nothing pinned the legacy derivation applies: smart → cheapest group,
	// fast → the literal `fast` group, which this provider does not have.
	r, ok = ResolveTierGroupRatioForGroup(ratios, 0.044, runtime.TierSmart, "")
	require.True(t, ok)
	assert.InDelta(t, 0.044, r, 1e-9)
	if _, ok := ResolveTierGroupRatioForGroup(ratios, 0.044, runtime.TierFast, ""); ok {
		t.Fatal("an unpinned fast tier with no `fast` group must stay unpriced")
	}
}

// ---------------------------------------------------------------------------
// The engine prices a candidate node with the node's OWN pinned groups
// ---------------------------------------------------------------------------

const tierGroupTestModel = "gpt-4o"

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

	// The provider prices per group and does NOT list the group our OpenLux-style
	// account pinned (`default`) — only the premium groups it advertises.
	require.NotNil(t, yh.PriceManager())
	yh.PriceManager().SetProviderPrices(copied.NodeID, map[string]ProviderModelPrice{
		tierGroupTestModel: {
			InputPricePer1k:      0.0025,
			OutputPricePer1k:     0.01,
			BaseInputPricePer1k:  0.0025,
			BaseOutputPricePer1k: 0.01,
			CheapestGroupRatio:   0.044,
			GroupRatios:          map[string]float64{"Azure-Gpt-1": 0.044, "Azure-Gpt-6": 0.9},
			Available:            true,
		},
	})
	return yh, rt
}

// TestNodePinnedBillingGroupPricesBothTiers is the regression guard for the real
// install: the account's keys live in a group the provider's pricing payload does
// not list (`default`), so the legacy derivation priced smart at the cheapest
// advertised group (a fraction of the real cost) and left fast completely
// unpriced — every fast request failed with ErrNoPricingAvailable.
func TestNodePinnedBillingGroupPricesBothTiers(t *testing.T) {
	owner := "tier-group-owner"
	node := &health.ExecutionNode{
		NodeID:     "111",
		Provider:   "openlux",
		State:      "healthy",
		TierGroups: TierGroupNames{Smart: "default"}.Map(),
	}
	yh, rt := seedTierGroupTestPool(t, node, owner)
	ctx := context.Background()

	// Smart is priced at the pinned group's rate — the official rate here, since
	// the provider does not advertise `default` — not at the cheapest group.
	in, _, ok := yh.PriceManager().ResolveNodePrice(ctx, node, tierGroupTestModel)
	require.True(t, ok)
	assert.InDelta(t, 0.0025, in, 1e-9, "smart must use the pinned group, not the cheapest advertised one")

	// Fast is priceable too (the pinned group applies to it as well) and is
	// actually served by the node.
	fastCtx := runtime.ContextWithTier(ctx, runtime.TierFast)
	in, _, ok = yh.PriceManager().ResolveNodePrice(fastCtx, node, tierGroupTestModel)
	require.True(t, ok, "a fast request must be priceable once the group is pinned")
	assert.InDelta(t, 0.0025, in, 1e-9)

	payload := []byte(`{"model":"` + tierGroupTestModel + `"}`)
	_, err := yh.HandleRequest(fastCtx, owner, "conv-pinned-fast", payload, io.Discard, "openai")
	require.NoError(t, err)
	assert.Equal(t, "111", rt.lastNodeID)
}

// TestNodeWithoutPinnedGroupLeavesFastUnpriced documents the legacy behaviour the
// pin replaces: the group the key actually bills in is unknown, so the fast tier
// (which looks for a group literally named `fast`) is unpriced and the engine
// refuses the request instead of silently selling it at a smart rate.
func TestNodeWithoutPinnedGroupLeavesFastUnpriced(t *testing.T) {
	owner := "tier-group-owner-legacy"
	node := &health.ExecutionNode{
		NodeID:   "112",
		Provider: "openlux",
		State:    "healthy",
	}
	yh, _ := seedTierGroupTestPool(t, node, owner)
	ctx := context.Background()

	fastCtx := runtime.ContextWithTier(ctx, runtime.TierFast)
	_, _, ok := yh.PriceManager().ResolveNodePrice(fastCtx, node, tierGroupTestModel)
	assert.False(t, ok, "without a pinned group the fast tier stays unpriced")

	payload := []byte(`{"model":"` + tierGroupTestModel + `"}`)
	_, err := yh.HandleRequest(fastCtx, owner, "conv-unpinned-fast", payload, io.Discard, "openai")
	require.ErrorIs(t, err, ErrNoPricingAvailable)
}

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
	groups := ParseTierGroupNames(map[string]any{
		ExtraSmartGroupName:    "  default ",
		ExtraFastGroupName:     "Azure-Gpt-6",
		ExtraDetectedGroupName: "Azure-Gpt-1",
	})
	assert.Equal(t, "default", groups.Smart)
	assert.Equal(t, "Azure-Gpt-6", groups.Fast)
	assert.Equal(t, "Azure-Gpt-1", groups.Detected)
	assert.False(t, groups.IsZero())

	empty := ParseTierGroupNames(nil)
	assert.True(t, empty.IsZero())

	// A non-string value is ignored rather than panicking.
	ignored := ParseTierGroupNames(map[string]any{ExtraSmartGroupName: 42, ExtraFastGroupName: nil})
	assert.True(t, ignored.IsZero())
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

	pinned := NodeTierGroups(map[string]any{
		ExtraSmartGroupName:    "Azure-Gpt-1",
		ExtraDetectedGroupName: "Azure-Gpt-6",
	})
	assert.Equal(t, "Azure-Gpt-1", GroupNameForTier(pinned, runtime.TierSmart))
	assert.Equal(t, "Azure-Gpt-1", GroupNameForTier(pinned, runtime.TierFast),
		"fast inherits the smart group when it has none of its own")
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

// TestNodeWithoutGroupFallsBackToTheOfficialRate: with nothing configured a
// request is priced at the provider's OFFICIAL rate (its `default` group), never
// at the cheapest advertised group — understating the cost is what made the
// published price 20x below what the provider actually charges.
func TestNodeWithoutGroupFallsBackToTheOfficialRate(t *testing.T) {
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
	assert.InDelta(t, 0.0025, in, 1e-9, "the unlisted default group must bill at the official rate")

	// Fast is priceable as well (same default group) — no more 400 on a fast key.
	fastCtx := runtime.ContextWithTier(ctx, runtime.TierFast)
	in, _, ok = yh.PriceManager().ResolveNodePrice(fastCtx, node, tierGroupTestModel)
	require.True(t, ok)
	assert.InDelta(t, 0.0025, in, 1e-9)

	payload := []byte(`{"model":"` + tierGroupTestModel + `"}`)
	_, err := yh.HandleRequest(fastCtx, owner, "conv-default-fast", payload, io.Discard, "openai")
	require.NoError(t, err)
	assert.Equal(t, "110", rt.lastNodeID)
}

// TestNodePinnedGroupWinsOverTheCheapestAdvertisedGroup: pinning the real
// discounted group prices the request at that group's rate (this is how an
// account whose key really is in a discounted group gets the lower price).
func TestNodePinnedGroupWinsOverTheCheapestAdvertisedGroup(t *testing.T) {
	owner := "tier-group-owner-pinned"
	node := &health.ExecutionNode{
		NodeID:     "111",
		Provider:   "openlux",
		State:      "healthy",
		TierGroups: NodeTierGroups(map[string]any{ExtraSmartGroupName: "Azure-Gpt-6"}),
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

// TestDetectedGroupPricesUntilAnAdminPinsOne: the group discovered from the
// provider API (balance sync) is used when nothing is pinned, and an explicit pin
// still wins.
func TestDetectedGroupPricesUntilAnAdminPinsOne(t *testing.T) {
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

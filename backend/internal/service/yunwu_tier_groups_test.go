//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/runtime"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/yunwu"
)

// The catalog and the charge must price a tier with the billing group the key
// actually lives in. The groups are pinned per account (`smart_group_name` /
// `fast_group_name`) and surfaced through the provider scope, so both paths read
// the same value.
func TestComputeYunwuScopeCollectsPinnedTierGroups(t *testing.T) {
	repo := &mockAccountRepoForPricing{schedulable: []Account{
		{
			Platform:      PlatformYunwu,
			AccountGroups: []AccountGroup{{AccountID: 1, GroupID: 7}},
			Extra: map[string]any{
				"provider":                "openlux",
				yunwu.ExtraSmartGroupName: "default",
			},
		},
	}}

	scope, pricing := computeYunwuScope(context.Background(), repo)
	require.NotEmpty(t, scope, "the account must widen the family scope")
	require.Equal(t, yunwu.TierGroupNames{Smart: "default"}, pricing.TierGroups["openlux"])

	ctx := ContextWithYunwuTierGroups(context.Background(), pricing.TierGroups)
	require.Equal(t, "default", yunwuTierGroupForProvider(ctx, "openlux", runtime.TierSmart))
	// Both keys of the account live in the same group: the fast tier inherits it.
	require.Equal(t, "default", yunwuTierGroupForProvider(ctx, "openlux", runtime.TierFast))
	// A provider that pinned nothing keeps the legacy derivation.
	require.Equal(t, "", yunwuTierGroupForProvider(ctx, "api-top", runtime.TierFast))
	require.Equal(t, "", yunwuTierGroupForProvider(context.Background(), "openlux", runtime.TierSmart))
}

func TestComputeYunwuScopeIgnoresUngroupedAndNonYunwuAccounts(t *testing.T) {
	repo := &mockAccountRepoForPricing{schedulable: []Account{
		{
			Platform: PlatformYunwu, // no groups -> the engine can never route to it
			Extra:    map[string]any{"provider": "api-top", yunwu.ExtraSmartGroupName: "kiro"},
		},
		{
			Platform:      PlatformAnthropic,
			AccountGroups: []AccountGroup{{AccountID: 2, GroupID: 7}},
			Extra:         map[string]any{yunwu.ExtraSmartGroupName: "whatever"},
		},
	}}

	scope, pricing := computeYunwuScope(context.Background(), repo)
	require.Empty(t, scope)
	require.Empty(t, pricing.TierGroups)
	require.Empty(t, pricing.Observed)
}

// The balance sync's reconciliation result (extra.observed_group_ratio) travels
// with the provider scope and becomes a price FLOOR: the highest reading among a
// provider's accounts wins, so a cheap-looking account can never pull the price
// below what another account really pays.
func TestComputeYunwuScopeCollectsTheObservedRatioFloor(t *testing.T) {
	repo := &mockAccountRepoForPricing{schedulable: []Account{
		{
			Platform:      PlatformYunwu,
			AccountGroups: []AccountGroup{{AccountID: 1, GroupID: 7}},
			Extra: map[string]any{
				"provider":                      "openlux",
				yunwu.ExtraObservedRatioEnabled: true,
				yunwu.ExtraObservedGroupRatio:   1.25,
			},
		},
		{
			Platform:      PlatformYunwu,
			AccountGroups: []AccountGroup{{AccountID: 2, GroupID: 7}},
			Extra: map[string]any{
				"provider":                      "openlux",
				yunwu.ExtraObservedRatioEnabled: true,
				yunwu.ExtraObservedGroupRatio:   1.4,
			},
		},
	}}

	_, pricing := computeYunwuScope(context.Background(), repo)
	require.InDelta(t, 1.4, pricing.Observed["openlux"], 1e-9)

	// A node that did NOT opt in contributes nothing, even with a value stored.
	repoOff := &mockAccountRepoForPricing{schedulable: []Account{
		{
			Platform:      PlatformYunwu,
			AccountGroups: []AccountGroup{{AccountID: 3, GroupID: 7}},
			Extra:         map[string]any{"provider": "openlux", yunwu.ExtraObservedGroupRatio: 3.0},
		},
	}}
	_, off := computeYunwuScope(context.Background(), repoOff)
	require.Empty(t, off.Observed, "the reconciliation is opt-in per node")

	ctx := ContextWithYunwuPricingScope(context.Background(), pricing)
	require.InDelta(t, 1.4, yunwuObservedRatioForProvider(ctx, "openlux"), 1e-9)
	require.InDelta(t, 0.0, yunwuObservedRatioForProvider(ctx, "api-top"), 1e-9)
	require.InDelta(t, 0.0, yunwuObservedRatioForProvider(context.Background(), "openlux"), 1e-9)
}

func TestWithYunwuProviderScopeCarriesThePinnedGroups(t *testing.T) {
	resetYunwuProviderScopeCache()
	t.Cleanup(resetYunwuProviderScopeCache)

	repo := &mockAccountRepoForPricing{schedulable: []Account{
		{
			Platform:      PlatformYunwu,
			AccountGroups: []AccountGroup{{AccountID: 1, GroupID: 7}},
			Extra: map[string]any{
				"provider":                "openlux",
				yunwu.ExtraSmartGroupName: "default",
				yunwu.ExtraFastGroupName:  "Azure-Gpt-6",
			},
		},
	}}

	ctx := withYunwuProviderScope(context.Background(), repo)
	require.Equal(t, "default", yunwuTierGroupForProvider(ctx, "openlux", runtime.TierSmart))
	require.Equal(t, "Azure-Gpt-6", yunwuTierGroupForProvider(ctx, "openlux", runtime.TierFast))
	require.Equal(t, "Azure-Gpt-6", yunwuTierGroupForProvider(ctx, "OpenLux", runtime.TierFast),
		"the provider key is canonicalized")
}

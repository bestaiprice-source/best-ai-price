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

	scope, tierGroups := computeYunwuScope(context.Background(), repo)
	require.NotEmpty(t, scope, "the account must widen the family scope")
	require.Equal(t, yunwu.TierGroupNames{Smart: "default"}, tierGroups["openlux"])

	ctx := ContextWithYunwuTierGroups(context.Background(), tierGroups)
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

	scope, tierGroups := computeYunwuScope(context.Background(), repo)
	require.Empty(t, scope)
	require.Empty(t, tierGroups)
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

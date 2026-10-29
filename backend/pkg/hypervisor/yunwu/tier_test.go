package yunwu

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/runtime"
)

func TestSupportedTiersForCredentials(t *testing.T) {
	cases := []struct {
		name  string
		creds map[string]any
		want  []string
	}{
		{"legacy api_key is smart only", map[string]any{"api_key": "sk"}, []string{runtime.TierSmart}},
		{"token counts as smart", map[string]any{"token": "sk"}, []string{runtime.TierSmart}},
		{"both keys", map[string]any{"api_key": "s", "api_key_fast": "f"}, []string{runtime.TierSmart, runtime.TierFast}},
		{"fast only", map[string]any{"api_key_fast": "f"}, []string{runtime.TierFast}},
		{"no credentials", map[string]any{}, nil},
		{"blank fast key ignored", map[string]any{"api_key": "s", "api_key_fast": "  "}, []string{runtime.TierSmart}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SupportedTiersForCredentials(tc.creds)
			if len(got) != len(tc.want) {
				t.Fatalf("SupportedTiersForCredentials(%v) = %v, want %v", tc.creds, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("SupportedTiersForCredentials(%v) = %v, want %v", tc.creds, got, tc.want)
				}
			}
		})
	}
}

func TestNodeSupportsBillingTier(t *testing.T) {
	// Unknown / empty tiers are tier-agnostic (never excluded) — this is what
	// keeps pre-existing nodes (and test nodes) working.
	if !NodeSupportsBillingTier(nil, runtime.TierFast) {
		t.Fatal("node with no advertised tiers must be allowed for any tier")
	}
	if !NodeSupportsBillingTier([]string{}, runtime.TierFast) {
		t.Fatal("node with empty tiers must be allowed for any tier")
	}
	// No tier requested -> no restriction.
	if !NodeSupportsBillingTier([]string{runtime.TierSmart}, "") {
		t.Fatal("empty requested tier must not restrict")
	}
	// An explicit tier list is strict: a smart-only node cannot serve fast.
	if NodeSupportsBillingTier([]string{runtime.TierSmart}, runtime.TierFast) {
		t.Fatal("smart-only node must NOT serve the fast tier")
	}
	if !NodeSupportsBillingTier([]string{runtime.TierSmart}, runtime.TierSmart) {
		t.Fatal("smart-only node must serve the smart tier")
	}
	if !NodeSupportsBillingTier([]string{runtime.TierFast}, "FAST") {
		t.Fatal("tier matching must be case-insensitive")
	}
}

func TestTierGroupName(t *testing.T) {
	if got := TierGroupName(runtime.TierFast); got != "fast" {
		t.Fatalf("fast group name = %q, want fast", got)
	}
	if got := TierGroupName("SMART"); got != "smart" {
		t.Fatalf("smart group name = %q, want smart", got)
	}
	// Unknown/empty tier falls back to the default smart group.
	if got := TierGroupName("turbo"); got != runtime.TierSmart {
		t.Fatalf("unknown tier group name = %q, want smart", got)
	}
}

// The tier's group is resolved from the pinned/detected group, and a group the
// provider does not list is billed at the official rate (1.0) — the same rule
// One-API/New-API apply — so a request is never priced below the official rate
// by accident.
func TestResolveTierGroupRatioForGroupDefaultsAndPins(t *testing.T) {
	ratios := map[string]float64{"Azure-Gpt-1": 0.044, "Azure-Gpt-6": 0.9}

	// A group the provider lists wins.
	if r := ResolveTierGroupRatioForGroup(ratios, "Azure-Gpt-6"); r != 0.9 {
		t.Fatalf("listed group ratio = %v, want 0.9", r)
	}
	// The default group is not listed -> official rate.
	if r := ResolveTierGroupRatioForGroup(ratios, DefaultProviderGroupName); r != 1.0 {
		t.Fatalf("default group ratio = %v, want 1.0 (official)", r)
	}
	if r := ResolveTierGroupRatioForGroup(ratios, "some-unknown-group"); r != 1.0 {
		t.Fatalf("unlisted group ratio = %v, want 1.0 (official)", r)
	}
	// Nothing pinned at all -> the default group.
	if r := ResolveTierGroupRatioForGroup(ratios, ""); r != 1.0 {
		t.Fatalf("empty group ratio = %v, want 1.0 (official)", r)
	}
	// The provider does not price per group: the caller uses its single rate.
	if r := ResolveTierGroupRatioForGroup(nil, DefaultProviderGroupName); r != 1.0 {
		t.Fatalf("no group pricing ratio = %v, want 1.0", r)
	}
}

func TestTierGroupNamesForDefaults(t *testing.T) {
	// Nothing pinned and nothing detected: the provider's default group.
	if got := (TierGroupNames{}).For(runtime.TierSmart); got != DefaultProviderGroupName {
		t.Fatalf("smart default group = %q, want %q", got, DefaultProviderGroupName)
	}
	if got := (TierGroupNames{}).For(runtime.TierFast); got != DefaultProviderGroupName {
		t.Fatalf("fast default group = %q, want %q", got, DefaultProviderGroupName)
	}
	// A detected group (written by the balance sync) is used when nothing is pinned.
	detected := TierGroupNames{Detected: "Azure-Gpt-6"}
	if got := detected.For(runtime.TierSmart); got != "Azure-Gpt-6" {
		t.Fatalf("detected group = %q, want Azure-Gpt-6", got)
	}
	// An explicit pin always wins over the detected value.
	pinned := TierGroupNames{Smart: "Azure-Gpt-1", Detected: "Azure-Gpt-6"}
	if got := pinned.For(runtime.TierSmart); got != "Azure-Gpt-1" {
		t.Fatalf("pinned group = %q, want Azure-Gpt-1", got)
	}
	if got := pinned.For(runtime.TierFast); got != "Azure-Gpt-1" {
		t.Fatalf("fast must inherit the smart pin, got %q", got)
	}
	if got := (TierGroupNames{Fast: "Azure-Gpt-6", Smart: "Azure-Gpt-1"}).For(runtime.TierFast); got != "Azure-Gpt-6" {
		t.Fatalf("explicit fast pin = %q, want Azure-Gpt-6", got)
	}
}

func TestNormalizeTierReExport(t *testing.T) {
	cases := map[string]string{
		"":        "",
		"  ":      "",
		"smart":   runtime.TierSmart,
		"SMART":   runtime.TierSmart,
		" fast ":  runtime.TierFast,
		"Fast":    runtime.TierFast,
		"turbo":   "",
		"fastest": "",
	}
	for in, want := range cases {
		if got := NormalizeTier(in); got != want {
			t.Fatalf("NormalizeTier(%q) = %q, want %q", in, got, want)
		}
	}
}

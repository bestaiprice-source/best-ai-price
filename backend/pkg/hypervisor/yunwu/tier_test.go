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

// The tier's group is resolved from the pinned/detected group; a group the
// provider does not list falls back to the tier default — the official rate for
// smart, the official rate plus the fast safety margin for fast (1.0 / 1.1) — so
// a request is never priced below the official rate by accident.
func TestResolveTierGroupRatioForGroupDefaultsAndPins(t *testing.T) {
	ratios := map[string]float64{"Azure-Gpt-1": 0.044, "Azure-Gpt-6": 0.9}

	// A group the provider lists wins for whichever tier asks (the smart tier is
	// the default when no tier is in the context).
	if r := ResolveTierGroupRatioForGroup(ratios, runtime.TierSmart, "Azure-Gpt-6"); r != 0.9 {
		t.Fatalf("listed group ratio = %v, want 0.9", r)
	}
	if r := ResolveTierGroupRatioForGroup(ratios, runtime.TierFast, "Azure-Gpt-1"); r != 0.044 {
		t.Fatalf("listed group ratio for fast = %v, want 0.044", r)
	}
	// The default group is not listed -> tier defaults.
	if r := ResolveTierGroupRatioForGroup(ratios, runtime.TierSmart, DefaultProviderGroupName); r != DefaultSmartGroupRatio {
		t.Fatalf("smart default group ratio = %v, want %v", r, DefaultSmartGroupRatio)
	}
	if r := ResolveTierGroupRatioForGroup(ratios, runtime.TierFast, DefaultProviderGroupName); r != DefaultFastGroupRatio {
		t.Fatalf("fast default group ratio = %v, want %v", r, DefaultFastGroupRatio)
	}
	if r := ResolveTierGroupRatioForGroup(ratios, runtime.TierFast, "some-unknown-group"); r != DefaultFastGroupRatio {
		t.Fatalf("unlisted fast group ratio = %v, want %v", r, DefaultFastGroupRatio)
	}
	// No group name and no group pricing at all: still the tier default.
	if r := ResolveTierGroupRatioForGroup(nil, runtime.TierSmart, ""); r != DefaultSmartGroupRatio {
		t.Fatalf("empty group ratio = %v, want %v", r, DefaultSmartGroupRatio)
	}
	if r := ResolveTierGroupRatioForGroup(nil, runtime.TierFast, ""); r != DefaultFastGroupRatio {
		t.Fatalf("empty fast group ratio = %v, want %v", r, DefaultFastGroupRatio)
	}
}

// The reconciliation floor (option 2) guarantees we never charge below what the
// provider actually charged, and never lowers a price on its own.
func TestObservedGroupRatioAndFloor(t *testing.T) {
	// The provider charged 1.1x what the official rate would cost us.
	ratio, ok := ObservedGroupRatio(10.0, 21.0, 10.0)
	if !ok || ratio != 1.1 {
		t.Fatalf("observed ratio = %v/%v, want 1.1/true", ratio, ok)
	}
	// Readings that cannot be trusted are refused instead of pricing anything.
	if _, ok := ObservedGroupRatio(10, 21, 0); ok {
		t.Fatal("a zero cost basis must not produce a ratio")
	}
	if _, ok := ObservedGroupRatio(10, 10, 5); ok {
		t.Fatal("no provider spend must not produce a ratio")
	}
	if _, ok := ObservedGroupRatio(0, 1000, 1); ok {
		t.Fatal("an absurd ratio must be refused")
	}

	// The correction moves the rate to what the provider is measured to charge —
	// in BOTH directions (a one-way floor could never repair a wrong assumption).
	if got := ApplyObservedCorrection(1.0, 1.15); got != 1.15 {
		t.Fatalf("a dearer reading must lift 1.0 to 1.15, got %v", got)
	}
	if got := ApplyObservedCorrection(1.0, 0.088); got != 0.088 {
		t.Fatalf("a cheaper reading must lower the rate, got %v", got)
	}
	if got := ApplyObservedCorrection(1.1, 0.5); got != 0.55 {
		t.Fatalf("the correction must scale the ratio, got %v", got)
	}
	if got := ApplyObservedCorrection(1.1, 0); got != 1.1 {
		t.Fatalf("no observation must leave the price alone, got %v", got)
	}
	if got := ApplyObservedCorrection(1.1, 1); got != 1.1 {
		t.Fatalf("a neutral reading must leave the price alone, got %v", got)
	}
	if got := ApplyObservedCorrection(1.1, MaxObservedCorrection*2); got != 1.1 {
		t.Fatalf("an out-of-range correction must be ignored, got %v", got)
	}
	// A realistic repair: the model is advertised in a group priced 0.088 while the
	// key is really billed at 1.0 — roughly 11x, and it must still be usable.
	if got := ApplyObservedCorrection(0.088, 11.36); got < 0.999 || got > 1.001 {
		t.Fatalf("an 11x mispricing must be repairable, got %v", got)
	}
}

func TestResolveModelGroupRatio(t *testing.T) {
	ratios := map[string]float64{"kiro": 0.12, "Claude max企业": 1.4}

	// No usable detected group ⇒ the cheapest group the MODEL is enabled for.
	if got := ResolveModelGroupRatio(ratios, TierSmartName, DefaultProviderGroupName, 0.12); got != 0.12 {
		t.Fatalf("cheapest enabled group = %v, want 0.12", got)
	}
	if got := ResolveModelGroupRatio(ratios, TierFastName, "", 0.12); got != 0.12 {
		t.Fatalf("fast must use the same per-model basis, got %v", got)
	}
	// A group the provider really reported wins over the cheapest advertised one.
	if got := ResolveModelGroupRatio(ratios, TierSmartName, "Claude max企业", 0.12); got != 1.4 {
		t.Fatalf("a detected group must win, got %v", got)
	}
	// Nothing known at all ⇒ the official rate (never cheaper than reality).
	if got := ResolveModelGroupRatio(ratios, TierSmartName, DefaultProviderGroupName, 0); got != DefaultSmartGroupRatio {
		t.Fatalf("unknown group = %v, want %v", got, DefaultSmartGroupRatio)
	}
	if got := ResolveModelGroupRatio(nil, TierFastName, "", 0); got != DefaultFastGroupRatio {
		t.Fatalf("unknown fast group = %v, want %v", got, DefaultFastGroupRatio)
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
	// The `default` placeholder is NOT a pin: it must let the detected group
	// decide instead of shadowing it.
	defaultPin := TierGroupNames{Smart: "default", Detected: "Azure-Gpt-6"}
	if got := defaultPin.For(runtime.TierSmart); got != "Azure-Gpt-6" {
		t.Fatalf("default pin must defer to the detected group, got %q", got)
	}
	if got := defaultPin.For(runtime.TierFast); got != "Azure-Gpt-6" {
		t.Fatalf("fast with a default pin must defer to the detected group, got %q", got)
	}
	if got := (TierGroupNames{Fast: "default", Smart: "Azure-Gpt-1"}).For(runtime.TierFast); got != "Azure-Gpt-1" {
		t.Fatalf("a default fast pin must fall back to the smart pin, got %q", got)
	}
	// ... and with nothing detected it stays the official-rate group.
	if got := (TierGroupNames{Smart: "default"}).For(runtime.TierSmart); got != DefaultProviderGroupName {
		t.Fatalf("default pin without detection = %q, want %q", got, DefaultProviderGroupName)
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

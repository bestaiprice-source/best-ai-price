package yunwu

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/runtime"
)

func TestObservedRatioEnabledIsOptIn(t *testing.T) {
	cases := []struct {
		value any
		want  bool
	}{
		{nil, false},
		{true, true},
		{false, false},
		{"true", true},
		{"TRUE", true},
		{" on ", true},
		{"1", true},
		{"yes", true},
		{"no", false},
		{"", false},
		{"maybe", false},
		{1, true},
		{0, false},
		{1.0, true},
		{0.0, false},
	}
	for _, tc := range cases {
		extra := map[string]any{}
		if tc.value != nil {
			extra[ExtraObservedRatioEnabled] = tc.value
		}
		if got := ObservedRatioEnabled(extra); got != tc.want {
			t.Fatalf("ObservedRatioEnabled(%v) = %v, want %v", tc.value, got, tc.want)
		}
	}
	if ObservedRatioEnabled(nil) {
		t.Fatal("a node without extra must not be opted in")
	}
}

// The measurement needs two readings of the provider's own counter and our cost
// basis for the same interval; everything else is refused instead of becoming a
// price.
func TestMeasureObservedCorrectionGuards(t *testing.T) {
	if _, ok := MeasureObservedCorrection(100, 100, 10); ok {
		t.Fatal("no provider spend must not produce a correction")
	}
	if _, ok := MeasureObservedCorrection(100, 120, 0); ok {
		t.Fatal("no cost basis must not produce a correction")
	}
	if _, ok := MeasureObservedCorrection(100, 1000, 1); ok {
		t.Fatal("an absurd correction must be refused")
	}

	// Reality is 40% dearer than the basis we billed at.
	c, ok := MeasureObservedCorrection(100, 114, 10)
	if !ok || c != 1.4 {
		t.Fatalf("correction = %v/%v, want 1.4/true", c, ok)
	}
	// A cheaper reading is still returned (the floor ignores it) — it is evidence
	// about cost, not a licence to cut a price.
	if c, ok := MeasureObservedCorrection(100, 105, 10); !ok || c != 0.5 {
		t.Fatalf("cheap reading = %v/%v, want 0.5/true", c, ok)
	}
}

// Every measurement is PER KEY: the fast slot never borrows the smart value (a
// separate key has its own provider counter), and only a node that serves both
// tiers with one key shares one measurement.
func TestObservedGroupRatioIsPerTier(t *testing.T) {
	smartOnly := map[string]any{ExtraObservedGroupRatio: 1.2}
	if got := ObservedGroupRatioForTier(smartOnly, TierSmartName); got != 1.2 {
		t.Fatalf("smart ratio = %v, want 1.2", got)
	}
	if got := ObservedGroupRatioForTier(smartOnly, TierFastName); got != 0 {
		t.Fatalf("fast must not borrow the smart measurement, got %v", got)
	}

	both := map[string]any{
		ExtraObservedGroupRatio:     1.2,
		ExtraObservedGroupRatioFast: 1.45,
	}
	if got := ObservedGroupRatioForTier(both, TierSmartName); got != 1.2 {
		t.Fatalf("smart ratio = %v, want 1.2", got)
	}
	if got := ObservedGroupRatioForTier(both, TierFastName); got != 1.45 {
		t.Fatalf("fast ratio = %v, want 1.45", got)
	}
	if got := ObservedGroupRatioForTier(both, ""); got != 1.2 {
		t.Fatalf("an unset tier is smart, got %v", got)
	}

	// One key for both tiers: there is only one measurement to share.
	singleKey := map[string]any{ExtraObservedGroupRatio: 1.3, runtime.AccountExtraTieringDisabled: true}
	if got := ObservedGroupRatioForTier(singleKey, TierFastName); got != 1.3 {
		t.Fatalf("single-key node must share its one measurement, got %v", got)
	}
}

func TestObservedExtraKeysAreSeparatedByTier(t *testing.T) {
	smart := observedExtraKeysForTier(TierSmartName)
	fast := observedExtraKeysForTier(TierFastName)
	if smart.Ratio != ExtraObservedGroupRatio || smart.At != ExtraObservedGroupRatioAt ||
		smart.ProviderUsed != ExtraObservedGroupRatioProviderUsedUSD || smart.Basis != ExtraObservedGroupRatioBasisUSD {
		t.Fatalf("smart slots must keep the historical names: %+v", smart)
	}
	if fast.Ratio != ExtraObservedGroupRatio+"_fast" || fast.At != ExtraObservedGroupRatioAt+"_fast" ||
		fast.ProviderUsed != ExtraObservedGroupRatioProviderUsedUSD+"_fast" || fast.Basis != ExtraObservedGroupRatioBasisUSD+"_fast" {
		t.Fatalf("fast must have its own slots: %+v", fast)
	}
	if smart.Ratio == fast.Ratio {
		t.Fatal("the two tiers must not share a measurement slot")
	}
}

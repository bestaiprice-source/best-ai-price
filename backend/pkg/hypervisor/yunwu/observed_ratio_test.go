package yunwu

import "testing"

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

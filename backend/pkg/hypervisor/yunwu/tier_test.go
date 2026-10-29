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

func TestResolveTierGroupRatio(t *testing.T) {
	ratios := map[string]float64{"fast": 0.5, "smart": 0.1}

	// A known tier group wins over the cheapest ratio.
	if r, ok := ResolveTierGroupRatio(ratios, 0.1, runtime.TierFast); !ok || r != 0.5 {
		t.Fatalf("fast ratio = %v/%v, want 0.5/true", r, ok)
	}
	if r, ok := ResolveTierGroupRatio(ratios, 0.1, runtime.TierSmart); !ok || r != 0.1 {
		t.Fatalf("smart ratio = %v/%v, want 0.1/true", r, ok)
	}

	// Provider has no fast group: an EXPLICIT fast request has no price (fail
	// closed) instead of being sold at the smart rate.
	if r, ok := ResolveTierGroupRatio(map[string]float64{"smart": 0.1}, 0.1, runtime.TierFast); ok {
		t.Fatalf("fast ratio without a fast group = %v/%v, want unpriced", r, ok)
	}
	// The DEFAULT (smart) tier keeps the legacy cheapest behaviour.
	if r, ok := ResolveTierGroupRatio(map[string]float64{"promo": 0.05}, 0.05, runtime.TierSmart); !ok || r != 0.05 {
		t.Fatalf("smart fallback ratio = %v/%v, want 0.05/true", r, ok)
	}
	// No group info at all: both tiers are priced at the single rate.
	if r, ok := ResolveTierGroupRatio(nil, 1.0, runtime.TierSmart); !ok || r != 1.0 {
		t.Fatalf("no-group smart ratio = %v/%v, want 1.0/true", r, ok)
	}
	if r, ok := ResolveTierGroupRatio(nil, 1.0, runtime.TierFast); !ok || r != 1.0 {
		t.Fatalf("no-group fast ratio = %v/%v, want 1.0/true", r, ok)
	}
	// The provider prices per group but has no rate for the fast group: unpriced.
	if _, ok := ResolveTierGroupRatio(map[string]float64{"smart": 0.1}, 0.1, runtime.TierFast); ok {
		t.Fatal("fast ratio on a group-priced provider without a fast group must be unpriced")
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

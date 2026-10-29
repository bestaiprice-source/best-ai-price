package runtime

import (
	"context"
	"testing"
)

func TestNormalizeTier(t *testing.T) {
	cases := map[string]string{
		"fast":     TierFast,
		"FAST":     TierFast,
		"  fast  ": TierFast,
		"smart":    TierSmart,
		"Smart":    TierSmart,
		"":         "",
		"turbo":    "",
		"fast-ish": "",
	}
	for input, want := range cases {
		if got := NormalizeTier(input); got != want {
			t.Fatalf("NormalizeTier(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestTierFromGroupNames(t *testing.T) {
	cases := []struct {
		name  string
		names []string
		want  string
	}{
		{"no groups defaults to smart", nil, TierSmart},
		{"unrelated groups default to smart", []string{"yunwu", "default"}, TierSmart},
		{"fast group", []string{"fast"}, TierFast},
		{"smart group", []string{"smart"}, TierSmart},
		{"case insensitive", []string{"Fast"}, TierFast},
		{"primary wins over secondary", []string{"fast", "smart"}, TierFast},
		{"secondary fast still applies", []string{"yunwu", "fast"}, TierFast},
		{"smart before fast keeps smart", []string{"smart", "fast"}, TierSmart},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := TierFromGroupNames(tc.names); got != tc.want {
				t.Fatalf("TierFromGroupNames(%v) = %q, want %q", tc.names, got, tc.want)
			}
		})
	}
}

func TestTierContextRoundTrip(t *testing.T) {
	// Default when unset.
	if got := TierFromContext(context.Background()); got != TierSmart {
		t.Fatalf("default tier = %q, want %q", got, TierSmart)
	}
	// Explicit fast survives.
	ctx := ContextWithTier(context.Background(), "FAST")
	if got := TierFromContext(ctx); got != TierFast {
		t.Fatalf("fast tier = %q, want %q", got, TierFast)
	}
	// Unknown tier is ignored (stays default smart).
	ctx = ContextWithTier(context.Background(), "turbo")
	if got := TierFromContext(ctx); got != TierSmart {
		t.Fatalf("unknown tier = %q, want %q", got, TierSmart)
	}
	// Nil context is tolerated.
	if got := TierFromContext(nil); got != TierSmart {
		t.Fatalf("nil ctx tier = %q, want %q", got, TierSmart)
	}
	if ctx := ContextWithTier(nil, TierFast); ctx != nil {
		t.Fatalf("ContextWithTier(nil) returned %v, want nil", ctx)
	}
}

func TestNodeSupportsTier(t *testing.T) {
	legacy := map[string]any{"api_key": "sk-smart", "base_url": "https://api.openlux.ai/v1"}
	if !NodeSupportsTier(legacy, TierSmart) {
		t.Fatal("legacy node must support the smart tier")
	}
	if NodeSupportsTier(legacy, TierFast) {
		t.Fatal("legacy node must NOT support the fast tier")
	}

	both := map[string]any{"api_key": "sk-smart", "api_key_fast": "sk-fast"}
	if !NodeSupportsTier(both, TierSmart) || !NodeSupportsTier(both, TierFast) {
		t.Fatal("node with both keys must support both tiers")
	}

	// A node that only carries the fast key is fast-only.
	fastOnly := map[string]any{"api_key_fast": "sk-fast"}
	if NodeSupportsTier(fastOnly, TierSmart) {
		t.Fatal("fast-only node must NOT support the smart tier")
	}
	if !NodeSupportsTier(fastOnly, TierFast) {
		t.Fatal("fast-only node must support the fast tier")
	}

	// Alternative legacy field names still count as smart.
	if !NodeSupportsTier(map[string]any{"token": "sk"}, TierSmart) {
		t.Fatal("`token` credential must satisfy the smart tier")
	}
	// An encrypted envelope (value hidden) still counts as configured — only the
	// KEY is inspected.
	if !NodeSupportsTier(map[string]any{"api_key_fast": "enc:v1:deadbeef"}, TierFast) {
		t.Fatal("encrypted fast credential must count as configured")
	}
	// Empty / blank values do not count.
	if NodeSupportsTier(map[string]any{"api_key_fast": "   "}, TierFast) {
		t.Fatal("blank fast credential must not satisfy the fast tier")
	}
}

func TestCredentialFieldForAccount(t *testing.T) {
	both := map[string]any{"api_key": "sk-smart", "api_key_fast": "sk-fast"}
	if got := CredentialFieldForAccount(TierSmart, "api", "yunwu", both); got != CredentialKeySmart {
		t.Fatalf("smart pick = %q, want %q", got, CredentialKeySmart)
	}
	if got := CredentialFieldForAccount(TierFast, "api", "yunwu", both); got != CredentialKeyFast {
		t.Fatalf("fast pick = %q, want %q", got, CredentialKeyFast)
	}
	// Fast request on a node without a fast key must NOT fall back to the smart key.
	if got := CredentialFieldForAccount(TierFast, "api", "yunwu", map[string]any{"api_key": "sk-smart"}); got != "" {
		t.Fatalf("fast pick on smart-only node = %q, want empty", got)
	}
	// A non-yunwu (official) account ignores the tier entirely.
	if got := CredentialFieldForAccount(TierFast, "api", "anthropic", map[string]any{"api_key": "sk-x"}); got != CredentialKeySmart {
		t.Fatalf("official account pick = %q, want %q", got, CredentialKeySmart)
	}
	// OAuth accounts keep access_token first.
	oauth := map[string]any{"access_token": "at", "api_key": "sk"}
	if got := CredentialFieldForAccount(TierSmart, "oauth", "anthropic", oauth); got != "access_token" {
		t.Fatalf("oauth pick = %q, want access_token", got)
	}
}

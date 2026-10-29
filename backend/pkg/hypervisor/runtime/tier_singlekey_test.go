package runtime

import "testing"

// Single-key mode: two keys per node (fast + smart) is a per-provider
// arrangement, not a universal one. These tests pin the two places the flag can
// change behaviour — the credential actually used, and which tiers the node
// advertises (the latter lives in yunwu, tested there).

func TestTieringDisabledReadsBoolAndStringSpellings(t *testing.T) {
	cases := []struct {
		name  string
		extra map[string]any
		want  bool
	}{
		{"nil extra", nil, false},
		{"absent key", map[string]any{"provider": "openlux"}, false},
		{"bool true", map[string]any{AccountExtraTieringDisabled: true}, true},
		{"bool false", map[string]any{AccountExtraTieringDisabled: false}, false},
		{"string true", map[string]any{AccountExtraTieringDisabled: "true"}, true},
		{"string TRUE with spaces", map[string]any{AccountExtraTieringDisabled: " TRUE "}, true},
		{"string 1", map[string]any{AccountExtraTieringDisabled: "1"}, true},
		{"string false", map[string]any{AccountExtraTieringDisabled: "false"}, false},
		{"unrelated type", map[string]any{AccountExtraTieringDisabled: 42}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := TieringDisabled(tc.extra); got != tc.want {
				t.Fatalf("TieringDisabled(%v) = %v, want %v", tc.extra, got, tc.want)
			}
		})
	}
}

func TestCredentialFieldForTieredAccountFastTierWithoutFlagUsesFastKey(t *testing.T) {
	both := map[string]any{"api_key": "smart-key", "api_key_fast": "fast-key"}

	// Without the flag the fast tier keeps its dedicated key (unchanged).
	if got := CredentialFieldForAccountTiered(TierFast, "apikey", "yunwu", both, false); got != CredentialKeyFast {
		t.Fatalf("fast tier must use the fast key, got %q", got)
	}
	// And it must never fall back to the smart key when the fast one is absent.
	smartOnly := map[string]any{"api_key": "smart-key"}
	if got := CredentialFieldForAccountTiered(TierFast, "apikey", "yunwu", smartOnly, false); got != "" {
		t.Fatalf("a fast request must not fall back to the smart key, got %q", got)
	}
}

func TestCredentialFieldForTieredAccountSingleKeyModeIgnoresTier(t *testing.T) {
	both := map[string]any{"api_key": "smart-key", "api_key_fast": "fast-key"}

	// With the flag the node has ONE key: a fast request uses the regular
	// (smart/legacy) field instead of failing on the missing fast key.
	for _, tier := range []string{TierFast, TierSmart, ""} {
		got := CredentialFieldForAccountTiered(tier, "apikey", "yunwu", both, true)
		if got != CredentialKeySmart {
			t.Fatalf("tier %q in single-key mode must use %q, got %q", tier, CredentialKeySmart, got)
		}
	}

	// A node whose only key lives under a legacy field still resolves.
	legacy := map[string]any{"token": "legacy-key"}
	if got := CredentialFieldForAccountTiered(TierFast, "apikey", "yunwu", legacy, true); got != "token" {
		t.Fatalf("expected the legacy field to be found, got %q", got)
	}
}

func TestCredentialFieldForTieredAccountKeepsOAuthPrecedence(t *testing.T) {
	creds := map[string]any{"access_token": "oauth-token", "api_key": "key", "api_key_fast": "fast"}
	if got := CredentialFieldForAccountTiered(TierFast, "oauth", "openai", creds, true); got != "access_token" {
		t.Fatalf("oauth accounts keep access_token first, got %q", got)
	}
}

// The legacy entry point must behave exactly as before the flag existed.
func TestCredentialFieldForAccountStillDelegatesWithTieringEnabled(t *testing.T) {
	both := map[string]any{"api_key": "smart-key", "api_key_fast": "fast-key"}
	if got := CredentialFieldForAccount(TierFast, "apikey", "yunwu", both); got != CredentialKeyFast {
		t.Fatalf("legacy call must keep the fast key, got %q", got)
	}
	if got := CredentialFieldForAccount(TierSmart, "apikey", "yunwu", both); got != CredentialKeySmart {
		t.Fatalf("legacy call must keep the smart key, got %q", got)
	}
}

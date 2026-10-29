package yunwu

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/runtime"
)

// A node in single-key mode must be a candidate for BOTH tiers, while a normal
// two-key node keeps advertising exactly the tiers its credentials cover.

func TestSupportedTiersForAccountWithoutFlagFollowsCredentials(t *testing.T) {
	smartOnly := map[string]any{"api_key": "smart"}
	if got := SupportedTiersForAccount(smartOnly, nil); len(got) != 1 || got[0] != runtime.TierSmart {
		t.Fatalf("smart-only node must advertise just smart, got %v", got)
	}

	both := map[string]any{"api_key": "smart", "api_key_fast": "fast"}
	got := SupportedTiersForAccount(both, map[string]any{"provider": "openlux"})
	if len(got) != 2 {
		t.Fatalf("a two-key node must advertise both tiers, got %v", got)
	}

	// The fast key alone is not a smart node.
	fastOnly := map[string]any{"api_key_fast": "fast"}
	if got := SupportedTiersForAccount(fastOnly, nil); len(got) != 1 || got[0] != runtime.TierFast {
		t.Fatalf("fast-only node must advertise just fast, got %v", got)
	}
}

func TestSupportedTiersForAccountSingleKeyModeAdvertisesBothTiers(t *testing.T) {
	// One key, flag on: the tier must stop influencing this node's routing, so a
	// fast request stays a legitimate candidate instead of being filtered out by
	// NodeSupportsBillingTier.
	extra := map[string]any{runtime.AccountExtraTieringDisabled: true}
	got := SupportedTiersForAccount(map[string]any{"api_key": "one-key"}, extra)
	if len(got) != 2 {
		t.Fatalf("single-key node must advertise both tiers, got %v", got)
	}
	if !NodeSupportsBillingTier(got, runtime.TierFast) || !NodeSupportsBillingTier(got, runtime.TierSmart) {
		t.Fatalf("both tiers must be schedulable on a single-key node, got %v", got)
	}

	// Flag present but false leaves the node on its credential-derived tiers.
	off := map[string]any{runtime.AccountExtraTieringDisabled: false}
	if got := SupportedTiersForAccount(map[string]any{"api_key": "one-key"}, off); len(got) != 1 {
		t.Fatalf("flag off must not change the advertised tiers, got %v", got)
	}
}

// The advertised tiers and the credential the runtime picks must agree: a node
// advertised for the fast tier must be able to serve it.
func TestSingleKeyNodeIsBothAdvertisedAndServableForFast(t *testing.T) {
	creds := map[string]any{"api_key": "one-key"}
	extra := map[string]any{runtime.AccountExtraTieringDisabled: true}

	tiers := SupportedTiersForAccount(creds, extra)
	if !NodeSupportsBillingTier(tiers, runtime.TierFast) {
		t.Fatalf("node must be advertised for fast, got %v", tiers)
	}
	field := runtime.CredentialFieldForAccountTiered(runtime.TierFast, "apikey", "yunwu", creds, runtime.TieringDisabled(extra))
	if field != runtime.CredentialKeySmart {
		t.Fatalf("the advertised fast request must resolve to the node's one key, got %q", field)
	}
}

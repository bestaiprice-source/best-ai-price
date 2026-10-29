package runtime

import (
	"context"
	"strings"
)

// Billing tiers for unofficial (yunwu) providers.
//
// A provider issues its API keys inside two billing groups — "fast" and
// "smart" — which differ in speed and price. BestAIPrice holds BOTH keys on the
// SAME execution node and selects the key that matches the tier of the calling
// API key's group, so a fast request is never served (or billed) with the
// cheaper smart key, and a smart request is never charged the fast rate.
const (
	TierFast  = "fast"
	TierSmart = "smart"
)

// Credential field names. The smart tier keeps the legacy "api_key" field so
// every node provisioned before this feature stays valid.
const (
	CredentialKeySmart = "api_key"
	CredentialKeyFast  = "api_key_fast"
)

// AccountExtraTieringDisabled is the account `extra` flag that puts a node back
// into SINGLE-KEY mode.
//
// Two keys per node (fast + smart) is a per-provider arrangement, not a
// universal one: some upstreams issue one key and price it identically for both
// tiers, so splitting it adds nothing. With this flag set the tier stops
// influencing that node's routing (`yunwu.SupportedTiersForAccount` advertises
// both tiers) and every request uses the one key the node actually has.
const AccountExtraTieringDisabled = "tiering_disabled"

// TieringDisabled reports whether an account's `extra` turns the two-key
// arrangement off. Accepts a real bool plus the string spellings an admin form
// or a hand-edited JSON may produce.
func TieringDisabled(extra map[string]any) bool {
	if extra == nil {
		return false
	}
	switch v := extra[AccountExtraTieringDisabled].(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "1", "yes", "on":
			return true
		}
	}
	return false
}

// smartCredentialFields mirrors the legacy runtime lookup order (see
// AnthropicRuntime.Execute), so a node that stores its key under "key"/"token"/
// "access_token" still counts as a smart node.
var smartCredentialFields = []string{"api_key", "key", "token", "access_token", "setup_token", "session_key"}

// fastCredentialFields lists the fields that satisfy the fast tier. The fast
// tier deliberately has NO fallback: a node without a dedicated fast key must
// never be drained for a fast request.
var fastCredentialFields = []string{CredentialKeyFast}

// NormalizeTier maps a tier name/alias onto TierFast / TierSmart, returning ""
// for anything that is not a known tier.
func NormalizeTier(tier string) string {
	switch strings.ToLower(strings.TrimSpace(tier)) {
	case TierFast:
		return TierFast
	case TierSmart:
		return TierSmart
	default:
		return ""
	}
}

// TierFromGroupNames resolves the request tier from the group names an API key
// is bound to, in order. The tiers are ordinary groups named "fast"/"smart"
// (case-insensitive); the first one wins, so the key's primary group takes
// precedence over its secondary ones. A key carrying no tier group — every key
// in production today — falls back to smart, leaving the existing behaviour
// unchanged.
func TierFromGroupNames(names []string) string {
	for _, name := range names {
		if tier := NormalizeTier(name); tier != "" {
			return tier
		}
	}
	return TierSmart
}

// CredentialFieldsForTier returns the credential fields to try for a tier, in
// order. The fast tier tries only its dedicated field; the smart tier keeps the
// full legacy fallback list.
func CredentialFieldsForTier(tier string) []string {
	if NormalizeTier(tier) == TierFast {
		return fastCredentialFields
	}
	return smartCredentialFields
}

// NodeSupportsTier reports whether a node's (possibly still-encrypted)
// credentials carry a key for the tier. Only the map KEYS are inspected, so the
// answer never requires decrypting the secret.
func NodeSupportsTier(credentials map[string]any, tier string) bool {
	for _, field := range CredentialFieldsForTier(tier) {
		raw, ok := credentials[field]
		if !ok || raw == nil {
			continue
		}
		if s, isString := raw.(string); isString {
			if strings.TrimSpace(s) != "" {
				return true
			}
			continue
		}
		return true
	}
	return false
}

// CredentialFieldForAccount returns the credential field a request must use,
// given the request tier, the account type/platform and the decrypted
// credentials. It returns "" when no matching field is configured.
//
// Rules, in priority order:
//   - unofficial (yunwu) platform + fast tier  -> ONLY the dedicated fast field
//     (never the smart key; the two are priced differently);
//   - OAuth / setup-token accounts             -> access_token first;
//   - everything else                          -> the legacy api_key-first list.
func CredentialFieldForAccount(tier, accountType, platform string, credentials map[string]any) string {
	return CredentialFieldForAccountTiered(tier, accountType, platform, credentials, false)
}

// CredentialFieldForAccountTiered is CredentialFieldForAccount plus the node's
// single-key switch.
//
// With tiering disabled the request tier is ignored and the legacy smart lookup
// is used, so a fast request is served by the node's ONE key instead of failing
// with "tier fast key is not configured for node". The tier still travels with
// the request (pricing is resolved from the primary provider, not from the
// serving node), so only the credential choice changes.
func CredentialFieldForAccountTiered(tier, accountType, platform string, credentials map[string]any, tieringDisabled bool) string {
	fields := smartCredentialFields
	if accountType == "oauth" || accountType == "setup-token" {
		fields = []string{"access_token", "setup_token", "api_key", "key", "token", "session_key"}
	}
	if !tieringDisabled && platform == "yunwu" && NormalizeTier(tier) == TierFast {
		fields = fastCredentialFields
	}
	for _, field := range fields {
		if val, ok := credentials[field].(string); ok && strings.TrimSpace(val) != "" {
			return field
		}
	}
	return ""
}

// tierKey is the context key carrying the resolved request tier.
type tierKey struct{}

// ContextWithTier attaches the resolved billing tier ("fast"/"smart") so the
// engine can filter nodes and the runtime can pick the matching provider key. An
// empty or unknown tier is ignored, leaving the default smart behaviour.
func ContextWithTier(ctx context.Context, tier string) context.Context {
	if ctx == nil {
		return ctx
	}
	if normalized := NormalizeTier(tier); normalized != "" {
		return context.WithValue(ctx, tierKey{}, normalized)
	}
	return ctx
}

// TierFromContext returns the request tier, defaulting to smart when unset.
func TierFromContext(ctx context.Context) string {
	if ctx != nil {
		if v, ok := ctx.Value(tierKey{}).(string); ok {
			if normalized := NormalizeTier(v); normalized != "" {
				return normalized
			}
		}
	}
	return TierSmart
}

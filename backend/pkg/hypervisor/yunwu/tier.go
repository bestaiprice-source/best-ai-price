package yunwu

import (
	"context"
	"errors"
	"strings"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/runtime"
)

// ErrTierNotConfigured is returned when the requested billing tier has no
// candidate node provisioned with a key for it. The engine never silently
// downgrades a fast request onto a smart-priced key (their prices differ), so
// this is a hard, explicit failure instead.
var ErrTierNotConfigured = errors.New("billing tier is not configured on any available provider node")

// SupportedTiersForCredentials lists the billing tiers a node can serve, based
// on which credential fields are present. Only the map KEYS are read, so the
// answer is valid for still-encrypted credentials.
func SupportedTiersForCredentials(credentials map[string]any) []string {
	tiers := make([]string, 0, 2)
	if runtime.NodeSupportsTier(credentials, runtime.TierSmart) {
		tiers = append(tiers, runtime.TierSmart)
	}
	if runtime.NodeSupportsTier(credentials, runtime.TierFast) {
		tiers = append(tiers, runtime.TierFast)
	}
	return tiers
}

// SupportedTiersForAccount lists the billing tiers a node serves, honouring the
// per-node single-key switch.
//
// With tiering disabled the node's ONE key serves BOTH tiers, so the scheduler
// must not exclude it from either: a fast request is a legitimate candidate for
// such a node, and the runtime resolves it to the same key. Without the flag the
// tiers come from the credentials as before.
//
// This is the ONLY place the switch is applied to routing; the runtime applies
// the same flag to the credential choice, so a node can never be advertised for
// a tier it would then fail to serve.
func SupportedTiersForAccount(credentials map[string]any, extra map[string]any) []string {
	if runtime.TieringDisabled(extra) {
		return []string{runtime.TierSmart, runtime.TierFast}
	}
	return SupportedTiersForCredentials(credentials)
}

// NodeSupportsBillingTier reports whether the node's advertised tiers cover the
// requested tier. An empty tier means "no tier restriction".
//
// A node with NO advertised tiers is treated as tier-agnostic (it is allowed)
// rather than excluded: nodes persisted before this feature existed, and nodes
// provisioned outside the Yunwu admin, carry no tier information, and excluding
// them would turn a new gate into an outage. Only a node that explicitly
// declares its tiers is held to them, which is what keeps a fast request off a
// smart-only node (and the reverse).
func NodeSupportsBillingTier(nodeTiers []string, tier string) bool {
	normalized := runtime.NormalizeTier(tier)
	if normalized == "" || len(nodeTiers) == 0 {
		return true
	}
	for _, t := range nodeTiers {
		if t == normalized {
			return true
		}
	}
	return false
}

// TierGroupName returns the upstream provider billing group a tier's key
// belongs to. The provider issues its keys inside two groups named exactly
// "fast" and "smart", so the tier name IS the group name — which keeps the
// catalog (priced per provider) and the charge (resolved per provider) on the
// same rate and avoids a catalog↔billing parity gap.
func TierGroupName(tier string) string {
	if normalized := runtime.NormalizeTier(tier); normalized != "" {
		return normalized
	}
	return runtime.TierSmart
}

// Extra keys that pin the provider billing group an account's keys were issued
// in. They are account-level (`account.Extra`) settings, set in the admin panel
// next to the fast key.
const (
	ExtraSmartGroupName = "smart_group_name"
	ExtraFastGroupName  = "fast_group_name"
)

// TierGroupNames is the provider billing group each of an account's keys lives
// in. Both fields are optional: an empty value keeps the legacy behaviour (the
// smart tier falls back to the provider's cheapest enabled group, the fast tier
// to the literal "fast" group).
//
// This exists because the group a key was issued in decides what the provider
// actually charges us: providers name their groups freely (`Azure-Gpt-1`, `kiro`,
// `Claude-满血`, or a group the pricing payload does not even list, which the
// provider bills at the official rate). Deriving the tier's rate from the tier
// NAME alone silently mispriced every account whose keys are not in a group
// literally called `fast`.
type TierGroupNames struct {
	Smart string
	Fast  string
}

// ParseTierGroupNames reads the pinned groups from an account's extra map.
func ParseTierGroupNames(extra map[string]any) TierGroupNames {
	return TierGroupNames{
		Smart: extraString(extra, ExtraSmartGroupName),
		Fast:  extraString(extra, ExtraFastGroupName),
	}
}

// For returns the pinned group name for a tier. The fast tier falls back to the
// smart group: providers commonly issue both keys inside the same group, so
// configuring it once is enough.
func (g TierGroupNames) For(tier string) string {
	if runtime.NormalizeTier(tier) == runtime.TierFast {
		if g.Fast != "" {
			return g.Fast
		}
		return g.Smart
	}
	return g.Smart
}

// IsZero reports whether nothing is pinned, i.e. the legacy derivation applies.
func (g TierGroupNames) IsZero() bool {
	return g.Smart == "" && g.Fast == ""
}

// Merge keeps the receiver's values and fills the gaps from other — used when
// deriving the provider-level group names of a provider's accounts (the first
// configured value wins).
func (g TierGroupNames) Merge(other TierGroupNames) TierGroupNames {
	out := g
	if out.Smart == "" {
		out.Smart = other.Smart
	}
	if out.Fast == "" {
		out.Fast = other.Fast
	}
	return out
}

// Map renders the pinned groups as a tier→group-name map, skipping empty ones.
// The scheduler's node struct carries the groups this way.
func (g TierGroupNames) Map() map[string]string {
	out := make(map[string]string, 2)
	if g.Smart != "" {
		out[runtime.TierSmart] = g.Smart
	}
	if g.Fast != "" {
		out[runtime.TierFast] = g.Fast
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ParseTierGroupMap reads a node's tier→group-name map (empty string values are
// dropped).
func ParseTierGroupMap(raw map[string]string) TierGroupNames {
	if len(raw) == 0 {
		return TierGroupNames{}
	}
	out := TierGroupNames{}
	for tier, name := range raw {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		switch runtime.NormalizeTier(tier) {
		case runtime.TierFast:
			out.Fast = name
		case runtime.TierSmart:
			out.Smart = name
		}
	}
	return out
}

// GroupNameForTier returns the pinned group for a tier out of a node's
// tier→group-name map, with the fast→smart fallback of TierGroupNames.For.
func GroupNameForTier(groups map[string]string, tier string) string {
	return ParseTierGroupMap(groups).For(tier)
}

func extraString(extra map[string]any, key string) string {
	if len(extra) == 0 {
		return ""
	}
	raw, ok := extra[key]
	if !ok || raw == nil {
		return ""
	}
	if s, isString := raw.(string); isString {
		return strings.TrimSpace(s)
	}
	return ""
}

// ResolveTierGroupRatioForGroup resolves the discount multiplier for a tier,
// honouring the provider group the account's key was pinned to.
//
// With a pinned group the answer mirrors how the provider itself bills that key:
//
//   - the group's ratio when the provider publishes one;
//   - the OFFICIAL rate (1.0) when the provider prices per group but does not
//     list the pinned group — that is what One-API/New-API charge for a group
//     missing from `group_ratio` (and what a group named `default` gets);
//   - the provider's single rate when it does not price per group at all.
//
// Without a pinned group the legacy behaviour applies (see ResolveTierGroupRatio).
func ResolveTierGroupRatioForGroup(groupRatios map[string]float64, cheapest float64, tier, groupName string) (float64, bool) {
	groupName = strings.TrimSpace(groupName)
	if groupName == "" {
		return ResolveTierGroupRatio(groupRatios, cheapest, tier)
	}
	if len(groupRatios) == 0 {
		// The provider does not price per group: its single rate already is the
		// price of whatever key we hold.
		if cheapest > 0 {
			return cheapest, true
		}
		return 1.0, true
	}
	if ratio, ok := groupRatios[groupName]; ok && ratio > 0 {
		return ratio, true
	}
	// Pinned group is not listed: the provider bills it at the official rate.
	return 1.0, true
}

// ResolveTierGroupRatio picks the discount multiplier (group_ratio) a request
// must be priced with:
//
//   - when the provider exposes a ratio for the tier's group, that ratio is the
//     real cost of the key we are about to use, so it wins;
//   - otherwise the DEFAULT (smart) tier keeps the legacy cheapest-enabled-group
//     behaviour, so existing traffic is priced exactly as before;
//   - an EXPLICIT non-default tier (fast) with no known ratio for its group
//     yields no price at all (ok=false): the model is then treated as
//     unavailable for that tier, and the engine fails / the catalog omits it —
//     a fast request is never silently sold at a smart rate.
func ResolveTierGroupRatio(groupRatios map[string]float64, cheapest float64, tier string) (float64, bool) {
	normalized := runtime.NormalizeTier(tier)
	if normalized == "" {
		normalized = runtime.TierSmart
	}
	// The provider does not price per group at all (BestAIPrice / channels
	// formats): both tiers share the single rate, so any tier can be priced.
	if len(groupRatios) == 0 {
		if cheapest > 0 {
			return cheapest, true
		}
		return 1.0, true
	}
	if r, ok := groupRatios[TierGroupName(normalized)]; ok && r > 0 {
		return r, true
	}
	if normalized == runtime.TierSmart {
		// Legacy/default fallback: cheapest enabled group.
		if cheapest > 0 {
			return cheapest, true
		}
		return 1.0, true
	}
	// The provider prices per group but exposes no rate for this tier's group:
	// no price (fail closed) instead of another group's rate.
	return 0, false
}

// TierFromContext returns the billing tier of the current request (default
// smart). Thin re-export of runtime.TierFromContext.
func TierFromContext(ctx context.Context) string {
	return runtime.TierFromContext(ctx)
}

// ContextWithTier attaches the resolved billing tier to the request context.
// Thin re-export of runtime.ContextWithTier so gateway/service code only needs to
// import this package.
func ContextWithTier(ctx context.Context, tier string) context.Context {
	return runtime.ContextWithTier(ctx, tier)
}

// TierFromGroupNames resolves the billing tier from the group names an API key
// is bound to. Thin re-export of runtime.TierFromGroupNames.
func TierFromGroupNames(names []string) string {
	return runtime.TierFromGroupNames(names)
}

// NormalizeTier maps a tier name onto TierSmart / TierFast (case-insensitive,
// trimmed) and returns "" for anything else, so callers can tell "unset/invalid"
// from a real tier. Thin re-export of runtime.NormalizeTier.
func NormalizeTier(tier string) string {
	return runtime.NormalizeTier(tier)
}

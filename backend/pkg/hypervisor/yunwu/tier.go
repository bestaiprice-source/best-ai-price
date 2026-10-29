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
	ExtraSmartGroupName    = "smart_group_name"
	ExtraFastGroupName     = "fast_group_name"
	ExtraDetectedGroupName = "detected_group_name"
)

// DefaultProviderGroupName is the group a provider puts a key in when none was
// chosen, and the name we assume when nothing is pinned. One-API/New-API bill a
// group that is missing from their price list at the OFFICIAL rate, so this
// never understates the cost of a request.
const DefaultProviderGroupName = "default"

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
	// Detected is the group discovered from the provider API for this account
	// (`extra.detected_group_name`, written by the balance sync). It is only a
	// best-effort enrichment: an explicit pin always wins, and it is ignored
	// when empty.
	Detected string
}

// ParseTierGroupNames reads the pinned groups from an account's extra map.
func ParseTierGroupNames(extra map[string]any) TierGroupNames {
	return TierGroupNames{
		Smart:    extraString(extra, ExtraSmartGroupName),
		Fast:     extraString(extra, ExtraFastGroupName),
		Detected: extraString(extra, ExtraDetectedGroupName),
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
		if g.Smart != "" {
			return g.Smart
		}
	} else if g.Smart != "" {
		return g.Smart
	}
	if g.Detected != "" {
		return g.Detected
	}
	// Nothing is pinned: the provider's default group. One-API/New-API bill a
	// group that is not in their price list at the official rate, so this is the
	// honest (never cheaper than reality) assumption — pinning the real group
	// lowers the price when the provider really does discount it.
	return DefaultProviderGroupName
}

// IsZero reports whether nothing is pinned, i.e. the legacy derivation applies.
func (g TierGroupNames) IsZero() bool {
	return g.Smart == "" && g.Fast == "" && g.Detected == ""
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
	if out.Detected == "" {
		out.Detected = other.Detected
	}
	return out
}

// NodeTierGroups renders the tier→group-name map a scheduler node carries: both
// tiers resolved (explicit pin > auto-detected group > the provider default), so
// the price lookup never has to guess again.
func NodeTierGroups(extra map[string]any) map[string]string {
	names := ParseTierGroupNames(extra)
	return map[string]string{
		runtime.TierSmart: names.For(runtime.TierSmart),
		runtime.TierFast:  names.For(runtime.TierFast),
	}
}

// GroupNameForTier returns the group a tier bills in from a node's resolved
// tier→group-name map ("default" when the map has no entry for it).
func GroupNameForTier(groups map[string]string, tier string) string {
	if len(groups) > 0 {
		normalized := runtime.NormalizeTier(tier)
		if normalized == "" {
			normalized = runtime.TierSmart
		}
		if name := strings.TrimSpace(groups[normalized]); name != "" {
			return name
		}
	}
	return DefaultProviderGroupName
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

// ResolveTierGroupRatioForGroup resolves the discount multiplier for a tier from
// the provider group the account's key lives in. It mirrors how the provider
// itself bills that key:
//
//   - the group's published ratio when the provider lists it (the real cost of
//     the key we are about to use);
//   - the OFFICIAL rate (1.0) when the group is not listed — that is what
//     One-API/New-API charge for a group missing from `group_ratio`, which is
//     what a group named `default` gets;
//   - 1.0 when the provider does not price per group at all (the caller then
//     uses its single stored rate instead of the ratio).
//
// It never returns "unpriced": a request is always priced at least at the
// official rate, so a key whose group differs from our assumption can only be
// charged MORE than our estimate, never less.
func ResolveTierGroupRatioForGroup(groupRatios map[string]float64, groupName string) float64 {
	groupName = strings.TrimSpace(groupName)
	if groupName == "" {
		groupName = DefaultProviderGroupName
	}
	if len(groupRatios) == 0 {
		return 1.0
	}
	if ratio, ok := groupRatios[groupName]; ok && ratio > 0 {
		return ratio
	}
	// Not listed: the provider bills this group at the official rate.
	return 1.0
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

package yunwu

import (
	"regexp"
	"strings"
)

// This file holds the ONE model-name normalization shared by every component
// that has to agree on "which upstream model is this request about":
//
//   - the gateway's public catalog + billing (internal/service),
//   - the hypervisor's per-node price lookup (ProviderPriceManager),
//   - the upstream dispatch payload (ResolveUpstreamModelID / prepareNodePayload).
//
// It used to live in internal/service only, while the node-level price lookup
// carried its own much smaller alias table and the dispatch path used a
// different vendor-prefix list. A model the catalog priced therefore had no
// price on the routing node and the gateway answered HTTP 400 "model pricing is
// not available" for a model it had just advertised as available with a price
// (e.g. `xai/grok-4` — the prefix was not stripped — or
// `google/gemini-2.0-flash`, priced in the catalog through an alias the node
// lookup did not know). Keeping the rules in ONE place is what makes catalog and
// routing parity structural instead of a coincidence.

var (
	modelDateSnapshotRegex = regexp.MustCompile(`-\d{8}$`)
	modelVersionDashRegex  = regexp.MustCompile(`(\d+)-(\d+)`)
	modelVersionDotRegex   = regexp.MustCompile(`(\d+)\.(\d+)`)
)

// VendorPrefixes are the vendor/slug prefixes the canonical model IDs use
// (see service.ToCanonicalModelID) plus the older upstream spellings we may
// still receive. Both the price lookup and the upstream dispatch strip them.
var VendorPrefixes = []string{
	"models/",
	"anthropic/", "openai/", "google/", "deepseek/", "moonshot/", "moonshotai/",
	"zhipu/", "z-ai/", "qwen/", "alibaba/", "xai/", "minimax/", "meta/",
	"meta-llama/", "mistral/", "bytedance/", "kuaishou/", "cohere/", "other/",
}

// DefaultModelAliases maps a model spelling to the name the upstream providers
// actually price/serve. It is the union of the aliases the gateway catalog and
// the hypervisor node lookup used to carry separately.
var DefaultModelAliases = map[string]string{
	// Google — dated/deprecated spellings the aggregators no longer list.
	"gemini-1.5-flash": "gemini-2.5-flash",
	"gemini-1.5-pro":   "gemini-2.5-pro",
	"gemini-2.0-flash": "gemini-2.5-flash",
	"gemini-3-pro":     "gemini-3-pro-preview",
	"gemini-3.1-pro":   "gemini-3.1-pro-preview",
	"gemini-3-flash":   "gemini-3-flash-preview",

	// DeepSeek — legacy marketing names → concrete upstream ids.
	"deepseek-chat":     "deepseek-v3",
	"deepseek-v3":       "deepseek-chat",
	"deepseek-coder":    "deepseek-v3",
	"deepseek-reasoner": "deepseek-r1",
	"deepseek-r1":       "deepseek-reasoner",

	// Anthropic — -latest / undated ↔ dated snapshots (both directions).
	"claude-3-5-sonnet":          "claude-3-5-sonnet-20241022",
	"claude-3-5-sonnet-latest":   "claude-3-5-sonnet",
	"claude-3-5-sonnet-20241022": "claude-3-5-sonnet",
	"claude-3-7-sonnet":          "claude-3-7-sonnet-20250219",
	"claude-3-7-sonnet-20250219": "claude-3-7-sonnet",
	"claude-3-opus":              "claude-3-opus-20240229",
	"claude-3-opus-20240229":     "claude-3-opus",
	"claude-3-opus-latest":       "claude-3-opus",
	"claude-3-haiku":             "claude-3-haiku-20240307",
	"claude-3-haiku-20240307":    "claude-3-haiku",
	"claude-3-5-haiku":           "claude-3-5-haiku-20241022",
	"claude-3-5-haiku-latest":    "claude-3-5-haiku",

	// OpenAI
	"gpt-4o-latest": "chatgpt-4o-latest",
}

// DefaultModelAliasesCopy returns a mutable copy of the alias table.
func DefaultModelAliasesCopy() map[string]string {
	m := make(map[string]string, len(DefaultModelAliases))
	for k, v := range DefaultModelAliases {
		m[k] = v
	}
	return m
}

// ModelNameCandidates generates prioritized candidate model names for upstream
// price lookups: exact raw name first, then vendor-prefix stripped, then
// underscore/date-snapshot/dot-dash variants, then alias targets.
//
// It is a pure function because every caller must agree exactly: pricing lookups
// (catalog, billing, per-node routing) and the model-health read model, which
// joins a monitored upstream id such as `claude-3-7-sonnet-20250219` onto the
// catalog's `anthropic/claude-3-7-sonnet`.
func ModelNameCandidates(modelName string, aliases map[string]string) []string {
	raw := strings.ToLower(strings.TrimSpace(modelName))
	if raw == "" {
		return nil
	}

	seen := make(map[string]bool)
	var candidates []string

	add := func(name string) {
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "" && !seen[name] {
			seen[name] = true
			candidates = append(candidates, name)
		}
	}

	// 1. Exact raw model
	add(raw)

	// 2. Strip slash prefixes (e.g. "google/gemini-2.5-flash" -> "gemini-2.5-flash",
	// "models/gemini-1.5-pro" -> "gemini-1.5-pro")
	if strings.Contains(raw, "/") {
		parts := strings.Split(raw, "/")
		add(parts[len(parts)-1])
	}
	// 3. Trimming the prefix from the raw spelling only works while the vendor
	// part is intact; walk the prefix set against the raw name.
	for _, p := range VendorPrefixes {
		if strings.HasPrefix(raw, p) {
			add(strings.TrimPrefix(raw, p))
		}
	}

	// 4. Date snapshots, underscore replacements & dot/dash version variations
	initialLen := len(candidates)
	for i := 0; i < initialLen; i++ {
		c := candidates[i]
		if strings.Contains(c, "_") {
			add(strings.ReplaceAll(c, "_", "-"))
			add(strings.ReplaceAll(c, "_", "."))
		}
		if modelDateSnapshotRegex.MatchString(c) {
			add(modelDateSnapshotRegex.ReplaceAllString(c, ""))
		}
		if modelVersionDashRegex.MatchString(c) {
			add(modelVersionDashRegex.ReplaceAllString(c, "$1.$2"))
		}
		if modelVersionDotRegex.MatchString(c) {
			add(modelVersionDotRegex.ReplaceAllString(c, "$1-$2"))
		}
		add(strings.ReplaceAll(c, "-", "."))
		add(strings.ReplaceAll(c, ".", "-"))
	}

	// 5. Aliases
	if len(aliases) == 0 {
		aliases = DefaultModelAliases
	}
	aliasLen := len(candidates)
	for i := 0; i < aliasLen; i++ {
		c := candidates[i]
		if target, ok := aliases[c]; ok && target != "" {
			tClean := strings.ToLower(strings.TrimSpace(target))
			add(tClean)
			if modelVersionDashRegex.MatchString(tClean) {
				add(modelVersionDashRegex.ReplaceAllString(tClean, "$1.$2"))
			}
			if modelVersionDotRegex.MatchString(tClean) {
				add(modelVersionDotRegex.ReplaceAllString(tClean, "$1-$2"))
			}
		}
	}

	return candidates
}

// MatchModelName walks the candidates of a requested model and returns the first
// one for which known(name) reports true — i.e. the name the price table uses,
// which is also the name the provider serves upstream.
func MatchModelName(modelName string, aliases map[string]string, known func(string) bool) (string, bool) {
	if known == nil {
		return "", false
	}
	for _, c := range ModelNameCandidates(modelName, aliases) {
		if known(c) {
			return c, true
		}
	}
	return "", false
}

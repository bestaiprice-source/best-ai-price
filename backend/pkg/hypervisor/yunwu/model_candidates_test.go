package yunwu

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The catalog (internal/service) and the per-node price lookup must resolve a
// requested model to the same provider-side name. These cases pin the shared
// normalization that makes that hold.

func TestModelNameCandidates_RealWorldSpellings(t *testing.T) {
	contains := func(list []string, want string) bool {
		for _, v := range list {
			if v == want {
				return true
			}
		}
		return false
	}

	cases := []struct {
		model      string
		mustHave   []string
		mustNotHav string
	}{
		// The live bug report: the vendor prefix was not stripped for the node
		// price lookup, so a priced `xai/grok-4` was rejected with HTTP 400.
		{model: "xai/grok-4", mustHave: []string{"grok-4", "xai/grok-4"}},
		{model: "other/mimo-v2.5", mustHave: []string{"mimo-v2.5"}},
		{model: "other/veo_3_1-fast", mustHave: []string{"veo_3_1-fast", "veo-3-1-fast", "veo.3.1-fast"}},
		// Dot/dash spelling variants (catalog publishes `google/gemini-2-5-flash`,
		// the providers list `gemini-2.5-flash`).
		{model: "google/gemini-2-5-flash", mustHave: []string{"gemini-2.5-flash"}},
		{model: "google/gemini-3-5-flash", mustHave: []string{"gemini-3.5-flash"}},
		{model: "moonshot/kimi-k2-6", mustHave: []string{"kimi-k2.6"}},
		{model: "zhipu/glm-5-2", mustHave: []string{"glm-5.2"}},
		{model: "deepseek/deepseek-v3.1", mustHave: []string{"deepseek-v3-1"}},
		// Alias (an alias the node lookup did not carry before).
		{model: "google/gemini-2.0-flash", mustHave: []string{"gemini-2.5-flash"}},
		{model: "claude-3-5-sonnet-latest", mustHave: []string{"claude-3-5-sonnet"}},
		// Date snapshots are stripped so a monitored dated id joins the catalog id.
		{model: "anthropic/claude-3-7-sonnet-20250219", mustHave: []string{"claude-3-7-sonnet"}},
	}
	for _, tc := range cases {
		got := ModelNameCandidates(tc.model, nil)
		for _, want := range tc.mustHave {
			assert.Truef(t, contains(got, want), "candidates(%q) = %v, missing %q", tc.model, got, want)
		}
	}

	// The exact spelling always comes first, so a provider that serves the
	// requested name verbatim keeps it.
	assert.Equal(t, "claude-sonnet-5", ModelNameCandidates("claude-sonnet-5", nil)[0])
	assert.Equal(t, "google/gemini-2-5-flash", ModelNameCandidates("google/gemini-2-5-flash", nil)[0])
	assert.Empty(t, ModelNameCandidates("   ", nil))
}

func TestMatchModelName(t *testing.T) {
	// A provider table listing the provider's own names.
	table := map[string]bool{"gemini-2.5-flash": true, "grok-4": true, "claude-sonnet-5": true}

	matched, ok := MatchModelName("google/gemini-2-5-flash", nil, func(n string) bool { return table[n] })
	assert.True(t, ok)
	assert.Equal(t, "gemini-2.5-flash", matched)

	matched, ok = MatchModelName("xai/grok-4", nil, func(n string) bool { return table[n] })
	assert.True(t, ok)
	assert.Equal(t, "grok-4", matched)

	// Exact match wins over any variant.
	matched, ok = MatchModelName("claude-sonnet-5", nil, func(n string) bool { return table[n] })
	assert.True(t, ok)
	assert.Equal(t, "claude-sonnet-5", matched)

	_, ok = MatchModelName("totally-unknown-model", nil, func(n string) bool { return table[n] })
	assert.False(t, ok)
	_, ok = MatchModelName("grok-4", nil, nil)
	assert.False(t, ok)
}

func TestStripVendorPrefixCoversCatalogVendors(t *testing.T) {
	assert.Equal(t, "grok-4", StripVendorPrefix("xai/grok-4"))
	assert.Equal(t, "mimo-v2.5", StripVendorPrefix("other/mimo-v2.5"))
	assert.Equal(t, "seedance-1-0", StripVendorPrefix("bytedance/seedance-1-0"))
	assert.Equal(t, "kling-v2", StripVendorPrefix("kuaishou/kling-v2"))
	assert.Equal(t, "gemini-2.5-flash", StripVendorPrefix("google/gemini-2.5-flash"))
	assert.Equal(t, "gemini-2.5-flash", StripVendorPrefix("models/gemini-2.5-flash"))
	// Unknown prefixes are preserved (some aggregators serve vendor-prefixed ids).
	assert.Equal(t, "acme/thing", StripVendorPrefix("acme/thing"))
}

func TestDefaultModelAliasesCopyIsIndependent(t *testing.T) {
	cp := DefaultModelAliasesCopy()
	cp["xai/grok-4"] = "mutated"
	assert.NotEqual(t, "mutated", DefaultModelAliases["xai/grok-4"])
	assert.Equal(t, DefaultModelAliases["gemini-2.0-flash"], cp["gemini-2.0-flash"])
	// Empty/nil alias tables fall back to the defaults, never to no aliases.
	assert.Contains(t, ModelNameCandidates("gemini-2.0-flash", nil), "gemini-2.5-flash")
	assert.Contains(t, ModelNameCandidates("gemini-2.0-flash", map[string]string{}), "gemini-2.5-flash")
}

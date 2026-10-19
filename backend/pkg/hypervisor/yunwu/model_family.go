package yunwu

import (
	"strings"
)

const (
	FamilyClaude = "claude"
	FamilyGPT    = "gpt"
	FamilyGemini = "gemini"
	FamilyKimi   = "kimi"
	FamilyOther  = "other"

	DefaultProvider = "openlux"

	ProviderOpenLux     = "openlux"
	ProviderPortdan     = "portdan"
	ProviderAPITop      = "api-top"
	ProviderYuanqiaoyun = "yuanqiaoyun"
	ProviderXLLM        = "x-llm"

	DefaultOpenLuxBaseURL     = "https://api.openlux.ai/v1"
	DefaultPortdanBaseURL     = "https://portdan.com/v1"
	DefaultAPITopBaseURL      = "https://api-top.com/v1"
	DefaultYuanqiaoyunBaseURL = "https://yuanqiaoyun.com/v1"
	DefaultXLLMBaseURL        = "https://x-llm.net/v1"
)

// DefaultBaseURLForProvider returns the standard base URL for a given provider.
func DefaultBaseURLForProvider(provider string) string {
	switch CanonicalProvider(provider) {
	case ProviderPortdan:
		return DefaultPortdanBaseURL
	case ProviderAPITop:
		return DefaultAPITopBaseURL
	case ProviderYuanqiaoyun:
		return DefaultYuanqiaoyunBaseURL
	case ProviderXLLM:
		return DefaultXLLMBaseURL
	default:
		return DefaultOpenLuxBaseURL
	}
}

// CanonicalProvider normalizes provider aliases.
func CanonicalProvider(provider string) string {
	p := strings.ToLower(strings.TrimSpace(provider))
	switch p {
	case "openlux", "yunwu", "api.openlux.ai":
		return ProviderOpenLux
	case "portdan", "portdan.com", "dandan":
		return ProviderPortdan
	case "api-top", "apitop", "api_top", "api-top.com", "top-api":
		return ProviderAPITop
	case "yuanqiaoyun", "yuanqiao", "yuanqiaoyun.com":
		return ProviderYuanqiaoyun
	case "x-llm", "x-llm.net", "xllm":
		return ProviderXLLM
	default:
		if p == "" {
			return DefaultProvider
		}
		return p
	}
}

// DefaultModelFamilies returns all supported model families by default.
func DefaultModelFamilies() []string {
	return []string{FamilyClaude, FamilyGPT, FamilyGemini, FamilyKimi, FamilyOther}
}

// CanonicalFamily normalizes known aliases to one of the 5 standard families.
func CanonicalFamily(family string) string {
	f := strings.ToLower(strings.TrimSpace(family))
	switch f {
	case "claude", "anthropic":
		return FamilyClaude
	case "gpt", "openai", "chatgpt":
		return FamilyGPT
	case "gemini", "google":
		return FamilyGemini
	case "kimi", "moonshot":
		return FamilyKimi
	case "other", "others", "misc",
		// Vendor names that classify as the "other" family (see ClassifyModelFamily).
		"deepseek", "qwen", "llama", "mistral", "codestral", "grok", "xai",
		"minimax", "zhipu", "glm", "chatglm", "meta", "bytedance", "doubao",
		"seedance", "kuaishou", "kling":
		return FamilyOther
	default:
		return f
	}
}

// ClassifyModelFamily determines which model family a given model name belongs to.
func ClassifyModelFamily(model string) string {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return FamilyOther
	}

	// Strip provider prefix if present (e.g. "anthropic/claude-3-5-sonnet" -> "claude-3-5-sonnet")
	if idx := strings.LastIndex(m, "/"); idx != -1 {
		prefix := m[:idx]
		modelSuffix := m[idx+1:]
		if strings.Contains(prefix, "anthropic") {
			return FamilyClaude
		}
		if strings.Contains(prefix, "openai") {
			return FamilyGPT
		}
		if strings.Contains(prefix, "google") {
			return FamilyGemini
		}
		if strings.Contains(prefix, "moonshot") || strings.Contains(prefix, "kimi") {
			return FamilyKimi
		}
		m = modelSuffix
	}

	// Claude / Anthropic
	if strings.HasPrefix(m, "claude") {
		return FamilyClaude
	}

	// GPT / OpenAI
	if strings.HasPrefix(m, "gpt") ||
		strings.HasPrefix(m, "o1") ||
		strings.HasPrefix(m, "o3") ||
		strings.HasPrefix(m, "chatgpt") ||
		strings.HasPrefix(m, "text-") ||
		strings.HasPrefix(m, "dall-e") ||
		strings.HasPrefix(m, "whisper") ||
		strings.HasPrefix(m, "tts-") ||
		strings.HasPrefix(m, "davinci") ||
		strings.HasPrefix(m, "curie") ||
		strings.HasPrefix(m, "babbage") ||
		strings.HasPrefix(m, "ada") {
		return FamilyGPT
	}

	// Gemini / Google
	if strings.HasPrefix(m, "gemini") ||
		strings.HasPrefix(m, "imagen") ||
		strings.HasPrefix(m, "veo") ||
		strings.HasPrefix(m, "lyria") ||
		strings.HasPrefix(m, "chirp") {
		return FamilyGemini
	}

	// Kimi / Moonshot
	if strings.HasPrefix(m, "kimi") ||
		strings.HasPrefix(m, "moonshot") {
		return FamilyKimi
	}

	// All other models (e.g. DeepSeek, Qwen, LLaMA, Mistral, Grok)
	return FamilyOther
}

// NodeSupportsFamily checks whether a node's capabilities slice includes the given model family.
// If capabilities is empty, or contains "all" or "*", it is considered compatible with everything.
func NodeSupportsFamily(capabilities []string, family string) bool {
	if len(capabilities) == 0 {
		return true
	}
	target := CanonicalFamily(family)
	for _, cap := range capabilities {
		norm := CanonicalFamily(cap)
		if norm == "all" || norm == "*" || norm == target {
			return true
		}
	}
	return false
}

// ParseNodeModelFamilies extracts the configured model families from account Extra metadata.
// Returns DefaultModelFamilies() if not configured or empty.
func ParseNodeModelFamilies(extra map[string]any) []string {
	if extra == nil {
		return DefaultModelFamilies()
	}

	raw, ok := extra["supported_model_families"]
	if !ok || raw == nil {
		return DefaultModelFamilies()
	}

	var parsed []string
	switch v := raw.(type) {
	case []string:
		parsed = v
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				parsed = append(parsed, strings.TrimSpace(s))
			}
		}
	case string:
		for _, s := range strings.Split(v, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				parsed = append(parsed, s)
			}
		}
	}

	if len(parsed) == 0 {
		return DefaultModelFamilies()
	}

	var result []string
	seen := make(map[string]bool)
	for _, f := range parsed {
		canon := CanonicalFamily(f)
		if canon != "" && !seen[canon] {
			seen[canon] = true
			result = append(result, canon)
		}
	}

	if len(result) == 0 {
		return DefaultModelFamilies()
	}
	return result
}

// ParseNodeProvider extracts the upstream provider from account Extra metadata (defaulting to "openlux").
func ParseNodeProvider(extra map[string]any) string {
	if extra == nil {
		return DefaultProvider
	}
	if p, ok := extra["provider"].(string); ok && strings.TrimSpace(p) != "" {
		return CanonicalProvider(p)
	}
	return DefaultProvider
}

// CanonicalModelAliases maps vendor-prefixed or alias model IDs to canonical base IDs.
var CanonicalModelAliases = map[string]string{
	"deepseek/deepseek-chat":               "deepseek-chat",
	"deepseek/deepseek-reasoner":           "deepseek-reasoner",
	"deepseek-chat":                        "deepseek-chat",
	"deepseek-reasoner":                    "deepseek-reasoner",
	"anthropic/claude-3-7-sonnet":          "claude-3-7-sonnet",
	"anthropic/claude-3-7-sonnet-20250219": "claude-3-7-sonnet",
	"anthropic/claude-3.7-sonnet":          "claude-3-7-sonnet",
	"claude-3.7-sonnet":                    "claude-3-7-sonnet",
	"anthropic/claude-3-5-sonnet":          "claude-3-5-sonnet",
	"anthropic/claude-3-5-sonnet-20241022": "claude-3-5-sonnet",
	"anthropic/claude-3.5-sonnet":          "claude-3-5-sonnet",
	"claude-3.5-sonnet":                    "claude-3-5-sonnet",
	"anthropic/claude-3-5-haiku":           "claude-3-5-haiku",
	"anthropic/claude-3-opus":              "claude-3-opus",
	"anthropic/claude-3-haiku":             "claude-3-haiku",
	"openai/gpt-4o":                        "gpt-4o",
	"openai/gpt-4o-mini":                   "gpt-4o-mini",
	"openai/o1":                            "o1",
	"openai/o1-mini":                       "o1-mini",
	"openai/o3-mini":                       "o3-mini",
	"openai/chatgpt-4o-latest":             "chatgpt-4o-latest",
	"google/gemini-2.5-pro":                "gemini-2.5-pro",
	"google/gemini-2.5-flash":              "gemini-2.5-flash",
	"google/gemini-2.0-flash":              "gemini-2.0-flash",
	"google/gemini-1.5-pro":                "gemini-1.5-pro",
	"google/gemini-1.5-flash":              "gemini-1.5-flash",
	"google/gemini-3-pro":                  "gemini-3-pro",
	"google/gemini-3.1-pro":                "gemini-3.1-pro",
	"google/gemini-3-flash":                "gemini-3-flash",
	"moonshot/kimi-k1.5":                   "kimi-k1.5",
}

// DefaultUpstreamModelMapping defines mappings from canonical models to upstream provider models.
var DefaultUpstreamModelMapping = map[string]string{
	"deepseek-chat":     "deepseek-v3",
	"deepseek-reasoner": "deepseek-r1",
	"gemini-3-pro":      "gemini-3-pro-preview",
	"gemini-3.1-pro":    "gemini-3.1-pro-preview",
	"gemini-3-flash":    "gemini-3-flash-preview",
}

// ProviderUpstreamModelMapping allows provider-specific upstream overrides.
var ProviderUpstreamModelMapping = map[string]map[string]string{
	ProviderOpenLux: {
		"deepseek-chat":     "deepseek-v3",
		"deepseek-reasoner": "deepseek-r1",
	},
	ProviderAPITop: {
		"deepseek-chat":     "deepseek-v3",
		"deepseek-reasoner": "deepseek-r1",
	},
	ProviderPortdan: {
		"deepseek-chat":     "deepseek-v3",
		"deepseek-reasoner": "deepseek-r1",
	},
}

// StripVendorPrefix removes standard provider/vendor prefixes such as "anthropic/", "openai/", "google/", "deepseek/", etc.
func StripVendorPrefix(model string) string {
	m := strings.TrimSpace(model)
	if idx := strings.Index(m, "/"); idx != -1 {
		prefix := strings.ToLower(m[:idx])
		switch prefix {
		case "anthropic", "openai", "google", "deepseek", "moonshot", "meta-llama", "qwen", "mistral", "cohere", "minimax", "zhipu":
			return m[idx+1:]
		}
	}
	return m
}

// matchWildcardMapping checks if a requested model matches a pattern (e.g. "anthropic/*" -> "*")
func matchWildcardMapping(pattern, target, model string) (string, bool) {
	if pattern == model {
		return target, true
	}
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*") + "/"
		if strings.HasPrefix(model, prefix) {
			suffix := strings.TrimPrefix(model, prefix)
			if target == "*" {
				return suffix, true
			}
			if strings.HasSuffix(target, "/*") {
				targetPrefix := strings.TrimSuffix(target, "/*") + "/"
				return targetPrefix + suffix, true
			}
			return target, true
		}
	}
	return "", false
}

// ParseAccountModelMapping extracts model mapping table from account Extra metadata.
func ParseAccountModelMapping(extra map[string]any) map[string]string {
	if extra == nil {
		return nil
	}
	raw, ok := extra["model_mapping"]
	if !ok || raw == nil {
		return nil
	}
	switch m := raw.(type) {
	case map[string]string:
		return m
	case map[string]any:
		res := make(map[string]string, len(m))
		for k, v := range m {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				res[k] = strings.TrimSpace(s)
			}
		}
		return res
	}
	return nil
}

// ResolveUpstreamModelID resolves a client-requested model ID to the upstream provider's model ID.
// 1. Checks account-level model_mapping (exact and wildcard)
// 2. Checks provider-specific upstream mapping
// 3. Checks default upstream mapping
// 4. Checks canonical model aliases and strips vendor prefix if needed
func ResolveUpstreamModelID(provider string, requestedModel string, accountModelMapping map[string]string) string {
	m := strings.TrimSpace(requestedModel)
	if m == "" {
		return ""
	}

	// 1. Account-level model_mapping (if configured)
	if len(accountModelMapping) > 0 {
		// Exact match
		if target, ok := accountModelMapping[m]; ok && strings.TrimSpace(target) != "" {
			m = strings.TrimSpace(target)
		} else {
			// Wildcard match
			for pattern, target := range accountModelMapping {
				if mapped, ok := matchWildcardMapping(pattern, target, m); ok {
					m = mapped
					break
				}
			}
		}
	}

	// 2. Provider-specific upstream mapping
	prov := CanonicalProvider(provider)
	if pMap, ok := ProviderUpstreamModelMapping[prov]; ok {
		if upstream, ok := pMap[m]; ok && upstream != "" {
			return upstream
		}
		if upstream, ok := pMap[strings.ToLower(m)]; ok && upstream != "" {
			return upstream
		}
		// Try stripped vendor prefix
		stripped := StripVendorPrefix(m)
		if upstream, ok := pMap[stripped]; ok && upstream != "" {
			return upstream
		}
	}

	// 3. Default upstream mapping
	if upstream, ok := DefaultUpstreamModelMapping[m]; ok && upstream != "" {
		return upstream
	}
	if upstream, ok := DefaultUpstreamModelMapping[strings.ToLower(m)]; ok && upstream != "" {
		return upstream
	}
	stripped := StripVendorPrefix(m)
	if upstream, ok := DefaultUpstreamModelMapping[stripped]; ok && upstream != "" {
		return upstream
	}

	// 4. Canonical model aliases (or stripped vendor prefix)
	if canon, ok := CanonicalModelAliases[strings.ToLower(m)]; ok && canon != "" {
		return canon
	}

	return stripped
}

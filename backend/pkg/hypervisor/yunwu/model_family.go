package yunwu

import (
	"strings"
)

const (
	FamilyClaude = "claude"
	FamilyGPT    = "gpt"
	FamilyGemini = "gemini"
	FamilyOther  = "other"

	DefaultProvider = "openlux"
)

// DefaultModelFamilies returns all supported model families by default.
func DefaultModelFamilies() []string {
	return []string{FamilyClaude, FamilyGPT, FamilyGemini, FamilyOther}
}

// CanonicalFamily normalizes known aliases to one of the 4 standard families.
func CanonicalFamily(family string) string {
	f := strings.ToLower(strings.TrimSpace(family))
	switch f {
	case "claude", "anthropic":
		return FamilyClaude
	case "gpt", "openai", "chatgpt":
		return FamilyGPT
	case "gemini", "google":
		return FamilyGemini
	case "other", "others", "misc":
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
		trimmed := strings.TrimSpace(p)
		if strings.EqualFold(trimmed, "yunwu") {
			return DefaultProvider
		}
		return strings.ToLower(trimmed)
	}
	return DefaultProvider
}

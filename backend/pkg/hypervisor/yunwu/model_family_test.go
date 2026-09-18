package yunwu

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassifyModelFamily(t *testing.T) {
	tests := []struct {
		model    string
		expected string
	}{
		// Claude
		{"claude-3-5-sonnet-20241022", FamilyClaude},
		{"claude-3-7-sonnet", FamilyClaude},
		{"claude-3-opus-20240229", FamilyClaude},
		{"claude-2.1", FamilyClaude},
		{"anthropic/claude-3-5-sonnet", FamilyClaude},

		// GPT / OpenAI
		{"gpt-4o", FamilyGPT},
		{"gpt-4o-mini", FamilyGPT},
		{"gpt-3.5-turbo", FamilyGPT},
		{"o1-preview", FamilyGPT},
		{"o1-mini", FamilyGPT},
		{"o3-mini", FamilyGPT},
		{"chatgpt-4o-latest", FamilyGPT},
		{"text-embedding-3-small", FamilyGPT},
		{"dall-e-3", FamilyGPT},
		{"whisper-1", FamilyGPT},
		{"tts-1-hd", FamilyGPT},
		{"openai/gpt-4o", FamilyGPT},

		// Gemini / Google
		{"gemini-1.5-pro", FamilyGemini},
		{"gemini-2.0-flash-exp", FamilyGemini},
		{"gemini-2.5-flash", FamilyGemini},
		{"imagen-3.0-generate-002", FamilyGemini},
		{"veo-2.0-generate-001", FamilyGemini},
		{"lyria-realtime", FamilyGemini},
		{"chirp-v2", FamilyGemini},
		{"google/gemini-1.5-pro", FamilyGemini},

		// Other / 3rd-party models
		{"deepseek-chat", FamilyOther},
		{"deepseek-reasoner", FamilyOther},
		{"qwen-2.5-72b-instruct", FamilyOther},
		{"llama-3.3-70b-instruct", FamilyOther},
		{"mistral-large-latest", FamilyOther},
		{"grok-2-1212", FamilyOther},
		{"", FamilyOther},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := ClassifyModelFamily(tt.model)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestCanonicalFamily(t *testing.T) {
	assert.Equal(t, FamilyClaude, CanonicalFamily("claude"))
	assert.Equal(t, FamilyClaude, CanonicalFamily("anthropic"))
	assert.Equal(t, FamilyGPT, CanonicalFamily("gpt"))
	assert.Equal(t, FamilyGPT, CanonicalFamily("openai"))
	assert.Equal(t, FamilyGPT, CanonicalFamily("chatgpt"))
	assert.Equal(t, FamilyGemini, CanonicalFamily("gemini"))
	assert.Equal(t, FamilyGemini, CanonicalFamily("google"))
	assert.Equal(t, FamilyOther, CanonicalFamily("other"))
	assert.Equal(t, FamilyOther, CanonicalFamily("others"))
	assert.Equal(t, FamilyOther, CanonicalFamily("misc"))
	assert.Equal(t, "custom", CanonicalFamily("custom"))
}

func TestNodeSupportsFamily(t *testing.T) {
	// Empty capabilities supports everything
	assert.True(t, NodeSupportsFamily(nil, FamilyClaude))
	assert.True(t, NodeSupportsFamily([]string{}, FamilyGPT))
	assert.True(t, NodeSupportsFamily([]string{"*"}, FamilyGemini))
	assert.True(t, NodeSupportsFamily([]string{"all"}, FamilyOther))

	// Specific capabilities
	caps := []string{"claude", "gpt"}
	assert.True(t, NodeSupportsFamily(caps, FamilyClaude))
	assert.True(t, NodeSupportsFamily(caps, FamilyGPT))
	assert.False(t, NodeSupportsFamily(caps, FamilyGemini))
	assert.False(t, NodeSupportsFamily(caps, FamilyOther))

	// Aliases in capabilities
	legacyCaps := []string{"anthropic", "google"}
	assert.True(t, NodeSupportsFamily(legacyCaps, FamilyClaude))
	assert.False(t, NodeSupportsFamily(legacyCaps, FamilyGPT))
	assert.True(t, NodeSupportsFamily(legacyCaps, FamilyGemini))
}

func TestParseNodeModelFamilies(t *testing.T) {
	// Nil extra -> defaults
	assert.Equal(t, DefaultModelFamilies(), ParseNodeModelFamilies(nil))

	// Empty extra -> defaults
	assert.Equal(t, DefaultModelFamilies(), ParseNodeModelFamilies(map[string]any{}))

	// []string in extra
	extraSliceStr := map[string]any{
		"supported_model_families": []string{"claude", "gemini"},
	}
	assert.Equal(t, []string{FamilyClaude, FamilyGemini}, ParseNodeModelFamilies(extraSliceStr))

	// []any (JSON unmarshal output) in extra
	extraSliceAny := map[string]any{
		"supported_model_families": []any{"gpt", "other", "claude"},
	}
	assert.Equal(t, []string{FamilyGPT, FamilyOther, FamilyClaude}, ParseNodeModelFamilies(extraSliceAny))

	// Comma-separated string
	extraCSV := map[string]any{
		"supported_model_families": "claude, gpt, other",
	}
	assert.Equal(t, []string{FamilyClaude, FamilyGPT, FamilyOther}, ParseNodeModelFamilies(extraCSV))

	// Deduplication and normalization
	extraDup := map[string]any{
		"supported_model_families": []any{"anthropic", "claude", "openai", "gpt"},
	}
	assert.Equal(t, []string{FamilyClaude, FamilyGPT}, ParseNodeModelFamilies(extraDup))

	// Empty slices or invalid types
	assert.Equal(t, DefaultModelFamilies(), ParseNodeModelFamilies(map[string]any{"supported_model_families": []string{}}))
	assert.Equal(t, DefaultModelFamilies(), ParseNodeModelFamilies(map[string]any{"supported_model_families": []any{123, nil}}))
	assert.Equal(t, DefaultModelFamilies(), ParseNodeModelFamilies(map[string]any{"supported_model_families": 42}))
}

func TestParseNodeProvider(t *testing.T) {
	assert.Equal(t, "openlux", ParseNodeProvider(nil))
	assert.Equal(t, "openlux", ParseNodeProvider(map[string]any{}))
	assert.Equal(t, "openlux", ParseNodeProvider(map[string]any{"provider": "yunwu"}))
	assert.Equal(t, "openlux", ParseNodeProvider(map[string]any{"provider": "OpenLux"}))
	assert.Equal(t, "portdan", ParseNodeProvider(map[string]any{"provider": "portdan"}))
	assert.Equal(t, "portdan", ParseNodeProvider(map[string]any{"provider": "portdan.com"}))
	assert.Equal(t, "api-top", ParseNodeProvider(map[string]any{"provider": "api-top"}))
	assert.Equal(t, "api-top", ParseNodeProvider(map[string]any{"provider": "apitop"}))
	assert.Equal(t, "yuanqiaoyun", ParseNodeProvider(map[string]any{"provider": "yuanqiaoyun"}))
	assert.Equal(t, "yuanqiaoyun", ParseNodeProvider(map[string]any{"provider": "yuanqiaoyun.com"}))
	assert.Equal(t, "custom-upstream", ParseNodeProvider(map[string]any{"provider": "custom-upstream"}))

	assert.Equal(t, "https://api.openlux.ai/v1", DefaultBaseURLForProvider("openlux"))
	assert.Equal(t, "https://portdan.com/v1", DefaultBaseURLForProvider("portdan"))
	assert.Equal(t, "https://api-top.com/v1", DefaultBaseURLForProvider("api-top"))
	assert.Equal(t, "https://yuanqiaoyun.com/v1", DefaultBaseURLForProvider("yuanqiaoyun"))
	assert.Equal(t, "https://api.openlux.ai/v1", DefaultBaseURLForProvider("unknown-provider"))
	assert.Equal(t, "openlux", CanonicalProvider(""))
}

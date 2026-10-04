package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/gemini"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestAPIKeyAllowsRequestedModel(t *testing.T) {
	apiKey := &service.APIKey{AllowedModels: []string{"gpt-5.4", "gemini-2.5-pro", "anthropic/claude-3-7-sonnet"}}

	if !apiKeyAllowsRequestedModel(apiKey, "gpt-5.4") {
		t.Fatalf("expected exact model to be allowed")
	}
	if !apiKeyAllowsRequestedModel(apiKey, "openai/gpt-5.4") {
		t.Fatalf("expected openai/ prefixed model to be allowed")
	}
	if !apiKeyAllowsRequestedModel(apiKey, "models/gemini-2.5-pro") {
		t.Fatalf("expected models/ prefixed gemini model to be allowed")
	}
	if !apiKeyAllowsRequestedModel(apiKey, "google/gemini-2.5-pro") {
		t.Fatalf("expected google/ prefixed gemini model to be allowed")
	}
	if !apiKeyAllowsRequestedModel(apiKey, "claude-3-7-sonnet") {
		t.Fatalf("expected stripped claude model to be allowed when canonical configured")
	}
	if apiKeyAllowsRequestedModel(apiKey, "gpt-5.5") {
		t.Fatalf("expected non-allowlisted model to be rejected")
	}

	// Wildcard
	wildcardKey := &service.APIKey{AllowedModels: []string{"*"}}
	if !apiKeyAllowsRequestedModel(wildcardKey, "any-model") {
		t.Fatalf("expected wildcard key to allow any model")
	}
}

func TestAPIKeyIsYunwu(t *testing.T) {
	if apiKeyIsYunwu(nil) {
		t.Fatalf("expected nil apiKey to not be yunwu")
	}

	keyWithGroup := &service.APIKey{
		Group: &service.Group{Platform: "yunwu", Name: "Default Yunwu"},
	}
	if !apiKeyIsYunwu(keyWithGroup) {
		t.Fatalf("expected key with yunwu group platform to be yunwu")
	}

	keyWithMultiGroup := &service.APIKey{
		Group: &service.Group{Platform: "openai"},
		Groups: []*service.Group{
			{Platform: "openai"},
			{Platform: "yunwu", Name: "BestAIPrice Yunwu"},
		},
	}
	if !apiKeyIsYunwu(keyWithMultiGroup) {
		t.Fatalf("expected key with yunwu in Groups to be yunwu")
	}

	nonYunwuKey := &service.APIKey{
		Group: &service.Group{Platform: "openai"},
		Groups: []*service.Group{
			{Platform: "openai"},
		},
	}
	if apiKeyIsYunwu(nonYunwuKey) {
		t.Fatalf("expected non-yunwu key to not be yunwu")
	}
}

func TestFilterOpenAIModelsForAPIKey(t *testing.T) {
	apiKey := &service.APIKey{AllowedModels: []string{"gpt-5.4"}}
	models := []openai.Model{
		{ID: "gpt-5.4"},
		{ID: "gpt-5.5"},
	}

	filtered := filterOpenAIModelsForAPIKey(apiKey, models)
	if len(filtered) != 1 || filtered[0].ID != "gpt-5.4" {
		t.Fatalf("unexpected filtered models: %#v", filtered)
	}
}

func TestFilterGeminiModelsForAPIKey(t *testing.T) {
	apiKey := &service.APIKey{AllowedModels: []string{"gemini-2.5-pro"}}
	models := []gemini.Model{
		{Name: "models/gemini-2.5-pro"},
		{Name: "models/gemini-2.5-flash"},
	}

	filtered := filterGeminiModelsForAPIKey(apiKey, models)
	if len(filtered) != 1 || filtered[0].Name != "models/gemini-2.5-pro" {
		t.Fatalf("unexpected filtered models: %#v", filtered)
	}
}

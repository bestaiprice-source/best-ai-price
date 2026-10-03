package service

import (
	"context"
	"math"
	"strings"
)

// PublicModelPricingItem represents an item in the public models pricing catalog (BestAIPrice format).
type PublicModelPricingItem struct {
	CanonicalModelID string   `json:"canonical_model_id"`
	ModelID          string   `json:"model_id,omitempty"`
	InputPricePer1M  *float64 `json:"input_price_per_1m"`
	InputPrice       *float64 `json:"input_price,omitempty"`
	OutputPricePer1M *float64 `json:"output_price_per_1m"`
	OutputPrice      *float64 `json:"output_price,omitempty"`
	NativePriceUSD   *float64 `json:"native_price_usd"`
	NativePriceUnit  *string  `json:"native_price_unit"`
	Available        bool     `json:"available"`
	Status           string   `json:"status,omitempty"`
}

// ToCanonicalModelID normalizes any model name or slug into "vendor/model-slug" format.
func ToCanonicalModelID(model string) string {
	raw := strings.TrimSpace(model)
	if raw == "" {
		return ""
	}

	// If already in vendor/slug format
	if strings.Contains(raw, "/") {
		parts := strings.Split(raw, "/")
		if len(parts) >= 2 {
			vendor := strings.ToLower(strings.TrimSpace(parts[0]))
			slug := strings.ToLower(strings.TrimSpace(parts[len(parts)-1]))
			if vendor == "publishers" || vendor == "models" || vendor == "projects" {
				return ToCanonicalModelID(parts[len(parts)-1])
			}
			return vendor + "/" + slug
		}
	}

	lower := strings.ToLower(raw)

	switch {
	case strings.HasPrefix(lower, "claude"):
		return "anthropic/" + lower
	case strings.HasPrefix(lower, "gpt") ||
		strings.HasPrefix(lower, "o1") ||
		strings.HasPrefix(lower, "o3") ||
		strings.HasPrefix(lower, "o4") ||
		strings.HasPrefix(lower, "chatgpt") ||
		strings.HasPrefix(lower, "dall-e") ||
		strings.HasPrefix(lower, "text-embedding") ||
		strings.HasPrefix(lower, "whisper") ||
		strings.HasPrefix(lower, "tts") ||
		lower == "sora" || strings.HasPrefix(lower, "sora-"):
		return "openai/" + lower
	case strings.HasPrefix(lower, "gemini") || strings.HasPrefix(lower, "imagen") || strings.HasPrefix(lower, "learnlm"):
		return "google/" + lower
	case strings.HasPrefix(lower, "deepseek"):
		return "deepseek/" + lower
	case strings.HasPrefix(lower, "kimi") || strings.HasPrefix(lower, "moonshot"):
		return "moonshot/" + lower
	case strings.HasPrefix(lower, "glm") || strings.HasPrefix(lower, "chatglm") || strings.HasPrefix(lower, "cogview") || strings.HasPrefix(lower, "cogvideo"):
		return "zhipu/" + lower
	case strings.HasPrefix(lower, "qwen"):
		return "qwen/" + lower
	case strings.HasPrefix(lower, "grok"):
		return "xai/" + lower
	case strings.HasPrefix(lower, "minimax"):
		return "minimax/" + lower
	case strings.HasPrefix(lower, "mistral") || strings.HasPrefix(lower, "codestral"):
		return "mistral/" + lower
	case strings.HasPrefix(lower, "llama"):
		return "meta/" + lower
	case strings.HasPrefix(lower, "kling"):
		return "kuaishou/" + lower
	case strings.HasPrefix(lower, "seedance"):
		return "bytedance/" + lower
	default:
		return "other/" + lower
	}
}

var knownCanonicalModelIDs = []string{
	// Anthropic
	"anthropic/claude-3-7-sonnet",
	"anthropic/claude-3-5-sonnet",
	"anthropic/claude-3-5-haiku",
	"anthropic/claude-3-opus",
	"anthropic/claude-sonnet-4-6",
	"anthropic/claude-sonnet-5",
	"anthropic/claude-opus-4-6",
	"anthropic/claude-opus-4-7",
	"anthropic/claude-opus-4-8",
	"anthropic/claude-opus-5",
	"anthropic/claude-haiku-4-5",
	"anthropic/claude-fable-5",

	// OpenAI
	"openai/gpt-4o",
	"openai/gpt-4o-mini",
	"openai/o1",
	"openai/o1-mini",
	"openai/o3-mini",
	"openai/chatgpt-4o-latest",
	"openai/gpt-5",
	"openai/gpt-5.1",
	"openai/gpt-5.2",
	"openai/gpt-5.3-codex",
	"openai/gpt-5.4",
	"openai/gpt-5.5",
	"openai/gpt-5.6-luna",
	"openai/gpt-5.6-sol",
	"openai/gpt-5.6-terra",
	"openai/gpt-image-2",
	"openai/sora",

	// Google
	"google/gemini-2.0-flash",
	"google/gemini-2.5-flash",
	"google/gemini-2-5-flash",
	"google/gemini-1.5-pro",
	"google/gemini-3-flash-preview",
	"google/gemini-3-1-pro-preview",
	"google/gemini-3-1-flash-lite",
	"google/gemini-3-5-flash",
	"google/gemini-3-5-flash-lite",
	"google/gemini-3-6-flash",

	// DeepSeek
	"deepseek/deepseek-v3",
	"deepseek/deepseek-r1",

	// Moonshot / Kimi
	"moonshot/kimi-latest",
	"moonshot/kimi-k1.5",
	"moonshot/kimi-k2-6",
	"moonshot/kimi-k2-7-code",
	"moonshot/kimi-k3",

	// Zhipu / GLM
	"zhipu/glm-4",
	"zhipu/glm-4-5-air",
	"zhipu/glm-4-5v",
	"zhipu/glm-5-1",
	"zhipu/glm-5-2",
	"zhipu/glm-5-3",
	"zhipu/glm-asr",
	"zhipu/glm-image",

	// Qwen / Alibaba
	"qwen/qwen-2.5-72b",
	"qwen/qwen-max",
	"qwen/qwen-plus",

	// xAI
	"xai/grok-2",
	"xai/grok-beta",

	// Minimax
	"minimax/minimax-text-01",
}

var fallbackOfficialPrices = map[string]struct {
	InputPer1M  float64
	OutputPer1M float64
	NativeUSD   float64
	NativeUnit  string
}{
	"anthropic/claude-3-7-sonnet":   {InputPer1M: 3.0, OutputPer1M: 15.0},
	"anthropic/claude-3-5-sonnet":   {InputPer1M: 3.0, OutputPer1M: 15.0},
	"anthropic/claude-3-5-haiku":    {InputPer1M: 0.8, OutputPer1M: 4.0},
	"anthropic/claude-3-opus":       {InputPer1M: 15.0, OutputPer1M: 75.0},
	"anthropic/claude-sonnet-4-6":   {InputPer1M: 3.0, OutputPer1M: 15.0},
	"anthropic/claude-sonnet-5":     {InputPer1M: 3.0, OutputPer1M: 15.0},
	"anthropic/claude-opus-4-6":     {InputPer1M: 15.0, OutputPer1M: 75.0},
	"anthropic/claude-opus-4-7":     {InputPer1M: 15.0, OutputPer1M: 75.0},
	"anthropic/claude-opus-4-8":     {InputPer1M: 15.0, OutputPer1M: 75.0},
	"anthropic/claude-opus-5":       {InputPer1M: 15.0, OutputPer1M: 75.0},
	"anthropic/claude-haiku-4-5":    {InputPer1M: 0.8, OutputPer1M: 4.0},
	"anthropic/claude-fable-5":      {InputPer1M: 3.0, OutputPer1M: 15.0},
	"openai/gpt-4o":                 {InputPer1M: 2.5, OutputPer1M: 10.0},
	"openai/gpt-4o-mini":            {InputPer1M: 0.15, OutputPer1M: 0.6},
	"openai/o1":                     {InputPer1M: 15.0, OutputPer1M: 60.0},
	"openai/o1-mini":                {InputPer1M: 1.10, OutputPer1M: 4.40},
	"openai/o3-mini":                {InputPer1M: 1.10, OutputPer1M: 4.40},
	"openai/chatgpt-4o-latest":      {InputPer1M: 5.0, OutputPer1M: 15.0},
	"openai/gpt-5":                  {InputPer1M: 2.5, OutputPer1M: 10.0},
	"openai/gpt-5.1":                {InputPer1M: 2.5, OutputPer1M: 10.0},
	"openai/gpt-5.2":                {InputPer1M: 2.5, OutputPer1M: 10.0},
	"openai/gpt-5.3-codex":          {InputPer1M: 2.5, OutputPer1M: 10.0},
	"openai/gpt-5.4":                {InputPer1M: 2.5, OutputPer1M: 10.0},
	"openai/gpt-5.5":                {InputPer1M: 2.5, OutputPer1M: 10.0},
	"openai/gpt-5.6-luna":           {InputPer1M: 2.5, OutputPer1M: 10.0},
	"openai/gpt-5.6-sol":            {InputPer1M: 2.5, OutputPer1M: 10.0},
	"openai/gpt-5.6-terra":          {InputPer1M: 2.5, OutputPer1M: 10.0},
	"openai/gpt-image-2":            {NativeUSD: 0.04, NativeUnit: "image"},
	"openai/sora":                   {NativeUSD: 0.10, NativeUnit: "second"},
	"google/gemini-2.0-flash":       {InputPer1M: 0.10, OutputPer1M: 0.40},
	"google/gemini-2.5-flash":       {InputPer1M: 0.075, OutputPer1M: 0.30},
	"google/gemini-2-5-flash":       {InputPer1M: 0.075, OutputPer1M: 0.30},
	"google/gemini-1.5-pro":         {InputPer1M: 1.25, OutputPer1M: 5.0},
	"google/gemini-3-flash-preview": {InputPer1M: 0.10, OutputPer1M: 0.40},
	"google/gemini-3-1-pro-preview": {InputPer1M: 1.25, OutputPer1M: 5.0},
	"google/gemini-3-1-flash-lite":  {InputPer1M: 0.075, OutputPer1M: 0.30},
	"google/gemini-3-5-flash":       {InputPer1M: 0.075, OutputPer1M: 0.30},
	"google/gemini-3-5-flash-lite":  {InputPer1M: 0.075, OutputPer1M: 0.30},
	"google/gemini-3-6-flash":       {InputPer1M: 0.075, OutputPer1M: 0.30},
	"deepseek/deepseek-v3":          {InputPer1M: 0.14, OutputPer1M: 0.28},
	"deepseek/deepseek-r1":          {InputPer1M: 0.55, OutputPer1M: 2.19},
	"moonshot/kimi-latest":          {InputPer1M: 1.0, OutputPer1M: 5.0},
	"moonshot/kimi-k1.5":            {InputPer1M: 1.0, OutputPer1M: 5.0},
	"moonshot/kimi-k2-6":            {InputPer1M: 1.5, OutputPer1M: 7.5},
	"moonshot/kimi-k2-7-code":       {InputPer1M: 1.5, OutputPer1M: 7.5},
	"moonshot/kimi-k3":              {InputPer1M: 2.76, OutputPer1M: 13.8},
	"zhipu/glm-4":                   {InputPer1M: 1.0, OutputPer1M: 3.0},
	"zhipu/glm-4-5-air":             {InputPer1M: 0.5, OutputPer1M: 1.5},
	"zhipu/glm-4-5v":                {InputPer1M: 0.8, OutputPer1M: 2.5},
	"zhipu/glm-5-1":                 {InputPer1M: 1.0, OutputPer1M: 3.5},
	"zhipu/glm-5-2":                 {InputPer1M: 1.0, OutputPer1M: 3.5},
	"zhipu/glm-5-3":                 {InputPer1M: 1.104, OutputPer1M: 3.864},
	"zhipu/glm-asr":                 {NativeUSD: 0.001, NativeUnit: "second"},
	"zhipu/glm-image":               {NativeUSD: 0.02, NativeUnit: "image"},
	"qwen/qwen-2.5-72b":             {InputPer1M: 0.40, OutputPer1M: 1.20},
	"qwen/qwen-max":                 {InputPer1M: 1.60, OutputPer1M: 6.40},
	"qwen/qwen-plus":                {InputPer1M: 0.40, OutputPer1M: 1.20},
	"xai/grok-2":                    {InputPer1M: 2.0, OutputPer1M: 10.0},
	"xai/grok-beta":                 {InputPer1M: 5.0, OutputPer1M: 15.0},
	"minimax/minimax-text-01":       {InputPer1M: 0.20, OutputPer1M: 1.10},
}

// ResolveYunwuRateMultiplier resolves the effective rate multiplier for the yunwu group.
func (s *GatewayService) ResolveYunwuRateMultiplier(ctx context.Context) float64 {
	if s != nil && s.groupRepo != nil {
		// 1. Look for active group with platform "yunwu"
		if groups, err := s.groupRepo.ListActiveByPlatform(ctx, PlatformYunwu); err == nil && len(groups) > 0 {
			for _, g := range groups {
				if g.RateMultiplier > 0 {
					return g.RateMultiplier
				}
			}
		}

		// 2. Look for active group with name "yunwu"
		if groups, err := s.groupRepo.ListActive(ctx); err == nil && len(groups) > 0 {
			for _, g := range groups {
				if (strings.EqualFold(g.Name, "yunwu") || strings.EqualFold(g.Platform, "yunwu") || strings.Contains(strings.ToLower(g.Name), "yunwu")) && g.RateMultiplier > 0 {
					return g.RateMultiplier
				}
			}
		}
	}

	// 3. Look for schedulable account for platform "yunwu" with rate multiplier
	if s != nil && s.accountRepo != nil {
		if accounts, err := s.accountRepo.ListSchedulable(ctx); err == nil && len(accounts) > 0 {
			for _, acc := range accounts {
				if (strings.EqualFold(acc.Platform, "yunwu") || strings.EqualFold(acc.Platform, PlatformYunwu)) && acc.RateMultiplier != nil && *acc.RateMultiplier > 0 {
					return *acc.RateMultiplier
				}
			}
		}
	}

	// 4. Fallback to default configuration rate multiplier
	if s != nil && s.cfg != nil && s.cfg.Default.RateMultiplier > 0 {
		return s.cfg.Default.RateMultiplier
	}

	return 1.0
}

// GetPublicModelsPricing aggregates all available models and their selling pricing (including yunwu markup).
func (s *GatewayService) GetPublicModelsPricing(ctx context.Context) []PublicModelPricingItem {
	availableSet := make(map[string]bool)
	schedulablePlatforms := make(map[string]bool)
	yunwuMultiplier := s.ResolveYunwuRateMultiplier(ctx)
	if yunwuMultiplier <= 0 {
		yunwuMultiplier = 1.0
	}

	if s != nil && s.accountRepo != nil {
		if models := s.GetAvailableModels(ctx, nil, ""); len(models) > 0 {
			for _, m := range models {
				mClean := strings.ToLower(strings.TrimSpace(m))
				availableSet[mClean] = true
				availableSet[ToCanonicalModelID(mClean)] = true
			}
		}

		if accounts, err := s.accountRepo.ListSchedulable(ctx); err == nil && len(accounts) > 0 {
				for _, acc := range accounts {
					p := strings.ToLower(strings.TrimSpace(acc.Platform))
					schedulablePlatforms[p] = true
					for _, m := range acc.GetFetchedModelIDs() {
						mClean := strings.ToLower(strings.TrimSpace(m))
						availableSet[mClean] = true
						availableSet[ToCanonicalModelID(mClean)] = true
					}
					for m := range acc.GetModelMapping() {
						mClean := strings.ToLower(strings.TrimSpace(m))
						availableSet[mClean] = true
						availableSet[ToCanonicalModelID(mClean)] = true
					}
				}
			}
		}

	hasAnySchedulableAccounts := len(schedulablePlatforms) > 0

	// Ordered unique canonical model IDs
	seen := make(map[string]bool)
	candidateList := make([]string, 0, len(knownCanonicalModelIDs)+len(availableSet))

	for _, cid := range knownCanonicalModelIDs {
		if !seen[cid] {
			seen[cid] = true
			candidateList = append(candidateList, cid)
		}
	}

	for rawModel := range availableSet {
		cid := ToCanonicalModelID(rawModel)
		if cid != "" && !seen[cid] {
			seen[cid] = true
			candidateList = append(candidateList, cid)
		}
	}

	items := make([]PublicModelPricingItem, 0, len(candidateList))

	for _, cid := range candidateList {
		parts := strings.SplitN(cid, "/", 2)
		vendor := parts[0]
		slug := parts[1]

		item := PublicModelPricingItem{
			CanonicalModelID: cid,
			ModelID:          cid,
		}

		// Resolve available status
		isAvailable := false
		if availableSet[cid] || availableSet[slug] || availableSet[strings.ToLower(rawModelWithoutVersion(slug))] {
			isAvailable = true
		} else if hasAnySchedulableAccounts && len(availableSet) == 0 {
			switch vendor {
			case "anthropic":
				isAvailable = schedulablePlatforms["anthropic"] || schedulablePlatforms["claude"]
			case "openai":
				isAvailable = schedulablePlatforms["openai"] || schedulablePlatforms["codex"]
			case "google":
				isAvailable = schedulablePlatforms["gemini"] || schedulablePlatforms["google"]
			case "moonshot", "zhipu", "deepseek", "qwen", "minimax", "xai":
				isAvailable = schedulablePlatforms["yunwu"] || schedulablePlatforms["openlux"] || schedulablePlatforms["kiro"]
			}
		}

		item.Available = isAvailable
		if isAvailable {
			item.Status = "available"
		} else {
			item.Status = "unavailable"
		}

		// Resolve pricing from billingService
		var pricing *ModelPricing
		if s != nil && s.billingService != nil {
			// Try slug, cid, normalized dot/dash variants
			candidates := []string{slug, cid, strings.ReplaceAll(slug, "-", "."), strings.ReplaceAll(slug, ".", "-")}
			for _, c := range candidates {
				if p, err := s.billingService.GetModelPricingWithContext(ctx, c); err == nil && p != nil {
					pricing = p
					break
				}
			}
		}

		if pricing != nil {
			if pricing.FixedPricePerRequest > 0 {
				fixed := roundFloat(pricing.FixedPricePerRequest*yunwuMultiplier, 4)
				unit := "request"
				item.NativePriceUSD = &fixed
				item.NativePriceUnit = &unit
			} else if pricing.InputPricePerToken > 0 || pricing.OutputPricePerToken > 0 {
				in1m := roundFloat(pricing.InputPricePerToken*1_000_000*yunwuMultiplier, 4)
				out1m := roundFloat(pricing.OutputPricePerToken*1_000_000*yunwuMultiplier, 4)
				item.InputPricePer1M = &in1m
				item.InputPrice = &in1m
				item.OutputPricePer1M = &out1m
				item.OutputPrice = &out1m
			} else if pricing.ImageOutputPricePerToken > 0 {
				native := roundFloat(pricing.ImageOutputPricePerToken*yunwuMultiplier, 4)
				unit := "image"
				item.NativePriceUSD = &native
				item.NativePriceUnit = &unit
			}
		}

		// Fallback to official pricing table if not resolved
		if item.InputPricePer1M == nil && item.NativePriceUSD == nil {
			if fb, exists := fallbackOfficialPrices[cid]; exists {
				if fb.NativeUSD > 0 {
					n := roundFloat(fb.NativeUSD*yunwuMultiplier, 4)
					u := fb.NativeUnit
					item.NativePriceUSD = &n
					item.NativePriceUnit = &u
				} else {
					in1m := roundFloat(fb.InputPer1M*yunwuMultiplier, 4)
					out1m := roundFloat(fb.OutputPer1M*yunwuMultiplier, 4)
					item.InputPricePer1M = &in1m
					item.InputPrice = &in1m
					item.OutputPricePer1M = &out1m
					item.OutputPrice = &out1m
				}
			}
		}

		items = append(items, item)
	}

	return items
}

func roundFloat(val float64, precision int) float64 {
	ratio := math.Pow(10, float64(precision))
	return math.Round(val*ratio) / ratio
}

func rawModelWithoutVersion(slug string) string {
	// Strip date snapshots like -20241022 or -20250219
	if idx := strings.LastIndex(slug, "-"); idx > 0 && len(slug)-idx-1 == 8 {
		isDigits := true
		for _, ch := range slug[idx+1:] {
			if ch < '0' || ch > '9' {
				isDigits = false
				break
			}
		}
		if isDigits {
			return slug[:idx]
		}
	}
	return slug
}

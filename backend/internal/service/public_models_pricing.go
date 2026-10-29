package service

import (
	"context"
	"math"
	"regexp"
	"strings"
	"time"
)

var (
	versionDashRegex = regexp.MustCompile(`(\d+)-(\d+)`)
	versionDotRegex  = regexp.MustCompile(`(\d+)\.(\d+)`)
)

// Availability evidence sources, strongest first.
//
// The catalog answers one question per model: can a user call this model
// through our API? Without evidence all we know is that some pool for the
// vendor exists, which is NOT proof the model is routable (an aggregator lists
// plenty of models it cannot serve). The public feed therefore publishes where
// the claim comes from, and the UI paints unverified models differently.
const (
	// AvailabilitySourceModels — the model id was listed by the live model
	// catalogue of a schedulable upstream account (fetched via a free GET).
	AvailabilitySourceModels = "models"
	// AvailabilitySourceMapping — the model id is a key of a node's configured
	// model_mapping (an operator explicitly declared it).
	AvailabilitySourceMapping = "mapping"
	// AvailabilitySourceVendor — only the vendor-level pool exists; nothing
	// confirms this specific model.
	AvailabilitySourceVendor = "vendor"
)

// modelAvailabilityEvidence records which upstream list advertised a model and
// when that list was fetched.
type modelAvailabilityEvidence struct {
	source    string
	checkedAt time.Time
}

func availabilitySourceRank(source string) int {
	switch source {
	case AvailabilitySourceModels:
		return 0
	case AvailabilitySourceMapping:
		return 1
	default:
		return 2
	}
}

// modelEvidenceKeys returns every lookup key a raw upstream model id should be
// indexed under: the raw id, its canonical form, and the date-stripped form —
// so an upstream snapshot id (`claude-3-7-sonnet-20250219`) also confirms the
// catalog's date-stripped model (`anthropic/claude-3-7-sonnet`).
func modelEvidenceKeys(rawModel string) []string {
	clean := strings.ToLower(strings.TrimSpace(rawModel))
	if clean == "" {
		return nil
	}
	keys := []string{clean}
	for _, candidate := range []string{clean, rawModelWithoutVersion(clean)} {
		if canonical := ToCanonicalModelID(candidate); canonical != "" {
			keys = append(keys, canonical)
		}
	}
	return keys
}

// recordModelEvidence keeps the strongest (then newest) evidence per key.
func recordModelEvidence(evidence map[string]modelAvailabilityEvidence, rawModel, source string, checkedAt time.Time) {
	for _, key := range modelEvidenceKeys(rawModel) {
		current, ok := evidence[key]
		if ok {
			if availabilitySourceRank(current.source) < availabilitySourceRank(source) {
				continue
			}
			if current.source == source && !checkedAt.After(current.checkedAt) {
				continue
			}
		}
		evidence[key] = modelAvailabilityEvidence{source: source, checkedAt: checkedAt}
	}
}

// lookupModelEvidence resolves the evidence for a catalog model, trying the
// same normalization keys the availability set is matched on.
func lookupModelEvidence(evidence map[string]modelAvailabilityEvidence, canonicalID, slug string) (modelAvailabilityEvidence, bool) {
	for _, key := range []string{
		strings.ToLower(canonicalID),
		strings.ToLower(slug),
		strings.ToLower(rawModelWithoutVersion(slug)),
	} {
		if key == "" {
			continue
		}
		if entry, ok := evidence[key]; ok {
			return entry, true
		}
	}
	return modelAvailabilityEvidence{}, false
}

// PublicModelPricingItem represents an item in the public models pricing catalog (BestAIPrice format).
type PublicModelPricingItem struct {
	CanonicalModelID string   `json:"canonical_model_id"`
	ModelID          string   `json:"model_id,omitempty"`
	InputPricePer1M  *float64 `json:"input_price_per_1m"`
	InputPrice       *float64 `json:"input_price,omitempty"`
	OutputPricePer1M *float64 `json:"output_price_per_1m"`
	OutputPrice      *float64 `json:"output_price,omitempty"`
	// Cache prices (retail, per 1M tokens). Cache write = cache creation.
	CacheReadPricePer1M  *float64 `json:"cache_read_price_per_1m,omitempty"`
	CacheWritePricePer1M *float64 `json:"cache_write_price_per_1m,omitempty"`
	// Official vendor list prices (per 1M tokens) and how far below them we sell.
	// Publish-only: the gateway never charges these.
	OfficialInputPricePer1M  *float64 `json:"official_input_price_per_1m,omitempty"`
	OfficialOutputPricePer1M *float64 `json:"official_output_price_per_1m,omitempty"`
	DiscountPercent          *int     `json:"discount_percent,omitempty"`
	NativePriceUSD           *float64 `json:"native_price_usd"`
	NativePriceUnit          *string  `json:"native_price_unit"`
	Available                bool     `json:"available"`
	Status                   string   `json:"status,omitempty"`
	// Why we claim the model is usable (publish-only, never read by billing).
	// AvailabilityCheckedAt is when the supporting upstream model list was
	// fetched; it is omitted for operator-declared mappings and vendor guesses.
	AvailabilitySource    string     `json:"availability_source,omitempty"`
	AvailabilityVerified  bool       `json:"availability_verified"`
	AvailabilityCheckedAt *time.Time `json:"availability_checked_at,omitempty"`
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
	"google/gemini-2.5-pro",
	"google/gemini-1.5-pro",
	"google/gemini-1.5-flash",
	"google/gemini-3-flash-preview",
	"google/gemini-3.1-pro-preview",
	"google/gemini-3-1-pro-preview",
	"google/gemini-3.1-flash-lite",
	"google/gemini-3-1-flash-lite",
	"google/gemini-3.5-flash",
	"google/gemini-3-5-flash",
	"google/gemini-3.5-flash-lite",
	"google/gemini-3-5-flash-lite",
	"google/gemini-3.6-flash",
	"google/gemini-3-6-flash",
	"google/gemini-3.7-flash",
	"google/gemini-3.8-flash",

	// DeepSeek
	"deepseek/deepseek-v3",
	"deepseek/deepseek-r1",
	"deepseek/deepseek-chat",
	"deepseek/deepseek-reasoner",
	"deepseek/deepseek-v3.1",
	"deepseek/deepseek-v3.2",
	"deepseek/deepseek-v4-flash",
	"deepseek/deepseek-v4-pro",
	"deepseek/deepseek-v4.1-flash",

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

type rateMultiplierContextKey struct{}

// ContextWithRateMultiplier attaches an explicit rate multiplier to context
func ContextWithRateMultiplier(ctx context.Context, multiplier float64) context.Context {
	return context.WithValue(ctx, rateMultiplierContextKey{}, multiplier)
}

// RateMultiplierFromContext extracts explicit rate multiplier from context if present
func RateMultiplierFromContext(ctx context.Context) (float64, bool) {
	if ctx == nil {
		return 0, false
	}
	if v, ok := ctx.Value(rateMultiplierContextKey{}).(float64); ok {
		return v, true
	}
	return 0, false
}

// ResolveYunwuRateMultiplier resolves the effective rate multiplier for the yunwu group.
func (s *GatewayService) ResolveYunwuRateMultiplier(ctx context.Context) float64 {
	if m, ok := RateMultiplierFromContext(ctx); ok && m > 0 {
		return m
	}

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

	// 4. Fallback to default configuration rate multiplier (base default 1.10 = 10% markup)
	if s != nil && s.cfg != nil && s.cfg.Default.RateMultiplier > 0 {
		return s.cfg.Default.RateMultiplier
	}

	return 1.10
}

// GetPublicModelsPricing aggregates all available models and their selling pricing (including yunwu markup).
func (s *GatewayService) GetPublicModelsPricing(ctx context.Context) []PublicModelPricingItem {
	availableSet := make(map[string]bool)
	schedulablePlatforms := make(map[string]bool)
	// Per-model availability evidence: which upstream list actually advertises
	// the model. Empty evidence means we cannot confirm any specific model.
	evidence := make(map[string]modelAvailabilityEvidence)
	yunwuMultiplier := s.ResolveYunwuRateMultiplier(ctx)
	if yunwuMultiplier <= 0 {
		yunwuMultiplier = 1.0
	}

	// Restrict the retail basis to the providers we actually hold schedulable
	// accounts for, so the published "primary" price is one we can serve from
	// (parity with the billing path, which sets the same scope).
	if s != nil {
		ctx = withYunwuProviderScope(ctx, s.accountRepo)
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
				fetchedAt := acc.GetModelsFetchedAt()
				for _, m := range acc.GetFetchedModelIDs() {
					mClean := strings.ToLower(strings.TrimSpace(m))
					availableSet[mClean] = true
					availableSet[ToCanonicalModelID(mClean)] = true
					if fetchedAt != nil {
						recordModelEvidence(evidence, m, AvailabilitySourceModels, *fetchedAt)
					} else {
						recordModelEvidence(evidence, m, AvailabilitySourceModels, time.Time{})
					}
				}
				for m := range acc.GetModelMapping() {
					mClean := strings.ToLower(strings.TrimSpace(m))
					availableSet[mClean] = true
					availableSet[ToCanonicalModelID(mClean)] = true
					recordModelEvidence(evidence, m, AvailabilitySourceMapping, time.Time{})
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

	if s != nil && s.billingService != nil {
		for _, m := range s.billingService.GetYunwuKnownModelNames(ctx) {
			cid := ToCanonicalModelID(m)
			if cid != "" && !seen[cid] {
				seen[cid] = true
				candidateList = append(candidateList, cid)
			}
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
		//
		// A schedulable unofficial (yunwu) account can serve ANY model family
		// (claude/gpt/gemini/kimi/other), so it must make those vendors available
		// too — otherwise only the moonshot/zhipu/deepseek/qwen/minimax/xai vendors
		// would be treated as available while claude/gpt/gemini were wrongly
		// marked unavailable even though a yunwu provider prices them.
		yunwuPool := schedulablePlatforms["yunwu"] || schedulablePlatforms["openlux"] || schedulablePlatforms["kiro"]
		isAvailable := false
		// Where the availability claim comes from, and whether it proves that
		// THIS model is routable (see the AvailabilitySource* constants).
		availSource := ""
		availVerified := false
		var availCheckedAt *time.Time
		if ev, ok := lookupModelEvidence(evidence, cid, slug); ok {
			isAvailable = true
			availSource = ev.source
			availVerified = true
			if !ev.checkedAt.IsZero() {
				checked := ev.checkedAt
				availCheckedAt = &checked
			}
		} else if availableSet[cid] || availableSet[slug] || availableSet[strings.ToLower(rawModelWithoutVersion(slug))] {
			isAvailable = true
			availSource = AvailabilitySourceModels
			availVerified = true
		} else if hasAnySchedulableAccounts && len(availableSet) == 0 {
			switch vendor {
			case "anthropic":
				isAvailable = yunwuPool || schedulablePlatforms["anthropic"] || schedulablePlatforms["claude"]
			case "openai":
				isAvailable = yunwuPool || schedulablePlatforms["openai"] || schedulablePlatforms["codex"]
			case "google":
				isAvailable = yunwuPool || schedulablePlatforms["gemini"] || schedulablePlatforms["google"]
			case "moonshot", "zhipu", "deepseek", "qwen", "minimax", "xai":
				isAvailable = yunwuPool
			default:
				// Unknown vendor: a yunwu pool can still serve it.
				isAvailable = yunwuPool
			}
			if isAvailable {
				// Vendor-level guess only: a pool for this vendor exists, but no
				// upstream list confirms this particular model.
				availSource = AvailabilitySourceVendor
			}
		}

		// Retail price = the PRIMARY provider's price (cheapest provider for the
		// model) × markup. Used for the catalog AND billing, so the advertised
		// price is what is charged even on a Backup-provider failover.
		var pricing *ModelPricing
		if s != nil && s.billingService != nil {
			if p, err := s.billingService.GetYunwuPrimaryModelPricing(ctx, cid); err == nil && p != nil {
				pricing = p
			} else if p, err := s.billingService.GetYunwuPrimaryModelPricing(ctx, slug); err == nil && p != nil {
				pricing = p
			} else if p, err := s.billingService.GetYunwuOnlyModelPricing(ctx, cid); err == nil && p != nil {
				pricing = p
			} else if p, err := s.billingService.GetYunwuOnlyModelPricing(ctx, slug); err == nil && p != nil {
				pricing = p
			}
			// NOTE: no fallback to LiteLLM / static price tables here. The catalog
			// must publish ONLY the upstream-yunwu retail basis, otherwise an
			// "available but unpriced upstream" model would advertise a price that
			// the yunwu billing path never charges (it fails safe instead).
		}

		// §7: per-model markup overrides the global multiplier for this model.
		multiplier := yunwuMultiplier
		if s != nil && s.settingService != nil {
			multiplier = s.settingService.ApplyModelMarkup(ctx, cid, multiplier)
		}

		if pricing != nil {
			if pricing.FixedPricePerRequest > 0 {
				// 6 decimals: per-unit media prices can be tiny (e.g. $0.0016),
				// so 4-decimal rounding would visibly diverge from the charged amount.
				fixed := roundFloat(pricing.FixedPricePerRequest*multiplier, 6)
				unit := strings.TrimSpace(pricing.FixedPriceUnit)
				if unit == "" {
					unit = "request"
				}
				item.NativePriceUSD = &fixed
				item.NativePriceUnit = &unit
			} else if pricing.InputPricePerToken > 0 || pricing.OutputPricePerToken > 0 {
				in1m := roundFloat(pricing.InputPricePerToken*1_000_000*multiplier, 4)
				out1m := roundFloat(pricing.OutputPricePerToken*1_000_000*multiplier, 4)
				item.InputPricePer1M = &in1m
				item.InputPrice = &in1m
				item.OutputPricePer1M = &out1m
				item.OutputPrice = &out1m
				// Cache pricing is part of the published retail price: the same
				// per-token basis × markup, so the catalog matches what billing
				// charges for cache reads/writes.
				if pricing.CacheReadPricePerToken > 0 {
					cr := roundFloat(pricing.CacheReadPricePerToken*1_000_000*multiplier, 4)
					item.CacheReadPricePer1M = &cr
				}
				if pricing.CacheCreationPricePerToken > 0 {
					cw := roundFloat(pricing.CacheCreationPricePerToken*1_000_000*multiplier, 4)
					item.CacheWritePricePer1M = &cw
				}
				// How far below the vendor list price we sell, on the same paying
				// basis as the advertised price (markup included).
				best := 0
				if pricing.OfficialInputPricePerToken > 0 {
					officialIn := roundFloat(pricing.OfficialInputPricePerToken*1_000_000, 4)
					item.OfficialInputPricePer1M = &officialIn
					if d := discountPercent(pricing.OfficialInputPricePerToken, pricing.InputPricePerToken*multiplier); d > best {
						best = d
					}
				}
				if pricing.OfficialOutputPricePerToken > 0 {
					officialOut := roundFloat(pricing.OfficialOutputPricePerToken*1_000_000, 4)
					item.OfficialOutputPricePer1M = &officialOut
					if d := discountPercent(pricing.OfficialOutputPricePerToken, pricing.OutputPricePerToken*multiplier); d > best {
						best = d
					}
				}
				if best > 0 {
					item.DiscountPercent = &best
				}
			}
		}

		// If no upstream provider pricing exists, the model is unpriced (null) and unavailable
		if item.InputPricePer1M == nil && item.NativePriceUSD == nil {
			item.Available = false
			item.Status = "unavailable"
		} else {
			item.Available = isAvailable
			if isAvailable {
				item.Status = "available"
				// Publish the availability evidence so the catalog can tell
				// "we can serve this model" apart from "we only have a pool for
				// the vendor".
				item.AvailabilitySource = availSource
				item.AvailabilityVerified = availVerified
				item.AvailabilityCheckedAt = availCheckedAt
			} else {
				item.Status = "unavailable"
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

// discountPercent reports how far below the vendor list price a rate is, as a
// whole percent. Returns 0 when there is no discount (or the inputs cannot be
// compared), so the catalog only ever shows a positive saving.
func discountPercent(officialPerToken, chargedPerToken float64) int {
	if officialPerToken <= 0 || chargedPerToken <= 0 || chargedPerToken >= officialPerToken {
		return 0
	}
	pct := int(math.Round((1 - chargedPerToken/officialPerToken) * 100))
	if pct < 1 {
		return 0
	}
	if pct > 99 {
		pct = 99
	}
	return pct
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

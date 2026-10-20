package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/yunwu"
	"golang.org/x/sync/singleflight"
)

const (
	DefaultYunwuPricingURL   = "https://api.openlux.ai/api/pricing"
	DefaultPortdanPricingURL = "https://bestaiprice.com/gateway/data/providers.json"
	oneAPIBasePricePer1k     = 0.002 // Standard One-API / New-API base price per 1000 tokens ($2/1M tokens)
)

// DefaultPricingEndpoints lists the public pricing endpoints across unofficial New-API providers (OpenLux, Top-API, Yuanqiaoyun, X-LLM)
var DefaultPricingEndpoints = []string{
	"https://api.openlux.ai/api/pricing",
	"https://api-top.com/api/pricing",
	"https://yuanqiaoyun.com/api/pricing",
	"https://x-llm.net/api/pricing",
}

// DefaultProviderPricingURLs maps known provider identifiers to their official pricing endpoints
var DefaultProviderPricingURLs = map[string]string{
	"openlux":     "https://api.openlux.ai/api/pricing",
	"api-top":     "https://api-top.com/api/pricing",
	"yuanqiaoyun": "https://yuanqiaoyun.com/api/pricing",
	"x-llm":       "https://x-llm.net/api/pricing",
	"portdan":     DefaultPortdanPricingURL,
}

// CanonicalPricingProvider normalizes provider names for pricing lookup
func CanonicalPricingProvider(provider string) string {
	p := strings.ToLower(strings.TrimSpace(provider))
	switch p {
	case "openlux", "yunwu", "api.openlux.ai":
		return "openlux"
	case "api-top", "top-api", "api_top", "apitop", "api-top.com":
		return "api-top"
	case "yuanqiaoyun", "yuanqiao", "yuanqiaoyun.com":
		return "yuanqiaoyun"
	case "x-llm", "x-llm.net", "xllm":
		return "x-llm"
	case "portdan", "portdan.com":
		return "portdan"
	default:
		if p == "" {
			return "openlux"
		}
		return p
	}
}

type pricingProviderKey struct{}

// ContextWithPricingProvider attaches a provider name to the context for provider-specific pricing lookups
func ContextWithPricingProvider(ctx context.Context, provider string) context.Context {
	return context.WithValue(ctx, pricingProviderKey{}, CanonicalPricingProvider(provider))
}

// PricingProviderFromContext extracts the provider name from context if present
func PricingProviderFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(pricingProviderKey{}).(string); ok {
		return v
	}
	return ""
}

type yunwuPrimaryPricingKey struct{}

// ContextWithYunwuPrimaryPricing marks the context so that Yunwu pricing
// resolution uses the PRIMARY provider's price — the cheapest provider for the
// model among the configured unofficial providers (matching the engine's
// default "cheapest = Primary" route). The catalog and the billing for a
// completed Unofficial-provider request both use this basis, so the advertised
// and charged price are the primary provider's price × markup, even when the
// request itself was served by a Backup provider.
func ContextWithYunwuPrimaryPricing(ctx context.Context) context.Context {
	if ctx == nil {
		return ctx
	}
	return context.WithValue(ctx, yunwuPrimaryPricingKey{}, true)
}

// YunwuPrimaryPricingFromContext reports whether primary-provider retail pricing
// is required for this context.
func YunwuPrimaryPricingFromContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(yunwuPrimaryPricingKey{}).(bool)
	return v
}

type yunwuProviderScopeKey struct{}

// ContextWithYunwuProviders restricts unofficial-provider pricing resolution to
// the given providers for EVERY model family (convenience wrapper around
// ContextWithYunwuProviderScope).
func ContextWithYunwuProviders(ctx context.Context, providers []string) context.Context {
	if ctx == nil || len(providers) == 0 {
		return ctx
	}
	scope := make(map[string][]string, 5)
	for _, fam := range yunwu.DefaultModelFamilies() {
		scope[fam] = providers
	}
	return context.WithValue(ctx, yunwuProviderScopeKey{}, scope)
}

// ContextWithYunwuProviderScope restricts unofficial-provider pricing resolution
// to a per-model-family set of providers — the ones we hold schedulable accounts
// for AND that support the family. This mirrors the engine's family routing so
// the published/charged "primary" price is always a provider that can actually
// serve the model. Catalog and billing must set the same scope for parity.
func ContextWithYunwuProviderScope(ctx context.Context, scope map[string][]string) context.Context {
	if ctx == nil || len(scope) == 0 {
		return ctx
	}
	return context.WithValue(ctx, yunwuProviderScopeKey{}, scope)
}

// yunwuProviderScopeFromContext returns the per-family provider scope, if set.
func yunwuProviderScopeFromContext(ctx context.Context) (map[string][]string, bool) {
	if ctx == nil {
		return nil, false
	}
	v, ok := ctx.Value(yunwuProviderScopeKey{}).(map[string][]string)
	if !ok || len(v) == 0 {
		return nil, false
	}
	return v, true
}

// filterProvidersToScope keeps only the providers allowed for the model's family
// by the (optional) context scope. When a scope is present but no provider
// supports the model's family, it returns nil — the model is then unpriced
// (matching the engine, which would find no compatible node).
func filterProvidersToScope(ctx context.Context, providers []string, model string) []string {
	scope, ok := yunwuProviderScopeFromContext(ctx)
	if !ok {
		return providers
	}
	allowedList := scope[yunwu.CanonicalFamily(yunwu.ClassifyModelFamily(model))]
	if len(allowedList) == 0 {
		return nil
	}
	allowed := make(map[string]bool, len(allowedList))
	for _, p := range allowedList {
		allowed[CanonicalPricingProvider(p)] = true
	}
	out := make([]string, 0, len(providers))
	for _, p := range providers {
		if allowed[CanonicalPricingProvider(p)] {
			out = append(out, p)
		}
	}
	return out
}

// YunwuPricingRawItem matches the JSON structure returned by api.openlux.ai/api/pricing
type YunwuPricingRawItem struct {
	ModelName       string  `json:"model_name"`
	QuotaType       int     `json:"quota_type"`
	ModelRatio      float64 `json:"model_ratio"`
	ModelPrice      float64 `json:"model_price"`
	CompletionRatio float64 `json:"completion_ratio"`
	// EnableGroups lists the billing groups the model is available for; the
	// provider's top-level group_ratio maps a group to its discount multiplier.
	// The effective price is model_ratio × group_ratio.
	EnableGroups []string `json:"enable_groups"`
	// GroupRatio is the applied discount multiplier (cheapest enabled group),
	// resolved at parse time from the provider's group_ratio map. 0 means "no
	// group information" and is treated as 1.0.
	GroupRatio float64 `json:"-"`
	// Official vendor list prices (per 1M tokens), when the source publishes
	// them (BestAIPrice catalog records). Publish-only: never used for billing.
	OfficialInputPriceUSDPer1M  float64 `json:"-"`
	OfficialOutputPriceUSDPer1M float64 `json:"-"`
	CacheRatio                  float64 `json:"cache_ratio"`
	Available                   bool    `json:"available"`
	// FixedPriceUnit carries the native billing unit for a fixed per-unit price
	// (e.g. "request" or "second"); populated from BestAIPrice native_price_unit.
	FixedPriceUnit string `json:"-"`
}

type YunwuPricingResponse struct {
	Data []YunwuPricingRawItem `json:"data"`
}

// effectiveGroupRatio resolves the discount multiplier for a model from the
// provider's top-level group_ratio map: the cheapest ratio among the groups the
// model is enabled for (mirrors how the BestAIPrice catalog prices an offer).
// Returns 1.0 (no discount) when no group information is available.
func effectiveGroupRatio(enabled []string, groupRatios map[string]float64) float64 {
	if len(groupRatios) == 0 {
		return 1.0
	}
	best := 0.0
	for _, g := range enabled {
		if r, ok := groupRatios[g]; ok && r > 0 && (best == 0 || r < best) {
			best = r
		}
	}
	if best <= 0 {
		return 1.0
	}
	return best
}

// YunwuPricingService fetches and resolves real-time pricing strictly isolated per provider
type YunwuPricingService struct {
	pricingURL   string
	providerURLs map[string]string
	httpClient   *http.Client
	sf           singleflight.Group

	mu              sync.RWMutex
	providerPricing map[string]map[string]*YunwuPricingRawItem
	providerUpdated map[string]time.Time
	aliases         map[string]string
}

var defaultAliases = map[string]string{
	"gemini-3-pro":             "gemini-3-pro-preview",
	"gemini-3.1-pro":           "gemini-3.1-pro-preview",
	"gemini-3-flash":           "gemini-3-flash-preview",
	"gemini-1.5-flash":         "gemini-2.5-flash",
	"gemini-1.5-pro":           "gemini-2.5-pro",
	"gemini-2.0-flash":         "gemini-2.5-flash",
	"deepseek-chat":            "deepseek-v3",
	"deepseek-reasoner":        "deepseek-r1",
	"deepseek-coder":           "deepseek-v3",
	"claude-3-5-sonnet-latest": "claude-3-5-sonnet",
	"claude-3-5-haiku-latest":  "claude-3-5-haiku",
	"claude-3-opus-latest":     "claude-3-opus",
	"gpt-4o-latest":            "chatgpt-4o-latest",
}

var (
	modelDateSnapshotRegex = regexp.MustCompile(`-\d{8}$`)
	modelVersionDashRegex  = regexp.MustCompile(`(\d+)-(\d+)`)
	modelVersionDotRegex   = regexp.MustCompile(`(\d+)\.(\d+)`)
)

func copyDefaultAliases() map[string]string {
	m := make(map[string]string, len(defaultAliases))
	for k, v := range defaultAliases {
		m[k] = v
	}
	return m
}

// NewYunwuPricingService creates a new Yunwu pricing service
func NewYunwuPricingService() *YunwuPricingService {
	url := strings.TrimSpace(os.Getenv("YUNWU_PRICING_URL"))
	if url == "" {
		url = strings.TrimSpace(os.Getenv("OPENLUX_PRICING_URL"))
	}
	if url == "" {
		url = DefaultYunwuPricingURL
	}

	providerURLs := make(map[string]string)
	for k, v := range DefaultProviderPricingURLs {
		providerURLs[k] = v
	}
	providerURLs["openlux"] = url
	if portdanURL := strings.TrimSpace(os.Getenv("PORTDAN_PRICING_URL")); portdanURL != "" {
		providerURLs["portdan"] = portdanURL
	}

	return &YunwuPricingService{
		pricingURL:   url,
		providerURLs: providerURLs,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		providerPricing: make(map[string]map[string]*YunwuPricingRawItem),
		providerUpdated: make(map[string]time.Time),
		aliases:         copyDefaultAliases(),
	}
}

// NewYunwuPricingServiceWithClient creates an instance with custom URL and HTTP client (useful for tests)
func NewYunwuPricingServiceWithClient(pricingURL string, client *http.Client) *YunwuPricingService {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	url := pricingURL
	providerURLs := make(map[string]string)
	if url == "" {
		url = DefaultYunwuPricingURL
		for k, v := range DefaultProviderPricingURLs {
			providerURLs[k] = v
		}
	}
	providerURLs["openlux"] = url

	return &YunwuPricingService{
		pricingURL:      url,
		providerURLs:    providerURLs,
		httpClient:      client,
		providerPricing: make(map[string]map[string]*YunwuPricingRawItem),
		providerUpdated: make(map[string]time.Time),
		aliases:         copyDefaultAliases(),
	}
}

// SetProviderPricingURL sets or overrides the pricing endpoint URL for a provider.
func (s *YunwuPricingService) SetProviderPricingURL(provider string, endpoint string) {
	if s == nil {
		return
	}
	prov := CanonicalPricingProvider(provider)
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.providerURLs == nil {
		s.providerURLs = make(map[string]string)
	}
	s.providerURLs[prov] = endpoint
	if prov == "openlux" {
		s.pricingURL = endpoint
	}
}

// GetKnownModelNames returns all model names known across configured providers.
func (s *YunwuPricingService) GetKnownModelNames(ctx context.Context) []string {
	if s == nil {
		return nil
	}
	// Warm every configured provider so the union of known models (including
	// image/video models priced only by a non-default provider) is enumerated.
	s.mu.RLock()
	providers := make([]string, 0, len(s.providerURLs))
	for p := range s.providerURLs {
		providers = append(providers, p)
	}
	s.mu.RUnlock()
	if len(providers) == 0 {
		_, _ = s.FetchPricing(ctx)
	} else {
		for _, p := range providers {
			_, _ = s.FetchPricingForProvider(ctx, p)
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := make(map[string]bool)
	var names []string
	for _, pmap := range s.providerPricing {
		for name := range pmap {
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	return names
}

// SetProviderURL overrides or adds an endpoint URL for a specific provider
func (s *YunwuPricingService) SetProviderURL(provider, url string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	prov := CanonicalPricingProvider(provider)
	s.providerURLs[prov] = strings.TrimSpace(url)
}

// FetchPricingForProvider queries the pricing endpoint for a specific provider.
// This guarantees that prices from different providers never merge or cross-contaminate.
func (s *YunwuPricingService) FetchPricingForProvider(ctx context.Context, provider string) (map[string]*YunwuPricingRawItem, error) {
	prov := CanonicalPricingProvider(provider)

	s.mu.RLock()
	cached, hasCache := s.providerPricing[prov]
	lastUpdated := s.providerUpdated[prov]
	endpoint, ok := s.providerURLs[prov]
	s.mu.RUnlock()

	// If cached pricing exists and is fresh (within 10 minutes), return immediately
	if hasCache && len(cached) > 0 && time.Since(lastUpdated) < 10*time.Minute {
		return cached, nil
	}

	if !ok || endpoint == "" {
		if prov == "openlux" {
			endpoint = s.pricingURL
		} else {
			if hasCache && len(cached) > 0 {
				return cached, nil
			}
			return nil, fmt.Errorf("no pricing endpoint configured for provider %s", prov)
		}
	}

	flightKey := "fetch_pricing_" + prov
	v, err, _ := s.sf.Do(flightKey, func() (interface{}, error) {
		reqCtx := ctx
		if reqCtx == nil {
			var cancel context.CancelFunc
			reqCtx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
		}

		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create pricing request for %s (%s): %w", prov, endpoint, err)
		}

		resp, err := s.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch pricing from %s (%s): %w", prov, endpoint, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("pricing endpoint for %s (%s) returned HTTP %d", prov, endpoint, resp.StatusCode)
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read pricing response from %s (%s): %w", prov, endpoint, err)
		}

		pricingMap := make(map[string]*YunwuPricingRawItem)

		// 1. Try One-API standard format: {"data": [...], "group_ratio": {...}}
		var payload struct {
			Data       []YunwuPricingRawItem `json:"data"`
			GroupRatio map[string]float64    `json:"group_ratio"`
		}
		if err := json.Unmarshal(bodyBytes, &payload); err == nil && len(payload.Data) > 0 {
			for i := range payload.Data {
				item := payload.Data[i]
				key := strings.ToLower(strings.TrimSpace(item.ModelName))
				if key == "" {
					continue
				}
				// One-API reports the OFFICIAL ratio; the real price is
				// ratio × group_ratio of the group the model is enabled for.
				item.GroupRatio = effectiveGroupRatio(item.EnableGroups, payload.GroupRatio)
				pricingMap[key] = &item
			}
		} else {
			// 2. Try BestAIPrice format: [{"provider_domain": "...", "model_name": "...", "input_price_usd_per_1m": ...}]
			var providersList []struct {
				ProviderDomain      string   `json:"provider_domain"`
				ProviderName        string   `json:"provider_name"`
				ModelName           string   `json:"model_name"`
				CanonicalModelID    string   `json:"canonical_model_id"`
				InputPriceUSDPer1M  *float64 `json:"input_price_usd_per_1m"`
				OutputPriceUSDPer1M *float64 `json:"output_price_usd_per_1m"`
				// Vendor list prices published by the catalog, used to compute the
				// "discount vs official" shown next to our retail price.
				OfficialInputPriceUSDPer1M  *float64 `json:"official_input_price_usd_per_1m"`
				OfficialOutputPriceUSDPer1M *float64 `json:"official_output_price_usd_per_1m"`
				NativePriceUSD              *float64 `json:"native_price_usd"`
				NativePriceUnit             *string  `json:"native_price_unit"`
				Available                   bool     `json:"available"`
			}
			if err := json.Unmarshal(bodyBytes, &providersList); err == nil && len(providersList) > 0 && (providersList[0].ProviderDomain != "" || providersList[0].ProviderName != "" || providersList[0].ModelName != "") {
				basePerToken := oneAPIBasePricePer1k / 1000.0 // 2e-6
				for _, item := range providersList {
					pDomain := strings.ToLower(strings.TrimSpace(item.ProviderDomain))
					pName := strings.ToLower(strings.TrimSpace(item.ProviderName))
					if prov == "portdan" && !(strings.Contains(pDomain, "portdan") || strings.Contains(pName, "portdan")) {
						continue
					}
					var inPrice, outPrice float64
					if item.InputPriceUSDPer1M != nil {
						inPrice = *item.InputPriceUSDPer1M / 1_000_000.0
					}
					if item.OutputPriceUSDPer1M != nil {
						outPrice = *item.OutputPriceUSDPer1M / 1_000_000.0
					}
					modelRatio := inPrice / basePerToken
					compRatio := 1.0
					if inPrice > 0 && outPrice > 0 {
						compRatio = outPrice / inPrice
					}
					rawItem := &YunwuPricingRawItem{
						ModelName:       item.ModelName,
						ModelRatio:      modelRatio,
						CompletionRatio: compRatio,
						Available:       true,
					}
					if item.OfficialInputPriceUSDPer1M != nil {
						rawItem.OfficialInputPriceUSDPer1M = *item.OfficialInputPriceUSDPer1M
					}
					if item.OfficialOutputPriceUSDPer1M != nil {
						rawItem.OfficialOutputPriceUSDPer1M = *item.OfficialOutputPriceUSDPer1M
					}
					if inPrice == 0 && outPrice == 0 && item.NativePriceUSD != nil {
						rawItem.ModelPrice = *item.NativePriceUSD
						if item.NativePriceUnit != nil && strings.TrimSpace(*item.NativePriceUnit) != "" {
							rawItem.FixedPriceUnit = strings.ToLower(strings.TrimSpace(*item.NativePriceUnit))
						}
					}
					key := strings.ToLower(strings.TrimSpace(item.ModelName))
					if key != "" {
						pricingMap[key] = rawItem
					}
					if item.CanonicalModelID != "" {
						parts := strings.Split(item.CanonicalModelID, "/")
						slug := strings.ToLower(strings.TrimSpace(parts[len(parts)-1]))
						if slug != "" {
							pricingMap[slug] = rawItem
						}
					}
				}
			} else {
				// 3. Try sub2api channel format: [{"platforms": [{"supported_models": [...]}]}]
				var channelList []struct {
					Platforms []struct {
						SupportedModels []struct {
							Name    string `json:"name"`
							Pricing *struct {
								InputPrice      *float64 `json:"input_price"`
								OutputPrice     *float64 `json:"output_price"`
								PerRequestPrice *float64 `json:"per_request_price"`
							} `json:"pricing"`
						} `json:"supported_models"`
					} `json:"platforms"`
				}
				if err := json.Unmarshal(bodyBytes, &channelList); err == nil && len(channelList) > 0 {
					basePerToken := oneAPIBasePricePer1k / 1000.0 // 2e-6
					for _, ch := range channelList {
						for _, pl := range ch.Platforms {
							for _, sm := range pl.SupportedModels {
								if sm.Pricing == nil {
									continue
								}
								inPrice := 0.0
								outPrice := 0.0
								if sm.Pricing.InputPrice != nil {
									inPrice = *sm.Pricing.InputPrice
								}
								if sm.Pricing.OutputPrice != nil {
									outPrice = *sm.Pricing.OutputPrice
								}
								modelRatio := inPrice / basePerToken
								compRatio := 1.0
								if inPrice > 0 && outPrice > 0 {
									compRatio = outPrice / inPrice
								}
								rawItem := &YunwuPricingRawItem{
									ModelName:       sm.Name,
									ModelRatio:      modelRatio,
									CompletionRatio: compRatio,
									Available:       true,
								}
								if inPrice == 0 && outPrice == 0 && sm.Pricing.PerRequestPrice != nil {
									rawItem.ModelPrice = *sm.Pricing.PerRequestPrice
								}
								key := strings.ToLower(strings.TrimSpace(sm.Name))
								if key != "" {
									pricingMap[key] = rawItem
								}
							}
						}
					}
				}
			}
		}

		if len(pricingMap) == 0 {
			return nil, fmt.Errorf("failed to decode pricing response from %s (%s)", prov, endpoint)
		}

		s.mu.Lock()
		s.providerPricing[prov] = pricingMap
		s.providerUpdated[prov] = time.Now()
		s.mu.Unlock()

		return pricingMap, nil
	})

	if err != nil {
		// Graceful fallback to last known pricing for this specific provider
		s.mu.RLock()
		cached := s.providerPricing[prov]
		s.mu.RUnlock()

		if len(cached) > 0 {
			slog.Warn("failed to fetch fresh pricing for provider, falling back to cached snapshot",
				"provider", prov,
				"error", err,
				"cached_models", len(cached))
			return cached, nil
		}
		return nil, err
	}

	return v.(map[string]*YunwuPricingRawItem), nil
}

// FetchPricing queries the default pricing endpoint (OpenLux) with isolated storage.
func (s *YunwuPricingService) FetchPricing(ctx context.Context) (map[string]*YunwuPricingRawItem, error) {
	return s.FetchPricingForProvider(ctx, "openlux")
}

// ResolveModelCandidates generates prioritized candidate model names for upstream price lookups.
func (s *YunwuPricingService) ResolveModelCandidates(modelName string) []string {
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

	// 2. Strip slash prefixes (e.g. "google/gemini-2.5-flash" -> "gemini-2.5-flash", "models/gemini-1.5-pro" -> "gemini-1.5-pro")
	if strings.Contains(raw, "/") {
		parts := strings.Split(raw, "/")
		add(parts[len(parts)-1])
	}
	prefixesToTrim := []string{
		"models/",
		"anthropic/", "openai/", "google/", "deepseek/",
		"moonshot/", "zhipu/", "qwen/", "xai/", "minimax/",
		"meta/", "mistral/", "bytedance/", "kuaishou/", "other/",
	}
	for _, p := range prefixesToTrim {
		if strings.HasPrefix(raw, p) {
			add(strings.TrimPrefix(raw, p))
		}
	}

	// 3. Date snapshots, underscore replacements & dot/dash version variations
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

	// 4. Aliases
	aliases := defaultAliases
	if s != nil && len(s.aliases) > 0 {
		aliases = s.aliases
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

// GetModelPricingForProvider resolves model pricing for a specific provider.
func (s *YunwuPricingService) GetModelPricingForProvider(ctx context.Context, provider string, modelName string) (*ModelPricing, bool) {
	if modelName == "" {
		return nil, false
	}

	prices, err := s.FetchPricingForProvider(ctx, provider)
	if err != nil || len(prices) == 0 {
		return nil, false
	}

	candidates := s.ResolveModelCandidates(modelName)
	var item *YunwuPricingRawItem
	for _, c := range candidates {
		if raw, ok := prices[c]; ok && raw != nil {
			item = raw
			break
		}
	}

	if item == nil {
		return nil, false
	}

	pricing := &ModelPricing{}

	if item.ModelPrice > 0 {
		// Fixed price per unit (e.g. image generation, video generation);
		// the provider's group discount applies to fixed prices as well.
		groupRatio := item.GroupRatio
		if groupRatio <= 0 {
			groupRatio = 1.0
		}
		pricing.FixedPricePerRequest = item.ModelPrice * groupRatio
		pricing.FixedPriceUnit = item.FixedPriceUnit
		return pricing, true
	}

	// Token-based pricing:
	// Base price in One API standard is $0.002 per 1,000 tokens ($0.000002 / token)
	basePerToken := oneAPIBasePricePer1k / 1000.0 // 2e-6
	groupRatio := item.GroupRatio
	if groupRatio <= 0 {
		groupRatio = 1.0
	}
	inputPerToken := item.ModelRatio * basePerToken * groupRatio

	compRatio := item.CompletionRatio
	if compRatio <= 0 {
		compRatio = 1.0
	}
	outputPerToken := item.ModelRatio * compRatio * basePerToken * groupRatio

	cacheReadPerToken := 0.0
	if item.CacheRatio > 0 {
		cacheReadPerToken = item.ModelRatio * item.CacheRatio * basePerToken * groupRatio
	} else {
		// Default cache read cost is 50% of input cost
		cacheReadPerToken = inputPerToken * 0.5
	}

	cacheCreationPerToken := inputPerToken * 1.25

	pricing.InputPricePerToken = inputPerToken
	pricing.OutputPricePerToken = outputPerToken
	pricing.CacheReadPricePerToken = cacheReadPerToken
	pricing.CacheCreationPricePerToken = cacheCreationPerToken
	// Official (list) prices: the same base rate WITHOUT the group discount. Used
	// by the catalog only, to show how far below the vendor price we sell.
	pricing.OfficialInputPricePerToken = item.ModelRatio * basePerToken
	pricing.OfficialOutputPricePerToken = item.ModelRatio * compRatio * basePerToken
	if item.OfficialInputPriceUSDPer1M > 0 {
		pricing.OfficialInputPricePerToken = item.OfficialInputPriceUSDPer1M / 1_000_000
	}
	if item.OfficialOutputPriceUSDPer1M > 0 {
		pricing.OfficialOutputPricePerToken = item.OfficialOutputPriceUSDPer1M / 1_000_000
	}

	return pricing, true
}

// GetModelPricing resolves model pricing according to Yunwu rates.
// If a provider is specified in context, it resolves pricing strictly for that provider.
// Otherwise it searches across all configured providers starting with OpenLux.
func (s *YunwuPricingService) GetModelPricing(ctx context.Context, modelName string) (*ModelPricing, bool) {
	return s.GetModelPricingAcrossProviders(ctx, modelName)
}

// GetModelPricingAcrossProviders resolves model pricing across all known/configured
// Yunwu providers, honoring the (optional) provider/family scope: an explicit
// per-node provider from context is tried first (it is the actual serving node),
// then the in-scope providers are searched in order.
func (s *YunwuPricingService) GetModelPricingAcrossProviders(ctx context.Context, modelName string) (*ModelPricing, bool) {
	if s == nil {
		return nil, false
	}
	prov := PricingProviderFromContext(ctx)
	if prov != "" {
		if p, ok := s.GetModelPricingForProvider(ctx, prov, modelName); ok && p != nil {
			return p, true
		}
	}
	for _, p := range filterProvidersToScope(ctx, s.allProviderNames(), modelName) {
		if p == prov {
			continue
		}
		if pr, ok := s.GetModelPricingForProvider(ctx, p, modelName); ok && pr != nil {
			return pr, true
		}
	}
	return nil, false
}

// allProviderNames returns every configured unofficial provider (defaults incl.
// openlux plus any dynamically discovered/overridden ones), de-duplicated.
func (s *YunwuPricingService) allProviderNames() []string {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	all := make([]string, 0, len(s.providerURLs)+len(s.providerPricing)+1)
	seen := make(map[string]bool)
	all = append(all, "openlux")
	seen["openlux"] = true
	for p := range s.providerURLs {
		if !seen[p] {
			seen[p] = true
			all = append(all, p)
		}
	}
	for p := range s.providerPricing {
		if !seen[p] {
			seen[p] = true
			all = append(all, p)
		}
	}
	return all
}

// GetModelPricingPrimaryAcrossProviders resolves the pricing of the PRIMARY
// provider for a model — the cheapest provider among the configured unofficial
// providers, matching the engine's default "cheapest = Primary" route and the
// deterministic Primary/Backup selection (`sortNodesByPrice` / `pricePrimaryBackup`).
// The published and charged price is always the primary provider's price ×
// markup, even when a request is served by a Backup provider ("цена приводится
// к цене основного провайдера").
//
// Token-priced providers are ranked by input+output cost per token; fixed
// (per-request) providers by their fixed price. Token providers win over fixed
// ones when a model is priced both ways.
func (s *YunwuPricingService) GetModelPricingPrimaryAcrossProviders(ctx context.Context, modelName string) (*ModelPricing, bool) {
	if s == nil {
		return nil, false
	}

	var bestToken *ModelPricing
	bestTokenCost := math.MaxFloat64
	var bestFixed *ModelPricing
	bestFixedCost := math.MaxFloat64

	for _, p := range filterProvidersToScope(ctx, s.allProviderNames(), modelName) {
		pr, ok := s.GetModelPricingForProvider(ctx, p, modelName)
		if !ok || pr == nil {
			continue
		}
		if pr.InputPricePerToken > 0 || pr.OutputPricePerToken > 0 {
			cost := pr.InputPricePerToken + pr.OutputPricePerToken
			if cost > 0 && cost < bestTokenCost {
				bestTokenCost = cost
				bestToken = pr
			}
			continue
		}
		if pr.FixedPricePerRequest > 0 && pr.FixedPricePerRequest < bestFixedCost {
			bestFixedCost = pr.FixedPricePerRequest
			bestFixed = pr
		}
	}

	if bestToken != nil {
		return cloneModelPricing(bestToken), true
	}
	if bestFixed != nil {
		return cloneModelPricing(bestFixed), true
	}
	return nil, false
}

func cloneModelPricing(pr *ModelPricing) *ModelPricing {
	return &ModelPricing{
		InputPricePerToken:         pr.InputPricePerToken,
		OutputPricePerToken:        pr.OutputPricePerToken,
		CacheReadPricePerToken:     pr.CacheReadPricePerToken,
		CacheCreationPricePerToken: pr.CacheCreationPricePerToken,
		FixedPricePerRequest:       pr.FixedPricePerRequest,
		FixedPriceUnit:             pr.FixedPriceUnit,
		ImageOutputPricePerToken:   pr.ImageOutputPricePerToken,
		// Publish-only: the catalog needs the official list price of the provider
		// it chose as primary.
		OfficialInputPricePerToken:  pr.OfficialInputPricePerToken,
		OfficialOutputPricePerToken: pr.OfficialOutputPricePerToken,
	}
}

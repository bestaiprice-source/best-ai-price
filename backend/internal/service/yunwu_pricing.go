package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	DefaultYunwuPricingURL = "https://api.openlux.ai/api/pricing"
	oneAPIBasePricePer1k   = 0.002 // Standard One-API / New-API base price per 1000 tokens ($2/1M tokens)
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

// YunwuPricingRawItem matches the JSON structure returned by api.openlux.ai/api/pricing
type YunwuPricingRawItem struct {
	ModelName       string  `json:"model_name"`
	QuotaType       int     `json:"quota_type"`
	ModelRatio      float64 `json:"model_ratio"`
	ModelPrice      float64 `json:"model_price"`
	CompletionRatio float64 `json:"completion_ratio"`
	CacheRatio      float64 `json:"cache_ratio"`
	Available       bool    `json:"available"`
}

type YunwuPricingResponse struct {
	Data []YunwuPricingRawItem `json:"data"`
}

// YunwuPricingService fetches and resolves real-time pricing strictly isolated per provider
type YunwuPricingService struct {
	pricingURL      string
	providerURLs    map[string]string
	httpClient      *http.Client
	sf              singleflight.Group

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

	return &YunwuPricingService{
		pricingURL:      url,
		providerURLs:    providerURLs,
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

// GetKnownModelNames returns all model names known across configured providers.
func (s *YunwuPricingService) GetKnownModelNames(ctx context.Context) []string {
	if s == nil {
		return nil
	}
	_, _ = s.FetchPricing(ctx)
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

		var payload YunwuPricingResponse
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return nil, fmt.Errorf("failed to decode pricing response from %s (%s): %w", prov, endpoint, err)
		}

		pricingMap := make(map[string]*YunwuPricingRawItem, len(payload.Data))
		for i := range payload.Data {
			item := payload.Data[i]
			key := strings.ToLower(strings.TrimSpace(item.ModelName))
			if key != "" {
				pricingMap[key] = &item
			}
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
		// Fixed price per request (e.g. image generation, video generation)
		pricing.FixedPricePerRequest = item.ModelPrice
		return pricing, true
	}

	// Token-based pricing:
	// Base price in One API standard is $0.002 per 1,000 tokens ($0.000002 / token)
	basePerToken := oneAPIBasePricePer1k / 1000.0 // 2e-6
	inputPerToken := item.ModelRatio * basePerToken

	compRatio := item.CompletionRatio
	if compRatio <= 0 {
		compRatio = 1.0
	}
	outputPerToken := item.ModelRatio * compRatio * basePerToken

	cacheReadPerToken := 0.0
	if item.CacheRatio > 0 {
		cacheReadPerToken = item.ModelRatio * item.CacheRatio * basePerToken
	} else {
		// Default cache read cost is 50% of input cost
		cacheReadPerToken = inputPerToken * 0.5
	}

	cacheCreationPerToken := inputPerToken * 1.25

	pricing.InputPricePerToken = inputPerToken
	pricing.OutputPricePerToken = outputPerToken
	pricing.CacheReadPricePerToken = cacheReadPerToken
	pricing.CacheCreationPricePerToken = cacheCreationPerToken

	return pricing, true
}

// GetModelPricing resolves model pricing according to Yunwu rates.
// If a provider is specified in context, it resolves pricing strictly for that provider.
// Otherwise it searches across all configured providers starting with OpenLux.
func (s *YunwuPricingService) GetModelPricing(ctx context.Context, modelName string) (*ModelPricing, bool) {
	return s.GetModelPricingAcrossProviders(ctx, modelName)
}

// GetModelPricingAcrossProviders resolves model pricing across all known/configured Yunwu providers.
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
	if p, ok := s.GetModelPricingForProvider(ctx, "openlux", modelName); ok && p != nil {
		return p, true
	}
	s.mu.RLock()
	providers := make([]string, 0, len(s.providerURLs))
	for p := range s.providerURLs {
		if p != "openlux" && p != prov {
			providers = append(providers, p)
		}
	}
	s.mu.RUnlock()

	for _, p := range providers {
		if pr, ok := s.GetModelPricingForProvider(ctx, p, modelName); ok && pr != nil {
			return pr, true
		}
	}
	return nil, false
}

// GetModelPricingMaxAcrossProviders resolves the highest cost tier among all active routes
// for a model to guarantee retail pricing covers both Primary and Backup costs:
// retail = max(primary_cost, backup_cost) * (1 + markup).
func (s *YunwuPricingService) GetModelPricingMaxAcrossProviders(ctx context.Context, modelName string) (*ModelPricing, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.RLock()
	allProviders := make([]string, 0, len(s.providerURLs)+len(s.providerPricing)+1)
	seen := make(map[string]bool)
	allProviders = append(allProviders, "openlux")
	seen["openlux"] = true
	for p := range s.providerURLs {
		if !seen[p] {
			seen[p] = true
			allProviders = append(allProviders, p)
		}
	}
	for p := range s.providerPricing {
		if !seen[p] {
			seen[p] = true
			allProviders = append(allProviders, p)
		}
	}
	s.mu.RUnlock()

	var maxPricing *ModelPricing
	found := false

	for _, p := range allProviders {
		if pr, ok := s.GetModelPricingForProvider(ctx, p, modelName); ok && pr != nil {
			if !found {
				maxPricing = &ModelPricing{
					InputPricePerToken:         pr.InputPricePerToken,
					OutputPricePerToken:        pr.OutputPricePerToken,
					CacheReadPricePerToken:     pr.CacheReadPricePerToken,
					CacheCreationPricePerToken: pr.CacheCreationPricePerToken,
					FixedPricePerRequest:       pr.FixedPricePerRequest,
					ImageOutputPricePerToken:   pr.ImageOutputPricePerToken,
				}
				found = true
			} else {
				if pr.InputPricePerToken > maxPricing.InputPricePerToken {
					maxPricing.InputPricePerToken = pr.InputPricePerToken
				}
				if pr.OutputPricePerToken > maxPricing.OutputPricePerToken {
					maxPricing.OutputPricePerToken = pr.OutputPricePerToken
				}
				if pr.CacheReadPricePerToken > maxPricing.CacheReadPricePerToken {
					maxPricing.CacheReadPricePerToken = pr.CacheReadPricePerToken
				}
				if pr.CacheCreationPricePerToken > maxPricing.CacheCreationPricePerToken {
					maxPricing.CacheCreationPricePerToken = pr.CacheCreationPricePerToken
				}
				if pr.FixedPricePerRequest > maxPricing.FixedPricePerRequest {
					maxPricing.FixedPricePerRequest = pr.FixedPricePerRequest
				}
				if pr.ImageOutputPricePerToken > maxPricing.ImageOutputPricePerToken {
					maxPricing.ImageOutputPricePerToken = pr.ImageOutputPricePerToken
				}
			}
		}
	}

	return maxPricing, found
}


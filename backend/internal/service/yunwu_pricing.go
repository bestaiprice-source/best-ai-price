package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	DefaultYunwuPricingURL = "https://api.openlux.ai/api/pricing"
	oneAPIBasePricePer1k   = 0.002 // Standard One-API / New-API base price per 1000 tokens ($2/1M tokens)
)

// DefaultPricingEndpoints lists the public pricing endpoints across unofficial New-API providers (OpenLux, Top-API, Yuanqiaoyun)
var DefaultPricingEndpoints = []string{
	"https://api.openlux.ai/api/pricing",
	"https://api-top.com/api/pricing",
	"https://yuanqiaoyun.com/api/pricing",
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

// YunwuPricingService fetches and resolves real-time pricing from Yunwu (OpenLux, Top-API, Yuanqiaoyun)
type YunwuPricingService struct {
	pricingURL  string
	pricingURLs []string
	httpClient  *http.Client
	sf          singleflight.Group

	mu          sync.RWMutex
	lastPricing map[string]*YunwuPricingRawItem
	lastUpdated time.Time
	aliases     map[string]string
}

// NewYunwuPricingService creates a new Yunwu pricing service
func NewYunwuPricingService() *YunwuPricingService {
	url := strings.TrimSpace(os.Getenv("YUNWU_PRICING_URL"))
	if url == "" {
		url = strings.TrimSpace(os.Getenv("OPENLUX_PRICING_URL"))
	}
	var urls []string
	if url != "" {
		urls = []string{url}
	} else {
		urls = append([]string(nil), DefaultPricingEndpoints...)
	}

	return &YunwuPricingService{
		pricingURL:  urls[0],
		pricingURLs: urls,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		lastPricing: make(map[string]*YunwuPricingRawItem),
		aliases: map[string]string{
			"gemini-3-pro":   "gemini-3-pro-preview",
			"gemini-3.1-pro": "gemini-3.1-pro-preview",
			"gemini-3-flash": "gemini-3-flash-preview",
		},
	}
}

// NewYunwuPricingServiceWithClient creates an instance with custom URL and HTTP client (useful for tests)
func NewYunwuPricingServiceWithClient(pricingURL string, client *http.Client) *YunwuPricingService {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	var urls []string
	if pricingURL != "" {
		urls = []string{pricingURL}
	} else {
		urls = append([]string(nil), DefaultPricingEndpoints...)
	}
	return &YunwuPricingService{
		pricingURL:  urls[0],
		pricingURLs: urls,
		httpClient:  client,
		lastPricing: make(map[string]*YunwuPricingRawItem),
		aliases: map[string]string{
			"gemini-3-pro":   "gemini-3-pro-preview",
			"gemini-3.1-pro": "gemini-3.1-pro-preview",
			"gemini-3-flash": "gemini-3-flash-preview",
		},
	}
}

// FetchPricing queries pricing endpoints (OpenLux, Top-API, Yuanqiaoyun) and aggregates pricing.
// Singleflight is used to avoid duplicate concurrent HTTP requests under load.
// If remote calls fail, it falls back to the last successfully cached pricing.
func (s *YunwuPricingService) FetchPricing(ctx context.Context) (map[string]*YunwuPricingRawItem, error) {
	v, err, _ := s.sf.Do("fetch_pricing", func() (interface{}, error) {
		reqCtx := ctx
		if reqCtx == nil {
			var cancel context.CancelFunc
			reqCtx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
		}

		pricingMap := make(map[string]*YunwuPricingRawItem)
		var lastErr error
		successCount := 0

		for _, endpoint := range s.pricingURLs {
			req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
			if err != nil {
				lastErr = fmt.Errorf("failed to create pricing request for %s: %w", endpoint, err)
				continue
			}

			resp, err := s.httpClient.Do(req)
			if err != nil {
				lastErr = fmt.Errorf("failed to fetch pricing from %s: %w", endpoint, err)
				continue
			}

			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				lastErr = fmt.Errorf("pricing endpoint %s returned HTTP %d", endpoint, resp.StatusCode)
				continue
			}

			var payload YunwuPricingResponse
			err = json.NewDecoder(resp.Body).Decode(&payload)
			resp.Body.Close()
			if err != nil {
				lastErr = fmt.Errorf("failed to decode pricing response from %s: %w", endpoint, err)
				continue
			}

			successCount++
			for i := range payload.Data {
				item := payload.Data[i]
				key := strings.ToLower(strings.TrimSpace(item.ModelName))
				if key != "" {
					if _, exists := pricingMap[key]; !exists {
						pricingMap[key] = &item
					}
				}
			}
		}

		if successCount == 0 {
			if lastErr == nil {
				lastErr = fmt.Errorf("no pricing endpoints configured")
			}
			return nil, lastErr
		}

		s.mu.Lock()
		s.lastPricing = pricingMap
		s.lastUpdated = time.Now()
		s.mu.Unlock()

		return pricingMap, nil
	})

	if err != nil {
		// Graceful fallback to last known pricing if available
		s.mu.RLock()
		cached := s.lastPricing
		s.mu.RUnlock()

		if len(cached) > 0 {
			slog.Warn("failed to fetch fresh Yunwu pricing, falling back to cached snapshot", "error", err, "cached_models", len(cached))
			return cached, nil
		}
		return nil, err
	}

	return v.(map[string]*YunwuPricingRawItem), nil
}

// GetModelPricing resolves model pricing according to Yunwu rates.
func (s *YunwuPricingService) GetModelPricing(ctx context.Context, modelName string) (*ModelPricing, bool) {
	if modelName == "" {
		return nil, false
	}

	prices, err := s.FetchPricing(ctx)
	if err != nil || len(prices) == 0 {
		return nil, false
	}

	normModel := strings.ToLower(strings.TrimSpace(modelName))

	// Check direct match
	item, ok := prices[normModel]
	if !ok {
		// Check configured aliases (e.g. gemini-3-pro -> gemini-3-pro-preview)
		if target, hasAlias := s.aliases[normModel]; hasAlias {
			item, ok = prices[strings.ToLower(target)]
		}
	}
	if !ok {
		// Strip common prefixes like models/
		if strings.HasPrefix(normModel, "models/") {
			trimmed := strings.TrimPrefix(normModel, "models/")
			item, ok = prices[trimmed]
		}
	}

	if !ok || item == nil {
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

package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/model"
)

const defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36"
const defaultBestAIPriceURL = "https://bestaiprice.com/gateway/data/providers.json"

type OneAPIScraper struct {
	client *http.Client
}

func NewOneAPIScraper() *OneAPIScraper {
	return &OneAPIScraper{
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (s *OneAPIScraper) Scrape(ctx context.Context, vendorURL string) (*model.VendorScrapeResult, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(vendorURL), "/")
	if strings.HasSuffix(baseURL, "/v1") {
		baseURL = strings.TrimSuffix(baseURL, "/v1")
	}

	var systemName string
	var payMethods []model.PaymentMethod
	currencyType := "USD"
	usdExchangeRate := 1.0

	// 1. Fetch /api/status for payment methods and general info (optional; non-200 or failure is non-fatal)
	statusURL := fmt.Sprintf("%s/api/status", baseURL)
	if req, err := http.NewRequestWithContext(ctx, "GET", statusURL, nil); err == nil {
		req.Header.Set("User-Agent", defaultUserAgent)
		req.Header.Set("Accept", "application/json")
		if resp, err := s.client.Do(req); err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				var statusResp struct {
					Data struct {
						SystemName      string                `json:"system_name"`
						PayMethods      []model.PaymentMethod `json:"pay_methods"`
						CurrencyType    string                `json:"currency_type"`
						UsdExchangeRate float64               `json:"usd_exchange_rate"`
					} `json:"data"`
				}
				if err := json.NewDecoder(resp.Body).Decode(&statusResp); err == nil {
					systemName = statusResp.Data.SystemName
					payMethods = statusResp.Data.PayMethods
					if statusResp.Data.CurrencyType != "" {
						currencyType = statusResp.Data.CurrencyType
					}
					if statusResp.Data.UsdExchangeRate > 0 {
						usdExchangeRate = statusResp.Data.UsdExchangeRate
					}
				}
			}
		}
	}

	// 2. Fetch /api/pricing for models
	pricingURL := fmt.Sprintf("%s/api/pricing", baseURL)
	var models []model.VendorModel

	if reqPricing, err := http.NewRequestWithContext(ctx, "GET", pricingURL, nil); err == nil {
		reqPricing.Header.Set("User-Agent", defaultUserAgent)
		reqPricing.Header.Set("Accept", "application/json")
		if respPricing, err := s.client.Do(reqPricing); err == nil {
			defer respPricing.Body.Close()
			if respPricing.StatusCode == http.StatusOK {
				var pricingResp struct {
					Data []struct {
						ModelName       string  `json:"model_name"`
						ModelRatio      float64 `json:"model_ratio"`
						ModelPrice      float64 `json:"model_price"`
						CompletionRatio float64 `json:"completion_ratio"`
						Available       *bool   `json:"available"`
					} `json:"data"`
				}
				if err := json.NewDecoder(respPricing.Body).Decode(&pricingResp); err == nil {
					for _, item := range pricingResp.Data {
						trimmedName := strings.TrimSpace(item.ModelName)
						if trimmedName == "" {
							continue
						}

						var inputPrice, outputPrice float64
						if item.ModelPrice > 0 {
							inputPrice = item.ModelPrice
							outputPrice = item.ModelPrice
						} else {
							basePricePer1k := 0.002
							inputPrice = item.ModelRatio * basePricePer1k
							compRatio := item.CompletionRatio
							if compRatio == 0 {
								compRatio = 1.0
							}
							outputPrice = item.ModelRatio * compRatio * basePricePer1k
						}

						isAvail := true
						if item.Available != nil {
							isAvail = *item.Available
						}

						models = append(models, model.VendorModel{
							ModelName:   trimmedName,
							InputPrice:  inputPrice,
							OutputPrice: outputPrice,
							IsAvailable: isAvail,
						})
					}
				}
			}
		}
	}

	// 3. Fallback to BestAIPrice if no models were found via /api/pricing (e.g. Portdan or custom provider)
	if len(models) == 0 {
		fallbackRes, err := s.scrapeFromBestAIPrice(ctx, vendorURL)
		if err == nil && fallbackRes != nil && len(fallbackRes.Models) > 0 {
			if systemName == "" || systemName == "Unknown Vendor" {
				systemName = fallbackRes.Name
			}
			if len(payMethods) == 0 && len(fallbackRes.PaymentMethods) > 0 {
				payMethods = fallbackRes.PaymentMethods
			}
			models = fallbackRes.Models
		}
	}

	if len(models) == 0 {
		return nil, fmt.Errorf("vendor %s does not expose a supported pricing endpoint", vendorURL)
	}

	if systemName == "" {
		if u, err := url.Parse(vendorURL); err == nil && u.Host != "" {
			systemName = u.Host
		} else {
			systemName = "Unknown Vendor"
		}
	}

	return &model.VendorScrapeResult{
		Name:           systemName,
		PaymentMethods: payMethods,
		CurrencyType:   currencyType,
		ExchangeRate:   usdExchangeRate,
		Models:         models,
	}, nil
}

type bestAIPriceRecord struct {
	ProviderName        string   `json:"provider_name"`
	ProviderDomain      string   `json:"provider_domain"`
	ProviderURL         string   `json:"provider_url"`
	ModelName           string   `json:"model_name"`
	CanonicalModelID    string   `json:"canonical_model_id"`
	InputPriceUSDPer1M  *float64 `json:"input_price_usd_per_1m"`
	OutputPriceUSDPer1M *float64 `json:"output_price_usd_per_1m"`
	PaymentMethods      []string `json:"payment_methods"`
}

func (s *OneAPIScraper) scrapeFromBestAIPrice(ctx context.Context, vendorURL string) (*model.VendorScrapeResult, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", defaultBestAIPriceURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bestaiprice returned %d", resp.StatusCode)
	}

	var records []bestAIPriceRecord
	if err := json.NewDecoder(resp.Body).Decode(&records); err != nil {
		return nil, err
	}

	u, _ := url.Parse(vendorURL)
	host := ""
	if u != nil {
		host = strings.ToLower(u.Host)
	}
	vLower := strings.ToLower(vendorURL)

	var matchedRecords []bestAIPriceRecord
	for _, r := range records {
		pDomain := strings.ToLower(r.ProviderDomain)
		pURL := strings.ToLower(r.ProviderURL)
		pName := strings.ToLower(r.ProviderName)

		isMatch := false
		if host != "" && (pDomain == host || strings.Contains(host, pDomain) || strings.Contains(pDomain, host)) {
			isMatch = true
		} else if pDomain != "" && strings.Contains(vLower, pDomain) {
			isMatch = true
		} else if host != "" && strings.Contains(pURL, host) {
			isMatch = true
		} else if (strings.Contains(vLower, "portdan") && strings.Contains(pName, "portdan")) ||
			(strings.Contains(vLower, "openlux") && strings.Contains(pName, "openlux")) ||
			(strings.Contains(vLower, "x-llm") && strings.Contains(pName, "x-llm")) ||
			(strings.Contains(vLower, "yuanqiao") && strings.Contains(pName, "yuanqiao")) ||
			(strings.Contains(vLower, "api-top") && strings.Contains(pName, "top")) {
			isMatch = true
		}

		if isMatch {
			matchedRecords = append(matchedRecords, r)
		}
	}

	if len(matchedRecords) == 0 {
		return nil, fmt.Errorf("no matching provider records found in BestAIPrice")
	}

	var models []model.VendorModel
	providerName := matchedRecords[0].ProviderName
	var pms []model.PaymentMethod
	seenPM := make(map[string]bool)

	for _, r := range matchedRecords {
		modelName := r.ModelName
		if modelName == "" {
			modelName = r.CanonicalModelID
		}
		if modelName == "" {
			continue
		}

		var inPrice, outPrice float64
		if r.InputPriceUSDPer1M != nil {
			inPrice = *r.InputPriceUSDPer1M / 1000.0 // convert $/1M to $/1k
		}
		if r.OutputPriceUSDPer1M != nil {
			outPrice = *r.OutputPriceUSDPer1M / 1000.0
		}

		models = append(models, model.VendorModel{
			ModelName:   modelName,
			InputPrice:  inPrice,
			OutputPrice: outPrice,
			IsAvailable: true,
		})

		for _, pm := range r.PaymentMethods {
			if !seenPM[pm] && pm != "" {
				seenPM[pm] = true
				pms = append(pms, model.PaymentMethod{
					Name: pm,
					Type: strings.ToLower(pm),
				})
			}
		}
	}

	return &model.VendorScrapeResult{
		Name:           providerName,
		PaymentMethods: pms,
		CurrencyType:   "USD",
		ExchangeRate:   1.0,
		Models:         models,
	}, nil
}

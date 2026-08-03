package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/model"
)

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
	baseURL := strings.TrimSuffix(vendorURL, "/")

	// 1. Fetch /api/status for payment methods and general info
	statusURL := fmt.Sprintf("%s/api/status", baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", statusURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create status request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status endpoint returned %d", resp.StatusCode)
	}

	var statusResp struct {
		Data struct {
			SystemName      string                `json:"system_name"`
			PayMethods      []model.PaymentMethod `json:"pay_methods"`
			CurrencyType    string                `json:"currency_type"`
			UsdExchangeRate float64               `json:"usd_exchange_rate"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
		return nil, fmt.Errorf("failed to decode status response: %w", err)
	}

	// 2. Fetch /api/pricing for models
	pricingURL := fmt.Sprintf("%s/api/pricing", baseURL)
	reqPricing, err := http.NewRequestWithContext(ctx, "GET", pricingURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create pricing request: %w", err)
	}

	respPricing, err := s.client.Do(reqPricing)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pricing: %w", err)
	}
	defer respPricing.Body.Close()

	if respPricing.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pricing endpoint returned %d", respPricing.StatusCode)
	}

	var pricingResp struct {
		Data []struct {
			ModelName       string  `json:"model_name"`
			ModelRatio      float64 `json:"model_ratio"`
			ModelPrice      float64 `json:"model_price"`
			CompletionRatio float64 `json:"completion_ratio"`
			Available       bool    `json:"available"`
		} `json:"data"`
	}

	if err := json.NewDecoder(respPricing.Body).Decode(&pricingResp); err != nil {
		return nil, fmt.Errorf("failed to decode pricing response: %w", err)
	}

	var models []model.VendorModel
	for _, item := range pricingResp.Data {
		var inputPrice, outputPrice float64
		
		if item.ModelPrice > 0 {
			// Fixed price per 1k tokens
			inputPrice = item.ModelPrice
			outputPrice = item.ModelPrice
		} else {
			// Ratio based on $0.002 standard for 1k tokens (One-API default standard)
			basePricePer1k := 0.002
			inputPrice = item.ModelRatio * basePricePer1k
			// If completion ratio is 0, it means it's the same as input or not applicable, default to 1 for multiplier if missing?
			// Usually in One API, completion_ratio applies to the model_ratio.
			compRatio := item.CompletionRatio
			if compRatio == 0 {
				compRatio = 1.0 // fallback
			}
			outputPrice = item.ModelRatio * compRatio * basePricePer1k
		}

		models = append(models, model.VendorModel{
			ModelName:   item.ModelName,
			InputPrice:  inputPrice,
			OutputPrice: outputPrice,
			IsAvailable: item.Available,
		})
	}

	name := statusResp.Data.SystemName
	if name == "" {
		name = "Unknown Vendor"
	}

	return &model.VendorScrapeResult{
		Name:           name,
		PaymentMethods: statusResp.Data.PayMethods,
		CurrencyType:   statusResp.Data.CurrencyType,
		ExchangeRate:   statusResp.Data.UsdExchangeRate,
		Models:         models,
	}, nil
}

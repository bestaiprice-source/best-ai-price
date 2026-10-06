package scraper

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOneAPIScraper_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"system_name":       "TestVendor",
					"currency_type":     "CNY",
					"usd_exchange_rate": 7.2,
					"pay_methods": []map[string]any{
						{"name": "Alipay", "type": "alipay"},
					},
				},
			})
		case "/api/pricing":
			avail := true
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{
						"model_name":       "claude-3-7-sonnet",
						"model_ratio":      1.5,
						"model_price":      0.0,
						"completion_ratio": 5.0,
						"available":        avail,
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	scraper := NewOneAPIScraper()
	res, err := scraper.Scrape(context.Background(), ts.URL)
	require.NoError(t, err)
	assert.Equal(t, "TestVendor", res.Name)
	assert.Equal(t, "CNY", res.CurrencyType)
	assert.Equal(t, 7.2, res.ExchangeRate)
	assert.Len(t, res.PaymentMethods, 1)
	require.Len(t, res.Models, 1)
	assert.Equal(t, "claude-3-7-sonnet", res.Models[0].ModelName)
	assert.InDelta(t, 0.003, res.Models[0].InputPrice, 0.0001)
	assert.InDelta(t, 0.015, res.Models[0].OutputPrice, 0.0001)
	assert.True(t, res.Models[0].IsAvailable)
}

func TestOneAPIScraper_StatusFails_PricingSucceeds(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/status":
			http.NotFound(w, r)
		case "/api/pricing":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{
						"model_name":  "gpt-4o",
						"model_price": 0.005,
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	scraper := NewOneAPIScraper()
	res, err := scraper.Scrape(context.Background(), ts.URL)
	require.NoError(t, err)
	assert.Equal(t, "USD", res.CurrencyType)
	assert.Len(t, res.Models, 1)
	assert.Equal(t, "gpt-4o", res.Models[0].ModelName)
	assert.Equal(t, 0.005, res.Models[0].InputPrice)
	assert.True(t, res.Models[0].IsAvailable)
}

func TestOneAPIScraper_Portdan_FallbackBestAIPrice(t *testing.T) {
	scraper := NewOneAPIScraper()
	// portdan.com does not have /api/pricing, so it triggers fallback to BestAIPrice
	res, err := scraper.Scrape(context.Background(), "https://portdan.com")
	require.NoError(t, err)
	assert.NotEmpty(t, res.Name)
	assert.True(t, len(res.Models) > 0, "should have models from BestAIPrice")
	assert.Equal(t, "USD", res.CurrencyType)
}

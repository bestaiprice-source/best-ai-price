package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheaperProviderChecker_FetchAndCompare(t *testing.T) {
	// 1. Setup mock BestAIPrice providers.json HTTP server
	candIn := 2.0
	candOut := 10.0
	testData := []BestAIPriceProviderRecord{
		{
			ProviderName:        "Test Cheap Cloud",
			ProviderDomain:      "cheapcloud.ai",
			ProviderURL:         "https://cheapcloud.ai",
			ModelName:           "claude-3-7-sonnet",
			CanonicalModelID:    "anthropic/claude-3-7-sonnet",
			InputPriceUSDPer1M:  &candIn,
			OutputPriceUSDPer1M: &candOut,
			TrustStatus:         "green",
		},
	}
	dataBytes, err := json.Marshal(testData)
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(dataBytes)
	}))
	defer server.Close()

	// 2. Setup mock DB
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	svc := NewCheaperProviderService(db, nil, nil, nil)
	svc.SetProvidersURL(server.URL)

	// 3. Mock SELECT ash_vendor_models
	// Current route has input_price = 0.003 ($3/1M), output_price = 0.015 ($15/1M), total = $18/1M
	// Candidate has total = $12/1M (33.33% cheaper)
	rows := sqlmock.NewRows([]string{"model_name", "input_price", "output_price", "vendor_name", "vendor_url"}).
		AddRow("claude-3-7-sonnet", 0.003, 0.015, "Current Expensive Provider", "https://expensive.com")

	mock.ExpectQuery(regexp.QuoteMeta("SELECT vm.model_name, vm.input_price, vm.output_price, v.name as vendor_name, v.url as vendor_url")).
		WillReturnRows(rows)

	// 4. Mock INSERT INTO ash_cheaper_provider_opportunities
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO ash_cheaper_provider_opportunities")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "telegram_sent"}).AddRow("opp-uuid-1", true))

	// 5. Mock INSERT INTO ash_events
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO ash_events")).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Run check
	opps, err := svc.CheckOpportunities(context.Background())
	require.NoError(t, err)
	require.Len(t, opps, 1)

	assert.Equal(t, "claude", opps[0].ModelFamily)
	assert.Equal(t, "cheapcloud.ai", opps[0].CandidateDomain)
	assert.Equal(t, 12.0, opps[0].CandidatePrice)
	assert.Equal(t, 18.0, opps[0].CurrentPrimaryPrice)
	assert.Equal(t, 33.33, opps[0].DiffPercent)
	assert.Equal(t, "https://bestaiprice.com/provider/cheapcloud.ai", *opps[0].ProviderCardURL)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCheaperProviderChecker_GetAndDismiss(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	svc := NewCheaperProviderService(db, nil, nil, nil)

	// Test GetOpportunities
	now := time.Now().UTC()
	rows := sqlmock.NewRows([]string{
		"id", "model_family", "canonical_model_id", "model_name", "current_primary_provider", "current_primary_price",
		"candidate_provider", "candidate_domain", "candidate_price", "diff_percent", "billing_unit",
		"status", "telegram_sent", "discovered_at", "updated_at",
	}).AddRow("opp-1", "claude", "anthropic/claude-3-7-sonnet", "claude-3-7-sonnet", "Current", 18.0,
		"Cheap", "cheap.ai", 12.0, 33.33, "tokens", "detected", true, now, now)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM ash_cheaper_provider_opportunities WHERE status = $1")).
		WithArgs("detected").
		WillReturnRows(rows)

	list, err := svc.GetOpportunities(context.Background(), "detected")
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, "opp-1", list[0].ID)
	assert.Equal(t, "claude", list[0].ModelFamily)

	// Test DismissOpportunity
	mock.ExpectExec(regexp.QuoteMeta("UPDATE ash_cheaper_provider_opportunities SET status = 'dismissed'")).
		WithArgs("opp-1").
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = svc.DismissOpportunity(context.Background(), "opp-1")
	require.NoError(t, err)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCheaperProviderChecker_Promote(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	svc := NewCheaperProviderService(db, nil, nil, nil)
	now := time.Now().UTC()

	// 1. Mock SELECT opportunity
	oppRow := sqlmock.NewRows([]string{
		"id", "model_family", "canonical_model_id", "model_name", "current_primary_provider", "current_primary_price",
		"candidate_provider", "candidate_domain", "candidate_price", "diff_percent", "billing_unit",
		"status", "telegram_sent", "discovered_at", "updated_at",
	}).AddRow("opp-2", "claude", "anthropic/claude-3-7-sonnet", "claude-3-7-sonnet", "Current", 18.0,
		"Cheap", "cheap.ai", 12.0, 33.33, "tokens", "detected", true, now, now)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM ash_cheaper_provider_opportunities WHERE id = $1")).
		WithArgs("opp-2").
		WillReturnRows(oppRow)

	// 2. Mock UPDATE opportunity status to promoted_primary
	mock.ExpectExec(regexp.QuoteMeta("UPDATE ash_cheaper_provider_opportunities SET status = $1, updated_at = NOW() WHERE id = $2")).
		WithArgs("promoted_primary", "opp-2").
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = svc.PromoteOpportunity(context.Background(), "opp-2", "primary", 0, 0)
	require.NoError(t, err)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCheaperProviderChecker_FamilyClassificationAndAlertGrouping(t *testing.T) {
	candIn1, candOut1 := 2.0, 10.0
	candIn2, candOut2 := 1.5, 7.5
	candIn3, candOut3 := 1.0, 4.0
	testData := []BestAIPriceProviderRecord{
		{
			ProviderName:        "Test Cheap Cloud",
			ProviderDomain:      "cheapcloud.ai",
			ProviderURL:         "https://cheapcloud.ai",
			ModelName:           "claude-3-7-sonnet",
			CanonicalModelID:    "anthropic/claude-3-7-sonnet",
			InputPriceUSDPer1M:  &candIn1,
			OutputPriceUSDPer1M: &candOut1,
			TrustStatus:         "green",
		},
		{
			ProviderName:        "Test Cheap Cloud",
			ProviderDomain:      "cheapcloud.ai",
			ProviderURL:         "https://cheapcloud.ai",
			ModelName:           "claude-3-5-sonnet",
			CanonicalModelID:    "anthropic/claude-3-5-sonnet",
			InputPriceUSDPer1M:  &candIn2,
			OutputPriceUSDPer1M: &candOut2,
			TrustStatus:         "green",
		},
		{
			ProviderName:        "GPT Low Cost",
			ProviderDomain:      "gptlowcost.io",
			ProviderURL:         "https://gptlowcost.io",
			ModelName:           "gpt-4o",
			CanonicalModelID:    "openai/gpt-4o",
			InputPriceUSDPer1M:  &candIn3,
			OutputPriceUSDPer1M: &candOut3,
			TrustStatus:         "green",
		},
	}
	dataBytes, err := json.Marshal(testData)
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(dataBytes)
	}))
	defer server.Close()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	svc := NewCheaperProviderService(db, nil, nil, nil)
	svc.SetProvidersURL(server.URL)

	rows := sqlmock.NewRows([]string{"model_name", "input_price", "output_price", "vendor_name", "vendor_url"}).
		AddRow("claude-3-7-sonnet", 0.003, 0.015, "Current Expensive Provider", "https://expensive.com").
		AddRow("claude-3-5-sonnet", 0.003, 0.012, "Current Expensive Provider", "https://expensive.com").
		AddRow("gpt-4o", 0.0025, 0.010, "OpenAI Direct", "https://openaidirect.com")

	mock.ExpectQuery(regexp.QuoteMeta("SELECT vm.model_name, vm.input_price, vm.output_price, v.name as vendor_name, v.url as vendor_url")).
		WillReturnRows(rows)

	// Expect 3 inserts into ash_cheaper_provider_opportunities
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO ash_cheaper_provider_opportunities")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "telegram_sent"}).AddRow("opp-c1", false))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO ash_events")).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO ash_cheaper_provider_opportunities")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "telegram_sent"}).AddRow("opp-c2", false))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO ash_events")).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO ash_cheaper_provider_opportunities")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "telegram_sent"}).AddRow("opp-g1", false))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO ash_events")).
		WillReturnResult(sqlmock.NewResult(1, 1))

	opps, err := svc.CheckOpportunities(context.Background())
	require.NoError(t, err)
	require.Len(t, opps, 3)

	assert.Equal(t, "claude", opps[0].ModelFamily)
	assert.Equal(t, "claude", opps[1].ModelFamily)
	assert.Equal(t, "gpt", opps[2].ModelFamily)

	// Test helpers
	assert.Equal(t, "Claude", formatFamilyName("claude"))
	assert.Equal(t, "GPT", formatFamilyName("gpt"))
	assert.Equal(t, "Gemini", formatFamilyName("gemini"))
	assert.Equal(t, "Kimi", formatFamilyName("kimi"))
	assert.Equal(t, "Other", formatFamilyName("other"))

	assert.Equal(t, "$12.00", formatPriceUSD(12.0))
	assert.Equal(t, "$0.250", formatPriceUSD(0.25))
	assert.Equal(t, "$0.0050", formatPriceUSD(0.005))

	require.NoError(t, mock.ExpectationsWereMet())
}



package model

import (
	"encoding/json"
	"time"
)

// PaymentMethod represents a parsed payment method from a vendor
type PaymentMethod struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Color string `json:"color,omitempty"`
}

// Vendor represents an API vendor in the system
type Vendor struct {
	ID             string          `json:"id" db:"id"`
	Name           string          `json:"name" db:"name"`
	URL            string          `json:"url" db:"url"`
	PaymentMethods PaymentMethodList `json:"payment_methods" db:"payment_methods"`
	CurrencyType   string          `json:"currency_type" db:"currency_type"`
	ExchangeRate   float64         `json:"exchange_rate" db:"exchange_rate"`
	Status         string          `json:"status" db:"status"`
	LastScrapedAt  *time.Time      `json:"last_scraped_at" db:"last_scraped_at"`
	CreatedAt      time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at" db:"updated_at"`
}

// VendorModel represents pricing for a specific model from a vendor
type VendorModel struct {
	ID          string    `json:"id" db:"id"`
	VendorID    string    `json:"vendor_id" db:"vendor_id"`
	ModelName   string    `json:"model_name" db:"model_name"`
	InputPrice  float64   `json:"input_price" db:"input_price"`
	OutputPrice float64   `json:"output_price" db:"output_price"`
	IsAvailable bool      `json:"is_available" db:"is_available"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// VendorScrapeResult is the result returned by a scraper
type VendorScrapeResult struct {
	Name           string          `json:"name"`
	PaymentMethods PaymentMethodList `json:"payment_methods"`
	CurrencyType   string          `json:"currency_type"`
	ExchangeRate   float64         `json:"exchange_rate"`
	Models         []VendorModel   `json:"models"`
}

// PaymentMethodList handles JSONB serialization for []PaymentMethod
type PaymentMethodList []PaymentMethod

// Scan implements the sql.Scanner interface for JSONB
func (p *PaymentMethodList) Scan(value interface{}) error {
	if value == nil {
		*p = []PaymentMethod{}
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(b, p)
}

// CheaperProviderOpportunity represents an identified cheaper upstream candidate for a model
type CheaperProviderOpportunity struct {
	ID                     string    `json:"id" db:"id"`
	ModelFamily            string    `json:"model_family" db:"model_family"`
	CanonicalModelID       string    `json:"canonical_model_id" db:"canonical_model_id"`
	ModelName              string    `json:"model_name" db:"model_name"`
	CurrentPrimaryProvider string    `json:"current_primary_provider" db:"current_primary_provider"`
	CurrentPrimaryPrice    float64   `json:"current_primary_price" db:"current_primary_price"`
	CurrentBackupProvider  *string   `json:"current_backup_provider,omitempty" db:"current_backup_provider"`
	CurrentBackupPrice     *float64  `json:"current_backup_price,omitempty" db:"current_backup_price"`
	CandidateProvider      string    `json:"candidate_provider" db:"candidate_provider"`
	CandidateDomain        string    `json:"candidate_domain" db:"candidate_domain"`
	CandidatePrice         float64   `json:"candidate_price" db:"candidate_price"`
	CandidateInputPrice    *float64  `json:"candidate_input_price,omitempty" db:"candidate_input_price"`
	CandidateOutputPrice   *float64  `json:"candidate_output_price,omitempty" db:"candidate_output_price"`
	DiffPercent            float64   `json:"diff_percent" db:"diff_percent"`
	BillingUnit            string    `json:"billing_unit" db:"billing_unit"`
	ProviderURL            *string   `json:"provider_url,omitempty" db:"provider_url"`
	ProviderCardURL        *string   `json:"provider_card_url,omitempty" db:"provider_card_url"`
	Status                 string    `json:"status" db:"status"` // 'detected', 'promoted_primary', 'promoted_backup', 'dismissed'
	TelegramSent           bool      `json:"telegram_sent" db:"telegram_sent"`
	DiscoveredAt           time.Time `json:"discovered_at" db:"discovered_at"`
	UpdatedAt              time.Time `json:"updated_at" db:"updated_at"`
}

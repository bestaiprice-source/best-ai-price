package model

import (
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
	PaymentMethods []PaymentMethod `json:"payment_methods" db:"payment_methods"`
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
	PaymentMethods []PaymentMethod `json:"payment_methods"`
	CurrencyType   string          `json:"currency_type"`
	ExchangeRate   float64         `json:"exchange_rate"`
	Models         []VendorModel   `json:"models"`
}

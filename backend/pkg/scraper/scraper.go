package scraper

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/model"
)

// VendorScraper defines the interface for scraping pricing and info from a vendor
type VendorScraper interface {
	// Scrape fetches all pricing and payment methods from the vendor
	Scrape(ctx context.Context, vendorURL string) (*model.VendorScrapeResult, error)
}

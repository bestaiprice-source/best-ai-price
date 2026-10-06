package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/pkg/scraper"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

type AdminVendorHandler struct {
	db             *sqlx.DB
	cheaperService *service.CheaperProviderService
}

func NewAdminVendorHandler(
	db *sql.DB,
	settingService *service.SettingService,
	groupRepo service.GroupRepository,
	accountRepo service.AccountRepository,
) *AdminVendorHandler {
	return &AdminVendorHandler{
		db:             sqlx.NewDb(db, "postgres"),
		cheaperService: service.NewCheaperProviderService(db, settingService, groupRepo, accountRepo),
	}
}

// GetVendors returns all vendors and their models
func (h *AdminVendorHandler) GetVendors(c *gin.Context) {
	ctx := c.Request.Context()

	var vendors []model.Vendor
	err := h.db.SelectContext(ctx, &vendors, `SELECT * FROM ash_vendors ORDER BY name ASC`)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to get vendors")
		return
	}

	var models []model.VendorModel
	err = h.db.SelectContext(ctx, &models, `SELECT * FROM ash_vendor_models`)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to get vendor models")
		return
	}

	// Group models by vendor
	type VendorWithModels struct {
		model.Vendor
		Models []model.VendorModel `json:"models"`
	}

	result := make([]VendorWithModels, 0)
	for _, v := range vendors {
		vm := VendorWithModels{Vendor: v, Models: []model.VendorModel{}}
		for _, m := range models {
			if m.VendorID == v.ID {
				vm.Models = append(vm.Models, m)
			}
		}
		result = append(result, vm)
	}

	response.Success(c, result)
}

type AddVendorRequest struct {
	URL string `json:"url" binding:"required"`
}

// AddVendor adds a new vendor by URL
func (h *AdminVendorHandler) AddVendor(c *gin.Context) {
	var req AddVendorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	ctx := c.Request.Context()
	var id string
	err := h.db.QueryRowContext(ctx, `
		INSERT INTO ash_vendors (name, url) 
		VALUES ('Pending Scrape', $1) 
		RETURNING id
	`, req.URL).Scan(&id)

	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to add vendor")
		return
	}

	// Trigger async scrape (or we can just return the ID and let frontend call scrape)
	response.Success(c, gin.H{"id": id})
}

// ScrapeVendor triggers a manual scrape for a vendor
func (h *AdminVendorHandler) ScrapeVendor(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()

	var vendorURL string
	err := h.db.QueryRowContext(ctx, `SELECT url FROM ash_vendors WHERE id = $1`, id).Scan(&vendorURL)
	if err != nil {
		response.Error(c, http.StatusNotFound, "Vendor not found")
		return
	}

	s := scraper.NewOneAPIScraper()
	res, err := s.Scrape(ctx, vendorURL)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to scrape vendor: "+err.Error())
		return
	}

	// Update vendor in a transaction
	tx, err := h.db.BeginTxx(ctx, nil)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to start transaction")
		return
	}
	defer tx.Rollback()

	now := time.Now()
	pmJSON, _ := json.Marshal(res.PaymentMethods)

	_, err = tx.ExecContext(ctx, `
		UPDATE ash_vendors 
		SET name = $1, payment_methods = $2, currency_type = $3, exchange_rate = $4, last_scraped_at = $5, updated_at = $5
		WHERE id = $6
	`, res.Name, pmJSON, res.CurrencyType, res.ExchangeRate, now, id)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to update vendor")
		return
	}

	// Update models (upsert)
	for _, m := range res.Models {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO ash_vendor_models (vendor_id, model_name, input_price, output_price, is_available)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (vendor_id, model_name) DO UPDATE 
			SET input_price = EXCLUDED.input_price, output_price = EXCLUDED.output_price, is_available = EXCLUDED.is_available, updated_at = $6
		`, id, m.ModelName, m.InputPrice, m.OutputPrice, m.IsAvailable, now)
		if err != nil {
			response.Error(c, http.StatusInternalServerError, "Failed to upsert models")
			return
		}

		// Also record snapshot in price history
		_, _ = tx.ExecContext(ctx, `
			INSERT INTO ash_vendor_price_history (vendor_id, model_name, input_price, output_price, is_available, recorded_at)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, id, m.ModelName, m.InputPrice, m.OutputPrice, m.IsAvailable, now)
	}

	if err := tx.Commit(); err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to commit transaction")
		return
	}

	response.Success(c, gin.H{"message": "Scrape successful", "models_count": len(res.Models)})
}

type ScrapeVendorByUrlRequest struct {
	Name string `json:"name" binding:"required"`
	URL  string `json:"url" binding:"required"`
}

// ScrapeVendorByUrl triggers a manual scrape for a vendor by URL, creating it if it doesn't exist
func (h *AdminVendorHandler) ScrapeVendorByUrl(c *gin.Context) {
	var req ScrapeVendorByUrlRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	ctx := c.Request.Context()

	// Check if exists
	var id string
	err := h.db.QueryRowContext(ctx, `SELECT id FROM ash_vendors WHERE url = $1`, req.URL).Scan(&id)
	if err != nil {
		if err == sql.ErrNoRows {
			// Insert
			err = h.db.QueryRowContext(ctx, `
				INSERT INTO ash_vendors (name, url) 
				VALUES ($1, $2) 
				RETURNING id
			`, req.Name, req.URL).Scan(&id)
			if err != nil {
				response.Error(c, http.StatusInternalServerError, "Failed to create vendor")
				return
			}
		} else {
			response.Error(c, http.StatusInternalServerError, "Database error")
			return
		}
	}

	s := scraper.NewOneAPIScraper()
	res, err := s.Scrape(ctx, req.URL)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to scrape vendor: "+err.Error())
		return
	}

	// Update vendor in a transaction
	tx, err := h.db.BeginTxx(ctx, nil)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to start transaction")
		return
	}
	defer tx.Rollback()

	now := time.Now()
	pmJSON, _ := json.Marshal(res.PaymentMethods)

	finalName := req.Name
	if res.Name != "" && res.Name != "Unknown" {
		finalName = res.Name
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE ash_vendors 
		SET name = $1, payment_methods = $2, currency_type = $3, exchange_rate = $4, last_scraped_at = $5, updated_at = $5
		WHERE id = $6
	`, finalName, pmJSON, res.CurrencyType, res.ExchangeRate, now, id)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to update vendor")
		return
	}

	// Update models (upsert)
	for _, m := range res.Models {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO ash_vendor_models (vendor_id, model_name, input_price, output_price, is_available)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (vendor_id, model_name) DO UPDATE 
			SET input_price = EXCLUDED.input_price, output_price = EXCLUDED.output_price, is_available = EXCLUDED.is_available, updated_at = $6
		`, id, m.ModelName, m.InputPrice, m.OutputPrice, m.IsAvailable, now)
		if err != nil {
			response.Error(c, http.StatusInternalServerError, "Failed to upsert models")
			return
		}

		// Also record snapshot in price history
		_, _ = tx.ExecContext(ctx, `
			INSERT INTO ash_vendor_price_history (vendor_id, model_name, input_price, output_price, is_available, recorded_at)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, id, m.ModelName, m.InputPrice, m.OutputPrice, m.IsAvailable, now)
	}

	if err := tx.Commit(); err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to commit transaction")
		return
	}

	response.Success(c, gin.H{"message": "Scrape successful", "models_count": len(res.Models)})
}

type VendorPriceHistoryItem struct {
	ID          string    `json:"id" db:"id"`
	VendorID    string    `json:"vendor_id" db:"vendor_id"`
	ModelName   string    `json:"model_name" db:"model_name"`
	InputPrice  float64   `json:"input_price" db:"input_price"`
	OutputPrice float64   `json:"output_price" db:"output_price"`
	IsAvailable bool      `json:"is_available" db:"is_available"`
	RecordedAt  time.Time `json:"recorded_at" db:"recorded_at"`
}

// GetVendorPriceHistory returns historical pricing snapshots for a vendor and/or model
func (h *AdminVendorHandler) GetVendorPriceHistory(c *gin.Context) {
	ctx := c.Request.Context()
	vendorID := c.Param("id")
	modelName := c.Query("model")

	var history []VendorPriceHistoryItem
	var err error

	if vendorID != "" && modelName != "" {
		err = h.db.SelectContext(ctx, &history, `
			SELECT id, vendor_id, model_name, input_price, output_price, is_available, recorded_at
			FROM ash_vendor_price_history
			WHERE vendor_id = $1 AND model_name = $2
			ORDER BY recorded_at DESC
			LIMIT 100
		`, vendorID, modelName)
	} else if vendorID != "" {
		err = h.db.SelectContext(ctx, &history, `
			SELECT id, vendor_id, model_name, input_price, output_price, is_available, recorded_at
			FROM ash_vendor_price_history
			WHERE vendor_id = $1
			ORDER BY recorded_at DESC
			LIMIT 100
		`, vendorID)
	} else if modelName != "" {
		err = h.db.SelectContext(ctx, &history, `
			SELECT id, vendor_id, model_name, input_price, output_price, is_available, recorded_at
			FROM ash_vendor_price_history
			WHERE model_name = $1
			ORDER BY recorded_at DESC
			LIMIT 100
		`, modelName)
	} else {
		err = h.db.SelectContext(ctx, &history, `
			SELECT id, vendor_id, model_name, input_price, output_price, is_available, recorded_at
			FROM ash_vendor_price_history
			ORDER BY recorded_at DESC
			LIMIT 100
		`)
	}

	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to get price history")
		return
	}

	response.Success(c, history)
}

// GetOpportunities returns recorded cheaper provider opportunities
func (h *AdminVendorHandler) GetOpportunities(c *gin.Context) {
	ctx := c.Request.Context()
	status := c.Query("status")
	list, err := h.cheaperService.GetOpportunities(ctx, status)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, list)
}

// CheckOpportunities triggers a live comparison against BestAIPrice providers.json
func (h *AdminVendorHandler) CheckOpportunities(c *gin.Context) {
	ctx := c.Request.Context()
	discovered, err := h.cheaperService.CheckOpportunities(ctx)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, gin.H{
		"message":          "Opportunities check completed",
		"discovered_count": len(discovered),
		"opportunities":    discovered,
	})
}

type PromoteOpportunityRequest struct {
	Target    string `json:"target"` // "primary" or "backup"
	GroupID   int64  `json:"group_id"`
	AccountID int64  `json:"account_id"`
}

// PromoteOpportunity assigns the candidate provider to Primary or Backup route
func (h *AdminVendorHandler) PromoteOpportunity(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")
	var req PromoteOpportunityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req.Target = "primary"
	}
	if req.Target == "" {
		req.Target = "primary"
	}

	if err := h.cheaperService.PromoteOpportunity(ctx, id, req.Target, req.GroupID, req.AccountID); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, gin.H{"message": "Opportunity promoted successfully", "target": req.Target})
}

// DismissOpportunity marks the opportunity as dismissed
func (h *AdminVendorHandler) DismissOpportunity(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")
	if err := h.cheaperService.DismissOpportunity(ctx, id); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, gin.H{"message": "Opportunity dismissed"})
}

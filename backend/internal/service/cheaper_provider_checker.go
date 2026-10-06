package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/yunwu"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

const (
	DefaultBestAIPriceProvidersURL = "https://bestaiprice.com/gateway/data/providers.json"
)

// BestAIPriceProviderRecord represents an entry in BestAIPrice providers.json
type BestAIPriceProviderRecord struct {
	ProviderName        string   `json:"provider_name"`
	ProviderDomain      string   `json:"provider_domain"`
	ProviderURL         string   `json:"provider_url"`
	ModelName           string   `json:"model_name"`
	CanonicalModelID    string   `json:"canonical_model_id"`
	InputPriceUSDPer1M  *float64 `json:"input_price_usd_per_1m"`
	OutputPriceUSDPer1M *float64 `json:"output_price_usd_per_1m"`
	NativePriceUSD      *float64 `json:"native_price_usd"`
	NativePriceUnit     *string  `json:"native_price_unit"`
	TrustStatus         string   `json:"trust_status"`
	SourceURL           string   `json:"source_url"`
}

type CheaperProviderService struct {
	db             *sqlx.DB
	settingService *SettingService
	groupRepo      GroupRepository
	accountRepo    AccountRepository
	httpClient     *http.Client
	providersURL   string
}

func NewCheaperProviderService(
	db *sql.DB,
	settingService *SettingService,
	groupRepo GroupRepository,
	accountRepo AccountRepository,
) *CheaperProviderService {
	url := os.Getenv("BESTAIPRICE_PROVIDERS_URL")
	if strings.TrimSpace(url) == "" {
		url = DefaultBestAIPriceProvidersURL
	}
	return &CheaperProviderService{
		db:             sqlx.NewDb(db, "postgres"),
		settingService: settingService,
		groupRepo:      groupRepo,
		accountRepo:    accountRepo,
		httpClient:     &http.Client{Timeout: 15 * time.Second},
		providersURL:   url,
	}
}

func (s *CheaperProviderService) SetProvidersURL(url string) {
	s.providersURL = url
}

func (s *CheaperProviderService) ensureTable(ctx context.Context) {
	_, _ = s.db.ExecContext(ctx, `
		CREATE EXTENSION IF NOT EXISTS pgcrypto;
		CREATE TABLE IF NOT EXISTS ash_cheaper_provider_opportunities (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			model_family TEXT NOT NULL DEFAULT 'other',
			canonical_model_id TEXT NOT NULL,
			model_name TEXT NOT NULL,
			current_primary_provider TEXT NOT NULL,
			current_primary_price NUMERIC(15, 6) NOT NULL,
			current_backup_provider TEXT,
			current_backup_price NUMERIC(15, 6),
			candidate_provider TEXT NOT NULL,
			candidate_domain TEXT NOT NULL,
			candidate_price NUMERIC(15, 6) NOT NULL,
			candidate_input_price NUMERIC(15, 6),
			candidate_output_price NUMERIC(15, 6),
			diff_percent NUMERIC(10, 2) NOT NULL,
			billing_unit TEXT NOT NULL DEFAULT 'tokens',
			provider_url TEXT,
			provider_card_url TEXT,
			status TEXT NOT NULL DEFAULT 'detected',
			telegram_sent BOOLEAN NOT NULL DEFAULT false,
			discovered_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			UNIQUE (canonical_model_id, candidate_domain)
		);
		ALTER TABLE ash_cheaper_provider_opportunities ADD COLUMN IF NOT EXISTS model_family TEXT NOT NULL DEFAULT 'other';
		CREATE INDEX IF NOT EXISTS idx_ash_cheaper_opportunities_status ON ash_cheaper_provider_opportunities(status);
		CREATE INDEX IF NOT EXISTS idx_ash_cheaper_opportunities_family ON ash_cheaper_provider_opportunities(model_family);
		CREATE INDEX IF NOT EXISTS idx_ash_cheaper_opportunities_model ON ash_cheaper_provider_opportunities(canonical_model_id);
	`)
}

// FetchCandidates retrieves all candidate providers from the BestAIPrice repository
func (s *CheaperProviderService) FetchCandidates(ctx context.Context) ([]BestAIPriceProviderRecord, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.providersURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create candidates request: %w", err)
	}
	req.Header.Set("User-Agent", "BestAIPrice-Gateway/1.0")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch candidates from %s: %w", s.providersURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch candidates status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read candidates body: %w", err)
	}

	var records []BestAIPriceProviderRecord
	if err := json.Unmarshal(body, &records); err != nil {
		return nil, fmt.Errorf("unmarshal candidates json: %w", err)
	}
	return records, nil
}

type currentRoutePricing struct {
	ModelName     string
	ProviderName  string
	InputPrice1M  float64
	OutputPrice1M float64
	Total1M       float64
	NativePrice   *float64
	NativeUnit    *string
}

// CheckOpportunities queries current model routes, compares with BestAIPrice candidates,
// and records cheaper alternatives while dispatching events & Telegram alerts.
func (s *CheaperProviderService) CheckOpportunities(ctx context.Context) ([]model.CheaperProviderOpportunity, error) {
	s.ensureTable(ctx)
	candidates, err := s.FetchCandidates(ctx)
	if err != nil {
		return nil, err
	}

	// 1. Query current active models and prices in system from ash_vendor_models and ash_vendors
	type dbModelRow struct {
		ModelName   string  `db:"model_name"`
		VendorName  string  `db:"vendor_name"`
		VendorURL   string  `db:"vendor_url"`
		InputPrice  float64 `db:"input_price"`  // per 1k in DB
		OutputPrice float64 `db:"output_price"` // per 1k in DB
	}
	var dbRows []dbModelRow
	_ = s.db.SelectContext(ctx, &dbRows, `
		SELECT vm.model_name, vm.input_price, vm.output_price, v.name as vendor_name, v.url as vendor_url
		FROM ash_vendor_models vm
		JOIN ash_vendors v ON v.id = vm.vendor_id
		WHERE vm.is_available = true
		ORDER BY vm.model_name, (vm.input_price + vm.output_price) ASC
	`)

	// Track existing active vendor names and domains
	currentVendorNames := make(map[string]bool)
	currentVendorDomains := make(map[string]bool)
	for _, r := range dbRows {
		vName := strings.ToLower(strings.TrimSpace(r.VendorName))
		if vName != "" {
			currentVendorNames[vName] = true
		}
		if r.VendorURL != "" {
			u := strings.ToLower(strings.TrimSpace(r.VendorURL))
			u = strings.TrimPrefix(u, "https://")
			u = strings.TrimPrefix(u, "http://")
			u = strings.TrimPrefix(u, "www.")
			if idx := strings.Index(u, "/"); idx != -1 {
				u = u[:idx]
			}
			if u != "" {
				currentVendorDomains[u] = true
			}
		}
	}

	// Group current routes by model (lowest price is Primary, second lowest is Backup)
	currentRoutes := make(map[string][]currentRoutePricing)
	for _, r := range dbRows {
		mKey := strings.ToLower(strings.TrimSpace(r.ModelName))
		in1M := r.InputPrice * 1000.0
		out1M := r.OutputPrice * 1000.0
		currentRoutes[mKey] = append(currentRoutes[mKey], currentRoutePricing{
			ModelName:     r.ModelName,
			ProviderName:  r.VendorName,
			InputPrice1M:  in1M,
			OutputPrice1M: out1M,
			Total1M:       in1M + out1M,
		})
	}

	var discovered []model.CheaperProviderOpportunity
	var newAlerts []model.CheaperProviderOpportunity
	now := time.Now().UTC()

	// 2. Evaluate candidates against current routes
	for _, cand := range candidates {
		cModel := strings.ToLower(strings.TrimSpace(cand.ModelName))
		cCanon := strings.ToLower(strings.TrimSpace(cand.CanonicalModelID))
		if cModel == "" && cCanon == "" {
			continue
		}

		// Look up matching current routes (by model name or by canonical ID / suffix)
		routes, found := currentRoutes[cModel]
		if !found && cCanon != "" {
			routes, found = currentRoutes[cCanon]
		}
		if !found && cCanon != "" {
			parts := strings.Split(cCanon, "/")
			routes, found = currentRoutes[parts[len(parts)-1]]
		}
		if !found || len(routes) == 0 {
			continue
		}

		primary := routes[0]
		var backup *currentRoutePricing
		if len(routes) > 1 {
			backup = &routes[1]
		}

		// Don't compare a candidate against itself or existing active vendors
		candDomain := strings.ToLower(strings.TrimSpace(cand.ProviderDomain))
		candName := strings.ToLower(strings.TrimSpace(cand.ProviderName))
		if currentVendorNames[candName] || currentVendorDomains[candDomain] {
			continue
		}
		if strings.Contains(strings.ToLower(primary.ProviderName), candDomain) ||
			strings.Contains(strings.ToLower(primary.ProviderName), candName) {
			continue
		}

		// 3. Normalized Price Comparison: strictly compare same billing units
		var isCheaper bool
		var diffPercent float64
		var candidatePrice float64
		var candidateInPrice *float64
		var candidateOutPrice *float64
		billingUnit := "tokens"

		if cand.InputPriceUSDPer1M != nil && cand.OutputPriceUSDPer1M != nil {
			candTotal := *cand.InputPriceUSDPer1M + *cand.OutputPriceUSDPer1M
			if primary.Total1M > 0 && candTotal < primary.Total1M {
				diffPercent = math.Round(((primary.Total1M-candTotal)/primary.Total1M)*10000) / 100
				if diffPercent >= 1.0 { // at least 1% savings threshold
					isCheaper = true
					candidatePrice = candTotal
					candidateInPrice = cand.InputPriceUSDPer1M
					candidateOutPrice = cand.OutputPriceUSDPer1M
					billingUnit = "tokens"
				}
			}
		} else if cand.NativePriceUSD != nil && cand.NativePriceUnit != nil && primary.NativePrice != nil && primary.NativeUnit != nil {
			if strings.EqualFold(*cand.NativePriceUnit, *primary.NativeUnit) && *cand.NativePriceUSD < *primary.NativePrice {
				diffPercent = math.Round(((*primary.NativePrice - *cand.NativePriceUSD) / *primary.NativePrice) * 10000) / 100
				if diffPercent >= 1.0 {
					isCheaper = true
					candidatePrice = *cand.NativePriceUSD
					candidateInPrice = cand.NativePriceUSD
					billingUnit = *cand.NativePriceUnit
				}
			}
		}

		if !isCheaper {
			continue
		}

		cardURL := fmt.Sprintf("https://bestaiprice.com/provider/%s", cand.ProviderDomain)
		if candDomain == "" {
			cardURL = cand.ProviderURL
		}
		canonID := cand.CanonicalModelID
		if canonID == "" {
			canonID = cand.ModelName
		}

		// Classify Model Family (e.g. claude, gpt, gemini, kimi, other)
		oppFamily := yunwu.ClassifyModelFamily(canonID)
		if oppFamily == yunwu.FamilyOther && cand.ModelName != "" {
			oppFamily = yunwu.ClassifyModelFamily(cand.ModelName)
		}

		var backupProvider *string
		var backupPrice *float64
		if backup != nil {
			backupProvider = &backup.ProviderName
			backupPrice = &backup.Total1M
		}

		opp := model.CheaperProviderOpportunity{
			ModelFamily:            oppFamily,
			CanonicalModelID:       canonID,
			ModelName:              cand.ModelName,
			CurrentPrimaryProvider: primary.ProviderName,
			CurrentPrimaryPrice:    primary.Total1M,
			CurrentBackupProvider:  backupProvider,
			CurrentBackupPrice:     backupPrice,
			CandidateProvider:      cand.ProviderName,
			CandidateDomain:        cand.ProviderDomain,
			CandidatePrice:         candidatePrice,
			CandidateInputPrice:    candidateInPrice,
			CandidateOutputPrice:   candidateOutPrice,
			DiffPercent:            diffPercent,
			BillingUnit:            billingUnit,
			ProviderURL:            &cand.ProviderURL,
			ProviderCardURL:        &cardURL,
			Status:                 "detected",
			DiscoveredAt:           now,
			UpdatedAt:              now,
		}

		// 4. Save opportunity into DB (upsert)
		var insertedID string
		var prevTelegramSent bool
		err := s.db.QueryRowContext(ctx, `
			INSERT INTO ash_cheaper_provider_opportunities 
				(model_family, canonical_model_id, model_name, current_primary_provider, current_primary_price, 
				 current_backup_provider, current_backup_price, candidate_provider, candidate_domain, 
				 candidate_price, candidate_input_price, candidate_output_price, diff_percent, 
				 billing_unit, provider_url, provider_card_url, status, telegram_sent, discovered_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, 'detected', false, $18, $18)
			ON CONFLICT (canonical_model_id, candidate_domain) DO UPDATE
			SET model_family = EXCLUDED.model_family,
			    current_primary_provider = EXCLUDED.current_primary_provider,
			    current_primary_price = EXCLUDED.current_primary_price,
			    current_backup_provider = EXCLUDED.current_backup_provider,
			    current_backup_price = EXCLUDED.current_backup_price,
			    candidate_price = EXCLUDED.candidate_price,
			    candidate_input_price = EXCLUDED.candidate_input_price,
			    candidate_output_price = EXCLUDED.candidate_output_price,
			    diff_percent = EXCLUDED.diff_percent,
			    provider_url = EXCLUDED.provider_url,
			    provider_card_url = EXCLUDED.provider_card_url,
			    updated_at = EXCLUDED.updated_at
			RETURNING id::text as id, telegram_sent
		`, opp.ModelFamily, opp.CanonicalModelID, opp.ModelName, opp.CurrentPrimaryProvider, opp.CurrentPrimaryPrice,
			opp.CurrentBackupProvider, opp.CurrentBackupPrice, opp.CandidateProvider, opp.CandidateDomain,
			opp.CandidatePrice, opp.CandidateInputPrice, opp.CandidateOutputPrice, opp.DiffPercent,
			opp.BillingUnit, opp.ProviderURL, opp.ProviderCardURL, now).Scan(&insertedID, &prevTelegramSent)

		if err == nil {
			opp.ID = insertedID
			opp.TelegramSent = prevTelegramSent

			// 5. Audit event in ash_events
			eventPayload, _ := json.Marshal(map[string]any{
				"model_family":       opp.ModelFamily,
				"canonical_model_id": opp.CanonicalModelID,
				"candidate_provider": opp.CandidateProvider,
				"candidate_domain":   opp.CandidateDomain,
				"diff_percent":       opp.DiffPercent,
				"current_primary":    opp.CurrentPrimaryProvider,
				"current_price":      opp.CurrentPrimaryPrice,
				"candidate_price":    opp.CandidatePrice,
			})
			_, _ = s.db.ExecContext(ctx, `
				INSERT INTO ash_events (owner_id, event_type, payload, created_at)
				VALUES ('system', 'CheaperProviderDiscovered', $1, NOW())
			`, eventPayload)

			discovered = append(discovered, opp)
			if !prevTelegramSent {
				newAlerts = append(newAlerts, opp)
			}
		}
	}

	// 6. Send Telegram notifications grouped by Model Family and Candidate Provider
	if len(newAlerts) > 0 && s.settingService != nil {
		type alertGroup struct {
			family          string
			candidateDomain string
			opps            []model.CheaperProviderOpportunity
		}
		groupMap := make(map[string]*alertGroup)
		var groupOrder []string

		for _, opp := range newAlerts {
			groupKey := fmt.Sprintf("%s|%s", opp.ModelFamily, strings.ToLower(opp.CandidateDomain))
			if _, exists := groupMap[groupKey]; !exists {
				groupMap[groupKey] = &alertGroup{
					family:          opp.ModelFamily,
					candidateDomain: opp.CandidateDomain,
					opps:            nil,
				}
				groupOrder = append(groupOrder, groupKey)
			}
			groupMap[groupKey].opps = append(groupMap[groupKey].opps, opp)
		}

		for _, key := range groupOrder {
			grp := groupMap[key]
			if len(grp.opps) == 0 {
				continue
			}

			totalSavings := 0.0
			maxSavings := 0.0
			currentProvidersSet := make(map[string]bool)
			var currentProvidersList []string
			var modelLines []string

			for i, o := range grp.opps {
				totalSavings += o.DiffPercent
				if o.DiffPercent > maxSavings {
					maxSavings = o.DiffPercent
				}
				if !currentProvidersSet[o.CurrentPrimaryProvider] {
					currentProvidersSet[o.CurrentPrimaryProvider] = true
					currentProvidersList = append(currentProvidersList, o.CurrentPrimaryProvider)
				}

				if i < 8 {
					unitSuffix := ""
					if o.BillingUnit != "tokens" {
						unitSuffix = fmt.Sprintf(" (%s)", o.BillingUnit)
					}
					modelLines = append(modelLines, fmt.Sprintf("• <code>%s</code>: %s vs %s (%s%s) — 🟢 <b>-%.1f%%</b>",
						o.ModelName,
						formatPriceUSD(o.CandidatePrice),
						formatPriceUSD(o.CurrentPrimaryPrice),
						o.CurrentPrimaryProvider,
						unitSuffix,
						o.DiffPercent,
					))
				}
			}

			if len(grp.opps) > 8 {
				modelLines = append(modelLines, fmt.Sprintf("• <i>... и ещё %d моделей</i>", len(grp.opps)-8))
			}

			avgSavings := totalSavings / float64(len(grp.opps))
			savingsStr := fmt.Sprintf("🟢 <b>-%.1f%%</b>", avgSavings)
			if len(grp.opps) > 1 && maxSavings > avgSavings+0.5 {
				savingsStr = fmt.Sprintf("🟢 <b>-%.1f%%</b> (до -%.1f%%)", avgSavings, maxSavings)
			}

			firstOpp := grp.opps[0]
			cardURL := ""
			if firstOpp.ProviderCardURL != nil {
				cardURL = *firstOpp.ProviderCardURL
			}
			if cardURL == "" {
				cardURL = fmt.Sprintf("https://bestaiprice.com/provider/%s", firstOpp.CandidateDomain)
			}

			familyNameDisplay := formatFamilyName(grp.family)
			currentDisplay := strings.Join(currentProvidersList, ", ")
			if currentDisplay == "" {
				currentDisplay = "Не указан"
			}

			tgMsg := fmt.Sprintf(
				"💡 <b>Найден более дешёвый поставщик для семейства %s!</b>\n\n"+
					"<b>Семейство:</b> %s\n"+
					"<b>Текущие провайдеры:</b> %s\n"+
					"<b>Новый кандидат:</b> %s (%s)\n"+
					"<b>Экономия по семейству:</b> %s\n\n"+
					"<b>Модели:</b>\n%s\n\n"+
					"<b>Карточка поставщика:</b> <a href=\"%s\">%s</a>",
				familyNameDisplay,
				familyNameDisplay,
				currentDisplay,
				firstOpp.CandidateProvider,
				firstOpp.CandidateDomain,
				savingsStr,
				strings.Join(modelLines, "\n"),
				cardURL,
				firstOpp.CandidateDomain,
			)

			if _, tgErr := s.settingService.SendTelegramNotification(ctx, tgMsg); tgErr == nil {
				var ids []string
				for _, o := range grp.opps {
					ids = append(ids, o.ID)
				}
				_, _ = s.db.ExecContext(ctx, `UPDATE ash_cheaper_provider_opportunities SET telegram_sent = true WHERE id = ANY($1)`, pq.Array(ids))
				for i := range discovered {
					for _, o := range grp.opps {
						if discovered[i].ID == o.ID {
							discovered[i].TelegramSent = true
						}
					}
				}
			} else {
				slog.Warn("[CheaperProviderChecker] Telegram dispatch failed for family", "family", grp.family, "candidate", grp.candidateDomain, "error", tgErr)
			}
		}
	}

	return discovered, nil
}

// GetOpportunities returns recorded cheaper provider opportunities filtered optionally by status
func (s *CheaperProviderService) GetOpportunities(ctx context.Context, status string) ([]model.CheaperProviderOpportunity, error) {
	var list []model.CheaperProviderOpportunity
	var err error
	queryWithStatus := `
		SELECT * FROM ash_cheaper_provider_opportunities
		WHERE status = $1
		ORDER BY diff_percent DESC, discovered_at DESC
	`
	queryAll := `
		SELECT * FROM ash_cheaper_provider_opportunities
		ORDER BY diff_percent DESC, discovered_at DESC
	`
	if status != "" {
		err = s.db.SelectContext(ctx, &list, queryWithStatus, status)
	} else {
		err = s.db.SelectContext(ctx, &list, queryAll)
	}
	if err != nil {
		s.ensureTable(ctx)
		if status != "" {
			_ = s.db.SelectContext(ctx, &list, queryWithStatus, status)
		} else {
			_ = s.db.SelectContext(ctx, &list, queryAll)
		}
		slog.Warn("[CheaperProviderChecker] select cheaper opportunities query error, returning empty list", "error", err)
	}
	if list == nil {
		list = []model.CheaperProviderOpportunity{}
	}
	return list, nil
}

// PromoteOpportunity assigns the candidate provider as Primary or Backup route for the specified group and model.
func (s *CheaperProviderService) PromoteOpportunity(ctx context.Context, opportunityID string, target string, groupID int64, accountID int64) error {
	var opp model.CheaperProviderOpportunity
	err := s.db.GetContext(ctx, &opp, `SELECT * FROM ash_cheaper_provider_opportunities WHERE id = $1`, opportunityID)
	if err != nil {
		return fmt.Errorf("opportunity not found: %w", err)
	}

	// 1. If groupID is not specified, pick active "yunwu" group
	if groupID <= 0 && s.groupRepo != nil {
		if groups, err := s.groupRepo.ListActiveByPlatform(ctx, PlatformYunwu); err == nil && len(groups) > 0 {
			groupID = groups[0].ID
		} else if groups, err := s.groupRepo.ListActive(ctx); err == nil && len(groups) > 0 {
			for _, g := range groups {
				if strings.Contains(strings.ToLower(g.Name), "yunwu") {
					groupID = g.ID
					break
				}
			}
		}
	}

	// 2. If valid group and account ID are provided, update ModelRouting on group
	if groupID > 0 && accountID > 0 && s.groupRepo != nil {
		grp, err := s.groupRepo.GetByID(ctx, groupID)
		if err == nil && grp != nil {
			if grp.ModelRouting == nil {
				grp.ModelRouting = make(map[string][]int64)
			}
			grp.ModelRoutingEnabled = true
			mKey := opp.CanonicalModelID
			currentRoutes := grp.ModelRouting[mKey]

			if strings.EqualFold(target, "backup") {
				if len(currentRoutes) == 0 {
					grp.ModelRouting[mKey] = []int64{accountID}
				} else if len(currentRoutes) == 1 {
					grp.ModelRouting[mKey] = append(currentRoutes, accountID)
				} else {
					grp.ModelRouting[mKey] = []int64{currentRoutes[0], accountID}
				}
			} else {
				// Default target: "primary"
				if len(currentRoutes) == 0 {
					grp.ModelRouting[mKey] = []int64{accountID}
				} else {
					grp.ModelRouting[mKey] = []int64{accountID, currentRoutes[0]}
				}
			}
			_ = s.groupRepo.Update(ctx, grp)
		}
	}

	newStatus := "promoted_primary"
	if strings.EqualFold(target, "backup") {
		newStatus = "promoted_backup"
	}

	_, err = s.db.ExecContext(ctx, `
		UPDATE ash_cheaper_provider_opportunities
		SET status = $1, updated_at = NOW()
		WHERE id = $2
	`, newStatus, opportunityID)
	return err
}

// DismissOpportunity marks the opportunity as dismissed by the administrator
func (s *CheaperProviderService) DismissOpportunity(ctx context.Context, opportunityID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE ash_cheaper_provider_opportunities
		SET status = 'dismissed', updated_at = NOW()
		WHERE id = $1
	`, opportunityID)
	return err
}

func formatFamilyName(family string) string {
	switch strings.ToLower(family) {
	case yunwu.FamilyClaude:
		return "Claude"
	case yunwu.FamilyGPT:
		return "GPT"
	case yunwu.FamilyGemini:
		return "Gemini"
	case yunwu.FamilyKimi:
		return "Kimi"
	default:
		return "Other"
	}
}

func formatPriceUSD(p float64) string {
	if p >= 1.0 {
		return fmt.Sprintf("$%.2f", p)
	}
	if p >= 0.01 {
		return fmt.Sprintf("$%.3f", p)
	}
	return fmt.Sprintf("$%.4f", p)
}


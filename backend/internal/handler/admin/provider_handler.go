package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/internal/credentials"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/gatewayctx"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/yunwu"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// ProviderHandler handles administrative operations for external AI upstream providers.
type ProviderHandler struct {
	adminService       service.AdminService
	entClient          *ent.Client
	redisClient        *redis.Client
	secretEncryptor    service.SecretEncryptor
	accountTestService *service.AccountTestService
	statsWorker        *yunwu.StatsWorker
	nodeManager        *health.NodeManager
	httpClient         *http.Client
}

// NewProviderHandler creates a new ProviderHandler instance.
func NewProviderHandler(
	adminService service.AdminService,
	entClient *ent.Client,
	redisClient *redis.Client,
	secretEncryptor service.SecretEncryptor,
	accountTestService *service.AccountTestService,
) *ProviderHandler {
	var store *storage.Storage
	if entClient != nil && redisClient != nil {
		store = storage.NewStorage(entClient, redisClient)
	}
	var sw *yunwu.StatsWorker
	var nm *health.NodeManager
	if store != nil {
		sw = yunwu.NewStatsWorker(store)
		nm = health.NewNodeManager(store)
	} else {
		sw = yunwu.NewStatsWorker(nil)
	}

	return &ProviderHandler{
		adminService:       adminService,
		entClient:          entClient,
		redisClient:        redisClient,
		secretEncryptor:    secretEncryptor,
		accountTestService: accountTestService,
		statsWorker:        sw,
		nodeManager:        nm,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SetStatsWorker sets the stats worker (useful for testing or custom configs).
func (h *ProviderHandler) SetStatsWorker(sw *yunwu.StatsWorker) {
	h.statsWorker = sw
}

// --- Request and Response DTOs ---

type ProviderSummaryStats struct {
	TotalProviders      int     `json:"total_providers"`
	ActiveProviders     int     `json:"active_providers"`
	LowBalanceProviders int     `json:"low_balance_providers"`
	TotalBalanceUSD     float64 `json:"total_balance_usd"`
	TotalSpendUSD       float64 `json:"total_spend_usd"`
}

type ProviderMetrics struct {
	LatencyMs int64   `json:"latency_ms"`
	ErrorRate float64 `json:"error_rate"`
	Requests  int64   `json:"requests"`
	SpendUSD  float64 `json:"spend_usd"`
}

type ProviderResponse struct {
	ID                     int64             `json:"id"`
	ProviderName           string            `json:"provider_name"`
	Provider               string            `json:"provider"`
	Domain                 string            `json:"domain"`
	BaseURL                string            `json:"base_url"`
	Protocol               string            `json:"protocol"`
	APIKeyMasked           string            `json:"api_key_masked"`
	SupportedModelFamilies []string          `json:"supported_model_families"`
	ModelMapping           map[string]string `json:"model_mapping"`
	BalanceUSD             float64           `json:"balance_usd"`
	BalanceSource          string            `json:"balance_source"`
	LastSyncedAt           *string           `json:"last_synced_at,omitempty"`
	MinBalanceUSD          float64           `json:"min_balance_usd"`
	Status                 string            `json:"status"` // active, disabled
	State                  string            `json:"state"`  // Healthy, Degraded, Down, Invalid key, Low balance, Disabled
	Metrics                ProviderMetrics   `json:"metrics"`
	Notes                  string            `json:"notes"`
	GroupIDs               []int64           `json:"group_ids"`
	Priority               int               `json:"priority"`
	Concurrency            int               `json:"concurrency"`
	CreatedAt              string            `json:"created_at"`
	UpdatedAt              string            `json:"updated_at"`
}

type ProviderListResponse struct {
	Summary ProviderSummaryStats `json:"summary"`
	Items   []*ProviderResponse  `json:"items"`
	Total   int64                `json:"total"`
	Page    int                  `json:"page"`
	PageSize int                 `json:"page_size"`
}

type CreateProviderRequest struct {
	ProviderName           string            `json:"provider_name" binding:"required"`
	Provider               string            `json:"provider"` // openlux, portdan, api-top, yuanqiaoyun, x-llm, custom
	Domain                 string            `json:"domain"`
	BaseURL                string            `json:"base_url"`
	Protocol               string            `json:"protocol" binding:"omitempty,oneof=openai anthropic gemini responses"`
	APIKey                 string            `json:"api_key" binding:"required"`
	SupportedModelFamilies []string          `json:"supported_model_families"`
	ModelMapping           map[string]string `json:"model_mapping"`
	MinBalanceUSD          *float64          `json:"min_balance_usd"`
	Status                 string            `json:"status" binding:"omitempty,oneof=active disabled"`
	Notes                  string            `json:"notes"`
	Priority               *int              `json:"priority"`
	Concurrency            *int              `json:"concurrency"`
	GroupIDs               []int64           `json:"group_ids"`
}

type UpdateProviderRequest struct {
	ProviderName           string            `json:"provider_name"`
	Provider               string            `json:"provider"`
	Domain                 string            `json:"domain"`
	BaseURL                string            `json:"base_url"`
	Protocol               string            `json:"protocol" binding:"omitempty,oneof=openai anthropic gemini responses"`
	APIKey                 string            `json:"api_key"`
	SupportedModelFamilies []string          `json:"supported_model_families"`
	ModelMapping           map[string]string `json:"model_mapping"`
	MinBalanceUSD          *float64          `json:"min_balance_usd"`
	Status                 string            `json:"status" binding:"omitempty,oneof=active disabled"`
	Notes                  *string           `json:"notes"`
	Priority               *int              `json:"priority"`
	Concurrency            *int              `json:"concurrency"`
	GroupIDs               *[]int64          `json:"group_ids"`
}

type TestConnectionRequest struct {
	BaseURL  string `json:"base_url"`
	APIKey   string `json:"api_key"`
	Protocol string `json:"protocol"`
}

type TestConnectionResponse struct {
	Success    bool   `json:"success"`
	LatencyMs  int64  `json:"latency_ms"`
	StatusCode int    `json:"status_code"`
	ModelCount int    `json:"model_count"`
	Message    string `json:"message"`
}

type BatchSyncSummary struct {
	Total           int     `json:"total"`
	Synced          int     `json:"synced"`
	Failed          int     `json:"failed"`
	TotalBalanceUSD float64 `json:"total_balance_usd"`
}

// --- Helper Functions ---

func MaskAPIKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	if len(key) <= 8 {
		return "sk-***"
	}
	prefixLen := 4
	suffixLen := 4
	if len(key) < 12 {
		prefixLen = 2
		suffixLen = 2
	}
	return key[:prefixLen] + "-***" + key[len(key)-suffixLen:]
}

func parseProviderDomain(domainStr, baseURL string) string {
	domainStr = strings.TrimSpace(domainStr)
	if domainStr != "" {
		return domainStr
	}
	if baseURL != "" {
		if u, err := url.Parse(baseURL); err == nil && u.Host != "" {
			return u.Host
		}
	}
	return ""
}

func determineProviderState(accStatus string, hasError bool, balanceUSD, minBalanceUSD float64, rt *health.NodeRuntimeState, nodeState string) string {
	if accStatus == "disabled" || accStatus == "inactive" {
		return "Disabled"
	}
	if accStatus == "error" || hasError {
		return "Invalid key"
	}
	if minBalanceUSD > 0 && balanceUSD <= minBalanceUSD {
		return "Low balance"
	}
	if nodeState == "unavailable" || (rt != nil && rt.TotalRequests >= 3 && rt.SuccessRate < 0.2) {
		return "Down"
	}
	if nodeState == "cooldown" || nodeState == "limited" || (rt != nil && rt.TotalRequests >= 3 && (rt.SuccessRate < 0.8 || rt.AvgLatencyMs > 5000)) {
		return "Degraded"
	}
	return "Healthy"
}

// accountToProviderResponse transforms a service.Account into a complete ProviderResponse.
func (h *ProviderHandler) accountToProviderResponse(ctx context.Context, acc *service.Account) *ProviderResponse {
	if acc == nil {
		return nil
	}

	extra := acc.Extra
	if extra == nil {
		extra = make(map[string]any)
	}

	prov := yunwu.ParseNodeProvider(extra)
	if prov == "openlux" && acc.Platform != "" && acc.Platform != domain.PlatformYunwu {
		prov = acc.Platform
	}

	baseURL, _ := extra["base_url"].(string)
	if baseURL == "" {
		switch strings.ToLower(acc.Platform) {
		case domain.PlatformAnthropic:
			baseURL = "https://api.anthropic.com"
		case domain.PlatformOpenAI:
			baseURL = "https://api.openai.com/v1"
		case domain.PlatformGemini:
			baseURL = "https://generativelanguage.googleapis.com"
		case domain.PlatformAntigravity:
			baseURL = "https://api.antigravity.ai"
		case domain.PlatformSora:
			baseURL = "https://api.openai.com/v1"
		default:
			baseURL = yunwu.DefaultBaseURLForProvider(prov)
		}
	}

	domainStr, _ := extra["domain"].(string)
	domainStr = parseProviderDomain(domainStr, baseURL)

	protocol, _ := extra["yunwu_protocol"].(string)
	if protocol == "" {
		if p, ok := extra["protocol"].(string); ok && p != "" {
			protocol = p
		} else {
			switch strings.ToLower(acc.Platform) {
			case domain.PlatformAnthropic:
				protocol = "anthropic"
			case domain.PlatformGemini:
				protocol = "gemini"
			case domain.PlatformOpenAI:
				protocol = "openai"
			default:
				protocol = "openai"
			}
		}
	}

	modelFamilies := yunwu.ParseNodeModelFamilies(extra)
	if len(modelFamilies) == 0 {
		switch strings.ToLower(acc.Platform) {
		case domain.PlatformAnthropic:
			modelFamilies = []string{"claude"}
		case domain.PlatformOpenAI:
			modelFamilies = []string{"gpt"}
		case domain.PlatformGemini:
			modelFamilies = []string{"gemini"}
		default:
			modelFamilies = []string{"claude", "gpt"}
		}
	}

	modelMapping := make(map[string]string)
	if rawMapping, ok := extra["model_mapping"].(map[string]any); ok {
		for k, v := range rawMapping {
			if vs, ok := v.(string); ok {
				modelMapping[k] = vs
			}
		}
	} else if directMap, ok := extra["model_mapping"].(map[string]string); ok {
		modelMapping = directMap
	}

	minBalanceUSD := 20.0
	if mb, ok := extra["min_balance_usd"].(float64); ok && mb >= 0 {
		minBalanceUSD = mb
	} else if mb, ok := extra["alert_threshold"].(float64); ok && mb >= 0 {
		minBalanceUSD = mb
	} else if mb, ok := extra["limit"].(float64); ok && mb >= 0 {
		minBalanceUSD = mb
	} else if mb, ok := extra["cost_threshold"].(float64); ok && mb >= 0 {
		minBalanceUSD = mb
	} else if mb, ok := extra["threshold"].(float64); ok && mb >= 0 {
		minBalanceUSD = mb
	}

	balanceUSD := 0.0
	if b, ok := extra["balance_usd"].(float64); ok {
		balanceUSD = b
	} else if b, ok := extra["balance"].(float64); ok {
		balanceUSD = b
	} else if b, ok := extra["total_available"].(float64); ok {
		balanceUSD = b
	}

	balanceSource, _ := extra["balance_source"].(string)
	if balanceSource == "" {
		balanceSource = "manual"
	}

	var lastSyncedAt *string
	if syncStr, ok := extra["balance_updated_at"].(string); ok && syncStr != "" {
		lastSyncedAt = &syncStr
	}

	// Pull API key / token and mask it
	apiKeyMasked := ""
	if acc.Credentials != nil {
		rawKey := ""
		for _, keyName := range []string{"api_key", "token", "access_token", "setup_token", "session_key", "refresh_token"} {
			if k, ok := acc.Credentials[keyName].(string); ok && k != "" {
				rawKey = k
				break
			}
		}
		if strings.HasPrefix(rawKey, credentials.EnvelopePrefix) && h.secretEncryptor != nil {
			if decrypted, err := credentials.DecryptMap(acc.Credentials, h.secretEncryptor); err == nil {
				for _, keyName := range []string{"api_key", "token", "access_token", "setup_token", "session_key", "refresh_token"} {
					if k, ok := decrypted[keyName].(string); ok && k != "" {
						rawKey = k
						break
					}
				}
			}
		}
		apiKeyMasked = MaskAPIKey(rawKey)
	}

	// Runtime metrics
	nodeID := strconv.FormatInt(acc.ID, 10)
	var rt *health.NodeRuntimeState
	nodeState := "healthy"
	if h.nodeManager != nil {
		if node, err := h.nodeManager.GetNode(ctx, nodeID); err == nil && node != nil {
			nodeState = node.State
		}
		if state, err := h.nodeManager.GetNodeRuntimeState(ctx, nodeID); err == nil && state != nil {
			rt = state
		}
	}

	// If balance snapshot exists in Redis, use it if newer
	if h.statsWorker != nil && h.redisClient != nil {
		if snap, err := h.statsWorker.Snapshot(ctx, nodeID, 100*365*24*time.Hour); err == nil && snap != nil {
			if snap.AvailableUSD > 0 || balanceUSD == 0 {
				balanceUSD = snap.AvailableUSD
			}
			if snap.Source != "" {
				balanceSource = snap.Source
			}
			if !snap.SynchronizedAt.IsZero() {
				s := snap.SynchronizedAt.Format(time.RFC3339)
				lastSyncedAt = &s
			}
		}
	}

	metrics := ProviderMetrics{}
	if rt != nil {
		metrics.LatencyMs = rt.AvgLatencyMs
		metrics.Requests = rt.TotalRequests
		if rt.TotalRequests > 0 {
			errRate := (1.0 - rt.SuccessRate) * 100.0
			if errRate < 0 {
				errRate = 0
			}
			metrics.ErrorRate = float64(int(errRate*100)) / 100.0
		}
		metrics.SpendUSD = float64(rt.UsedTokens) / 1_000_000.0 * 2.5
	}

	hasError := acc.ErrorMessage != ""
	state := determineProviderState(acc.Status, hasError, balanceUSD, minBalanceUSD, rt, nodeState)

	notes := ""
	if acc.Notes != nil {
		notes = *acc.Notes
	}

	groupIDs := acc.GroupIDs
	if groupIDs == nil {
		groupIDs = make([]int64, 0)
	}

	return &ProviderResponse{
		ID:                     acc.ID,
		ProviderName:           acc.Name,
		Provider:               prov,
		Domain:                 domainStr,
		BaseURL:                baseURL,
		Protocol:               protocol,
		APIKeyMasked:           apiKeyMasked,
		SupportedModelFamilies: modelFamilies,
		ModelMapping:           modelMapping,
		BalanceUSD:             balanceUSD,
		BalanceSource:          balanceSource,
		LastSyncedAt:           lastSyncedAt,
		MinBalanceUSD:          minBalanceUSD,
		Status:                 acc.Status,
		State:                  state,
		Metrics:                metrics,
		Notes:                  notes,
		GroupIDs:               groupIDs,
		Priority:               acc.Priority,
		Concurrency:            acc.Concurrency,
		CreatedAt:              acc.CreatedAt.Format(time.RFC3339),
		UpdatedAt:              acc.UpdatedAt.Format(time.RFC3339),
	}
}

// --- Handler Methods ---

// List GET /api/v1/admin/providers
func (h *ProviderHandler) List(c *gin.Context) {
	h.ListGateway(gatewayctx.FromGin(c))
}

func (h *ProviderHandler) ListGateway(c gatewayctx.GatewayContext) {
	ctx := c.Request().Context()
	page, pageSize := response.ParsePaginationValues(c)

	statusFilter := strings.TrimSpace(c.QueryValue("status"))
	stateFilter := strings.TrimSpace(c.QueryValue("state"))
	searchQuery := strings.TrimSpace(c.QueryValue("search"))

	// Query accounts across all platforms (all execution nodes) using adminService
	accounts, _, err := h.adminService.ListAccounts(ctx, 1, 1000, "", "", statusFilter, searchQuery, "", "", "", 0)
	if err != nil {
		response.ErrorFromContext(gatewayJSONResponder{ctx: c}, err)
		return
	}

	var allItems []*ProviderResponse
	var summary ProviderSummaryStats

	for i := range accounts {
		item := h.accountToProviderResponse(ctx, &accounts[i])
		if item == nil {
			continue
		}

		// Calculate summary statistics
		summary.TotalProviders++
		if item.Status == "active" {
			summary.ActiveProviders++
		}
		if item.State == "Low balance" {
			summary.LowBalanceProviders++
		}
		summary.TotalBalanceUSD += item.BalanceUSD
		summary.TotalSpendUSD += item.Metrics.SpendUSD

		// Apply state filter if provided
		if stateFilter != "" && stateFilter != "all" && !strings.EqualFold(item.State, stateFilter) {
			continue
		}

		allItems = append(allItems, item)
	}

	// Pagination on filtered items
	total := int64(len(allItems))
	start := (page - 1) * pageSize
	if start > len(allItems) {
		start = len(allItems)
	}
	end := start + pageSize
	if end > len(allItems) {
		end = len(allItems)
	}

	paginatedItems := allItems[start:end]

	resp := ProviderListResponse{
		Summary:  summary,
		Items:    paginatedItems,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}

	response.SuccessContext(gatewayJSONResponder{ctx: c}, resp)
}

// GetByID GET /api/v1/admin/providers/:id
func (h *ProviderHandler) GetByID(c *gin.Context) {
	h.GetByIDGateway(gatewayctx.FromGin(c))
}

func (h *ProviderHandler) GetByIDGateway(c gatewayctx.GatewayContext) {
	ctx := c.Request().Context()
	id, err := strconv.ParseInt(c.PathParam("id"), 10, 64)
	if err != nil || id <= 0 {
		response.ErrorFromContext(gatewayJSONResponder{ctx: c}, infraerrors.BadRequest("INVALID_PROVIDER_ID", "Invalid provider ID"))
		return
	}

	acc, err := h.adminService.GetAccount(ctx, id)
	if err != nil || acc == nil {
		response.ErrorFromContext(gatewayJSONResponder{ctx: c}, infraerrors.NotFound("PROVIDER_NOT_FOUND", "Provider not found"))
		return
	}

	resp := h.accountToProviderResponse(ctx, acc)
	response.SuccessContext(gatewayJSONResponder{ctx: c}, resp)
}

// Create POST /api/v1/admin/providers
func (h *ProviderHandler) Create(c *gin.Context) {
	h.CreateGateway(gatewayctx.FromGin(c))
}

func (h *ProviderHandler) CreateGateway(c gatewayctx.GatewayContext) {
	ctx := c.Request().Context()
	var req CreateProviderRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		response.ErrorContext(gatewayJSONResponder{ctx: c}, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	req.ProviderName = strings.TrimSpace(req.ProviderName)
	if req.ProviderName == "" {
		response.ErrorContext(gatewayJSONResponder{ctx: c}, http.StatusBadRequest, "provider_name is required")
		return
	}

	req.APIKey = strings.TrimSpace(req.APIKey)
	if req.APIKey == "" {
		response.ErrorContext(gatewayJSONResponder{ctx: c}, http.StatusBadRequest, "api_key is required")
		return
	}

	prov := yunwu.CanonicalProvider(req.Provider)
	baseURL := strings.TrimSpace(req.BaseURL)
	if baseURL == "" {
		baseURL = yunwu.DefaultBaseURLForProvider(prov)
	}

	domainStr := parseProviderDomain(req.Domain, baseURL)
	protocol := strings.TrimSpace(req.Protocol)
	if protocol == "" {
		protocol = "openai"
	}

	minBalance := 20.0
	if req.MinBalanceUSD != nil && *req.MinBalanceUSD >= 0 {
		minBalance = *req.MinBalanceUSD
	}

	modelFamilies := req.SupportedModelFamilies
	if len(modelFamilies) == 0 {
		modelFamilies = []string{"claude", "gpt"}
	}

	modelMapping := req.ModelMapping
	if modelMapping == nil {
		modelMapping = make(map[string]string)
	}

	// Encrypt credentials
	creds := map[string]any{
		"api_key": req.APIKey,
	}
	if h.secretEncryptor != nil {
		enc, err := credentials.EncryptMap(creds, h.secretEncryptor)
		if err != nil {
			response.ErrorFromContext(gatewayJSONResponder{ctx: c}, infraerrors.InternalServer("ENCRYPT_FAILED", "Failed to encrypt provider API key: "+err.Error()))
			return
		}
		creds = enc
	}

	extra := map[string]any{
		"provider":                  prov,
		"domain":                    domainStr,
		"base_url":                  baseURL,
		"yunwu_protocol":            protocol,
		"supported_model_families": modelFamilies,
		"model_mapping":             modelMapping,
		"min_balance_usd":           minBalance,
		"balance_usd":               0.0,
		"balance_source":            "manual",
		"balance_updated_at":        time.Now().UTC().Format(time.RFC3339),
	}

	groupIDs := req.GroupIDs
	if len(groupIDs) == 0 && h.entClient != nil {
		// Auto-bind to default yunwu group
		yunwuGroup, err := h.entClient.Group.Query().
			Where(
				group.Or(
					group.PlatformEqualFold(domain.PlatformYunwu),
					group.NameEqualFold("yunwu"),
				),
			).First(ctx)
		if err == nil && yunwuGroup != nil {
			groupIDs = []int64{yunwuGroup.ID}
		}
	}

	priority := 1
	if req.Priority != nil {
		priority = *req.Priority
	}

	concurrency := 10
	if req.Concurrency != nil && *req.Concurrency >= 0 {
		concurrency = *req.Concurrency
	}

	notes := req.Notes

	acc, err := h.adminService.CreateAccount(ctx, &service.CreateAccountInput{
		Name:                  req.ProviderName,
		Notes:                 &notes,
		Platform:              domain.PlatformYunwu,
		Type:                  "apikey",
		Credentials:           creds,
		Extra:                 extra,
		GroupIDs:              groupIDs,
		Priority:              priority,
		Concurrency:           concurrency,
		SkipMixedChannelCheck: true,
	})
	if err != nil {
		response.ErrorFromContext(gatewayJSONResponder{ctx: c}, err)
		return
	}

	if req.Status == "disabled" {
		updated, uErr := h.adminService.UpdateAccount(ctx, acc.ID, &service.UpdateAccountInput{
			Status: "disabled",
		})
		if uErr == nil && updated != nil {
			acc = updated
		}
	}

	resp := h.accountToProviderResponse(ctx, acc)
	response.SuccessContext(gatewayJSONResponder{ctx: c}, resp)
}

// Update PUT /api/v1/admin/providers/:id
func (h *ProviderHandler) Update(c *gin.Context) {
	h.UpdateGateway(gatewayctx.FromGin(c))
}

func (h *ProviderHandler) UpdateGateway(c gatewayctx.GatewayContext) {
	ctx := c.Request().Context()
	id, err := strconv.ParseInt(c.PathParam("id"), 10, 64)
	if err != nil || id <= 0 {
		response.ErrorFromContext(gatewayJSONResponder{ctx: c}, infraerrors.BadRequest("INVALID_PROVIDER_ID", "Invalid provider ID"))
		return
	}

	var req UpdateProviderRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		response.ErrorContext(gatewayJSONResponder{ctx: c}, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	acc, err := h.adminService.GetAccount(ctx, id)
	if err != nil || acc == nil {
		response.ErrorFromContext(gatewayJSONResponder{ctx: c}, infraerrors.NotFound("PROVIDER_NOT_FOUND", "Provider not found"))
		return
	}

	extra := acc.Extra
	if extra == nil {
		extra = make(map[string]any)
	}

	if req.Provider != "" {
		extra["provider"] = yunwu.CanonicalProvider(req.Provider)
	}
	if req.BaseURL != "" {
		extra["base_url"] = strings.TrimSpace(req.BaseURL)
	}
	if req.Domain != "" {
		extra["domain"] = strings.TrimSpace(req.Domain)
	} else if req.BaseURL != "" {
		extra["domain"] = parseProviderDomain("", req.BaseURL)
	}
	if req.Protocol != "" {
		extra["yunwu_protocol"] = req.Protocol
	}
	if req.SupportedModelFamilies != nil {
		extra["supported_model_families"] = req.SupportedModelFamilies
	}
	if req.ModelMapping != nil {
		extra["model_mapping"] = req.ModelMapping
	}
	if req.MinBalanceUSD != nil && *req.MinBalanceUSD >= 0 {
		extra["min_balance_usd"] = *req.MinBalanceUSD
	}

	name := acc.Name
	if strings.TrimSpace(req.ProviderName) != "" {
		name = strings.TrimSpace(req.ProviderName)
	}

	status := acc.Status
	if req.Status != "" {
		status = req.Status
	}

	creds := acc.Credentials
	trimmedKey := strings.TrimSpace(req.APIKey)
	if trimmedKey != "" && !strings.Contains(trimmedKey, "***") {
		newCreds := map[string]any{"api_key": trimmedKey}
		if h.secretEncryptor != nil {
			if enc, err := credentials.EncryptMap(newCreds, h.secretEncryptor); err == nil {
				creds = enc
			}
		} else {
			creds = newCreds
		}
	}

	updateInput := &service.UpdateAccountInput{
		Name:                  name,
		Notes:                 req.Notes,
		Type:                  acc.Type,
		Credentials:           creds,
		Extra:                 extra,
		Status:                status,
		Priority:              req.Priority,
		Concurrency:           req.Concurrency,
		GroupIDs:              req.GroupIDs,
		SkipMixedChannelCheck: true,
	}

	updatedAcc, err := h.adminService.UpdateAccount(ctx, id, updateInput)
	if err != nil {
		response.ErrorFromContext(gatewayJSONResponder{ctx: c}, err)
		return
	}

	resp := h.accountToProviderResponse(ctx, updatedAcc)
	response.SuccessContext(gatewayJSONResponder{ctx: c}, resp)
}

// Delete DELETE /api/v1/admin/providers/:id
func (h *ProviderHandler) Delete(c *gin.Context) {
	h.DeleteGateway(gatewayctx.FromGin(c))
}

func (h *ProviderHandler) DeleteGateway(c gatewayctx.GatewayContext) {
	ctx := c.Request().Context()
	id, err := strconv.ParseInt(c.PathParam("id"), 10, 64)
	if err != nil || id <= 0 {
		response.ErrorFromContext(gatewayJSONResponder{ctx: c}, infraerrors.BadRequest("INVALID_PROVIDER_ID", "Invalid provider ID"))
		return
	}

	if err := h.adminService.DeleteAccount(ctx, id); err != nil {
		response.ErrorFromContext(gatewayJSONResponder{ctx: c}, err)
		return
	}

	nodeID := strconv.FormatInt(id, 10)
	if h.nodeManager != nil {
		_ = h.nodeManager.DeleteNode(ctx, nodeID)
		_ = h.nodeManager.DeleteNodeRuntimeState(ctx, nodeID)
	}

	response.SuccessContext(gatewayJSONResponder{ctx: c}, map[string]any{"deleted": true, "id": id})
}

// SyncBalance POST /api/v1/admin/providers/:id/sync-balance
func (h *ProviderHandler) SyncBalance(c *gin.Context) {
	h.SyncBalanceGateway(gatewayctx.FromGin(c))
}

func (h *ProviderHandler) SyncBalanceGateway(c gatewayctx.GatewayContext) {
	ctx := c.Request().Context()
	id, err := strconv.ParseInt(c.PathParam("id"), 10, 64)
	if err != nil || id <= 0 {
		response.ErrorFromContext(gatewayJSONResponder{ctx: c}, infraerrors.BadRequest("INVALID_PROVIDER_ID", "Invalid provider ID"))
		return
	}

	acc, err := h.adminService.GetAccount(ctx, id)
	if err != nil || acc == nil {
		response.ErrorFromContext(gatewayJSONResponder{ctx: c}, infraerrors.NotFound("PROVIDER_NOT_FOUND", "Provider not found"))
		return
	}

	if h.statsWorker == nil {
		response.ErrorFromContext(gatewayJSONResponder{ctx: c}, infraerrors.InternalServer("SERVICE_UNAVAILABLE", "Balance sync service unavailable"))
		return
	}

	decrypted, err := credentials.DecryptMap(acc.Credentials, h.secretEncryptor)
	if err != nil {
		response.ErrorFromContext(gatewayJSONResponder{ctx: c}, infraerrors.InternalServer("DECRYPT_FAILED", "Failed to decrypt provider credentials"))
		return
	}

	apiKey := ""
	for _, keyName := range []string{"api_key", "token", "access_token", "setup_token", "session_key", "refresh_token"} {
		if k, ok := decrypted[keyName].(string); ok && k != "" {
			apiKey = k
			break
		}
	}
	if apiKey == "" {
		response.ErrorFromContext(gatewayJSONResponder{ctx: c}, infraerrors.BadRequest("NO_API_KEY", "Provider has no API key configured"))
		return
	}

	prov := yunwu.ParseNodeProvider(acc.Extra)
	if prov == "openlux" && acc.Platform != "" && acc.Platform != domain.PlatformYunwu {
		prov = acc.Platform
	}
	baseURL, _ := acc.Extra["base_url"].(string)
	if baseURL == "" {
		switch strings.ToLower(acc.Platform) {
		case domain.PlatformAnthropic:
			baseURL = "https://api.anthropic.com"
		case domain.PlatformOpenAI:
			baseURL = "https://api.openai.com/v1"
		case domain.PlatformGemini:
			baseURL = "https://generativelanguage.googleapis.com"
		case domain.PlatformAntigravity:
			baseURL = "https://api.antigravity.ai"
		default:
			baseURL = yunwu.DefaultBaseURLForProvider(prov)
		}
	}

	nodeID := strconv.FormatInt(id, 10)
	node := &health.ExecutionNode{
		NodeID:      nodeID,
		AccountName: acc.Name,
		Provider:    prov,
		State:       "healthy",
	}

	grants, err := h.statsWorker.SyncNodeStats(ctx, node, apiKey, baseURL)
	if err != nil {
		response.ErrorContext(gatewayJSONResponder{ctx: c}, http.StatusBadRequest, "Sync failed: "+err.Error())
		return
	}

	// Update account with synced balance
	nowStr := time.Now().UTC().Format(time.RFC3339)
	extra := acc.Extra
	if extra == nil {
		extra = make(map[string]any)
	}
	extra["balance_usd"] = grants.TotalAvailable
	extra["balance_updated_at"] = nowStr
	extra["balance_source"] = grants.Object
	_, _ = h.adminService.UpdateAccount(ctx, id, &service.UpdateAccountInput{
		Extra:                 extra,
		SkipMixedChannelCheck: true,
	})

	// Check if below threshold and send low-balance alert
	minBalance := 20.0
	if mb, ok := extra["min_balance_usd"].(float64); ok && mb >= 0 {
		minBalance = mb
	}
	if grants.TotalAvailable <= minBalance && h.statsWorker.Notifier() != nil {
		_, _ = h.statsWorker.Notifier().CheckAndNotify(ctx, nodeID, acc.Name, prov, grants.TotalAvailable)
	}

	response.SuccessContext(gatewayJSONResponder{ctx: c}, map[string]any{
		"id":              id,
		"balance_usd":     grants.TotalAvailable,
		"total_granted":   grants.TotalGranted,
		"total_used":      grants.TotalUsed,
		"source":          grants.Object,
		"last_synced_at":  nowStr,
	})
}

// SyncAllBalances POST /api/v1/admin/providers/sync-all
func (h *ProviderHandler) SyncAllBalances(c *gin.Context) {
	h.SyncAllBalancesGateway(gatewayctx.FromGin(c))
}

func (h *ProviderHandler) SyncAllBalancesGateway(c gatewayctx.GatewayContext) {
	ctx := c.Request().Context()
	if h.statsWorker == nil {
		response.ErrorFromContext(gatewayJSONResponder{ctx: c}, infraerrors.InternalServer("SERVICE_UNAVAILABLE", "Balance sync service unavailable"))
		return
	}

	accounts, _, err := h.adminService.ListAccounts(ctx, 1, 1000, "", "", "active", "", "", "", "", 0)
	if err != nil {
		response.ErrorFromContext(gatewayJSONResponder{ctx: c}, err)
		return
	}

	var mu sync.Mutex
	summary := BatchSyncSummary{
		Total: len(accounts),
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, 5) // max 5 concurrent syncs

	for i := range accounts {
		acc := accounts[i]
		wg.Add(1)
		sem <- struct{}{}
		go func(a service.Account) {
			defer wg.Done()
			defer func() { <-sem }()

			decrypted, err := credentials.DecryptMap(a.Credentials, h.secretEncryptor)
			if err != nil {
				mu.Lock()
				summary.Failed++
				mu.Unlock()
				return
			}

			apiKey := ""
			for _, keyName := range []string{"api_key", "token", "access_token", "setup_token", "session_key", "refresh_token"} {
				if k, ok := decrypted[keyName].(string); ok && k != "" {
					apiKey = k
					break
				}
			}
			if apiKey == "" {
				mu.Lock()
				summary.Failed++
				mu.Unlock()
				return
			}

			prov := yunwu.ParseNodeProvider(a.Extra)
			if prov == "openlux" && a.Platform != "" && a.Platform != domain.PlatformYunwu {
				prov = a.Platform
			}
			baseURL, _ := a.Extra["base_url"].(string)
			if baseURL == "" {
				switch strings.ToLower(a.Platform) {
				case domain.PlatformAnthropic:
					baseURL = "https://api.anthropic.com"
				case domain.PlatformOpenAI:
					baseURL = "https://api.openai.com/v1"
				case domain.PlatformGemini:
					baseURL = "https://generativelanguage.googleapis.com"
				case domain.PlatformAntigravity:
					baseURL = "https://api.antigravity.ai"
				default:
					baseURL = yunwu.DefaultBaseURLForProvider(prov)
				}
			}

			nodeID := strconv.FormatInt(a.ID, 10)
			node := &health.ExecutionNode{
				NodeID:      nodeID,
				AccountName: a.Name,
				Provider:    prov,
				State:       "healthy",
			}

			grants, syncErr := h.statsWorker.SyncNodeStats(ctx, node, apiKey, baseURL)
			mu.Lock()
			if syncErr != nil {
				summary.Failed++
			} else {
				summary.Synced++
				summary.TotalBalanceUSD += grants.TotalAvailable

				nowStr := time.Now().UTC().Format(time.RFC3339)
				extra := a.Extra
				if extra == nil {
					extra = make(map[string]any)
				}
				extra["balance_usd"] = grants.TotalAvailable
				extra["balance_updated_at"] = nowStr
				extra["balance_source"] = grants.Object
				_, _ = h.adminService.UpdateAccount(ctx, a.ID, &service.UpdateAccountInput{
					Extra:                 extra,
					SkipMixedChannelCheck: true,
				})

				minBalance := 20.0
				if mb, ok := extra["min_balance_usd"].(float64); ok && mb >= 0 {
					minBalance = mb
				}
				if grants.TotalAvailable <= minBalance && h.statsWorker.Notifier() != nil {
					_, _ = h.statsWorker.Notifier().CheckAndNotify(ctx, nodeID, a.Name, prov, grants.TotalAvailable)
				}
			}
			mu.Unlock()
		}(acc)
	}

	wg.Wait()

	response.SuccessContext(gatewayJSONResponder{ctx: c}, summary)
}

// TestConnection POST /api/v1/admin/providers/:id/test
func (h *ProviderHandler) TestConnection(c *gin.Context) {
	h.TestConnectionGateway(gatewayctx.FromGin(c))
}

func (h *ProviderHandler) TestConnectionGateway(c gatewayctx.GatewayContext) {
	ctx := c.Request().Context()
	id, err := strconv.ParseInt(c.PathParam("id"), 10, 64)
	if err != nil || id <= 0 {
		response.ErrorFromContext(gatewayJSONResponder{ctx: c}, infraerrors.BadRequest("INVALID_PROVIDER_ID", "Invalid provider ID"))
		return
	}

	acc, err := h.adminService.GetAccount(ctx, id)
	if err != nil || acc == nil {
		response.ErrorFromContext(gatewayJSONResponder{ctx: c}, infraerrors.NotFound("PROVIDER_NOT_FOUND", "Provider not found"))
		return
	}

	decrypted, err := credentials.DecryptMap(acc.Credentials, h.secretEncryptor)
	if err != nil {
		response.ErrorFromContext(gatewayJSONResponder{ctx: c}, infraerrors.InternalServer("DECRYPT_FAILED", "Failed to decrypt provider credentials"))
		return
	}

	apiKey := ""
	for _, keyName := range []string{"api_key", "token", "access_token", "setup_token", "session_key", "refresh_token"} {
		if k, ok := decrypted[keyName].(string); ok && k != "" {
			apiKey = k
			break
		}
	}
	baseURL, _ := acc.Extra["base_url"].(string)
	if baseURL == "" {
		switch strings.ToLower(acc.Platform) {
		case domain.PlatformAnthropic:
			baseURL = "https://api.anthropic.com"
		case domain.PlatformOpenAI:
			baseURL = "https://api.openai.com/v1"
		case domain.PlatformGemini:
			baseURL = "https://generativelanguage.googleapis.com"
		case domain.PlatformAntigravity:
			baseURL = "https://api.antigravity.ai"
		default:
			baseURL = yunwu.DefaultBaseURLForProvider(yunwu.ParseNodeProvider(acc.Extra))
		}
	}

	res := h.performConnectionTest(ctx, baseURL, apiKey)
	response.SuccessContext(gatewayJSONResponder{ctx: c}, res)
}

// TestDraftConnection POST /api/v1/admin/providers/test
func (h *ProviderHandler) TestDraftConnection(c *gin.Context) {
	h.TestDraftConnectionGateway(gatewayctx.FromGin(c))
}

func (h *ProviderHandler) TestDraftConnectionGateway(c gatewayctx.GatewayContext) {
	ctx := c.Request().Context()
	var req TestConnectionRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		response.ErrorContext(gatewayJSONResponder{ctx: c}, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	req.BaseURL = strings.TrimSpace(req.BaseURL)
	req.APIKey = strings.TrimSpace(req.APIKey)

	if req.BaseURL == "" {
		response.ErrorContext(gatewayJSONResponder{ctx: c}, http.StatusBadRequest, "base_url is required")
		return
	}
	if req.APIKey == "" {
		response.ErrorContext(gatewayJSONResponder{ctx: c}, http.StatusBadRequest, "api_key is required")
		return
	}

	res := h.performConnectionTest(ctx, req.BaseURL, req.APIKey)
	response.SuccessContext(gatewayJSONResponder{ctx: c}, res)
}

// performConnectionTest executes an HTTP test request to upstream base_url/v1/models and measures latency.
func (h *ProviderHandler) performConnectionTest(ctx context.Context, baseURL, apiKey string) TestConnectionResponse {
	cleanBase := strings.TrimRight(strings.TrimSuffix(strings.TrimRight(baseURL, "/"), "/v1"), "/")
	modelsURL := cleanBase + "/v1/models"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
	if err != nil {
		return TestConnectionResponse{
			Success: false,
			Message: "Invalid URL: " + err.Error(),
		}
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Accept", "application/json")

	start := time.Now()
	resp, err := h.httpClient.Do(req)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return TestConnectionResponse{
			Success:   false,
			LatencyMs: latency,
			Message:   "Network error: " + err.Error(),
		}
	}
	defer func() { _ = resp.Body.Close() }()

	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))

	if resp.StatusCode == http.StatusOK {
		var modelsPayload struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		_ = json.Unmarshal(bodyBytes, &modelsPayload)
		count := len(modelsPayload.Data)

		return TestConnectionResponse{
			Success:    true,
			LatencyMs:  latency,
			StatusCode: resp.StatusCode,
			ModelCount: count,
			Message:    fmt.Sprintf("Connection successful (discovered %d models)", count),
		}
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return TestConnectionResponse{
			Success:    false,
			LatencyMs:  latency,
			StatusCode: resp.StatusCode,
			Message:    "Authentication failed: invalid API key or permissions",
		}
	}

	msg := strings.TrimSpace(string(bodyBytes))
	if len(msg) > 200 {
		msg = msg[:200] + "..."
	}
	if msg == "" {
		msg = http.StatusText(resp.StatusCode)
	}

	return TestConnectionResponse{
		Success:    false,
		LatencyMs:  latency,
		StatusCode: resp.StatusCode,
		Message:    fmt.Sprintf("Upstream returned HTTP %d: %s", resp.StatusCode, msg),
	}
}

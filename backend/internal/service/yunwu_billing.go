package service

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/yunwu"
)

// RecordYunwuUsageInput carries the minimal information needed to bill an end
// user for a request that was executed through the Yunwu (unofficial providers)
// hypervisor engine. The engine resolves the concrete execution node; the
// gateway layer converts it into a normal usage/billing record so that user
// balance is deducted and usage_logs is written exactly like official routes.
type RecordYunwuUsageInput struct {
	Result           *ForwardResult
	APIKey           *APIKey
	User             *User
	AccountID        string
	Subscription     *UserSubscription
	InboundEndpoint  string
	UpstreamEndpoint string
	UserAgent        string
	IPAddress        string
	APIKeyService    APIKeyQuotaUpdater
}

// recordYunwuAccount loads the yunwu account that served a request, used as the
// billing account. The hypervisor identifies the executing node by account ID.
func (s *GatewayService) recordYunwuAccount(ctx context.Context, accountID string) *Account {
	if s == nil || s.accountRepo == nil {
		return nil
	}
	id, err := strconv.ParseInt(strings.TrimSpace(accountID), 10, 64)
	if err != nil || id <= 0 {
		return nil
	}
	acc, err := s.accountRepo.GetByID(ctx, id)
	if err != nil || acc == nil {
		return nil
	}
	return acc
}

// RecordYunwuUsage records usage and deducts the end-user balance for a request
// served by the Yunwu hypervisor (Anthropic /v1/messages protocol).
func (s *GatewayService) RecordYunwuUsage(ctx context.Context, input *RecordYunwuUsageInput) error {
	if s == nil || input == nil || input.Result == nil || input.APIKey == nil || input.User == nil {
		return nil
	}
	account := s.recordYunwuAccount(ctx, input.AccountID)
	if account == nil {
		slog.Warn("[YunwuBilling] Skipping usage billing: execution account not found",
			"account_id", input.AccountID,
			"model", input.Result.Model,
			"user_id", input.User.ID,
		)
		return nil
	}
	// Match catalog retail pricing (max across active providers).
	billingCtx := ContextWithYunwuMaxPricing(ctx)
	return s.RecordUsage(billingCtx, &RecordUsageInput{
		Result:           input.Result,
		APIKey:           input.APIKey,
		User:             input.User,
		Account:          account,
		Subscription:     input.Subscription,
		InboundEndpoint:  input.InboundEndpoint,
		UpstreamEndpoint: input.UpstreamEndpoint,
		UserAgent:        input.UserAgent,
		IPAddress:        input.IPAddress,
		APIKeyService:    input.APIKeyService,
	})
}

// OpenAIRecordYunwuUsageInput is the OpenAI-protocol counterpart.
type OpenAIRecordYunwuUsageInput struct {
	Result           *OpenAIForwardResult
	APIKey           *APIKey
	User             *User
	AccountID        string
	Subscription     *UserSubscription
	InboundEndpoint  string
	UpstreamEndpoint string
	UserAgent        string
	IPAddress        string
	APIKeyService    APIKeyQuotaUpdater
}

// RecordYunwuUsage records usage and deducts the end-user balance for a request
// served by the Yunwu hypervisor (OpenAI /v1/chat/completions and /responses).
func (s *OpenAIGatewayService) RecordYunwuUsage(ctx context.Context, input *OpenAIRecordYunwuUsageInput) error {
	if s == nil || input == nil || input.Result == nil || input.APIKey == nil || input.User == nil {
		return nil
	}
	if s.accountRepo == nil {
		return nil
	}
	id, err := strconv.ParseInt(strings.TrimSpace(input.AccountID), 10, 64)
	if err != nil || id <= 0 {
		return nil
	}
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil || account == nil {
		slog.Warn("[YunwuBilling] Skipping OpenAI usage billing: execution account not found",
			"account_id", input.AccountID,
			"model", input.Result.Model,
			"user_id", input.User.ID,
		)
		return nil
	}
	billingCtx := ContextWithYunwuMaxPricing(ctx)
	return s.RecordUsage(billingCtx, &OpenAIRecordUsageInput{
		Result:           input.Result,
		APIKey:           input.APIKey,
		User:             input.User,
		Account:          account,
		Subscription:     input.Subscription,
		InboundEndpoint:  input.InboundEndpoint,
		UpstreamEndpoint: input.UpstreamEndpoint,
		UserAgent:        input.UserAgent,
		IPAddress:        input.IPAddress,
		APIKeyService:    input.APIKeyService,
	})
}

// BuildYunwuForwardResult assembles a billing result from hypervisor metadata
// for the Anthropic protocol.
func BuildYunwuForwardResult(requestID, model string, inputTokens, outputTokens int64, stream bool, started time.Time) *ForwardResult {
	return &ForwardResult{
		RequestID: requestID,
		Model:     model,
		Stream:    stream,
		Duration:  time.Since(started),
		Usage: ClaudeUsage{
			InputTokens:  int(inputTokens),
			OutputTokens: int(outputTokens),
		},
	}
}

// BuildYunwuOpenAIForwardResult assembles a billing result for the OpenAI protocol.
func BuildYunwuOpenAIForwardResult(requestID, model string, inputTokens, outputTokens int64, stream bool, started time.Time) *OpenAIForwardResult {
	return &OpenAIForwardResult{
		RequestID: requestID,
		Model:     model,
		Stream:    stream,
		Duration:  time.Since(started),
		Usage: OpenAIUsage{
			InputTokens:  int(inputTokens),
			OutputTokens: int(outputTokens),
		},
	}
}

// WithYunwuModelRouting resolves the explicit Primary/Backup route for a model
// from the API key's group and attaches it to the context so the Yunwu engine
// restricts candidate nodes to the administrator-configured route.
func WithYunwuModelRouting(ctx context.Context, group *Group, model string) context.Context {
	if ctx == nil || group == nil || !group.ModelRoutingEnabled || len(group.ModelRouting) == 0 {
		return ctx
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return ctx
	}
	keys := make([]string, 0, 3)
	add := func(k string) {
		k = strings.ToLower(strings.TrimSpace(k))
		if k == "" {
			return
		}
		for _, existing := range keys {
			if existing == k {
				return
			}
		}
		keys = append(keys, k)
	}
	add(model)
	add(ToCanonicalModelID(model))
	if idx := strings.LastIndex(model, "/"); idx >= 0 && idx+1 < len(model) {
		add(model[idx+1:])
	}
	for _, k := range keys {
		if ids, ok := group.ModelRouting[k]; ok && len(ids) > 0 {
			return yunwu.ContextWithModelRouting(ctx, ids)
		}
	}
	// Case-insensitive fallback over map keys.
	for key, ids := range group.ModelRouting {
		lk := strings.ToLower(strings.TrimSpace(key))
		for _, want := range keys {
			if lk == want && len(ids) > 0 {
				return yunwu.ContextWithModelRouting(ctx, ids)
			}
		}
	}
	return ctx
}

// RecordYunwuMediaUsageInput carries the information needed to bill a completed
// Unofficial-provider media request (image / video / audio) that has no token
// usage to bill against.
type RecordYunwuMediaUsageInput struct {
	APIKey               *APIKey
	User                 *User
	AccountID            string
	Subscription         *UserSubscription
	Model                string
	MediaKind            string
	Units                int64
	UpstreamCostMicroUSD int64
	UserAgent            string
	IPAddress            string
	APIKeyService        APIKeyQuotaUpdater
}

// normalizeYunwuMediaKind maps engine media kinds to usage-log media types.
func normalizeYunwuMediaKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "images", "image":
		return "image"
	case "video", "videos":
		return "video"
	case "audio", "audio_input":
		return "audio"
	case "media":
		return "media"
	default:
		return "media"
	}
}

// recordYunwuMediaUsage writes a usage log and deducts the end-user balance for
// a flat-rate / per-unit Unofficial-provider request. It reuses the shared
// billing path (applyUsageBilling) so dedup/atomicity match token billing.
func recordYunwuMediaUsage(ctx context.Context, deps *billingDeps, billRepo UsageBillingRepository, logRepo UsageLogRepository, in *RecordYunwuMediaUsageInput, multiplier float64) error {
	if in == nil || deps == nil || in.APIKey == nil || in.User == nil {
		return nil
	}
	if in.UpstreamCostMicroUSD <= 0 || deps.accountRepo == nil {
		return nil
	}
	id, err := strconv.ParseInt(strings.TrimSpace(in.AccountID), 10, 64)
	if err != nil || id <= 0 {
		return nil
	}
	account, err := deps.accountRepo.GetByID(ctx, id)
	if err != nil || account == nil {
		slog.Warn("[YunwuBilling] Skipping media billing: execution account not found",
			"account_id", in.AccountID, "model", in.Model, "user_id", in.User.ID)
		return nil
	}
	if multiplier <= 0 {
		multiplier = 1.0
	}
	total := float64(in.UpstreamCostMicroUSD) / 1_000_000.0
	cost := &CostBreakdown{
		InputCost:  total,
		TotalCost:  total,
		ActualCost: total * multiplier,
	}

	isSubscriptionBilling := in.Subscription != nil && in.APIKey.Group != nil && in.APIKey.Group.IsSubscriptionType()
	billingType := BillingTypeBalance
	if isSubscriptionBilling {
		billingType = BillingTypeSubscription
	}
	accountRateMultiplier := account.BillingRateMultiplier()
	mediaKind := normalizeYunwuMediaKind(in.MediaKind)
	durationMs := 0
	requestID := resolveUsageBillingRequestID(ctx, "")
	usageLog := &UsageLog{
		UserID:                in.User.ID,
		APIKeyID:              in.APIKey.ID,
		AccountID:             account.ID,
		RequestID:             requestID,
		Model:                 in.Model,
		RequestedModel:        in.Model,
		InputCost:             cost.InputCost,
		TotalCost:             cost.TotalCost,
		ActualCost:            cost.ActualCost,
		RateMultiplier:        multiplier,
		AccountRateMultiplier: &accountRateMultiplier,
		BillingType:           billingType,
		MediaType:             &mediaKind,
		DurationMs:            &durationMs,
		CreatedAt:             time.Now(),
	}
	if in.APIKey.GroupID != nil {
		usageLog.GroupID = in.APIKey.GroupID
	}
	if in.Subscription != nil {
		usageLog.SubscriptionID = &in.Subscription.ID
	}
	if in.UserAgent != "" {
		ua := in.UserAgent
		usageLog.UserAgent = &ua
	}
	if in.IPAddress != "" {
		ip := in.IPAddress
		usageLog.IPAddress = &ip
	}

	if _, err := applyUsageBilling(ctx, requestID, usageLog, &postUsageBillingParams{
		Cost:                  cost,
		User:                  in.User,
		APIKey:                in.APIKey,
		Account:               account,
		Subscription:          in.Subscription,
		IsSubscriptionBill:    isSubscriptionBilling,
		AccountRateMultiplier: accountRateMultiplier,
		APIKeyService:         in.APIKeyService,
	}, deps, billRepo); err != nil {
		return err
	}
	writeUsageLogBestEffort(ctx, logRepo, usageLog, "service.yunwu_media")
	return nil
}

// yunwuMediaMultiplier resolves the effective retail markup for a media request.
func yunwuMediaMultiplier(group *Group, cfg *config.Config) float64 {
	if group != nil && group.RateMultiplier > 0 {
		return group.RateMultiplier
	}
	if cfg != nil && cfg.Default.RateMultiplier > 0 {
		return cfg.Default.RateMultiplier
	}
	return 1.10
}

// RecordYunwuMediaUsage bills a flat-rate / per-unit Unofficial-provider media
// request (Anthropic-protocol gateway handler).
func (s *GatewayService) RecordYunwuMediaUsage(ctx context.Context, in *RecordYunwuMediaUsageInput) error {
	if s == nil || in == nil {
		return nil
	}
	var group *Group
	if in.APIKey != nil {
		group = in.APIKey.Group
	}
	multiplier := yunwuMediaMultiplier(group, s.cfg)
	if s.settingService != nil {
		multiplier = s.settingService.ApplyModelMarkup(ctx, in.Model, multiplier)
	}
	return recordYunwuMediaUsage(ctx, s.billingDeps(), s.usageBillingRepo, s.usageLogRepo, in, multiplier)
}

// RecordYunwuMediaUsage bills a flat-rate / per-unit Unofficial-provider media
// request (OpenAI-protocol gateway handler).
func (s *OpenAIGatewayService) RecordYunwuMediaUsage(ctx context.Context, in *RecordYunwuMediaUsageInput) error {
	if s == nil || in == nil {
		return nil
	}
	var group *Group
	if in.APIKey != nil {
		group = in.APIKey.Group
	}
	multiplier := yunwuMediaMultiplier(group, s.cfg)
	if s.billingService != nil {
		multiplier = s.billingService.ApplyModelMarkup(ctx, in.Model, multiplier)
	}
	return recordYunwuMediaUsage(ctx, s.billingDeps(), s.usageBillingRepo, s.usageLogRepo, in, multiplier)
}

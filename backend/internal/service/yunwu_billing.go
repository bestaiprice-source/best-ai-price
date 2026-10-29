package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
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

// yunwuProviderScopeTTL bounds how long the account-derived provider scope is
// cached, so the retail-price basis does not require a DB query per request.
const yunwuProviderScopeTTL = 60 * time.Second

var (
	yunwuScopeMu       sync.RWMutex
	yunwuScopeAt       time.Time
	yunwuScopeByFamily map[string][]string
	yunwuPricingScope  YunwuPricingScope
)

// yunwuProviderScope returns the unofficial providers we actually hold
// schedulable accounts for, keyed by the model family each account supports
// (cached briefly). With this scope the retail "primary" provider is the cheapest
// provider WE can serve the model from. Returns nil when no accounts are found,
// which leaves the resolver unrestricted (identical for catalog and billing, so
// parity holds).
func yunwuProviderScope(ctx context.Context, repo AccountRepository) map[string][]string {
	scope, _ := yunwuScopeSnapshot(ctx, repo)
	return scope
}

// yunwuTierGroupScope returns, per canonical provider, the upstream billing
// groups its accounts pinned their keys to (`smart_group_name` /
// `fast_group_name` in account.Extra). The catalog and the charge resolve a
// tier's rate from this map, so the published price equals the billed one.
func yunwuTierGroupScope(ctx context.Context, repo AccountRepository) map[string]yunwu.TierGroupNames {
	_, pricing := yunwuScopeSnapshot(ctx, repo)
	return pricing.TierGroups
}

// yunwuScopeSnapshot returns the cached provider scope and tier-group map,
// computing both in ONE pass over the schedulable accounts.
func yunwuScopeSnapshot(ctx context.Context, repo AccountRepository) (map[string][]string, YunwuPricingScope) {
	if repo == nil {
		return nil, YunwuPricingScope{}
	}
	yunwuScopeMu.RLock()
	if time.Since(yunwuScopeAt) < yunwuProviderScopeTTL {
		family, pricing := yunwuScopeByFamily, yunwuPricingScope
		yunwuScopeMu.RUnlock()
		return family, pricing
	}
	yunwuScopeMu.RUnlock()

	family, pricing := computeYunwuScope(ctx, repo)

	yunwuScopeMu.Lock()
	yunwuScopeByFamily = family
	yunwuPricingScope = pricing
	yunwuScopeAt = time.Now()
	yunwuScopeMu.Unlock()
	return family, pricing
}

func computeYunwuProviderScope(ctx context.Context, repo AccountRepository) map[string][]string {
	scope, _ := computeYunwuScope(ctx, repo)
	return scope
}

func computeYunwuScope(ctx context.Context, repo AccountRepository) (map[string][]string, YunwuPricingScope) {
	accounts, err := repo.ListSchedulable(ctx)
	if err != nil {
		return nil, YunwuPricingScope{}
	}
	scope := make(map[string][]string)
	tierGroups := make(map[string]yunwu.TierGroupNames)
	observed := make(map[string]float64)
	seen := make(map[string]map[string]bool)
	add := func(family, provider string) {
		if family == "" || provider == "" {
			return
		}
		if seen[family] == nil {
			seen[family] = make(map[string]bool)
		}
		if seen[family][provider] {
			return
		}
		seen[family][provider] = true
		scope[family] = append(scope[family], provider)
	}
	for _, acc := range accounts {
		if !strings.EqualFold(acc.Platform, PlatformYunwu) {
			continue
		}
		// The engine only ever routes to a yunwu account that is bound to a group
		// (getAllYunwuNodes applies HasAccountGroups, and AccountService refuses to
		// create/update an ungrouped yunwu account). An account the engine can never
		// use must NOT pull the published/charged retail price down: the request
		// would be served by a different, more expensive provider, so every such
		// request would be sold below its own cost. Keep the scope no wider than
		// the engine's candidate set in this direction.
		if len(acc.AccountGroups) == 0 {
			continue
		}
		provider := yunwu.ParseNodeProvider(acc.Extra)
		// The provider bills whichever group its key was issued in; the groups
		// pinned on an account price that account's requests. Several accounts of
		// one provider normally pin the same groups — the first configured value
		// wins (the engine still prices each node with its OWN groups).
		if pinned := yunwu.ParseTierGroupNames(acc.Extra); !pinned.IsZero() {
			canonical := CanonicalPricingProvider(provider)
			tierGroups[canonical] = tierGroups[canonical].Merge(pinned)
		}
		// The observed multiplier is a FLOOR: when several accounts of one provider
		// disagree, the highest (safest) reading wins so a cheap-looking account can
		// never pull the price below what another account really pays.
		if ratio := yunwu.ObservedGroupRatioFromExtra(acc.Extra); ratio > 0 && yunwu.ObservedRatioEnabled(acc.Extra) {
			canonical := CanonicalPricingProvider(provider)
			if ratio > observed[canonical] {
				observed[canonical] = ratio
			}
		}
		// A node only prices models of the families it advertises.
		for _, fam := range yunwu.ParseNodeModelFamilies(acc.Extra) {
			add(yunwu.CanonicalFamily(fam), provider)
		}
	}
	return scope, YunwuPricingScope{TierGroups: tierGroups, Observed: observed}
}

// withYunwuProviderScope attaches the account-derived, family-aware provider scope
// to ctx when available, so both catalog and billing resolve the same primary
// provider. The pinned tier groups travel in the same context, so the price a
// tier resolves to is the price of the group its key bills in.
func withYunwuProviderScope(ctx context.Context, repo AccountRepository) context.Context {
	familyScope, pricing := yunwuScopeSnapshot(ctx, repo)
	if len(pricing.TierGroups) > 0 || len(pricing.Observed) > 0 {
		ctx = ContextWithYunwuPricingScope(ctx, pricing)
	}
	if len(familyScope) > 0 {
		return ContextWithYunwuProviderScope(ctx, familyScope)
	}
	return ctx
}

// yunwuTierGroupsKey keys the per-provider pricing knobs in a context.
type yunwuTierGroupsKey struct{}

// YunwuPricingScope carries the pricing knobs derived from the schedulable
// accounts, per canonical provider: the group each tier's key bills in and the
// FLOOR implied by the provider's own accounting (observed_group_ratio).
type YunwuPricingScope struct {
	TierGroups map[string]yunwu.TierGroupNames
	Observed   map[string]float64
}

// ContextWithYunwuPricingScope attaches a provider pricing scope to a context.
func ContextWithYunwuPricingScope(ctx context.Context, scope YunwuPricingScope) context.Context {
	if ctx == nil || (len(scope.TierGroups) == 0 && len(scope.Observed) == 0) {
		return ctx
	}
	return context.WithValue(ctx, yunwuTierGroupsKey{}, scope)
}

// ContextWithYunwuTierGroups attaches the provider→pinned-groups map to a context.
func ContextWithYunwuTierGroups(ctx context.Context, groups map[string]yunwu.TierGroupNames) context.Context {
	return ContextWithYunwuPricingScope(ctx, YunwuPricingScope{TierGroups: groups})
}

// yunwuPricingScopeFromContext returns the scope carried by a context.
func yunwuPricingScopeFromContext(ctx context.Context) (YunwuPricingScope, bool) {
	if ctx == nil {
		return YunwuPricingScope{}, false
	}
	scope, ok := ctx.Value(yunwuTierGroupsKey{}).(YunwuPricingScope)
	if !ok {
		return YunwuPricingScope{}, false
	}
	return scope, true
}

// yunwuTierGroupForProvider returns the provider billing group a tier's key lives
// in ("" when the provider's accounts pinned nothing — the tier default applies).
func yunwuTierGroupForProvider(ctx context.Context, provider, tier string) string {
	scope, ok := yunwuPricingScopeFromContext(ctx)
	if !ok || provider == "" {
		return ""
	}
	pinned, ok := scope.TierGroups[CanonicalPricingProvider(provider)]
	if !ok {
		return ""
	}
	return pinned.For(tier)
}

// yunwuObservedRatioForProvider returns the pricing floor the provider's own
// accounting implies (0 when nothing has been measured yet).
func yunwuObservedRatioForProvider(ctx context.Context, provider string) float64 {
	scope, ok := yunwuPricingScopeFromContext(ctx)
	if !ok || provider == "" {
		return 0
	}
	return scope.Observed[CanonicalPricingProvider(provider)]
}

// resetYunwuProviderScopeCache clears the cached provider scope. Billing uses a
// short TTL; tests call this to avoid cross-test pollution.
func resetYunwuProviderScopeCache() {
	yunwuScopeMu.Lock()
	yunwuScopeAt = time.Time{}
	yunwuScopeByFamily = nil
	yunwuPricingScope = YunwuPricingScope{}
	yunwuScopeMu.Unlock()
}

// lookupYunwuExecutionAccount resolves the execution node id reported by the
// hypervisor to the account row that served the request. It preserves the
// underlying error so a lookup failure (a cancelled context, a DB error) is
// never silently mistaken for a missing account — billing is skipped entirely in
// that case, so a hidden error means free traffic and no audit trail.
func lookupYunwuExecutionAccount(ctx context.Context, repo AccountRepository, accountID string) (*Account, error) {
	if repo == nil {
		return nil, errors.New("account repository is not wired")
	}
	id, err := strconv.ParseInt(strings.TrimSpace(accountID), 10, 64)
	if err != nil || id <= 0 {
		return nil, fmt.Errorf("invalid execution node id %q", accountID)
	}
	acc, err := repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if acc == nil {
		return nil, ErrAccountNotFound
	}
	return acc, nil
}

// recordYunwuAccount loads the yunwu account that served a request, used as the
// billing account. The hypervisor identifies the executing node by account ID.
func (s *GatewayService) recordYunwuAccount(ctx context.Context, accountID string) (*Account, error) {
	if s == nil {
		return nil, errors.New("gateway service is nil")
	}
	return lookupYunwuExecutionAccount(ctx, s.accountRepo, accountID)
}

// RecordYunwuUsage records usage and deducts the end-user balance for a request
// served by the Yunwu hypervisor (Anthropic /v1/messages protocol).
func (s *GatewayService) RecordYunwuUsage(ctx context.Context, input *RecordYunwuUsageInput) error {
	if s == nil || input == nil || input.Result == nil || input.APIKey == nil || input.User == nil {
		return nil
	}
	account, accountErr := s.recordYunwuAccount(ctx, input.AccountID)
	if account == nil {
		slog.Warn("[YunwuBilling] Skipping usage billing: execution account unavailable",
			"account_id", input.AccountID,
			"model", input.Result.Model,
			"user_id", input.User.ID,
			"error", accountErr,
		)
		return nil
	}
	// Match catalog retail pricing (max across active providers).
	billingCtx := ContextWithYunwuPrimaryPricing(ctx)
	billingCtx = withYunwuProviderScope(billingCtx, s.accountRepo)
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
	account, accountErr := lookupYunwuExecutionAccount(ctx, s.accountRepo, input.AccountID)
	if account == nil {
		slog.Warn("[YunwuBilling] Skipping OpenAI usage billing: execution account unavailable",
			"account_id", input.AccountID,
			"model", input.Result.Model,
			"user_id", input.User.ID,
			"error", accountErr,
		)
		return nil
	}
	billingCtx := ContextWithYunwuPrimaryPricing(ctx)
	billingCtx = withYunwuProviderScope(billingCtx, s.accountRepo)
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

// apiKeyGroupNames collects the names of every group an API key belongs to,
// primary group first, preserving order so the tier resolution is deterministic.
func apiKeyGroupNames(apiKey *APIKey) []string {
	if apiKey == nil {
		return nil
	}
	names := make([]string, 0, 1+len(apiKey.Groups))
	if apiKey.Group != nil {
		names = append(names, apiKey.Group.Name)
	}
	for _, g := range apiKey.Groups {
		if g != nil {
			names = append(names, g.Name)
		}
	}
	return names
}

// NormalizeAPIKeyTier validates the yunwu billing tier a caller asked for:
// "" (unset — derive from the key's groups later), "smart" or "fast". Anything
// else is rejected rather than silently dropped onto smart, so a typo cannot
// quietly serve a fast request with the cheaper key.
func NormalizeAPIKeyTier(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	tier := yunwu.NormalizeTier(trimmed)
	if tier == "" {
		return "", ErrInvalidAPIKeyTier
	}
	return tier, nil
}

// EffectiveYunwuTier resolves the billing tier of an API key: the tier chosen
// for the key when one was stored, otherwise the legacy derivation from the
// key's group names (so keys created before the tier column, and deployments
// that express the tier through `smart`/`fast` groups, keep working unchanged).
func EffectiveYunwuTier(apiKey *APIKey) string {
	if apiKey != nil {
		if tier := yunwu.NormalizeTier(apiKey.Tier); tier != "" {
			return tier
		}
	}
	return yunwu.TierFromGroupNames(apiKeyGroupNames(apiKey))
}

// WithYunwuTier resolves the billing tier (smart/fast) of an API key and
// attaches it to the context. The Yunwu engine then keeps only nodes holding a
// key for that tier and the runtime uses that tier's provider key — so a fast
// request is never served or billed with the cheaper smart key. The tier is the
// key's own attribute (see EffectiveYunwuTier); a key without one stays on
// smart, exactly as before.
func WithYunwuTier(ctx context.Context, apiKey *APIKey) context.Context {
	if ctx == nil || apiKey == nil {
		return ctx
	}
	return yunwu.ContextWithTier(ctx, EffectiveYunwuTier(apiKey))
}

// RecordYunwuMediaUsageInput carries the information needed to bill a completed
// Unofficial-provider media request (image / video / audio) that has no token
// usage to bill against.
type RecordYunwuMediaUsageInput struct {
	APIKey       *APIKey
	User         *User
	AccountID    string
	Subscription *UserSubscription
	Model        string
	MediaKind    string
	Units        int64
	// UnitPriceUSD is the per-unit upstream price resolved from the same
	// max-across-providers basis the public catalog publishes. When > 0 it is
	// authoritative and the charged amount is UnitPriceUSD × Units × multiplier,
	// guaranteeing the advertised image/video price equals the charged price.
	UnitPriceUSD         float64
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
func recordYunwuMediaUsage(ctx context.Context, deps *billingDeps, billRepo UsageBillingRepository, logRepo UsageLogRepository, bs *BillingService, in *RecordYunwuMediaUsageInput, multiplier float64) error {
	if in == nil || deps == nil || in.APIKey == nil || in.User == nil {
		return nil
	}
	account, accountErr := lookupYunwuExecutionAccount(ctx, deps.accountRepo, in.AccountID)
	if account == nil {
		slog.Warn("[YunwuBilling] Skipping media billing: execution account unavailable",
			"account_id", in.AccountID, "model", in.Model, "user_id", in.User.ID, "error", accountErr)
		return nil
	}
	if multiplier <= 0 {
		multiplier = 1.0
	}
	// Loss-making request guard (media): the serving provider may cost more than
	// the charged per-unit retail price.
	alertIfUnprofitableYunwuRequest(ctx, bs, yunwu.ParseNodeProvider(account.Extra), in.Model, multiplier)
	// Prefer the per-unit upstream price resolved from the same primary-provider
	// retail basis the catalog publishes, so the advertised price and
	// the charged price are identical. Fall back to the engine-provided cost
	// (node-configured media price / global default) only when the model has no
	// upstream fixed price.
	total, ok := yunwuMediaTotal(in.UnitPriceUSD, in.Units, in.UpstreamCostMicroUSD)
	if !ok {
		return nil
	}
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

// resolveYunwuMediaUnitPrice resolves the per-unit upstream price for a media
// (image/video/audio) model from the same primary-provider basis the public
// catalog publishes, so that the advertised and the charged price are identical.
// Returns 0 when the model has no fixed upstream price.
func resolveYunwuMediaUnitPrice(ctx context.Context, bs *BillingService, model string) float64 {
	if bs == nil || strings.TrimSpace(model) == "" {
		return 0
	}
	if p, err := bs.GetYunwuPrimaryModelPricing(ctx, model); err == nil && p != nil && p.FixedPricePerRequest > 0 {
		return p.FixedPricePerRequest
	}
	return 0
}

// yunwuMediaTotal computes the total upstream cost of a per-unit media request.
// The per-unit upstream price (same retail basis the catalog publishes) is
// authoritative; the engine-provided micro-USD cost (node-configured media
// price / global default) is only a fallback for models without an upstream
// fixed price. ok=false means there is nothing to bill.
func yunwuMediaTotal(unitPriceUSD float64, units int64, upstreamCostMicroUSD int64) (float64, bool) {
	if units <= 0 {
		units = 1
	}
	switch {
	case unitPriceUSD > 0:
		return unitPriceUSD * float64(units), true
	case upstreamCostMicroUSD > 0:
		return float64(upstreamCostMicroUSD) / 1_000_000.0, true
	default:
		return 0, false
	}
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
	// Mirror the text billing path: a per-user group rate override must shape the
	// media multiplier too, otherwise the catalog (which applies it) and the
	// charge would diverge.
	if in.User != nil && in.APIKey != nil && in.APIKey.GroupID != nil {
		multiplier = s.getUserGroupRateMultiplier(ctx, in.User.ID, *in.APIKey.GroupID, multiplier)
	}
	if s.settingService != nil {
		multiplier = s.settingService.ApplyModelMarkup(ctx, in.Model, multiplier)
	}
	ctx = withYunwuProviderScope(ctx, s.accountRepo)
	in.UnitPriceUSD = resolveYunwuMediaUnitPrice(ctx, s.billingService, in.Model)
	return recordYunwuMediaUsage(ctx, s.billingDeps(), s.usageBillingRepo, s.usageLogRepo, s.billingService, in, multiplier)
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
	// Mirror the text billing path: a per-user group rate override must shape the
	// media multiplier too, otherwise the catalog (which applies it) and the
	// charge would diverge.
	if in.User != nil && in.APIKey != nil && in.APIKey.GroupID != nil {
		resolver := s.userGroupRateResolver
		if resolver == nil {
			resolver = newUserGroupRateResolver(nil, nil, resolveUserGroupRateCacheTTL(s.cfg), nil, "service.openai_gateway")
		}
		multiplier = resolver.Resolve(ctx, in.User.ID, *in.APIKey.GroupID, multiplier)
	}
	if s.billingService != nil {
		multiplier = s.billingService.ApplyModelMarkup(ctx, in.Model, multiplier)
	}
	ctx = withYunwuProviderScope(ctx, s.accountRepo)
	in.UnitPriceUSD = resolveYunwuMediaUnitPrice(ctx, s.billingService, in.Model)
	return recordYunwuMediaUsage(ctx, s.billingDeps(), s.usageBillingRepo, s.usageLogRepo, s.billingService, in, multiplier)
}

// yunwuMargin describes whether a completed request was sold below the cost of
// the route that actually served it, and by how much (per unit).
type yunwuMargin struct {
	Unprofitable bool
	Leg          string  // "input" | "output" | "request"
	ChargedUSD   float64 // charged retail price per unit (primary price × markup)
	CostUSD      float64 // serving provider cost per unit
}

// computeYunwuMargin compares the serving provider's cost against the charged
// retail price (primary provider price × markup). A request is unprofitable when
// the serving route costs more than we charge. Token prices are compared per 1M
// tokens; fixed (per-request) prices per request. Mismatched pricing kinds are
// never flagged (they cannot be compared reliably).
func computeYunwuMargin(serving, primary *ModelPricing, multiplier float64) yunwuMargin {
	if serving == nil || primary == nil {
		return yunwuMargin{}
	}
	if multiplier <= 0 {
		multiplier = 1.0
	}
	if primary.FixedPricePerRequest > 0 || serving.FixedPricePerRequest > 0 {
		if primary.FixedPricePerRequest <= 0 || serving.FixedPricePerRequest <= 0 {
			return yunwuMargin{} // mixed pricing kinds
		}
		charged := primary.FixedPricePerRequest * multiplier
		cost := serving.FixedPricePerRequest
		if cost > charged {
			return yunwuMargin{Unprofitable: true, Leg: "request", ChargedUSD: charged, CostUSD: cost}
		}
		return yunwuMargin{}
	}
	inCharged := primary.InputPricePerToken * 1_000_000 * multiplier
	inCost := serving.InputPricePerToken * 1_000_000
	outCharged := primary.OutputPricePerToken * 1_000_000 * multiplier
	outCost := serving.OutputPricePerToken * 1_000_000
	if inCost > 0 && inCost > inCharged {
		return yunwuMargin{Unprofitable: true, Leg: "input", ChargedUSD: inCharged, CostUSD: inCost}
	}
	if outCost > 0 && outCost > outCharged {
		return yunwuMargin{Unprofitable: true, Leg: "output", ChargedUSD: outCharged, CostUSD: outCost}
	}
	return yunwuMargin{}
}

// alertIfUnprofitableYunwuRequest emits a Telegram alert when the provider that
// served a request is more expensive than the charged price (primary × markup) —
// i.e. a loss-making request. Deduplicated/rate-limited by the notifier.
func alertIfUnprofitableYunwuRequest(ctx context.Context, bs *BillingService, provider, model string, multiplier float64) {
	if bs == nil || bs.yunwuPricing == nil || strings.TrimSpace(provider) == "" || strings.TrimSpace(model) == "" {
		return
	}
	serving, ok := bs.yunwuPricing.GetModelPricingForProvider(ctx, provider, model)
	if !ok || serving == nil {
		return
	}
	primary, err := bs.GetYunwuPrimaryModelPricing(ctx, model)
	if err != nil || primary == nil {
		return
	}
	margin := computeYunwuMargin(serving, primary, multiplier)
	if !margin.Unprofitable {
		return
	}
	slog.Warn("[YunwuBilling] unprofitable request: served by a provider more expensive than the charged price",
		"provider", provider, "model", model, "leg", margin.Leg,
		"charged_usd", margin.ChargedUSD, "cost_usd", margin.CostUSD)
	if notifier := yunwu.GetDefaultNotifier(); notifier != nil {
		if _, err := notifier.NotifyUnprofitableRequest(ctx, provider, model, margin.Leg, margin.ChargedUSD, margin.CostUSD); err != nil {
			slog.Warn("[YunwuBilling] failed to send unprofitable-request alert", "error", err)
		}
	}
}

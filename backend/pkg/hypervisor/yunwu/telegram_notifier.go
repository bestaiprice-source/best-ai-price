package yunwu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/ent/account"
	"github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/ent/setting"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/config"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
	"github.com/redis/go-redis/v9"
)

type TelegramEventType string

const (
	EventProviderDownDegraded      TelegramEventType = "provider_down_degraded"
	EventUpstreamKeyInvalid        TelegramEventType = "upstream_key_invalid"
	EventProviderBalanceLow        TelegramEventType = "balance_low"
	EventBothProvidersUnavailable  TelegramEventType = "both_providers_unavailable"
	EventFailoverRateExceeded      TelegramEventType = "failover_rate_exceeded"
	EventErrorSpike                TelegramEventType = "error_spike_429_5xx"
	EventCheaperProviderFound      TelegramEventType = "cheaper_provider_found"
	EventPriceChangedSignificantly TelegramEventType = "price_changed_significantly"
)

const (
	DefaultCooldownProviderDownDegraded      = 30 * time.Minute
	DefaultCooldownUpstreamKeyInvalid        = 1 * time.Hour
	DefaultCooldownProviderBalanceLow        = 12 * time.Hour
	DefaultCooldownBothProvidersUnavailable  = 30 * time.Minute
	DefaultCooldownFailoverRateExceeded      = 30 * time.Minute
	DefaultCooldownErrorSpike                = 15 * time.Minute
	DefaultCooldownCheaperProviderFound      = 24 * time.Hour
	DefaultCooldownPriceChangedSignificantly = 24 * time.Hour

	DefaultFailoverThresholdRate = 0.20 // 20%
	DefaultFailoverMinRequests   = 5
	DefaultErrorSpikeThreshold   = 15 // 15 errors in window
	DefaultErrorSpikeWindowMins  = 5
)

var (
	defaultNotifierMu sync.RWMutex
	defaultNotifier   *TelegramNotifier
)

// GetDefaultNotifier returns the globally registered TelegramNotifier instance.
func GetDefaultNotifier() *TelegramNotifier {
	defaultNotifierMu.RLock()
	defer defaultNotifierMu.RUnlock()
	return defaultNotifier
}

// SetDefaultNotifier registers the globally accessible TelegramNotifier instance.
func SetDefaultNotifier(n *TelegramNotifier) {
	defaultNotifierMu.Lock()
	defer defaultNotifierMu.Unlock()
	defaultNotifier = n
}

type TelegramNotifier struct {
	store      *storage.Storage
	cfg        *config.HypervisorConfig
	httpClient *http.Client
	baseURL    string
	bot        *TelegramBot

	mu                sync.Mutex
	memAlerted        map[string]bool
	memCooldowns      map[string]time.Time
	memModelRequests  map[string][]int64
	memModelFailovers map[string][]int64
	memErrors429      map[string][]int64
	memErrors5xx      map[string][]int64
}

func alertKey(nodeID string) string {
	return "ash:yunwu:alerted:balance_low:" + nodeID
}

func cooldownKey(eventType TelegramEventType, entityID string) string {
	cleanEntity := strings.TrimSpace(entityID)
	if cleanEntity == "" {
		cleanEntity = "global"
	}
	return fmt.Sprintf("ash:telegram:cooldown:%s:%s", eventType, cleanEntity)
}

func NewTelegramNotifier(store *storage.Storage, cfg *config.HypervisorConfig) *TelegramNotifier {
	n := &TelegramNotifier{
		store: store,
		cfg:   cfg,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		baseURL:           "https://api.telegram.org",
		memAlerted:        make(map[string]bool),
		memCooldowns:      make(map[string]time.Time),
		memModelRequests:  make(map[string][]int64),
		memModelFailovers: make(map[string][]int64),
		memErrors429:      make(map[string][]int64),
		memErrors5xx:      make(map[string][]int64),
	}
	SetDefaultNotifier(n)
	return n
}

func (n *TelegramNotifier) SetBaseURL(url string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.baseURL = url
}

func (n *TelegramNotifier) SetHTTPClient(client *http.Client) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.httpClient = client
}

func (n *TelegramNotifier) SetBot(bot *TelegramBot) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.bot = bot
}

type notifierConfig struct {
	enabled      bool
	botToken     string
	threshold    float64
	allowedUsers []string
	recipients   []TelegramRecipient
	legacyChatID string
}

func (n *TelegramNotifier) resolveNotifierConfig(ctx context.Context) notifierConfig {
	cfg := notifierConfig{
		enabled:   true,
		threshold: 20.0,
	}

	if n.cfg != nil {
		cfg.enabled = n.cfg.TelegramAlertsEnabled
		cfg.botToken = n.cfg.TelegramBotToken
		cfg.legacyChatID = n.cfg.TelegramChatID
		if n.cfg.TelegramAlertThresholdUSD > 0 {
			cfg.threshold = n.cfg.TelegramAlertThresholdUSD
		}
		cfg.allowedUsers = append(cfg.allowedUsers, n.cfg.TelegramAllowedUsers...)
	}

	if n.store != nil && n.store.DB() != nil {
		keys := []string{
			config.SettingKeyTelegramBotToken,
			config.SettingKeyTelegramChatID,
			config.SettingKeyTelegramAlertThreshold,
			config.SettingKeyTelegramAlertsEnabled,
			config.SettingKeyTelegramAllowedUsers,
			config.SettingKeyTelegramActiveRecipients,
		}

		settings, err := n.store.DB().Setting.Query().
			Where(setting.KeyIn(keys...)).
			All(ctx)
		if err == nil {
			for _, s := range settings {
				switch s.Key {
				case config.SettingKeyTelegramBotToken:
					if strings.TrimSpace(s.Value) != "" {
						cfg.botToken = strings.TrimSpace(s.Value)
					}
				case config.SettingKeyTelegramChatID:
					if strings.TrimSpace(s.Value) != "" {
						cfg.legacyChatID = strings.TrimSpace(s.Value)
					}
				case config.SettingKeyTelegramAlertThreshold:
					if val, err := strconv.ParseFloat(strings.TrimSpace(s.Value), 64); err == nil && val >= 0 {
						cfg.threshold = val
					}
				case config.SettingKeyTelegramAlertsEnabled:
					if val, err := strconv.ParseBool(strings.TrimSpace(s.Value)); err == nil {
						cfg.enabled = val
					}
				case config.SettingKeyTelegramAllowedUsers:
					if parsed := parseAllowedUsers(s.Value); len(parsed) > 0 {
						cfg.allowedUsers = parsed
					}
				case config.SettingKeyTelegramActiveRecipients:
					var recs []TelegramRecipient
					if err := json.Unmarshal([]byte(s.Value), &recs); err == nil {
						cfg.recipients = recs
					}
				}
			}
		}
	}

	// Supplement with in-memory recipients from TelegramBot if available
	if n.bot != nil {
		botRecs := n.bot.Recipients()
		existingMap := make(map[int64]bool)
		for _, r := range cfg.recipients {
			existingMap[r.ChatID] = true
		}
		for _, br := range botRecs {
			if !existingMap[br.ChatID] {
				cfg.recipients = append(cfg.recipients, br)
			}
		}
	}

	return cfg
}

func (n *TelegramNotifier) resolveTargetChatIDs(cfg notifierConfig) []string {
	targetMap := make(map[string]struct{})

	// 1. Channel / Group / Dedicated chat ID (if configured)
	if trimmed := strings.TrimSpace(cfg.legacyChatID); trimmed != "" {
		targetMap[trimmed] = struct{}{}
	}

	// 2. All active recipients whose user ID or username is authorized
	for _, r := range cfg.recipients {
		if r.ChatID == 0 {
			continue
		}
		if IsTelegramUserAllowed(r.ChatID, r.Username, cfg.allowedUsers) {
			targetMap[strconv.FormatInt(r.ChatID, 10)] = struct{}{}
		}
	}

	// 3. Direct numeric user IDs in allowedUsers
	for _, u := range cfg.allowedUsers {
		trimmed := strings.TrimSpace(u)
		if _, err := strconv.ParseInt(trimmed, 10, 64); err == nil && trimmed != "0" {
			targetMap[trimmed] = struct{}{}
		}
	}

	result := make([]string, 0, len(targetMap))
	for id := range targetMap {
		result = append(result, id)
	}
	return result
}

// checkAndSetCooldown attempts to set a deduplication lock. Returns true if newly alerted (lock granted), false if under cooldown.
func (n *TelegramNotifier) checkAndSetCooldown(ctx context.Context, dedupKey string, cooldown time.Duration) bool {
	if n.store != nil && n.store.Redis() != nil {
		set, err := n.store.Redis().SetNX(ctx, dedupKey, "1", cooldown).Result()
		if err == nil {
			if !set {
				return false
			}
			n.mu.Lock()
			if n.memCooldowns == nil {
				n.memCooldowns = make(map[string]time.Time)
			}
			n.memCooldowns[dedupKey] = time.Now().Add(cooldown)
			n.mu.Unlock()
			return true
		}
	}

	n.mu.Lock()
	defer n.mu.Unlock()
	if n.memCooldowns == nil {
		n.memCooldowns = make(map[string]time.Time)
	}
	if exp, exists := n.memCooldowns[dedupKey]; exists && time.Now().Before(exp) {
		return false
	}
	n.memCooldowns[dedupKey] = time.Now().Add(cooldown)
	return true
}

func (n *TelegramNotifier) clearCooldown(ctx context.Context, dedupKey string) {
	if n.store != nil && n.store.Redis() != nil {
		_ = n.store.Redis().Del(ctx, dedupKey).Err()
	}
	n.mu.Lock()
	delete(n.memCooldowns, dedupKey)
	n.mu.Unlock()
}

func (n *TelegramNotifier) isCooldownActive(ctx context.Context, dedupKey string) bool {
	if n.store != nil && n.store.Redis() != nil {
		cnt, err := n.store.Redis().Exists(ctx, dedupKey).Result()
		if err == nil && cnt > 0 {
			return true
		}
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if exp, exists := n.memCooldowns[dedupKey]; exists && time.Now().Before(exp) {
		return true
	}
	return false
}

// CheckCooldownAndNotify sends a Telegram notification if not within cooldown/deduplication window.
func (n *TelegramNotifier) CheckCooldownAndNotify(
	ctx context.Context,
	eventType TelegramEventType,
	entityID string,
	cooldown time.Duration,
	formatFunc func() string,
) (bool, error) {
	nCfg := n.resolveNotifierConfig(ctx)
	if !nCfg.enabled || strings.TrimSpace(nCfg.botToken) == "" {
		return false, nil
	}

	targetChatIDs := n.resolveTargetChatIDs(nCfg)
	if len(targetChatIDs) == 0 {
		return false, nil
	}

	if cooldown <= 0 {
		switch eventType {
		case EventProviderDownDegraded:
			cooldown = DefaultCooldownProviderDownDegraded
		case EventUpstreamKeyInvalid:
			cooldown = DefaultCooldownUpstreamKeyInvalid
		case EventProviderBalanceLow:
			cooldown = DefaultCooldownProviderBalanceLow
		case EventBothProvidersUnavailable:
			cooldown = DefaultCooldownBothProvidersUnavailable
		case EventFailoverRateExceeded:
			cooldown = DefaultCooldownFailoverRateExceeded
		case EventErrorSpike:
			cooldown = DefaultCooldownErrorSpike
		case EventCheaperProviderFound:
			cooldown = DefaultCooldownCheaperProviderFound
		case EventPriceChangedSignificantly:
			cooldown = DefaultCooldownPriceChangedSignificantly
		default:
			cooldown = 15 * time.Minute
		}
	}

	dedupKey := cooldownKey(eventType, entityID)
	if !n.checkAndSetCooldown(ctx, dedupKey, cooldown) {
		// Suppressed due to cooldown / deduplication
		return false, nil
	}

	msg := formatFunc()
	sentCount := 0
	var lastErr error
	for _, chatID := range targetChatIDs {
		if err := n.sendTelegramMessage(ctx, nCfg.botToken, chatID, msg); err != nil {
			slog.Error("[TelegramNotifier] Failed to send Telegram alert",
				"event_type", string(eventType),
				"entity_id", entityID,
				"chat_id", chatID,
				"error", err,
			)
			lastErr = err
		} else {
			sentCount++
		}
	}

	if sentCount == 0 && lastErr != nil {
		n.clearCooldown(ctx, dedupKey)
		return false, lastErr
	}

	slog.Info("[TelegramNotifier] Dispatched Telegram alert",
		"event_type", string(eventType),
		"entity_id", entityID,
		"recipients_count", sentCount,
	)
	return true, nil
}

// 1. NotifyProviderDownOrDegraded dispatches alert when a provider or execution node becomes down or degraded.
func (n *TelegramNotifier) NotifyProviderDownOrDegraded(
	ctx context.Context,
	provider, nodeID, accountName, status, reason string,
) (bool, error) {
	entityID := fmt.Sprintf("%s:%s", provider, nodeID)
	return n.CheckCooldownAndNotify(ctx, EventProviderDownDegraded, entityID, DefaultCooldownProviderDownDegraded, func() string {
		return formatProviderDownDegradedMessage(provider, nodeID, accountName, status, reason)
	})
}

// 2. NotifyUpstreamKeyInvalid dispatches alert when upstream API key is invalid, disabled, or unauthorized.
func (n *TelegramNotifier) NotifyUpstreamKeyInvalid(
	ctx context.Context,
	provider, nodeID, accountName, keyMasked, reason string,
) (bool, error) {
	entityID := fmt.Sprintf("%s:%s", provider, nodeID)
	return n.CheckCooldownAndNotify(ctx, EventUpstreamKeyInvalid, entityID, DefaultCooldownUpstreamKeyInvalid, func() string {
		return formatUpstreamKeyInvalidMessage(provider, nodeID, accountName, keyMasked, reason)
	})
}

// 3. NotifyProviderBalanceLow dispatches alert when provider balance drops below threshold.
func (n *TelegramNotifier) NotifyProviderBalanceLow(
	ctx context.Context,
	provider, nodeID, accountName string,
	availableUSD, thresholdUSD float64,
) (bool, error) {
	dedupKey := alertKey(nodeID)
	if !n.checkAndSetCooldown(ctx, dedupKey, DefaultCooldownProviderBalanceLow) {
		return false, nil
	}

	nCfg := n.resolveNotifierConfig(ctx)
	if !nCfg.enabled || strings.TrimSpace(nCfg.botToken) == "" {
		return false, nil
	}
	targetChatIDs := n.resolveTargetChatIDs(nCfg)
	if len(targetChatIDs) == 0 {
		return false, nil
	}

	msg := formatAlertMessage(provider, accountName, nodeID, availableUSD, thresholdUSD)
	sentCount := 0
	var lastErr error
	for _, chatID := range targetChatIDs {
		if err := n.sendTelegramMessage(ctx, nCfg.botToken, chatID, msg); err != nil {
			lastErr = err
		} else {
			sentCount++
		}
	}

	if sentCount == 0 && lastErr != nil {
		n.clearCooldown(ctx, dedupKey)
		return false, lastErr
	}
	slog.Info("[TelegramNotifier] Successfully sent Telegram low balance alert",
		"node_id", nodeID,
		"provider", provider,
		"available_usd", availableUSD,
		"threshold", thresholdUSD,
		"recipients_count", sentCount,
	)
	return true, nil
}

// 4. NotifyBothProvidersUnavailable dispatches critical alert when all providers for a model are unavailable.
func (n *TelegramNotifier) NotifyBothProvidersUnavailable(
	ctx context.Context,
	modelID, primaryProvider, backupProvider, reason string,
) (bool, error) {
	return n.CheckCooldownAndNotify(ctx, EventBothProvidersUnavailable, modelID, DefaultCooldownBothProvidersUnavailable, func() string {
		return formatBothProvidersUnavailableMessage(modelID, primaryProvider, backupProvider, reason)
	})
}

// 5. NotifyFailoverRateExceeded dispatches alert when failover rate for a model exceeds threshold.
func (n *TelegramNotifier) NotifyFailoverRateExceeded(
	ctx context.Context,
	modelID string,
	failoverRate, thresholdRate float64,
	totalRequests, failoverCount int64,
) (bool, error) {
	return n.CheckCooldownAndNotify(ctx, EventFailoverRateExceeded, modelID, DefaultCooldownFailoverRateExceeded, func() string {
		return formatFailoverRateExceededMessage(modelID, failoverRate, thresholdRate, totalRequests, failoverCount)
	})
}

// 6. NotifyErrorSpike dispatches alert when 429 / 5xx error rate or count spikes.
func (n *TelegramNotifier) NotifyErrorSpike(
	ctx context.Context,
	scope string,
	count429, count5xx int64,
	windowMinutes int,
	reason string,
) (bool, error) {
	return n.CheckCooldownAndNotify(ctx, EventErrorSpike, scope, DefaultCooldownErrorSpike, func() string {
		return formatErrorSpikeMessage(scope, count429, count5xx, windowMinutes, reason)
	})
}

// 7. NotifyCheaperProviderFound dispatches alert when a cheaper candidate provider is found.
func (n *TelegramNotifier) NotifyCheaperProviderFound(
	ctx context.Context,
	family, modelName, currentProvider string,
	currentPrice float64,
	candidateProvider, candidateDomain string,
	candidatePrice, savingsPercent float64,
	providerCardURL string,
) (bool, error) {
	entityID := fmt.Sprintf("%s:%s", modelName, candidateDomain)
	return n.CheckCooldownAndNotify(ctx, EventCheaperProviderFound, entityID, DefaultCooldownCheaperProviderFound, func() string {
		return formatCheaperProviderFoundMessage(family, modelName, currentProvider, currentPrice, candidateProvider, candidateDomain, candidatePrice, savingsPercent, providerCardURL)
	})
}

// 8. NotifyPriceChangedSignificantly dispatches alert when upstream price change requires retail recalculation.
func (n *TelegramNotifier) NotifyPriceChangedSignificantly(
	ctx context.Context,
	provider, modelName string,
	oldInput, newInput, oldOutput, newOutput, changePercent, thresholdPercent float64,
) (bool, error) {
	entityID := fmt.Sprintf("%s:%s", provider, modelName)
	return n.CheckCooldownAndNotify(ctx, EventPriceChangedSignificantly, entityID, DefaultCooldownPriceChangedSignificantly, func() string {
		return formatPriceChangedSignificantlyMessage(provider, modelName, oldInput, newInput, oldOutput, newOutput, changePercent, thresholdPercent)
	})
}

// Backward-compatible CheckAndNotify used by StatsWorker and manual balance sync.
func (n *TelegramNotifier) CheckAndNotify(ctx context.Context, nodeID, accountName, provider string, availableUSD float64) (bool, error) {
	if strings.TrimSpace(nodeID) == "" {
		return false, nil
	}

	nCfg := n.resolveNotifierConfig(ctx)
	if !nCfg.enabled {
		return false, nil
	}

	// Filter: Alert ONLY for accounts belonging to the yunwu group
	if n.store != nil && n.store.DB() != nil {
		id, parseErr := strconv.ParseInt(nodeID, 10, 64)
		if parseErr == nil && id > 0 {
			inYunwuGroup, err := n.store.DB().Account.Query().
				Where(
					account.IDEQ(id),
					account.PlatformEqualFold("yunwu"),
					account.HasGroupsWith(
						group.Or(
							group.PlatformEqualFold("yunwu"),
							group.NameEqualFold("yunwu"),
							group.NameContainsFold("yunwu"),
						),
					),
				).Exist(ctx)
			if err == nil && !inYunwuGroup {
				slog.Debug("[TelegramNotifier] Skipping notification: node does not belong to yunwu group",
					"node_id", nodeID,
					"account_name", accountName,
				)
				return false, nil
			}
		}
	}

	threshold := nCfg.threshold
	if n.store != nil && n.store.DB() != nil {
		id, parseErr := strconv.ParseInt(nodeID, 10, 64)
		if parseErr == nil && id > 0 {
			if acc, err := n.store.DB().Account.Get(ctx, id); err == nil && acc != nil && acc.Extra != nil {
				if mb, ok := acc.Extra["min_balance_usd"].(float64); ok && mb > 0 {
					threshold = mb
				}
			}
		}
	}

	// Replenished above threshold: reset alert state
	if availableUSD > threshold {
		if n.IsAlerted(ctx, nodeID) {
			n.clearAlerted(ctx, nodeID)
			slog.Info("[TelegramNotifier] Yunwu node balance replenished above threshold, alert state reset",
				"node_id", nodeID,
				"provider", provider,
				"available_usd", availableUSD,
				"threshold", threshold,
			)
		}
		return false, nil
	}

	return n.NotifyProviderBalanceLow(ctx, provider, nodeID, accountName, availableUSD, threshold)
}

func (n *TelegramNotifier) markAlerted(ctx context.Context, nodeID string) (bool, error) {
	return n.checkAndSetCooldown(ctx, alertKey(nodeID), DefaultCooldownProviderBalanceLow), nil
}

func (n *TelegramNotifier) clearAlerted(ctx context.Context, nodeID string) {
	n.clearCooldown(ctx, alertKey(nodeID))
	n.mu.Lock()
	delete(n.memAlerted, nodeID)
	n.mu.Unlock()
}

func (n *TelegramNotifier) IsAlerted(ctx context.Context, nodeID string) bool {
	return n.isCooldownActive(ctx, alertKey(nodeID))
}

// Metrics recording: Model Requests & Failovers
func (n *TelegramNotifier) RecordModelRequest(ctx context.Context, model string) {
	if strings.TrimSpace(model) == "" {
		return
	}
	now := time.Now().Unix()
	if n.store != nil && n.store.Redis() != nil {
		key := "ash:metrics:model_req:" + model
		_ = n.store.Redis().ZAdd(ctx, key, redis.Z{Score: float64(now), Member: fmt.Sprintf("%d:%d", now, time.Now().UnixNano())}).Err()
		_ = n.store.Redis().ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("%d", now-1800)).Err()
		_ = n.store.Redis().Expire(ctx, key, 1*time.Hour).Err()
	}
	n.mu.Lock()
	if n.memModelRequests == nil {
		n.memModelRequests = make(map[string][]int64)
	}
	n.memModelRequests[model] = append(n.memModelRequests[model], now)
	cutoff := now - 1800
	reqs := n.memModelRequests[model]
	start := 0
	for start < len(reqs) && reqs[start] < cutoff {
		start++
	}
	n.memModelRequests[model] = reqs[start:]
	n.mu.Unlock()
}

func (n *TelegramNotifier) RecordModelFailover(ctx context.Context, model string) {
	if strings.TrimSpace(model) == "" {
		return
	}
	now := time.Now().Unix()
	if n.store != nil && n.store.Redis() != nil {
		key := "ash:metrics:model_failover:" + model
		_ = n.store.Redis().ZAdd(ctx, key, redis.Z{Score: float64(now), Member: fmt.Sprintf("%d:%d", now, time.Now().UnixNano())}).Err()
		_ = n.store.Redis().ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("%d", now-1800)).Err()
		_ = n.store.Redis().Expire(ctx, key, 1*time.Hour).Err()
	}
	n.mu.Lock()
	if n.memModelFailovers == nil {
		n.memModelFailovers = make(map[string][]int64)
	}
	n.memModelFailovers[model] = append(n.memModelFailovers[model], now)
	cutoff := now - 1800
	fos := n.memModelFailovers[model]
	start := 0
	for start < len(fos) && fos[start] < cutoff {
		start++
	}
	n.memModelFailovers[model] = fos[start:]
	n.mu.Unlock()
}

func (n *TelegramNotifier) CheckAndNotifyFailoverRate(ctx context.Context, model string) (bool, error) {
	var totalReqs, totalFailovers int64
	now := time.Now().Unix()
	cutoff := now - 1800

	if n.store != nil && n.store.Redis() != nil {
		reqKey := "ash:metrics:model_req:" + model
		foKey := "ash:metrics:model_failover:" + model
		_ = n.store.Redis().ZRemRangeByScore(ctx, reqKey, "-inf", fmt.Sprintf("%d", cutoff)).Err()
		_ = n.store.Redis().ZRemRangeByScore(ctx, foKey, "-inf", fmt.Sprintf("%d", cutoff)).Err()
		totalReqs, _ = n.store.Redis().ZCard(ctx, reqKey).Result()
		totalFailovers, _ = n.store.Redis().ZCard(ctx, foKey).Result()
	} else {
		n.mu.Lock()
		for _, t := range n.memModelRequests[model] {
			if t >= cutoff {
				totalReqs++
			}
		}
		for _, t := range n.memModelFailovers[model] {
			if t >= cutoff {
				totalFailovers++
			}
		}
		n.mu.Unlock()
	}

	if totalReqs < DefaultFailoverMinRequests {
		return false, nil
	}

	rate := float64(totalFailovers) / float64(totalReqs)
	if rate >= DefaultFailoverThresholdRate {
		return n.NotifyFailoverRateExceeded(ctx, model, rate*100, DefaultFailoverThresholdRate*100, totalReqs, totalFailovers)
	}
	return false, nil
}

// Metrics recording: 429 and 5xx errors
func (n *TelegramNotifier) RecordError(ctx context.Context, statusCode int, scope string) {
	if statusCode != http.StatusTooManyRequests && statusCode < 500 {
		return
	}
	if strings.TrimSpace(scope) == "" {
		scope = "global"
	}
	now := time.Now().Unix()
	is429 := statusCode == http.StatusTooManyRequests

	if n.store != nil && n.store.Redis() != nil {
		errKey := fmt.Sprintf("ash:metrics:errors:5xx:%s", scope)
		if is429 {
			errKey = fmt.Sprintf("ash:metrics:errors:429:%s", scope)
		}
		_ = n.store.Redis().ZAdd(ctx, errKey, redis.Z{Score: float64(now), Member: fmt.Sprintf("%d:%d", now, time.Now().UnixNano())}).Err()
		_ = n.store.Redis().ZRemRangeByScore(ctx, errKey, "-inf", fmt.Sprintf("%d", now-300)).Err()
		_ = n.store.Redis().Expire(ctx, errKey, 30*time.Minute).Err()
	}

	n.mu.Lock()
	if is429 {
		if n.memErrors429 == nil {
			n.memErrors429 = make(map[string][]int64)
		}
		n.memErrors429[scope] = append(n.memErrors429[scope], now)
	} else {
		if n.memErrors5xx == nil {
			n.memErrors5xx = make(map[string][]int64)
		}
		n.memErrors5xx[scope] = append(n.memErrors5xx[scope], now)
	}
	n.mu.Unlock()
}

func (n *TelegramNotifier) CheckAndNotifyErrorSpike(ctx context.Context, scope string) (bool, error) {
	if strings.TrimSpace(scope) == "" {
		scope = "global"
	}
	now := time.Now().Unix()
	cutoff := now - 300 // 5 minutes

	var c429, c5xx int64
	if n.store != nil && n.store.Redis() != nil {
		k429 := fmt.Sprintf("ash:metrics:errors:429:%s", scope)
		k5xx := fmt.Sprintf("ash:metrics:errors:5xx:%s", scope)
		_ = n.store.Redis().ZRemRangeByScore(ctx, k429, "-inf", fmt.Sprintf("%d", cutoff)).Err()
		_ = n.store.Redis().ZRemRangeByScore(ctx, k5xx, "-inf", fmt.Sprintf("%d", cutoff)).Err()
		c429, _ = n.store.Redis().ZCard(ctx, k429).Result()
		c5xx, _ = n.store.Redis().ZCard(ctx, k5xx).Result()
	} else {
		n.mu.Lock()
		for _, t := range n.memErrors429[scope] {
			if t >= cutoff {
				c429++
			}
		}
		for _, t := range n.memErrors5xx[scope] {
			if t >= cutoff {
				c5xx++
			}
		}
		n.mu.Unlock()
	}

	total := c429 + c5xx
	if total >= DefaultErrorSpikeThreshold {
		reason := fmt.Sprintf("Обнаружен резкий всплеск: %d ошибок за последние %d минут", total, DefaultErrorSpikeWindowMins)
		return n.NotifyErrorSpike(ctx, scope, c429, c5xx, DefaultErrorSpikeWindowMins, reason)
	}
	return false, nil
}

func (n *TelegramNotifier) SendDirectAlert(ctx context.Context, text string) (int, error) {
	nCfg := n.resolveNotifierConfig(ctx)
	if strings.TrimSpace(nCfg.botToken) == "" {
		return 0, fmt.Errorf("telegram bot token is not configured")
	}

	targetChatIDs := n.resolveTargetChatIDs(nCfg)
	if len(targetChatIDs) == 0 {
		return 0, fmt.Errorf("no authorized Telegram recipients found (whitelist users or send /start to bot)")
	}

	sentCount := 0
	var lastErr error
	for _, chatID := range targetChatIDs {
		if err := n.sendTelegramMessage(ctx, nCfg.botToken, chatID, text); err != nil {
			lastErr = err
		} else {
			sentCount++
		}
	}

	if sentCount == 0 && lastErr != nil {
		return 0, lastErr
	}
	return sentCount, nil
}

type telegramSendMessagePayload struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
}

type telegramResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description,omitempty"`
}

func (n *TelegramNotifier) sendTelegramMessage(ctx context.Context, botToken, chatID, text string) error {
	n.mu.Lock()
	client := n.httpClient
	baseURL := n.baseURL
	n.mu.Unlock()

	if client == nil {
		client = http.DefaultClient
	}

	endpoint := fmt.Sprintf("%s/bot%s/sendMessage", strings.TrimRight(baseURL, "/"), botToken)
	payload := telegramSendMessagePayload{
		ChatID:    chatID,
		Text:      text,
		ParseMode: "HTML",
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to encode telegram payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create telegram request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	var tgResp telegramResponse
	if err := json.Unmarshal(respBody, &tgResp); err == nil {
		if !tgResp.OK {
			return fmt.Errorf("telegram API error (status %d): %s", resp.StatusCode, tgResp.Description)
		}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// Message Formatters

func formatAlertMessage(provider, accountName, nodeID string, balance, threshold float64) string {
	if strings.TrimSpace(provider) == "" {
		provider = "Yunwu"
	}
	if strings.TrimSpace(accountName) == "" {
		accountName = nodeID
	}
	return fmt.Sprintf(
		"⚠️ <b>Низкий баланс провайдера: %s</b>\n\n"+
			"• <b>Провайдер:</b> %s\n"+
			"• <b>Аккаунт:</b> %s\n"+
			"• <b>Node ID:</b> <code>%s</code>\n"+
			"• <b>Текущий баланс:</b> $%.2f\n"+
			"• <b>Порог алерта:</b> $%.2f\n"+
			"• <b>Статус:</b> Требуется пополнение\n"+
			"• <b>Время:</b> %s",
		html.EscapeString(provider),
		html.EscapeString(provider),
		html.EscapeString(accountName),
		html.EscapeString(nodeID),
		balance,
		threshold,
		time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
	)
}

func formatProviderDownDegradedMessage(provider, nodeID, accountName, status, reason string) string {
	return fmt.Sprintf(
		"🔴 <b>[ALARM] Провайдер недоступен / деградирован</b>\n\n"+
			"• <b>Провайдер:</b> %s\n"+
			"• <b>Аккаунт:</b> %s\n"+
			"• <b>Node ID:</b> <code>%s</code>\n"+
			"• <b>Статус:</b> %s\n"+
			"• <b>Причина:</b> %s\n"+
			"• <b>Время:</b> %s",
		html.EscapeString(provider),
		html.EscapeString(accountName),
		html.EscapeString(nodeID),
		html.EscapeString(status),
		html.EscapeString(reason),
		time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
	)
}

func formatUpstreamKeyInvalidMessage(provider, nodeID, accountName, keyMasked, reason string) string {
	return fmt.Sprintf(
		"🚫 <b>[ALARM] Upstream API-ключ недействителен / отключен</b>\n\n"+
			"• <b>Провайдер:</b> %s\n"+
			"• <b>Аккаунт:</b> %s\n"+
			"• <b>Node ID:</b> <code>%s</code>\n"+
			"• <b>Ключ:</b> <code>%s</code>\n"+
			"• <b>Причина:</b> %s\n"+
			"• <b>Действие:</b> Требуется замена или повторная активация ключа\n"+
			"• <b>Время:</b> %s",
		html.EscapeString(provider),
		html.EscapeString(accountName),
		html.EscapeString(nodeID),
		html.EscapeString(keyMasked),
		html.EscapeString(reason),
		time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
	)
}

func formatBothProvidersUnavailableMessage(modelID, primaryProvider, backupProvider, reason string) string {
	return fmt.Sprintf(
		"🚨 <b>[КРИТИЧЕСКИЙ СБОЙ] Оба поставщика модели недоступны</b>\n\n"+
			"• <b>Модель:</b> <code>%s</code>\n"+
			"• <b>Основной поставщик:</b> %s (недоступен)\n"+
			"• <b>Резервный поставщик:</b> %s (недоступен)\n"+
			"• <b>Причина:</b> %s\n"+
			"• <b>Статус:</b> Запросы к модели отклоняются\n"+
			"• <b>Время:</b> %s",
		html.EscapeString(modelID),
		html.EscapeString(primaryProvider),
		html.EscapeString(backupProvider),
		html.EscapeString(reason),
		time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
	)
}

func formatFailoverRateExceededMessage(modelID string, failoverRate, thresholdRate float64, totalRequests, failoverCount int64) string {
	return fmt.Sprintf(
		"⚠️ <b>[ПРЕДУПРЕЖДЕНИЕ] Высокий Failover Rate по модели</b>\n\n"+
			"• <b>Модель:</b> <code>%s</code>\n"+
			"• <b>Failover Rate:</b> %.1f%%\n"+
			"• <b>Порог алерта:</b> %.1f%%\n"+
			"• <b>Всего запросов (окно):</b> %d\n"+
			"• <b>Переключений на резерв:</b> %d\n"+
			"• <b>Действие:</b> Проверьте стабильность основного поставщика\n"+
			"• <b>Время:</b> %s",
		html.EscapeString(modelID),
		failoverRate,
		thresholdRate,
		totalRequests,
		failoverCount,
		time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
	)
}

func formatErrorSpikeMessage(scope string, count429, count5xx int64, windowMinutes int, reason string) string {
	return fmt.Sprintf(
		"📈 <b>[ВСПЛЕСК ОШИБОК] Резкий рост 429 / 5xx</b>\n\n"+
			"• <b>Область / Контекст:</b> %s\n"+
			"• <b>Ошибок 429 (Rate Limit):</b> %d\n"+
			"• <b>Ошибок 5xx (Server Error):</b> %d\n"+
			"• <b>Окно наблюдения:</b> %d мин.\n"+
			"• <b>Причина / Детали:</b> %s\n"+
			"• <b>Время:</b> %s",
		html.EscapeString(scope),
		count429,
		count5xx,
		windowMinutes,
		html.EscapeString(reason),
		time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
	)
}

func formatCheaperProviderFoundMessage(family, modelName, currentProvider string, currentPrice float64, candidateProvider, candidateDomain string, candidatePrice, savingsPercent float64, providerCardURL string) string {
	cardLink := ""
	if strings.TrimSpace(providerCardURL) != "" {
		cardLink = fmt.Sprintf("\n• <b>Карточка на BestAIPrice:</b> <a href=\"%s\">Открыть</a>", html.EscapeString(providerCardURL))
	}
	return fmt.Sprintf(
		"💡 <b>[ОПТИМИЗАЦИЯ ЗАТРАТ] Найден более выгодный поставщик</b>\n\n"+
			"• <b>Семейство:</b> %s\n"+
			"• <b>Модель:</b> <code>%s</code>\n"+
			"• <b>Текущий провайдер:</b> %s ($%.4f/1M)\n"+
			"• <b>Кандидат:</b> %s (%s)\n"+
			"• <b>Цена кандидата:</b> $%.4f/1M\n"+
			"• <b>Экономия:</b> -%.1f%%%s\n"+
			"• <b>Время:</b> %s",
		html.EscapeString(family),
		html.EscapeString(modelName),
		html.EscapeString(currentProvider),
		currentPrice,
		html.EscapeString(candidateProvider),
		html.EscapeString(candidateDomain),
		candidatePrice,
		math.Abs(savingsPercent),
		cardLink,
		time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
	)
}

func formatPriceChangedSignificantlyMessage(provider, modelName string, oldInput, newInput, oldOutput, newOutput, changePercent, thresholdPercent float64) string {
	sign := "+"
	if changePercent < 0 {
		sign = ""
	}
	return fmt.Sprintf(
		"📊 <b>[ИЗМЕНЕНИЕ ЦЕН] Значительное изменение цены поставщика</b>\n\n"+
			"• <b>Провайдер:</b> %s\n"+
			"• <b>Модель:</b> <code>%s</code>\n"+
			"• <b>Input Price:</b> $%.4f → $%.4f\n"+
			"• <b>Output Price:</b> $%.4f → $%.4f\n"+
			"• <b>Изменение:</b> %s%.1f%% (порог: %.1f%%)\n"+
			"• <b>Внимание:</b> Требуется пересчёт розничных цен (retail pricing)\n"+
			"• <b>Время:</b> %s",
		html.EscapeString(provider),
		html.EscapeString(modelName),
		oldInput, newInput,
		oldOutput, newOutput,
		sign, changePercent,
		thresholdPercent,
		time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
	)
}

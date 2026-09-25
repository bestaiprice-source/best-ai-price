package yunwu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/ent/setting"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/config"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
)

type TelegramNotifier struct {
	store      *storage.Storage
	cfg        *config.HypervisorConfig
	httpClient *http.Client
	baseURL    string
	bot        *TelegramBot

	mu         sync.Mutex
	memAlerted map[string]bool
}

func alertKey(nodeID string) string {
	return "ash:yunwu:alerted:balance_low:" + nodeID
}

func NewTelegramNotifier(store *storage.Storage, cfg *config.HypervisorConfig) *TelegramNotifier {
	return &TelegramNotifier{
		store: store,
		cfg:   cfg,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		baseURL:    "https://api.telegram.org",
		memAlerted: make(map[string]bool),
	}
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

	// 1. All active recipients whose user ID or username is authorized
	for _, r := range cfg.recipients {
		if r.ChatID == 0 {
			continue
		}
		if IsTelegramUserAllowed(r.ChatID, r.Username, cfg.allowedUsers) {
			targetMap[strconv.FormatInt(r.ChatID, 10)] = struct{}{}
		}
	}

	// 2. Direct numeric user IDs in allowedUsers
	for _, u := range cfg.allowedUsers {
		trimmed := strings.TrimSpace(u)
		if _, err := strconv.ParseInt(trimmed, 10, 64); err == nil && trimmed != "0" {
			targetMap[trimmed] = struct{}{}
		}
	}

	// 3. Fallback legacy chat ID if specified and no other recipients
	if len(targetMap) == 0 && cfg.legacyChatID != "" {
		targetMap[cfg.legacyChatID] = struct{}{}
	}

	result := make([]string, 0, len(targetMap))
	for id := range targetMap {
		result = append(result, id)
	}
	return result
}

func (n *TelegramNotifier) markAlerted(ctx context.Context, nodeID string) (bool, error) {
	if n.store != nil && n.store.Redis() != nil {
		set, err := n.store.Redis().SetNX(ctx, alertKey(nodeID), "1", 0).Result()
		if err == nil {
			if !set {
				return false, nil
			}
			n.mu.Lock()
			n.memAlerted[nodeID] = true
			n.mu.Unlock()
			return true, nil
		}
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.memAlerted[nodeID] {
		return false, nil
	}
	n.memAlerted[nodeID] = true
	return true, nil
}

func (n *TelegramNotifier) clearAlerted(ctx context.Context, nodeID string) {
	if n.store != nil && n.store.Redis() != nil {
		_ = n.store.Redis().Del(ctx, alertKey(nodeID)).Err()
	}
	n.mu.Lock()
	delete(n.memAlerted, nodeID)
	n.mu.Unlock()
}

func (n *TelegramNotifier) IsAlerted(ctx context.Context, nodeID string) bool {
	if n.store != nil && n.store.Redis() != nil {
		cnt, err := n.store.Redis().Exists(ctx, alertKey(nodeID)).Result()
		if err == nil && cnt > 0 {
			return true
		}
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.memAlerted[nodeID]
}

func (n *TelegramNotifier) CheckAndNotify(ctx context.Context, nodeID, accountName, provider string, availableUSD float64) (bool, error) {
	if strings.TrimSpace(nodeID) == "" {
		return false, nil
	}

	nCfg := n.resolveNotifierConfig(ctx)
	if !nCfg.enabled {
		return false, nil
	}

	// Balance replenished above threshold: reset alert state
	if availableUSD > nCfg.threshold {
		if n.IsAlerted(ctx, nodeID) {
			n.clearAlerted(ctx, nodeID)
			slog.Info("[TelegramNotifier] Yunwu node balance replenished above threshold, alert state reset",
				"node_id", nodeID,
				"provider", provider,
				"available_usd", availableUSD,
				"threshold", nCfg.threshold,
			)
		}
		return false, nil
	}

	// Balance is <= threshold ($20 or lower).
	if strings.TrimSpace(nCfg.botToken) == "" {
		slog.Debug("[TelegramNotifier] Low balance detected but Telegram bot token not configured",
			"node_id", nodeID,
			"provider", provider,
			"available_usd", availableUSD,
			"threshold", nCfg.threshold,
		)
		return false, nil
	}

	targetChatIDs := n.resolveTargetChatIDs(nCfg)
	if len(targetChatIDs) == 0 {
		slog.Info("[TelegramNotifier] Low balance detected but no authorized Telegram users/recipients configured",
			"node_id", nodeID,
			"provider", provider,
			"available_usd", availableUSD,
			"threshold", nCfg.threshold,
		)
		return false, nil
	}

	newlyAlerted, err := n.markAlerted(ctx, nodeID)
	if err != nil {
		slog.Warn("[TelegramNotifier] Failed to mark alert state in store", "node_id", nodeID, "error", err)
	}
	if !newlyAlerted {
		// Already alerted once for this low-balance period; suppress duplicate
		return false, nil
	}

	msg := formatAlertMessage(provider, accountName, nodeID, availableUSD, nCfg.threshold)

	sentCount := 0
	var lastErr error
	for _, chatID := range targetChatIDs {
		if err := n.sendTelegramMessage(ctx, nCfg.botToken, chatID, msg); err != nil {
			slog.Error("[TelegramNotifier] Failed to send Telegram alert to recipient",
				"chat_id", chatID,
				"node_id", nodeID,
				"error", err,
			)
			lastErr = err
		} else {
			sentCount++
		}
	}

	if sentCount == 0 && lastErr != nil {
		// If all deliveries failed, reset state so retry occurs on next sync
		n.clearAlerted(ctx, nodeID)
		return false, lastErr
	}

	slog.Info("[TelegramNotifier] Successfully sent Telegram low balance alert",
		"node_id", nodeID,
		"provider", provider,
		"available_usd", availableUSD,
		"threshold", nCfg.threshold,
		"recipients_count", sentCount,
	)
	return true, nil
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

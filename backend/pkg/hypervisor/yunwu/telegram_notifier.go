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
			Timeout: 10 * time.Second,
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

func (n *TelegramNotifier) resolveConfig(ctx context.Context) (enabled bool, botToken, chatID string, threshold float64) {
	enabled = true
	threshold = 20.0
	if n.cfg != nil {
		enabled = n.cfg.TelegramAlertsEnabled
		botToken = n.cfg.TelegramBotToken
		chatID = n.cfg.TelegramChatID
		if n.cfg.TelegramAlertThresholdUSD > 0 {
			threshold = n.cfg.TelegramAlertThresholdUSD
		}
	}

	if n.store == nil || n.store.DB() == nil {
		return enabled, botToken, chatID, threshold
	}

	keys := []string{
		config.SettingKeyTelegramBotToken,
		config.SettingKeyTelegramChatID,
		config.SettingKeyTelegramAlertThreshold,
		config.SettingKeyTelegramAlertsEnabled,
	}

	settings, err := n.store.DB().Setting.Query().
		Where(setting.KeyIn(keys...)).
		All(ctx)
	if err != nil {
		return enabled, botToken, chatID, threshold
	}

	for _, s := range settings {
		switch s.Key {
		case config.SettingKeyTelegramBotToken:
			if strings.TrimSpace(s.Value) != "" {
				botToken = strings.TrimSpace(s.Value)
			}
		case config.SettingKeyTelegramChatID:
			if strings.TrimSpace(s.Value) != "" {
				chatID = strings.TrimSpace(s.Value)
			}
		case config.SettingKeyTelegramAlertThreshold:
			if val, err := strconv.ParseFloat(strings.TrimSpace(s.Value), 64); err == nil && val >= 0 {
				threshold = val
			}
		case config.SettingKeyTelegramAlertsEnabled:
			if val, err := strconv.ParseBool(strings.TrimSpace(s.Value)); err == nil {
				enabled = val
			}
		}
	}

	return enabled, botToken, chatID, threshold
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

	enabled, botToken, chatID, threshold := n.resolveConfig(ctx)
	if !enabled {
		return false, nil
	}

	// When balance is above threshold, clear any prior alert state so future drops will alert again.
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

	// Balance is <= threshold ($20 or lower).
	if botToken == "" || chatID == "" {
		slog.Debug("[TelegramNotifier] Low balance detected but Telegram credentials not configured",
			"node_id", nodeID,
			"provider", provider,
			"available_usd", availableUSD,
			"threshold", threshold,
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

	msg := formatAlertMessage(provider, accountName, nodeID, availableUSD, threshold)
	if err := n.sendTelegramMessage(ctx, botToken, chatID, msg); err != nil {
		// Clear alerted state so we can retry on next balance sync
		n.clearAlerted(ctx, nodeID)
		slog.Error("[TelegramNotifier] Failed to send Telegram low balance alert",
			"node_id", nodeID,
			"provider", provider,
			"error", err,
		)
		return false, err
	}

	slog.Info("[TelegramNotifier] Successfully sent Telegram low balance alert",
		"node_id", nodeID,
		"provider", provider,
		"available_usd", availableUSD,
		"threshold", threshold,
	)
	return true, nil
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
		"⚠️ <b>Low Balance Alert: %s</b>\n\n"+
			"• <b>Provider:</b> %s\n"+
			"• <b>Account:</b> %s\n"+
			"• <b>Node ID:</b> <code>%s</code>\n"+
			"• <b>Current Balance:</b> $%.2f\n"+
			"• <b>Threshold:</b> $%.2f\n"+
			"• <b>Status:</b> Immediate attention required\n"+
			"• <b>Time:</b> %s",
		html.EscapeString(provider),
		html.EscapeString(provider),
		html.EscapeString(accountName),
		html.EscapeString(nodeID),
		balance,
		threshold,
		time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
	)
}

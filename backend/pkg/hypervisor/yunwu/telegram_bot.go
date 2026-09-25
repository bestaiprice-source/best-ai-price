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
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
)

type TelegramRecipient struct {
	ChatID    int64     `json:"chat_id"`
	Username  string    `json:"username"`
	FirstName string    `json:"first_name"`
	JoinedAt  time.Time `json:"joined_at"`
}

type TelegramBot struct {
	store      *storage.Storage
	cfg        *config.HypervisorConfig
	sw         *StatsWorker
	nm         *health.NodeManager
	httpClient *http.Client
	baseURL    string

	mu           sync.RWMutex
	botToken     string
	enabled      bool
	threshold    float64
	allowedUsers []string
	recipients   map[int64]TelegramRecipient

	stopCh  chan struct{}
	running bool
}

func NewTelegramBot(store *storage.Storage, cfg *config.HypervisorConfig, sw *StatsWorker) *TelegramBot {
	bot := &TelegramBot{
		store: store,
		cfg:   cfg,
		sw:    sw,
		nm:    health.NewNodeManager(store),
		httpClient: &http.Client{
			Timeout: 35 * time.Second,
		},
		baseURL:    "https://api.telegram.org",
		recipients: make(map[int64]TelegramRecipient),
		threshold:  20.0,
		enabled:    true,
	}
	bot.Reload(context.Background())
	return bot
}

func (b *TelegramBot) SetBaseURL(url string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.baseURL = url
}

func (b *TelegramBot) SetHTTPClient(client *http.Client) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.httpClient = client
}

func (b *TelegramBot) IsRunning() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.running
}

func (b *TelegramBot) Recipients() []TelegramRecipient {
	b.mu.RLock()
	defer b.mu.RUnlock()
	res := make([]TelegramRecipient, 0, len(b.recipients))
	for _, r := range b.recipients {
		res = append(res, r)
	}
	return res
}

func (b *TelegramBot) Reload(ctx context.Context) {
	b.mu.Lock()
	defer b.mu.Unlock()

	enabled := true
	threshold := 20.0
	botToken := ""
	var allowedUsers []string

	if b.cfg != nil {
		enabled = b.cfg.TelegramAlertsEnabled
		botToken = b.cfg.TelegramBotToken
		if b.cfg.TelegramAlertThresholdUSD > 0 {
			threshold = b.cfg.TelegramAlertThresholdUSD
		}
		allowedUsers = append(allowedUsers, b.cfg.TelegramAllowedUsers...)
	}

	if b.store != nil && b.store.DB() != nil {
		keys := []string{
			config.SettingKeyTelegramBotToken,
			config.SettingKeyTelegramAlertsEnabled,
			config.SettingKeyTelegramAlertThreshold,
			config.SettingKeyTelegramAllowedUsers,
			config.SettingKeyTelegramActiveRecipients,
		}

		settings, err := b.store.DB().Setting.Query().
			Where(setting.KeyIn(keys...)).
			All(ctx)
		if err == nil {
			for _, s := range settings {
				switch s.Key {
				case config.SettingKeyTelegramBotToken:
					if strings.TrimSpace(s.Value) != "" {
						botToken = strings.TrimSpace(s.Value)
					}
				case config.SettingKeyTelegramAlertsEnabled:
					if val, err := strconv.ParseBool(strings.TrimSpace(s.Value)); err == nil {
						enabled = val
					}
				case config.SettingKeyTelegramAlertThreshold:
					if val, err := strconv.ParseFloat(strings.TrimSpace(s.Value), 64); err == nil && val >= 0 {
						threshold = val
					}
				case config.SettingKeyTelegramAllowedUsers:
					allowedUsers = parseAllowedUsers(s.Value)
				case config.SettingKeyTelegramActiveRecipients:
					var savedRecipients []TelegramRecipient
					if err := json.Unmarshal([]byte(s.Value), &savedRecipients); err == nil {
						for _, r := range savedRecipients {
							if r.ChatID != 0 {
								b.recipients[r.ChatID] = r
							}
						}
					}
				}
			}
		}
	}

	b.botToken = botToken
	b.enabled = enabled
	b.threshold = threshold
	b.allowedUsers = allowedUsers
}

func parseAllowedUsers(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var arr []string
	if strings.HasPrefix(raw, "[") {
		if err := json.Unmarshal([]byte(raw), &arr); err == nil {
			res := make([]string, 0, len(arr))
			for _, item := range arr {
				if t := strings.TrimSpace(item); t != "" {
					res = append(res, t)
				}
			}
			return res
		}
	}
	// Fallback to comma/newline-separated
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == ';'
	})
	res := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			res = append(res, t)
		}
	}
	return res
}

func (b *TelegramBot) IsUserAllowed(userID int64, username string) bool {
	b.mu.RLock()
	allowed := b.allowedUsers
	b.mu.RUnlock()

	return IsTelegramUserAllowed(userID, username, allowed)
}

func IsTelegramUserAllowed(userID int64, username string, allowedUsers []string) bool {
	if len(allowedUsers) == 0 {
		return false
	}
	cleanUsername := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(username), "@"))
	userIDStr := strconv.FormatInt(userID, 10)

	for _, u := range allowedUsers {
		trimmed := strings.TrimSpace(u)
		if trimmed == "" {
			continue
		}
		if trimmed == "*" {
			return true
		}
		if trimmed == userIDStr {
			return true
		}
		cleanAllowed := strings.ToLower(strings.TrimPrefix(trimmed, "@"))
		if cleanUsername != "" && cleanAllowed == cleanUsername {
			return true
		}
	}
	return false
}

func (b *TelegramBot) GetActiveRecipients() []TelegramRecipient {
	b.mu.RLock()
	defer b.mu.RUnlock()
	res := make([]TelegramRecipient, 0, len(b.recipients))
	for _, r := range b.recipients {
		res = append(res, r)
	}
	return res
}

func (b *TelegramBot) RegisterRecipient(ctx context.Context, chatID int64, username, firstName string) {
	b.mu.Lock()
	b.recipients[chatID] = TelegramRecipient{
		ChatID:    chatID,
		Username:  username,
		FirstName: firstName,
		JoinedAt:  time.Now().UTC(),
	}
	allRecipients := make([]TelegramRecipient, 0, len(b.recipients))
	for _, r := range b.recipients {
		allRecipients = append(allRecipients, r)
	}
	b.mu.Unlock()

	// Persist active recipients to DB
	if b.store != nil && b.store.DB() != nil {
		data, err := json.Marshal(allRecipients)
		if err == nil {
			_ = b.store.DB().Setting.Create().
				SetKey(config.SettingKeyTelegramActiveRecipients).
				SetValue(string(data)).
				SetUpdatedAt(time.Now()).
				OnConflictColumns(setting.FieldKey).
				UpdateNewValues().
				Exec(ctx)
		}
	}

	// Persist to Redis
	if b.store != nil && b.store.Redis() != nil {
		recipientJSON, _ := json.Marshal(TelegramRecipient{
			ChatID:    chatID,
			Username:  username,
			FirstName: firstName,
			JoinedAt:  time.Now().UTC(),
		})
		_ = b.store.Redis().HSet(ctx, "ash:yunwu:telegram:recipients", strconv.FormatInt(chatID, 10), string(recipientJSON)).Err()
	}
}

func (b *TelegramBot) Start(ctx context.Context) {
	b.mu.Lock()
	if b.running {
		b.mu.Unlock()
		return
	}
	b.stopCh = make(chan struct{})
	b.running = true
	b.mu.Unlock()

	go b.pollLoop(ctx)
}

func (b *TelegramBot) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.running {
		return
	}
	b.running = false
	close(b.stopCh)
}

type tgUpdate struct {
	UpdateID int64      `json:"update_id"`
	Message  *tgMessage `json:"message"`
}

type tgMessage struct {
	MessageID int64   `json:"message_id"`
	From      *tgUser `json:"from"`
	Chat      tgChat  `json:"chat"`
	Text      string  `json:"text"`
}

type tgUser struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	UserName  string `json:"username"`
}

type tgChat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type tgUpdatesResponse struct {
	OK          bool       `json:"ok"`
	Result      []tgUpdate `json:"result"`
	Description string     `json:"description,omitempty"`
}

func (b *TelegramBot) pollLoop(ctx context.Context) {
	var offset int64 = 0

	for {
		select {
		case <-b.stopCh:
			return
		case <-ctx.Done():
			return
		default:
		}

		b.mu.RLock()
		token := b.botToken
		enabled := b.enabled
		client := b.httpClient
		baseURL := b.baseURL
		b.mu.RUnlock()

		if !enabled || strings.TrimSpace(token) == "" {
			select {
			case <-b.stopCh:
				return
			case <-time.After(5 * time.Second):
				b.Reload(ctx)
				continue
			}
		}

		updates, nextOffset, err := b.fetchUpdates(ctx, client, baseURL, token, offset)
		if err != nil {
			slog.Debug("[TelegramBot] Polling getUpdates error", "error", err)
			select {
			case <-b.stopCh:
				return
			case <-time.After(3 * time.Second):
				continue
			}
		}

		offset = nextOffset
		for _, u := range updates {
			if u.Message == nil || u.Message.From == nil {
				continue
			}
			b.handleMessage(ctx, u.Message)
		}
	}
}

func (b *TelegramBot) fetchUpdates(ctx context.Context, client *http.Client, baseURL, token string, offset int64) ([]tgUpdate, int64, error) {
	endpoint := fmt.Sprintf("%s/bot%s/getUpdates", strings.TrimRight(baseURL, "/"), token)
	payload := map[string]any{
		"offset":          offset,
		"timeout":         20,
		"allowed_updates": []string{"message"},
	}
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, offset, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, offset, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	var tgResp tgUpdatesResponse
	if err := json.Unmarshal(respBody, &tgResp); err != nil {
		return nil, offset, fmt.Errorf("decode getUpdates: %w", err)
	}
	if !tgResp.OK {
		return nil, offset, fmt.Errorf("getUpdates API error: %s", tgResp.Description)
	}

	nextOffset := offset
	for _, item := range tgResp.Result {
		if item.UpdateID >= nextOffset {
			nextOffset = item.UpdateID + 1
		}
	}
	return tgResp.Result, nextOffset, nil
}

func (b *TelegramBot) handleMessage(ctx context.Context, msg *tgMessage) {
	from := msg.From
	chatID := msg.Chat.ID
	text := strings.TrimSpace(msg.Text)

	// Check whitelist authorization
	if !b.IsUserAllowed(from.ID, from.UserName) {
		slog.Warn("[TelegramBot] Unauthorized message attempt",
			"user_id", from.ID,
			"username", from.UserName,
			"chat_id", chatID,
			"text", text,
		)
		reply := fmt.Sprintf(
			"⛔ <b>Доступ ограничен</b>\n\n"+
				"Ваш Telegram ID: <code>%d</code>\n"+
				"Username: @%s\n\n"+
				"Вы не добавлены в список разрешенных пользователей в панели администратора. "+
				"Обратитесь к администратору для предоставления доступа.",
			from.ID,
			html.EscapeString(from.UserName),
		)
		_ = b.sendMessage(ctx, chatID, reply)
		return
	}

	// User is authorized! Register as active recipient
	b.RegisterRecipient(ctx, chatID, from.UserName, from.FirstName)

	cmd := strings.ToLower(strings.Fields(text)[0])
	// Strip bot mention if present, e.g. /balance@MyBot
	if idx := strings.Index(cmd, "@"); idx != -1 {
		cmd = cmd[:idx]
	}

	switch cmd {
	case "/start", "/help":
		b.handleStartCommand(ctx, chatID, from.FirstName)
	case "/balance":
		b.handleBalanceCommand(ctx, chatID)
	case "/status":
		b.handleStatusCommand(ctx, chatID)
	default:
		reply := "ℹ️ Доступные команды:\n• /balance — проверить балансы провайдеров\n• /status — статус нод и системы\n• /help — справка"
		_ = b.sendMessage(ctx, chatID, reply)
	}
}

func (b *TelegramBot) handleStartCommand(ctx context.Context, chatID int64, firstName string) {
	b.mu.RLock()
	threshold := b.threshold
	b.mu.RUnlock()

	msg := fmt.Sprintf(
		"👋 <b>Добро пожаловать, %s!</b>\n\n"+
			"Вы успешно авторизованы в боте мониторинга <b>AI Gateway</b>.\n"+
			"Бот будет присылать вам уведомления, если баланс любого провайдера опустится до <b>$%.2f</b> или ниже.\n\n"+
			"<b>Доступные команды:</b>\n"+
			"• /balance — проверить текущий баланс всех провайдеров\n"+
			"• /status — состояние нод и порогов\n"+
			"• /help — помощь по командам",
		html.EscapeString(firstName),
		threshold,
	)
	_ = b.sendMessage(ctx, chatID, msg)
}

func (b *TelegramBot) handleBalanceCommand(ctx context.Context, chatID int64) {
	b.mu.RLock()
	threshold := b.threshold
	b.mu.RUnlock()

	var nodes []health.ExecutionNode
	if b.store != nil && b.store.Redis() != nil {
		allNodes, err := b.nm.GetAllNodes(ctx)
		if err == nil {
			nodes = allNodes
		}
	}

	if len(nodes) == 0 {
		_ = b.sendMessage(ctx, chatID, "ℹ️ Нет зарегистрированных нод провайдеров в кластере.")
		return
	}

	var sb strings.Builder
	sb.WriteString("💳 <b>Балансы провайдеров Yunwu:</b>\n\n")

	hasLowBalance := false
	for _, n := range nodes {
		snap, err := b.sw.Snapshot(ctx, n.NodeID, 1*time.Hour)
		balanceUSD := 0.0
		status := "ok"
		if err == nil && snap != nil {
			balanceUSD = snap.AvailableUSD
		}

		provider := n.Provider
		if provider == "" {
			provider = "Yunwu"
		}
		accName := n.AccountName
		if accName == "" {
			accName = n.NodeID
		}

		warn := ""
		if balanceUSD <= threshold {
			warn = " ⚠️ <i>(Низкий баланс!)</i>"
			hasLowBalance = true
		}

		sb.WriteString(fmt.Sprintf("• <b>%s</b> (%s): $%.2f%s\n",
			html.EscapeString(provider),
			html.EscapeString(accName),
			balanceUSD,
			warn,
		))
		_ = status
	}

	sb.WriteString(fmt.Sprintf("\n<i>Порог предупреждения: $%.2f</i>", threshold))
	if hasLowBalance {
		sb.WriteString("\n⚠️ <b>Внимание:</b> обнаружены аккаунты с низким остатком средств!")
	}

	_ = b.sendMessage(ctx, chatID, sb.String())
}

func (b *TelegramBot) handleStatusCommand(ctx context.Context, chatID int64) {
	b.mu.RLock()
	threshold := b.threshold
	enabled := b.enabled
	recipientsCount := len(b.recipients)
	b.mu.RUnlock()

	var totalNodes, healthyNodes, cooldownNodes int
	if b.store != nil && b.store.Redis() != nil {
		nodes, _ := b.nm.GetAllNodes(ctx)
		totalNodes = len(nodes)
		for _, n := range nodes {
			if n.State == "healthy" {
				healthyNodes++
			} else if n.State == "cooldown" {
				cooldownNodes++
			}
		}
	}

	statusText := "🟢 Активны"
	if !enabled {
		statusText = "🔴 Отключены"
	}

	msg := fmt.Sprintf(
		"⚙️ <b>Статус системы AI Gateway</b>\n\n"+
			"• <b>Уведомления:</b> %s\n"+
			"• <b>Порог низкого баланса:</b> $%.2f\n"+
			"• <b>Всего нод провайдеров:</b> %d\n"+
			"• <b>Здоровых нод:</b> %d\n"+
			"• <b>Нод в кулдауне:</b> %d\n"+
			"• <b>Авторизованных получателей:</b> %d\n"+
			"• <b>Время сервера:</b> %s",
		statusText,
		threshold,
		totalNodes,
		healthyNodes,
		cooldownNodes,
		recipientsCount,
		time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
	)
	_ = b.sendMessage(ctx, chatID, msg)
}

func (b *TelegramBot) sendMessage(ctx context.Context, chatID int64, text string) error {
	b.mu.RLock()
	token := b.botToken
	client := b.httpClient
	baseURL := b.baseURL
	b.mu.RUnlock()

	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("telegram bot token is empty")
	}
	if client == nil {
		client = http.DefaultClient
	}

	endpoint := fmt.Sprintf("%s/bot%s/sendMessage", strings.TrimRight(baseURL, "/"), token)
	payload := telegramSendMessagePayload{
		ChatID:    strconv.FormatInt(chatID, 10),
		Text:      text,
		ParseMode: "HTML",
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("telegram returned status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/yunwu"
	"github.com/jmoiron/sqlx"
)

// Two-way support chat (§15).
//
// A user message from the support widget is stored in support_messages and
// forwarded through the SAME Telegram bot that carries the admin alerts. Each
// delivered Telegram message is remembered in support_telegram_links; when an
// admin uses Telegram's "Reply" on it, the bot's reply hook
// (yunwu.SetSupportReplyHandler) lands in HandleTelegramReply, which stores the
// answer as an 'admin' message the user sees in the widget.
//
// Only identifiers travel to Telegram (user id, e-mail, request id, model,
// page) — never API keys or credentials.

const (
	SupportDirectionUser  = "user"
	SupportDirectionAdmin = "admin"

	// SupportTopicSupport is the default: an ordinary message to the consultant.
	SupportTopicSupport = ""
	// SupportTopicManualInvoice marks a business request — the user wants an
	// invoice (e.g. for a bank transfer) instead of paying with crypto.
	SupportTopicManualInvoice = "manual_invoice"

	supportMaxBodyRunes   = 2000
	supportMaxFieldRunes  = 128
	supportMaxTopicRunes  = 64
	supportMaxAmountRunes = 32
	supportRateWindow     = time.Minute
	supportRateLimit      = 5
	supportListLimit      = 200
)

var (
	ErrSupportEmptyMessage = errors.New("support message is empty")
	ErrSupportRateLimited  = errors.New("too many support messages, please wait a minute")
)

// SupportMessage is one line of the user's support thread.
type SupportMessage struct {
	ID        int64     `json:"id" db:"id"`
	Direction string    `json:"direction" db:"direction"`
	Body      string    `json:"body" db:"body"`
	RequestID string    `json:"request_id" db:"request_id"`
	Model     string    `json:"model" db:"model"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	Read      bool      `json:"read" db:"-"`
}

// SupportThread is the user's conversation plus the unread admin replies.
type SupportThread struct {
	Items  []SupportMessage `json:"items"`
	Unread int              `json:"unread"`
}

// SupportSendInput is what the widget submits.
type SupportSendInput struct {
	Message   string
	RequestID string
	Model     string
	Page      string
	// Topic selects the Telegram headline (see supportTopicTitles). Empty means
	// an ordinary support message.
	Topic string
	// Amount is the top-up amount the user is asking an invoice for, when the
	// request came from the payment screen.
	Amount string
}

// SupportSendResult reports the stored message and whether Telegram got it.
type SupportSendResult struct {
	Message   SupportMessage `json:"message"`
	Delivered bool           `json:"delivered"`
}

type telegramSender interface {
	SendTelegramNotificationDetailed(ctx context.Context, msg string) ([]TelegramSentMessage, error)
}

type SupportService struct {
	db     *sqlx.DB
	sender telegramSender
}

func NewSupportService(db *sql.DB, settingService *SettingService) *SupportService {
	svc := &SupportService{}
	if db != nil {
		svc.db = sqlx.NewDb(db, "postgres")
	}
	if settingService != nil {
		svc.sender = settingService
	}
	return svc
}

// InstallTelegramReplyHandler routes admin Telegram replies into this service.
func (s *SupportService) InstallTelegramReplyHandler() {
	yunwu.SetSupportReplyHandler(s.HandleTelegramReply)
}

func truncateRunes(v string, max int) string {
	v = strings.TrimSpace(v)
	if utf8.RuneCountInString(v) <= max {
		return v
	}
	return string([]rune(v)[:max])
}

// SendUserMessage stores the user's message and forwards it to Telegram.
// The message is kept even when Telegram is unreachable (Delivered=false) so the
// widget can fall back to its e-mail / copy flow without losing the text.
func (s *SupportService) SendUserMessage(ctx context.Context, userID int64, in SupportSendInput) (*SupportSendResult, error) {
	body := truncateRunes(in.Message, supportMaxBodyRunes)
	if body == "" {
		return nil, ErrSupportEmptyMessage
	}
	requestID := truncateRunes(in.RequestID, supportMaxFieldRunes)
	model := truncateRunes(in.Model, supportMaxFieldRunes)
	page := truncateRunes(in.Page, 255)
	topic := truncateRunes(in.Topic, supportMaxTopicRunes)
	amount := sanitizeSupportAmount(in.Amount)

	var recent int
	if err := s.db.GetContext(ctx, &recent,
		`SELECT COUNT(*) FROM support_messages WHERE user_id = $1 AND direction = $2 AND created_at > $3`,
		userID, SupportDirectionUser, time.Now().Add(-supportRateWindow)); err != nil {
		return nil, fmt.Errorf("check support rate: %w", err)
	}
	if recent >= supportRateLimit {
		return nil, ErrSupportRateLimited
	}

	var msg SupportMessage
	if err := s.db.GetContext(ctx, &msg,
		`INSERT INTO support_messages (user_id, direction, body, request_id, model, page)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, direction, body, request_id, model, created_at`,
		userID, SupportDirectionUser, body, requestID, model, page); err != nil {
		return nil, fmt.Errorf("store support message: %w", err)
	}

	result := &SupportSendResult{Message: msg}
	if s.sender == nil {
		return result, nil
	}

	var contact struct {
		Email    string `db:"email"`
		Username string `db:"username"`
	}
	_ = s.db.GetContext(ctx, &contact, `SELECT email, username FROM users WHERE id = $1`, userID)

	sent, err := s.sender.SendTelegramNotificationDetailed(ctx, formatSupportTelegramMessage(supportTelegramNotice{
		UserID:    userID,
		Username:  contact.Username,
		Email:     contact.Email,
		Body:      body,
		RequestID: requestID,
		Model:     model,
		Page:      page,
		Topic:     topic,
		Amount:    amount,
	}))
	if err != nil {
		slog.Warn("[Support] telegram delivery failed", "user_id", userID, "error", err)
		return result, nil
	}
	for _, m := range sent {
		if m.ChatID == 0 || m.MessageID == 0 {
			continue
		}
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO support_telegram_links (chat_id, tg_message_id, support_message_id)
			 VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, m.ChatID, m.MessageID, msg.ID); err != nil {
			slog.Warn("[Support] failed to store telegram link", "error", err)
		}
	}
	result.Delivered = len(sent) > 0
	return result, nil
}

// supportTelegramNotice is everything rendered into the admin's Telegram copy.
type supportTelegramNotice struct {
	UserID    int64
	Username  string
	Email     string
	Body      string
	RequestID string
	Model     string
	Page      string
	Topic     string
	Amount    string
}

// supportTopicTitles maps a client-declared topic onto the Telegram headline.
// The client never controls the rendered title: an unknown (or empty) topic
// falls back to the generic support headline.
var supportTopicTitles = map[string]string{
	SupportTopicSupport:       "💬 <b>Сообщение в поддержку</b>",
	SupportTopicManualInvoice: "🧾 <b>Запрос счёта (банк / безнал)</b>",
}

func supportTopicTitle(topic string) string {
	if title, ok := supportTopicTitles[strings.TrimSpace(topic)]; ok {
		return title
	}
	return supportTopicTitles[SupportTopicSupport]
}

// sanitizeSupportAmount keeps a client-provided amount usable as a money value:
// digits and the usual separators only, capped in length. Anything else (an
// @handle, HTML, a sentence) is dropped rather than rendered into the alert.
func sanitizeSupportAmount(raw string) string {
	trimmed := truncateRunes(raw, supportMaxAmountRunes)
	var b strings.Builder
	for _, r := range trimmed {
		if (r >= '0' && r <= '9') || r == '.' || r == ',' {
			b.WriteRune(r)
		}
	}
	out := strings.Trim(b.String(), ".,")
	if out == "" {
		return ""
	}
	return out
}

func formatSupportTelegramMessage(n supportTelegramNotice) string {
	var b strings.Builder
	b.WriteString(supportTopicTitle(n.Topic) + "\n")
	fmt.Fprintf(&b, "• <b>User:</b> #%d", n.UserID)
	if n.Username != "" {
		fmt.Fprintf(&b, " @%s", html.EscapeString(n.Username))
	}
	if n.Email != "" {
		fmt.Fprintf(&b, " (%s)", html.EscapeString(n.Email))
	}
	b.WriteString("\n")
	if n.Amount != "" {
		fmt.Fprintf(&b, "• <b>Amount:</b> $%s\n", html.EscapeString(n.Amount))
	}
	if n.RequestID != "" {
		fmt.Fprintf(&b, "• <b>Request ID:</b> <code>%s</code>\n", html.EscapeString(n.RequestID))
	}
	if n.Model != "" {
		fmt.Fprintf(&b, "• <b>Model:</b> %s\n", html.EscapeString(n.Model))
	}
	if n.Page != "" {
		fmt.Fprintf(&b, "• <b>Page:</b> %s\n", html.EscapeString(n.Page))
	}
	fmt.Fprintf(&b, "\n%s\n\n", html.EscapeString(n.Body))
	b.WriteString("<i>Ответьте на это сообщение (Reply), чтобы написать пользователю.</i>")
	return b.String()
}

// HandleTelegramReply stores an admin's Telegram reply for the owner of the
// support message that was replied to. handled=false means the replied-to
// Telegram message is not a support message.
func (s *SupportService) HandleTelegramReply(ctx context.Context, chatID, replyToMessageID int64, text string) (bool, error) {
	text = truncateRunes(text, supportMaxBodyRunes)
	if text == "" {
		return false, nil
	}
	var userID int64
	err := s.db.GetContext(ctx, &userID,
		`SELECT m.user_id FROM support_telegram_links l
		   JOIN support_messages m ON m.id = l.support_message_id
		  WHERE l.chat_id = $1 AND l.tg_message_id = $2`, chatID, replyToMessageID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("lookup support link: %w", err)
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO support_messages (user_id, direction, body) VALUES ($1, $2, $3)`,
		userID, SupportDirectionAdmin, text); err != nil {
		return false, fmt.Errorf("store support reply: %w", err)
	}
	return true, nil
}

// GetThread returns the caller's own conversation (oldest first, capped).
func (s *SupportService) GetThread(ctx context.Context, userID int64) (*SupportThread, error) {
	type row struct {
		SupportMessage
		ReadAt sql.NullTime `db:"read_at"`
	}
	var rows []row
	if err := s.db.SelectContext(ctx, &rows,
		`SELECT id, direction, body, request_id, model, created_at, read_at FROM (
		     SELECT * FROM support_messages WHERE user_id = $1 ORDER BY id DESC LIMIT $2
		 ) t ORDER BY id ASC`, userID, supportListLimit); err != nil {
		return nil, fmt.Errorf("list support messages: %w", err)
	}
	thread := &SupportThread{Items: make([]SupportMessage, 0, len(rows))}
	for _, r := range rows {
		m := r.SupportMessage
		m.Read = r.ReadAt.Valid || m.Direction == SupportDirectionUser
		if !m.Read {
			thread.Unread++
		}
		thread.Items = append(thread.Items, m)
	}
	return thread, nil
}

// MarkRead marks the caller's unread admin replies as read.
func (s *SupportService) MarkRead(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE support_messages SET read_at = NOW()
		  WHERE user_id = $1 AND direction = $2 AND read_at IS NULL`, userID, SupportDirectionAdmin)
	return err
}

//go:build unit

package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeTelegramSender struct {
	sent []TelegramSentMessage
	err  error
	text string
}

func (f *fakeTelegramSender) SendTelegramNotificationDetailed(_ context.Context, msg string) ([]TelegramSentMessage, error) {
	f.text = msg
	return f.sent, f.err
}

func newSupportTestService(t *testing.T, sender telegramSender) (*SupportService, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	svc := NewSupportService(db, nil)
	svc.sender = sender
	return svc, mock
}

func expectInsertMessage(mock sqlmock.Sqlmock, recent int) {
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM support_messages`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(recent))
}

func TestSupportSendUserMessage_ForwardsAndStoresLinks(t *testing.T) {
	sender := &fakeTelegramSender{sent: []TelegramSentMessage{{ChatID: 11, MessageID: 101}, {ChatID: 12, MessageID: 102}}}
	svc, mock := newSupportTestService(t, sender)

	expectInsertMessage(mock, 0)
	mock.ExpectQuery(`INSERT INTO support_messages`).
		WithArgs(int64(7), "user", "help <b>me</b>", "req-1", "gpt-5", "/usage").
		WillReturnRows(sqlmock.NewRows([]string{"id", "direction", "body", "request_id", "model", "created_at"}).
			AddRow(42, "user", "help <b>me</b>", "req-1", "gpt-5", time.Now()))
	mock.ExpectQuery(`SELECT email, username FROM users`).WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"email", "username"}).AddRow("a@b.c", "l0s0s"))
	mock.ExpectExec(`INSERT INTO support_telegram_links`).WithArgs(int64(11), int64(101), int64(42)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO support_telegram_links`).WithArgs(int64(12), int64(102), int64(42)).WillReturnResult(sqlmock.NewResult(0, 1))

	res, err := svc.SendUserMessage(context.Background(), 7, SupportSendInput{
		Message: "  help <b>me</b> ", RequestID: "req-1", Model: "gpt-5", Page: "/usage",
	})
	require.NoError(t, err)
	assert.True(t, res.Delivered)
	assert.Equal(t, int64(42), res.Message.ID)
	// User text must be HTML-escaped; identifiers present; secrets never added.
	assert.Contains(t, sender.text, "help &lt;b&gt;me&lt;/b&gt;")
	assert.Contains(t, sender.text, "#7 @l0s0s (a@b.c)")
	assert.Contains(t, sender.text, "req-1")
	// An ordinary message keeps the generic headline.
	assert.Contains(t, sender.text, "💬 <b>Сообщение в поддержку</b>")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSupportSendUserMessage_ManualInvoiceTopicAndAmount(t *testing.T) {
	sender := &fakeTelegramSender{sent: []TelegramSentMessage{{ChatID: 11, MessageID: 101}}}
	svc, mock := newSupportTestService(t, sender)

	expectInsertMessage(mock, 0)
	mock.ExpectQuery(`INSERT INTO support_messages`).
		WithArgs(int64(7), "user", "invoice please", "", "", "/purchase").
		WillReturnRows(sqlmock.NewRows([]string{"id", "direction", "body", "request_id", "model", "created_at"}).
			AddRow(43, "user", "invoice please", "", "", time.Now()))
	mock.ExpectQuery(`SELECT email, username FROM users`).WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"email", "username"}).AddRow("a@b.c", "l0s0s"))
	mock.ExpectExec(`INSERT INTO support_telegram_links`).WillReturnResult(sqlmock.NewResult(0, 1))

	res, err := svc.SendUserMessage(context.Background(), 7, SupportSendInput{
		Message: "invoice please", Page: "/purchase",
		Topic: SupportTopicManualInvoice, Amount: "250.00",
	})
	require.NoError(t, err)
	assert.True(t, res.Delivered)
	// Business request gets its own headline and the amount is called out.
	assert.Contains(t, sender.text, "🧾 <b>Запрос счёта (банк / безнал)</b>")
	assert.Contains(t, sender.text, "• <b>Amount:</b> $250.00")
	assert.NotContains(t, sender.text, "💬 <b>Сообщение в поддержку</b>")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSupportTopicTitle_FallsBackAndAmountIsSanitized(t *testing.T) {
	// Unknown / empty topics must never leak client text into the headline.
	assert.Equal(t, supportTopicTitles[SupportTopicSupport], supportTopicTitle(""))
	assert.Equal(t, supportTopicTitles[SupportTopicSupport], supportTopicTitle("  "))
	assert.Equal(t, supportTopicTitles[SupportTopicSupport], supportTopicTitle("whatever"))
	assert.Equal(t, supportTopicTitles[SupportTopicManualInvoice], supportTopicTitle(" manual_invoice "))
	// Only digits and separators survive; anything else is dropped.
	assert.Equal(t, "250.00", sanitizeSupportAmount(" 250.00 "))
	assert.Equal(t, "1,000", sanitizeSupportAmount("1,000"))
	assert.Equal(t, "", sanitizeSupportAmount("@channel"))
	assert.Equal(t, "", sanitizeSupportAmount(""))
	assert.Equal(t, "42", sanitizeSupportAmount("<b>42</b>"))

	msg := formatSupportTelegramMessage(supportTelegramNotice{UserID: 7, Amount: ""})
	assert.NotContains(t, msg, "<b>Amount:</b>")
	assert.Contains(t, msg, supportTopicTitles[SupportTopicSupport])
}

func TestSupportSendUserMessage_TelegramDownStillStores(t *testing.T) {
	svc, mock := newSupportTestService(t, &fakeTelegramSender{err: errors.New("boom")})
	expectInsertMessage(mock, 0)
	mock.ExpectQuery(`INSERT INTO support_messages`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "direction", "body", "request_id", "model", "created_at"}).
			AddRow(1, "user", "x", "", "", time.Now()))
	mock.ExpectQuery(`SELECT email, username FROM users`).WillReturnError(sql.ErrNoRows)

	res, err := svc.SendUserMessage(context.Background(), 7, SupportSendInput{Message: "x"})
	require.NoError(t, err)
	assert.False(t, res.Delivered)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSupportSendUserMessage_ValidationAndRateLimit(t *testing.T) {
	svc, mock := newSupportTestService(t, &fakeTelegramSender{})
	_, err := svc.SendUserMessage(context.Background(), 7, SupportSendInput{Message: "   "})
	assert.ErrorIs(t, err, ErrSupportEmptyMessage)

	expectInsertMessage(mock, supportRateLimit)
	_, err = svc.SendUserMessage(context.Background(), 7, SupportSendInput{Message: "hi"})
	assert.ErrorIs(t, err, ErrSupportRateLimited)
}

func TestSupportSendUserMessage_TruncatesLongBody(t *testing.T) {
	svc, mock := newSupportTestService(t, nil)
	expectInsertMessage(mock, 0)
	long := strings.Repeat("я", supportMaxBodyRunes+50)
	mock.ExpectQuery(`INSERT INTO support_messages`).
		WithArgs(int64(7), "user", strings.Repeat("я", supportMaxBodyRunes), "", "", "").
		WillReturnRows(sqlmock.NewRows([]string{"id", "direction", "body", "request_id", "model", "created_at"}).
			AddRow(1, "user", "x", "", "", time.Now()))
	_, err := svc.SendUserMessage(context.Background(), 7, SupportSendInput{Message: long})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSupportHandleTelegramReply(t *testing.T) {
	svc, mock := newSupportTestService(t, nil)

	// Known support message -> stored as an admin reply for its owner.
	mock.ExpectQuery(`FROM support_telegram_links`).WithArgs(int64(11), int64(101)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(7))
	mock.ExpectExec(`INSERT INTO support_messages`).WithArgs(int64(7), "admin", "ответ").WillReturnResult(sqlmock.NewResult(1, 1))
	handled, err := svc.HandleTelegramReply(context.Background(), 11, 101, " ответ ")
	require.NoError(t, err)
	assert.True(t, handled)

	// Reply to something that is not a support message -> ignored.
	mock.ExpectQuery(`FROM support_telegram_links`).WillReturnError(sql.ErrNoRows)
	handled, err = svc.HandleTelegramReply(context.Background(), 11, 999, "x")
	require.NoError(t, err)
	assert.False(t, handled)

	// Empty text -> nothing.
	handled, err = svc.HandleTelegramReply(context.Background(), 11, 101, "  ")
	require.NoError(t, err)
	assert.False(t, handled)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSupportGetThread_CountsUnread(t *testing.T) {
	svc, mock := newSupportTestService(t, nil)
	now := time.Now()
	mock.ExpectQuery(`SELECT id, direction`).WithArgs(int64(7), supportListLimit).
		WillReturnRows(sqlmock.NewRows([]string{"id", "direction", "body", "request_id", "model", "created_at", "read_at"}).
			AddRow(1, "user", "q", "", "", now, nil).
			AddRow(2, "admin", "a1", "", "", now, now).
			AddRow(3, "admin", "a2", "", "", now, nil))
	th, err := svc.GetThread(context.Background(), 7)
	require.NoError(t, err)
	require.Len(t, th.Items, 3)
	assert.Equal(t, 1, th.Unread)
	assert.True(t, th.Items[0].Read)
	assert.False(t, th.Items[2].Read)
}

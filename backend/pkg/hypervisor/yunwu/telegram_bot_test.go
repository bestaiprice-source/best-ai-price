package yunwu

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/config"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsTelegramUserAllowed(t *testing.T) {
	tests := []struct {
		name         string
		userID       int64
		username     string
		allowedUsers []string
		expected     bool
	}{
		{
			name:         "empty whitelist rejects all",
			userID:       12345,
			username:     "alice",
			allowedUsers: []string{},
			expected:     false,
		},
		{
			name:         "wildcard allows anyone",
			userID:       99999,
			username:     "stranger",
			allowedUsers: []string{"*"},
			expected:     true,
		},
		{
			name:         "match by numeric user ID",
			userID:       12345678,
			username:     "random_user",
			allowedUsers: []string{"12345678", "87654321"},
			expected:     true,
		},
		{
			name:         "match by username without @",
			userID:       11111,
			username:     "bob_smith",
			allowedUsers: []string{"bob_smith"},
			expected:     true,
		},
		{
			name:         "match by username with @",
			userID:       11111,
			username:     "bob_smith",
			allowedUsers: []string{"@bob_smith"},
			expected:     true,
		},
		{
			name:         "case-insensitive username match",
			userID:       11111,
			username:     "Bob_Smith",
			allowedUsers: []string{"@bob_smith"},
			expected:     true,
		},
		{
			name:         "case-insensitive whitelist entry",
			userID:       11111,
			username:     "bob_smith",
			allowedUsers: []string{"@BOB_SMITH"},
			expected:     true,
		},
		{
			name:         "non-matching user rejected",
			userID:       99999,
			username:     "mallory",
			allowedUsers: []string{"12345678", "@alice", "bob"},
			expected:     false,
		},
		{
			name:         "empty username but matching ID",
			userID:       55555,
			username:     "",
			allowedUsers: []string{"55555"},
			expected:     true,
		},
		{
			name:         "spaces trimmed in whitelist",
			userID:       12345,
			username:     "carol",
			allowedUsers: []string{"  @carol  "},
			expected:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed := IsTelegramUserAllowed(tt.userID, tt.username, tt.allowedUsers)
			assert.Equal(t, tt.expected, allowed)
		})
	}
}

func TestTelegramBot_UnauthorizedUser(t *testing.T) {
	var sentMessages []telegramSendMessagePayload
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/bottest-bot-token/sendMessage" {
			var payload telegramSendMessagePayload
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &payload)
			mu.Lock()
			sentMessages = append(sentMessages, payload)
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer server.Close()

	cfg := &config.HypervisorConfig{
		TelegramBotToken:     "test-bot-token",
		TelegramAllowedUsers: []string{"@admin_user", "100200300"},
	}

	bot := NewTelegramBot(nil, cfg, nil)
	bot.SetBaseURL(server.URL)

	unauthorizedMsg := &tgMessage{
		MessageID: 10,
		From: &tgUser{
			ID:        999888,
			UserName:  "intruder",
			FirstName: "Intruder",
		},
		Chat: tgChat{
			ID:   999888,
			Type: "private",
		},
		Text: "/start",
	}

	bot.handleMessage(context.Background(), unauthorizedMsg)

	mu.Lock()
	defer mu.Unlock()
	// Silent drop: no messages sent to unauthorized users
	require.Empty(t, sentMessages)
	assert.Empty(t, bot.GetActiveRecipients())
}

func TestTelegramBot_AuthorizedUserSilentRegistration(t *testing.T) {
	var sentMessages []telegramSendMessagePayload
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/bottest-bot-token/sendMessage" {
			var payload telegramSendMessagePayload
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &payload)
			mu.Lock()
			sentMessages = append(sentMessages, payload)
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer server.Close()

	cfg := &config.HypervisorConfig{
		TelegramBotToken:          "test-bot-token",
		TelegramAllowedUsers:      []string{"@authorized_admin"},
		TelegramAlertThresholdUSD: 20.0,
		TelegramAlertsEnabled:     true,
	}

	bot := NewTelegramBot(nil, cfg, nil)
	bot.SetBaseURL(server.URL)

	// 1. Authorized user sends /start -> silently registers recipient without returning text
	startMsg := &tgMessage{
		MessageID: 10,
		From: &tgUser{
			ID:        1234567,
			UserName:  "authorized_admin",
			FirstName: "Admin",
		},
		Chat: tgChat{
			ID:   1234567,
			Type: "private",
		},
		Text: "/start",
	}

	bot.handleMessage(context.Background(), startMsg)

	mu.Lock()
	require.Empty(t, sentMessages)
	mu.Unlock()

	recipients := bot.GetActiveRecipients()
	require.Len(t, recipients, 1)
	assert.Equal(t, int64(1234567), recipients[0].ChatID)
	assert.Equal(t, "authorized_admin", recipients[0].Username)

	// 2. Any other text from authorized user also sends zero conversational text
	otherMsg := &tgMessage{
		MessageID: 11,
		From: &tgUser{
			ID:       1234567,
			UserName: "authorized_admin",
		},
		Chat: tgChat{
			ID: 1234567,
		},
		Text: "/help",
	}
	bot.handleMessage(context.Background(), otherMsg)

	mu.Lock()
	require.Empty(t, sentMessages)
	mu.Unlock()
}

func TestTelegramBot_PollingLifecycle(t *testing.T) {
	var pollCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/bottest-bot-token/getUpdates" {
			atomic.AddInt32(&pollCount, 1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":     true,
				"result": []any{},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer server.Close()

	cfg := &config.HypervisorConfig{
		TelegramBotToken:      "test-bot-token",
		TelegramAllowedUsers:  []string{"*"},
		TelegramAlertsEnabled: true,
	}

	bot := NewTelegramBot(nil, cfg, nil)
	bot.SetBaseURL(server.URL)
	bot.SetHTTPClient(server.Client())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert.False(t, bot.IsRunning())
	bot.Start(ctx)
	assert.True(t, bot.IsRunning())
	time.Sleep(100 * time.Millisecond)
	bot.Stop()
	assert.False(t, bot.IsRunning())

	assert.GreaterOrEqual(t, atomic.LoadInt32(&pollCount), int32(1))
}

func TestParseAllowedUsers(t *testing.T) {
	assert.Nil(t, parseAllowedUsers(""))
	assert.Nil(t, parseAllowedUsers("   "))

	// JSON array
	jsonInput := `["@alice", "123456", "bob"]`
	assert.Equal(t, []string{"@alice", "123456", "bob"}, parseAllowedUsers(jsonInput))

	// Delimiter-separated (comma, newline, semicolon)
	delimInput := "@alice\n123456; bob,  carol  "
	assert.Equal(t, []string{"@alice", "123456", "bob", "carol"}, parseAllowedUsers(delimInput))
}

func TestTelegramBot_RecipientPersistence(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := storage.NewStorage(nil, rdb)
	ctx := context.Background()

	cfg := &config.HypervisorConfig{
		TelegramBotToken:          "test-bot-token",
		TelegramAllowedUsers:      []string{"12345"},
		TelegramAlertThresholdUSD: 20.0,
		TelegramAlertsEnabled:     true,
	}

	bot := NewTelegramBot(store, cfg, nil)

	// Register recipient
	bot.RegisterRecipient(ctx, 12345, "tester", "Tester")
	assert.Len(t, bot.Recipients(), 1)
	assert.Equal(t, int64(12345), bot.Recipients()[0].ChatID)
	assert.Equal(t, "tester", bot.Recipients()[0].Username)

	// Calling RegisterRecipient again updates or preserves
	bot.RegisterRecipient(ctx, 12345, "tester_new", "TesterNew")
	assert.Len(t, bot.Recipients(), 1)
	assert.Equal(t, "tester_new", bot.Recipients()[0].Username)
}

func TestTelegramBot_SupportReplyRouting(t *testing.T) {
	var sent []telegramSendMessagePayload
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/bottest-bot-token/sendMessage" {
			var p telegramSendMessagePayload
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &p)
			mu.Lock()
			sent = append(sent, p)
			mu.Unlock()
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer server.Close()

	bot := NewTelegramBot(nil, &config.HypervisorConfig{
		TelegramBotToken:      "test-bot-token",
		TelegramAllowedUsers:  []string{"@admin"},
		TelegramAlertsEnabled: true,
	}, nil)
	bot.SetBaseURL(server.URL)
	t.Cleanup(func() { SetSupportReplyHandler(nil) })

	var gotChat, gotReplyTo int64
	var gotText string
	SetSupportReplyHandler(func(_ context.Context, chatID, replyTo int64, text string) (bool, error) {
		gotChat, gotReplyTo, gotText = chatID, replyTo, text
		return replyTo == 500, nil
	})

	reply := func(from string, replyTo int64) *tgMessage {
		return &tgMessage{
			MessageID:      20,
			From:           &tgUser{ID: 9, UserName: from},
			Chat:           tgChat{ID: 9},
			Text:           "  hello user  ",
			ReplyToMessage: &tgMessage{MessageID: replyTo},
		}
	}

	// Reply to a support message: routed and confirmed.
	bot.handleMessage(context.Background(), reply("admin", 500))
	assert.Equal(t, int64(9), gotChat)
	assert.Equal(t, int64(500), gotReplyTo)
	assert.Equal(t, "hello user", gotText)
	mu.Lock()
	require.Len(t, sent, 1)
	assert.Contains(t, sent[0].Text, "✅")
	mu.Unlock()

	// Reply to an ordinary alert (not a support message): ignored silently.
	bot.handleMessage(context.Background(), reply("admin", 501))
	mu.Lock()
	assert.Len(t, sent, 1)
	mu.Unlock()

	// Unauthorized user: never reaches the handler.
	gotText = ""
	bot.handleMessage(context.Background(), reply("stranger", 500))
	assert.Empty(t, gotText)
}

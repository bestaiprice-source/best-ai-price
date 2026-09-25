package yunwu

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/config"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTelegramNotifier_AlertTriggered(t *testing.T) {
	ctx := context.Background()
	var requestCount int32
	var receivedPayload telegramSendMessagePayload

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		assert.Equal(t, "/bottest-token/sendMessage", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		assert.NoError(t, json.Unmarshal(body, &receivedPayload))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
		})
	}))
	defer server.Close()

	cfg := &config.HypervisorConfig{
		TelegramBotToken:          "test-token",
		TelegramChatID:            "-100123456789",
		TelegramAlertThresholdUSD: 20.0,
		TelegramAlertsEnabled:     true,
	}

	notifier := NewTelegramNotifier(nil, cfg)
	notifier.SetBaseURL(server.URL)

	// Balance <= 20.0 triggers notification
	notified, err := notifier.CheckAndNotify(ctx, "node-101", "OpenLux-Account", "openlux", 18.50)
	require.NoError(t, err)
	assert.True(t, notified)
	assert.Equal(t, int32(1), atomic.LoadInt32(&requestCount))
	assert.Equal(t, "-100123456789", receivedPayload.ChatID)
	assert.Equal(t, "HTML", receivedPayload.ParseMode)
	assert.Contains(t, receivedPayload.Text, "OpenLux")
	assert.Contains(t, receivedPayload.Text, "OpenLux-Account")
	assert.Contains(t, receivedPayload.Text, "node-101")
	assert.Contains(t, receivedPayload.Text, "$18.50")
	assert.Contains(t, receivedPayload.Text, "$20.00")
	assert.True(t, notifier.IsAlerted(ctx, "node-101"))
}

func TestTelegramNotifier_Deduplication(t *testing.T) {
	ctx := context.Background()
	var requestCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer server.Close()

	cfg := &config.HypervisorConfig{
		TelegramBotToken:          "test-token",
		TelegramChatID:            "-100123456789",
		TelegramAlertThresholdUSD: 20.0,
		TelegramAlertsEnabled:     true,
	}

	notifier := NewTelegramNotifier(nil, cfg)
	notifier.SetBaseURL(server.URL)

	// First low balance check -> sends notification
	notified, err := notifier.CheckAndNotify(ctx, "node-102", "ApiTop-Acc", "api-top", 19.00)
	require.NoError(t, err)
	assert.True(t, notified)
	assert.Equal(t, int32(1), atomic.LoadInt32(&requestCount))

	// Subsequent check while still low -> deduplicated, no new notification
	notified, err = notifier.CheckAndNotify(ctx, "node-102", "ApiTop-Acc", "api-top", 15.00)
	require.NoError(t, err)
	assert.False(t, notified)
	assert.Equal(t, int32(1), atomic.LoadInt32(&requestCount))
}

func TestTelegramNotifier_ReplenishAndReAlert(t *testing.T) {
	ctx := context.Background()
	var requestCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer server.Close()

	cfg := &config.HypervisorConfig{
		TelegramBotToken:          "test-token",
		TelegramChatID:            "-100123456789",
		TelegramAlertThresholdUSD: 20.0,
		TelegramAlertsEnabled:     true,
	}

	notifier := NewTelegramNotifier(nil, cfg)
	notifier.SetBaseURL(server.URL)

	// 1. Initial drop below threshold
	notified, err := notifier.CheckAndNotify(ctx, "node-103", "Yuanqiao-1", "yuanqiaoyun", 10.00)
	require.NoError(t, err)
	assert.True(t, notified)
	assert.Equal(t, int32(1), atomic.LoadInt32(&requestCount))
	assert.True(t, notifier.IsAlerted(ctx, "node-103"))

	// 2. Recharged / replenished above threshold ($50.00 > $20.00)
	notified, err = notifier.CheckAndNotify(ctx, "node-103", "Yuanqiao-1", "yuanqiaoyun", 50.00)
	require.NoError(t, err)
	assert.False(t, notified)
	assert.Equal(t, int32(1), atomic.LoadInt32(&requestCount))
	assert.False(t, notifier.IsAlerted(ctx, "node-103"))

	// 3. Balance drops below threshold again ($15.00 <= $20.00) -> alerts again!
	notified, err = notifier.CheckAndNotify(ctx, "node-103", "Yuanqiao-1", "yuanqiaoyun", 15.00)
	require.NoError(t, err)
	assert.True(t, notified)
	assert.Equal(t, int32(2), atomic.LoadInt32(&requestCount))
	assert.True(t, notifier.IsAlerted(ctx, "node-103"))
}

func TestTelegramNotifier_DisabledOrUnconfigured(t *testing.T) {
	ctx := context.Background()

	// 1. Empty nodeID
	notifier := NewTelegramNotifier(nil, &config.HypervisorConfig{
		TelegramBotToken:      "tok",
		TelegramChatID:        "chat",
		TelegramAlertsEnabled: true,
	})
	notified, err := notifier.CheckAndNotify(ctx, "", "acc", "prov", 10.0)
	require.NoError(t, err)
	assert.False(t, notified)

	// 2. Alerts disabled
	notifierDisabled := NewTelegramNotifier(nil, &config.HypervisorConfig{
		TelegramBotToken:      "tok",
		TelegramChatID:        "chat",
		TelegramAlertsEnabled: false,
	})
	notified, err = notifierDisabled.CheckAndNotify(ctx, "n1", "acc", "prov", 10.0)
	require.NoError(t, err)
	assert.False(t, notified)

	// 3. Missing bot token
	notifierNoToken := NewTelegramNotifier(nil, &config.HypervisorConfig{
		TelegramBotToken:      "",
		TelegramChatID:        "chat",
		TelegramAlertsEnabled: true,
	})
	notified, err = notifierNoToken.CheckAndNotify(ctx, "n1", "acc", "prov", 10.0)
	require.NoError(t, err)
	assert.False(t, notified)

	// 4. Missing chat ID
	notifierNoChat := NewTelegramNotifier(nil, &config.HypervisorConfig{
		TelegramBotToken:      "tok",
		TelegramChatID:        "",
		TelegramAlertsEnabled: true,
	})
	notified, err = notifierNoChat.CheckAndNotify(ctx, "n1", "acc", "prov", 10.0)
	require.NoError(t, err)
	assert.False(t, notified)

	// 5. Balance above threshold with no prior alert
	notifierHealthy := NewTelegramNotifier(nil, &config.HypervisorConfig{
		TelegramBotToken:          "tok",
		TelegramChatID:            "chat",
		TelegramAlertThresholdUSD: 20.0,
		TelegramAlertsEnabled:     true,
	})
	notified, err = notifierHealthy.CheckAndNotify(ctx, "n1", "acc", "prov", 25.0)
	require.NoError(t, err)
	assert.False(t, notified)
}

func TestTelegramNotifier_APIError(t *testing.T) {
	ctx := context.Background()

	t.Run("HTTP 400 error resets alert state for retry", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          false,
				"description": "Bad Request: chat not found",
			})
		}))
		defer server.Close()

		cfg := &config.HypervisorConfig{
			TelegramBotToken:          "bad-token",
			TelegramChatID:            "invalid-chat",
			TelegramAlertThresholdUSD: 20.0,
			TelegramAlertsEnabled:     true,
		}

		notifier := NewTelegramNotifier(nil, cfg)
		notifier.SetBaseURL(server.URL)

		notified, err := notifier.CheckAndNotify(ctx, "node-err", "acc", "prov", 10.0)
		assert.Error(t, err)
		assert.False(t, notified)
		assert.Contains(t, err.Error(), "chat not found")
		// Alert state should be reset so retry is possible
		assert.False(t, notifier.IsAlerted(ctx, "node-err"))
	})

	t.Run("telegram response ok false with 200 OK", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          false,
				"description": "Unauthorized",
			})
		}))
		defer server.Close()

		cfg := &config.HypervisorConfig{
			TelegramBotToken:          "bad-token",
			TelegramChatID:            "chat",
			TelegramAlertThresholdUSD: 20.0,
			TelegramAlertsEnabled:     true,
		}

		notifier := NewTelegramNotifier(nil, cfg)
		notifier.SetBaseURL(server.URL)

		notified, err := notifier.CheckAndNotify(ctx, "node-err-2", "acc", "prov", 10.0)
		assert.Error(t, err)
		assert.False(t, notified)
		assert.Contains(t, err.Error(), "Unauthorized")
		assert.False(t, notifier.IsAlerted(ctx, "node-err-2"))
	})
}

func TestTelegramNotifier_DBOverridesAndRedis(t *testing.T) {
	ctx := context.Background()
	store, db := newTestYunwuStorageWithDB(t)

	var receivedPayload telegramSendMessagePayload
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		assert.Equal(t, "/botdb-token/sendMessage", r.URL.Path)
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		_ = json.Unmarshal(body, &receivedPayload)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer server.Close()

	// Seed DB settings
	now := time.Now()
	require.NoError(t, db.Setting.Create().SetKey(config.SettingKeyTelegramBotToken).SetValue("db-token").SetUpdatedAt(now).Exec(ctx))
	require.NoError(t, db.Setting.Create().SetKey(config.SettingKeyTelegramChatID).SetValue("db-chat-id").SetUpdatedAt(now).Exec(ctx))
	require.NoError(t, db.Setting.Create().SetKey(config.SettingKeyTelegramAlertThreshold).SetValue("35.0").SetUpdatedAt(now).Exec(ctx))
	require.NoError(t, db.Setting.Create().SetKey(config.SettingKeyTelegramAlertsEnabled).SetValue("true").SetUpdatedAt(now).Exec(ctx))

	cfg := &config.HypervisorConfig{
		TelegramBotToken:          "cfg-token",
		TelegramChatID:            "cfg-chat",
		TelegramAlertThresholdUSD: 10.0, // cfg is 10.0, DB is 35.0
		TelegramAlertsEnabled:     false, // cfg is false, DB overrides to true
	}

	notifier := NewTelegramNotifier(store, cfg)
	notifier.SetBaseURL(server.URL)

	// Balance is 25.0 USD (which is > cfg threshold 10.0, but <= DB threshold 35.0)
	notified, err := notifier.CheckAndNotify(ctx, "redis-node-1", "Portdan-Acc", "portdan", 25.0)
	require.NoError(t, err)
	assert.True(t, notified)
	assert.Equal(t, int32(1), atomic.LoadInt32(&requestCount))
	assert.Equal(t, "db-chat-id", receivedPayload.ChatID)
	assert.Contains(t, receivedPayload.Text, "$25.00")
	assert.Contains(t, receivedPayload.Text, "$35.00")
	assert.Contains(t, receivedPayload.Text, "Portdan-Acc")

	// Verify Redis key exists
	redisVal, err := store.Redis().Get(ctx, alertKey("redis-node-1")).Result()
	require.NoError(t, err)
	assert.Equal(t, "1", redisVal)
	assert.True(t, notifier.IsAlerted(ctx, "redis-node-1"))

	// Deduplication via Redis
	notified, err = notifier.CheckAndNotify(ctx, "redis-node-1", "Portdan-Acc", "portdan", 20.0)
	require.NoError(t, err)
	assert.False(t, notified)
	assert.Equal(t, int32(1), atomic.LoadInt32(&requestCount))

	// Replenish above DB threshold ($40.0 > $35.0)
	notified, err = notifier.CheckAndNotify(ctx, "redis-node-1", "Portdan-Acc", "portdan", 40.0)
	require.NoError(t, err)
	assert.False(t, notified)
	assert.False(t, notifier.IsAlerted(ctx, "redis-node-1"))

	// Redis key is deleted
	cnt, err := store.Redis().Exists(ctx, alertKey("redis-node-1")).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), cnt)
}

func TestStatsWorker_NotifierIntegration(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestYunwuStorageWithDB(t)

	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer server.Close()

	cfg := &config.HypervisorConfig{
		TelegramBotToken:          "integ-token",
		TelegramChatID:            "integ-chat",
		TelegramAlertThresholdUSD: 20.0,
		TelegramAlertsEnabled:     true,
	}

	sw := NewStatsWorker(store, cfg)
	sw.Notifier().SetBaseURL(server.URL)

	node := &health.ExecutionNode{
		NodeID:      "integ-node-1",
		AccountName: "IntegAccount",
		Provider:    "openlux",
	}

	grants := &YunwuCreditGrantsResponse{
		TotalAvailable: 15.0,
		TotalUsed:      5.0,
		TotalGranted:   20.0,
	}

	sw.recordSyncedSnapshot(ctx, node, grants, "credit_grants")

	// Allow goroutine to complete
	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&requestCount) == 1
	}, 2*time.Second, 20*time.Millisecond)

	assert.True(t, sw.Notifier().IsAlerted(ctx, "integ-node-1"))
}

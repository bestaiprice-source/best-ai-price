package runtime

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
	_ "modernc.org/sqlite"
)

func newTestStorage(t *testing.T) *storage.Storage {
	t.Helper()

	name := strings.ReplaceAll(t.Name(), "/", "_")
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", name)
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return storage.NewStorage(client, rdb)
}

func TestMockRuntimeSuccessAndFailures(t *testing.T) {
	store := newTestStorage(t)
	mockRt := NewMockRuntime(store)

	var buf bytes.Buffer
	res, err := mockRt.Execute(context.Background(), "node-1", []byte(`{"prompt":"hi"}`), &buf)
	require.NoError(t, err)
	assert.NotNil(t, res)
	assert.Contains(t, buf.String(), "message_start")

	// Force 429 scenario
	mockRt.Force429 = true
	_, err = mockRt.Execute(context.Background(), "node-1", []byte(`{}`), &buf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "429")

	// Force disconnect scenario
	mockRt.Force429 = false
	mockRt.ForceDisconnect = true
	_, err = mockRt.Execute(context.Background(), "node-1", []byte(`{}`), &buf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "closed abruptly")

	// Force timeout scenario
	mockRt.ForceDisconnect = false
	mockRt.ForceTimeout = true
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err = mockRt.Execute(ctx, "node-1", []byte(`{}`), &buf)
	assert.Error(t, err)
}

func TestAnthropicRuntimeExecution(t *testing.T) {
	store := newTestStorage(t)
	ctx := context.Background()

	// Create test account in DB
	acc, err := store.DB().Account.Create().
		SetName("anthropic-acc-1").
		SetPlatform("anthropic").
		SetType("oauth").
		SetStatus("active").
		SetCredentials(map[string]any{"access_token": "sk-ant-oat-test-token"}).
		Save(ctx)
	require.NoError(t, err)

	// Test Anthropic HTTP execution
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer sk-ant-oat-test-token", r.Header.Get("Authorization"))
		assert.Equal(t, "2023-06-01", r.Header.Get("anthropic-version"))

		w.Header().Set("anthropic-ratelimit-unified-5h-utilization", "0.45")
		w.Header().Set("anthropic-ratelimit-unified-7d-utilization", "0.20")
		w.Header().Set("anthropic-ratelimit-unified-5h-reset", "1700000000")
		w.WriteHeader(http.StatusOK)

		_, _ = w.Write([]byte("event: message_delta\ndata: {\"usage\":{\"input_tokens\":100,\"output_tokens\":200}}\n\n"))
	}))
	defer server.Close()

	// Update credentials with base_url pointing to test server
	_, err = store.DB().Account.UpdateOne(acc).
		SetCredentials(map[string]any{"access_token": "sk-ant-oat-test-token", "base_url": server.URL}).
		Save(ctx)
	require.NoError(t, err)

	rt := NewAnthropicRuntime(store)
	var buf bytes.Buffer
	res, err := rt.Execute(ctx, fmt.Sprintf("%d", acc.ID), []byte(`{"stream":true}`), &buf)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, int64(100), res.InputTokens)
	assert.Equal(t, int64(200), res.OutputTokens)

	// Test OpenAI execution format & error status
	errServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer errServer.Close()

	accOpenAI, err := store.DB().Account.Create().
		SetName("openai-acc-1").
		SetPlatform("openai").
		SetType("api_key").
		SetStatus("active").
		SetCredentials(map[string]any{"api_key": "sk-openai-key", "base_url": errServer.URL}).
		Save(ctx)
	require.NoError(t, err)

	_, err = rt.Execute(ctx, fmt.Sprintf("%d", accOpenAI.ID), []byte(`{}`), &bytes.Buffer{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "upstream error status 400")

	// Test OpenAI success execution format
	openAIServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer sk-openai-key", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n"))
	}))
	defer openAIServer.Close()

	_, err = store.DB().Account.UpdateOne(accOpenAI).
		SetCredentials(map[string]any{"api_key": "sk-openai-key", "base_url": openAIServer.URL}).
		Save(ctx)
	require.NoError(t, err)

	resOpenAI, err := rt.Execute(ctx, fmt.Sprintf("%d", accOpenAI.ID), []byte(`{"stream":true}`), &bytes.Buffer{})
	require.NoError(t, err)
	assert.NotNil(t, resOpenAI)
	assert.Equal(t, 200, resOpenAI.StatusCode)

	// Test MockRuntime OpenAI execution branch
	mockRt := NewMockRuntime(store)
	var mockBuf bytes.Buffer
	resMock, err := mockRt.Execute(ctx, fmt.Sprintf("%d", accOpenAI.ID), []byte(`{}`), &mockBuf)
	require.NoError(t, err)
	assert.NotNil(t, resMock)
	assert.Contains(t, mockBuf.String(), "chat.completion.chunk")



	// Invalid nodeID -> error
	_, err = rt.Execute(ctx, "non-existent-node", []byte(`{}`), &bytes.Buffer{})
	assert.Error(t, err)

	// Missing credentials -> error
	accNoKey, err := store.DB().Account.Create().
		SetName("no-key-acc").
		SetPlatform("anthropic").
		SetType("api_key").
		SetStatus("active").
		SetCredentials(map[string]any{}).
		Save(ctx)
	require.NoError(t, err)

	_, err = rt.Execute(ctx, fmt.Sprintf("%d", accNoKey.ID), []byte(`{}`), &bytes.Buffer{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty api_key")
}



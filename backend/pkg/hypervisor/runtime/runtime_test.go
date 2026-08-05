package runtime

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	coderws "github.com/coder/websocket"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/credentials"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
	_ "modernc.org/sqlite"
)

type runtimeTestCipher struct{}

type testLivePeer struct {
	first []byte
	sent  bool
	out   chan []byte
}

func (p *testLivePeer) Read(ctx context.Context) (WSMessageType, []byte, error) {
	if !p.sent {
		p.sent = true
		return WSMessageText, p.first, nil
	}
	<-ctx.Done()
	return WSMessageText, nil, ctx.Err()
}

func (p *testLivePeer) Write(_ context.Context, _ WSMessageType, payload []byte) error {
	p.out <- append([]byte(nil), payload...)
	return nil
}

func (runtimeTestCipher) Encrypt(value string) (string, error) { return "cipher(" + value + ")", nil }
func (runtimeTestCipher) Decrypt(value string) (string, error) {
	return strings.TrimSuffix(strings.TrimPrefix(value, "cipher("), ")"), nil
}

func TestAnthropicRuntimeDecryptsEnvelopeBeforeUpstreamUse(t *testing.T) {
	store := newTestStorage(t)
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer sk-encrypted", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer server.Close()

	acc, err := store.DB().Account.Create().
		SetName("encrypted-runtime-account").
		SetPlatform("openai").
		SetType("api_key").
		SetStatus("active").
		SetCredentials(map[string]any{
			"api_key":  credentials.EnvelopePrefix + "cipher(sk-encrypted)",
			"base_url": server.URL,
		}).
		Save(ctx)
	require.NoError(t, err)

	_, err = NewAnthropicRuntime(store, runtimeTestCipher{}).Execute(ctx, fmt.Sprintf("%d", acc.ID), []byte(`{"model":"gpt-test"}`), &bytes.Buffer{})
	require.NoError(t, err)
	stored, err := store.DB().Account.Get(ctx, acc.ID)
	require.NoError(t, err)
	assert.NotEqual(t, "sk-encrypted", stored.Credentials["api_key"])
}

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

func TestMockRuntimeRequiredAdvancedScenarios(t *testing.T) {
	store := newTestStorage(t)
	ctx := context.Background()

	for _, platform := range []string{"anthropic", "openai", "yunwu"} {
		t.Run(platform, func(t *testing.T) {
			acc, err := store.DB().Account.Create().
				SetName("mock-" + platform).
				SetPlatform(platform).
				SetType("api_key").
				SetStatus("active").
				SetCredentials(map[string]any{"api_key": "test"}).
				Save(ctx)
			require.NoError(t, err)
			nodeID := fmt.Sprintf("%d", acc.ID)

			mock := NewMockRuntime(store)
			mock.ForceToolUse = true
			var toolBuf bytes.Buffer
			_, err = mock.Execute(ctx, nodeID, nil, &toolBuf)
			require.NoError(t, err)
			assert.Contains(t, toolBuf.String(), "mock_tool")

			mock.ForceToolUse = false
			mock.ForceBrokenSSE = true
			_, err = mock.Execute(ctx, nodeID, nil, &bytes.Buffer{})
			require.ErrorContains(t, err, "broken SSE")

			mock.ForceBrokenSSE = false
			mock.ForceInvalidResponse = true
			_, err = mock.Execute(ctx, nodeID, nil, &bytes.Buffer{})
			require.ErrorContains(t, err, "invalid upstream response")
		})
	}
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

		body, _ := io.ReadAll(r.Body)
		assert.Contains(t, string(body), `"max_tokens":4096`)

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

func TestResolveUpstreamAPIURL(t *testing.T) {
	assert.Equal(t, "https://api.anthropic.com/v1/messages", resolveUpstreamAPIURL("", "anthropic", false))
	assert.Equal(t, "https://api.openai.com/v1/chat/completions", resolveUpstreamAPIURL("", "openai", true))

	assert.Equal(t, "https://yunwu.ai/v1/messages", resolveUpstreamAPIURL("https://yunwu.ai/v1", "yunwu", false))
	assert.Equal(t, "https://yunwu.ai/v1/messages", resolveUpstreamAPIURL("https://yunwu.ai", "yunwu", false))
	assert.Equal(t, "https://yunwu.ai/v1/chat/completions", resolveUpstreamAPIURL("https://yunwu.ai/v1", "openai", true))
	assert.Equal(t, "https://yunwu.ai/v1/messages", resolveUpstreamAPIURL("https://yunwu.ai/v1/messages", "yunwu", false))
}

func TestYunwuRuntimeExecution(t *testing.T) {
	store := newTestStorage(t)
	ctx := context.Background()

	yunwuServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer sk-yunwu-key", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer yunwuServer.Close()

	accYunwu, err := store.DB().Account.Create().
		SetName("yunwu-acc-1").
		SetPlatform("yunwu").
		SetType("api_key").
		SetStatus("active").
		SetCredentials(map[string]any{"api_key": "sk-yunwu-key", "base_url": yunwuServer.URL}).
		Save(ctx)
	require.NoError(t, err)

	rt := NewAnthropicRuntime(store)

	// Test GPT model request on Yunwu node
	var buf bytes.Buffer
	res, err := rt.Execute(ctx, fmt.Sprintf("%d", accYunwu.ID), []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`), &buf)
	require.NoError(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, 200, res.StatusCode)

	// Test streaming responses API transformation
	streamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\ndata: [DONE]\n\n"))
	}))
	defer streamServer.Close()

	accStream, err := store.DB().Account.Create().
		SetName("yunwu-stream-acc").
		SetPlatform("yunwu").
		SetType("api_key").
		SetStatus("active").
		SetCredentials(map[string]any{"api_key": "sk-yunwu-key", "base_url": streamServer.URL}).
		Save(ctx)
	require.NoError(t, err)

	var streamBuf bytes.Buffer
	resStream, err := rt.Execute(ctx, fmt.Sprintf("%d", accStream.ID), []byte(`{"model":"gpt-4o","input":"hi","stream":true}`), &streamBuf)
	require.NoError(t, err)
	assert.NotNil(t, resStream)
	assert.Contains(t, streamBuf.String(), "response.output_text.delta")
	assert.Contains(t, streamBuf.String(), "response.completed")
}

type headerBuffer struct {
	bytes.Buffer
	header http.Header
	status int
}

func (w *headerBuffer) Header() http.Header    { return w.header }
func (w *headerBuffer) WriteHeader(status int) { w.status = status }

func TestYunwuGeminiUsesOpenAIProtocolAndPreservesMediaResponse(t *testing.T) {
	store := newTestStorage(t)
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Equal(t, "Bearer sk-yunwu-gemini", r.Header.Get("Authorization"))
		var request map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		messages := request["messages"].([]any)
		content := messages[0].(map[string]any)["content"].([]any)
		assert.Equal(t, "image_url", content[1].(map[string]any)["type"])
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Disposition", `inline; filename="gemini.png"`)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("PNG"))
	}))
	defer server.Close()

	account, err := store.DB().Account.Create().
		SetName("yunwu-gemini").SetPlatform("yunwu").SetType("api_key").SetStatus("active").
		SetCredentials(map[string]any{"api_key": "sk-yunwu-gemini", "base_url": server.URL}).Save(ctx)
	require.NoError(t, err)

	writer := &headerBuffer{header: make(http.Header)}
	payload := []byte(`{"model":"gemini-2.5-flash-image","messages":[{"role":"user","content":[{"type":"text","text":"draw"},{"type":"image_url","image_url":{"url":"data:image/png;base64,AA=="}}]}]}`)
	metadata, err := NewAnthropicRuntime(store).Execute(ctx, fmt.Sprint(account.ID), payload, writer)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, metadata.StatusCode)
	assert.Equal(t, http.StatusCreated, writer.status)
	assert.Equal(t, "image/png", writer.header.Get("Content-Type"))
	assert.Equal(t, `inline; filename="gemini.png"`, writer.header.Get("Content-Disposition"))
	assert.Equal(t, "PNG", writer.String())
}

func TestYunwuExplicitGeminiProtocolUsesOpenAIEndpoint(t *testing.T) {
	store := newTestStorage(t)
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer server.Close()
	account, err := store.DB().Account.Create().
		SetName("yunwu-explicit-gemini").SetPlatform("yunwu").SetType("api_key").SetStatus("active").
		SetExtra(map[string]any{"yunwu_protocol": "gemini"}).
		SetCredentials(map[string]any{"api_key": "sk-test", "base_url": server.URL}).Save(ctx)
	require.NoError(t, err)
	_, err = NewAnthropicRuntime(store).Execute(ctx, fmt.Sprint(account.ID), []byte(`{"model":"custom-model","messages":[{"role":"user","content":"hi"}]}`), io.Discard)
	require.NoError(t, err)
}

func TestYunwuGLMAndDeepSeekUseOpenAIEndpoint(t *testing.T) {
	for _, model := range []string{"glm-4.5", "deepseek-r1"} {
		t.Run(model, func(t *testing.T) {
			store := newTestStorage(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/v1/chat/completions", r.URL.Path)
				assert.Equal(t, "Bearer sk-test", r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
			}))
			defer server.Close()
			account, err := store.DB().Account.Create().
				SetName("yunwu-" + model).SetPlatform("yunwu").SetType("api_key").SetStatus("active").
				SetCredentials(map[string]any{"api_key": "sk-test", "base_url": server.URL}).Save(context.Background())
			require.NoError(t, err)
			_, err = NewAnthropicRuntime(store).Execute(context.Background(), fmt.Sprint(account.ID), []byte(`{"model":"`+model+`","messages":[{"role":"user","content":"hi"}]}`), io.Discard)
			require.NoError(t, err)
		})
	}
}

func TestYunwuNativeGeminiEndpointPreservesInlineMedia(t *testing.T) {
	store := newTestStorage(t)
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1beta/models/gemini-2.5-flash:generateContent", r.URL.Path)
		assert.Equal(t, "Bearer sk-yunwu-native", r.Header.Get("Authorization"))
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), `"inlineData"`)
		assert.Contains(t, string(body), `"audio/wav"`)
		assert.NotContains(t, string(body), `"messages"`)
		assert.NotContains(t, string(body), `"max_tokens"`)
		assert.Equal(t, "resumable", r.Header.Get("X-Goog-Upload-Protocol"))
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("X-Goog-Upload-URL", "https://upload.example/session")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"UE5H"}}]}}]}`))
	}))
	defer server.Close()

	account, err := store.DB().Account.Create().
		SetName("yunwu-native-gemini").SetPlatform("yunwu").SetType("api_key").SetStatus("active").
		SetCredentials(map[string]any{"api_key": "sk-yunwu-native", "base_url": server.URL + "/v1"}).Save(ctx)
	require.NoError(t, err)

	payload := []byte(`{"contents":[{"role":"user","parts":[{"text":"describe"},{"inlineData":{"mimeType":"audio/wav","data":"UklGRg=="}}]}]}`)
	ctx = WithUpstreamRequest(ctx, http.MethodPost, "/v1beta/models/gemini-2.5-flash:generateContent", "application/json")
	ctx = WithUpstreamRequestHeaders(ctx, http.Header{"X-Goog-Upload-Protocol": []string{"resumable"}, "Authorization": []string{"client-secret-must-not-pass"}})
	writer := &headerBuffer{header: make(http.Header)}
	metadata, err := NewAnthropicRuntime(store).Execute(ctx, fmt.Sprint(account.ID), payload, writer)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, metadata.StatusCode)
	assert.Equal(t, "https://upload.example/session", writer.header.Get("X-Goog-Upload-URL"))
	assert.Equal(t, "application/json; charset=utf-8", writer.header.Get("Content-Type"))
	assert.Contains(t, writer.String(), `"mimeType":"image/png"`)
}

func TestYunwuUpstreamOverrideRejectsAbsoluteURL(t *testing.T) {
	store := newTestStorage(t)
	ctx := context.Background()
	account, err := store.DB().Account.Create().
		SetName("yunwu-safe-path").SetPlatform("yunwu").SetType("api_key").SetStatus("active").
		SetCredentials(map[string]any{"api_key": "sk-test", "base_url": "https://yunwu.ai/v1"}).Save(ctx)
	require.NoError(t, err)
	ctx = WithUpstreamRequest(ctx, http.MethodPost, "https://attacker.invalid/steal", "application/json")
	_, err = NewAnthropicRuntime(store).Execute(ctx, fmt.Sprint(account.ID), []byte(`{}`), &bytes.Buffer{})
	require.ErrorContains(t, err, "invalid upstream relative path")
}

func TestYunwuGeminiLiveWebSocketRelay(t *testing.T) {
	store := newTestStorage(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/ws/google.ai.generativelanguage.v1beta.GenerativeService.BidiGenerateContent", r.URL.Path)
		assert.Equal(t, "Bearer sk-live", r.Header.Get("Authorization"))
		conn, err := coderws.Accept(w, r, nil)
		require.NoError(t, err)
		defer conn.Close(coderws.StatusNormalClosure, "done")
		_, setup, err := conn.Read(r.Context())
		require.NoError(t, err)
		assert.Contains(t, string(setup), `"setup"`)
		require.NoError(t, conn.Write(r.Context(), coderws.MessageText, []byte(`{"serverContent":{"modelTurn":{"parts":[{"inlineData":{"mimeType":"audio/pcm","data":"AAAA"}}]}}}`)))
	}))
	defer server.Close()
	account, err := store.DB().Account.Create().
		SetName("yunwu-live").SetPlatform("yunwu").SetType("api_key").SetStatus("active").
		SetCredentials(map[string]any{"api_key": "sk-live", "base_url": server.URL}).Save(context.Background())
	require.NoError(t, err)
	peer := &testLivePeer{first: []byte(`{"setup":{"model":"models/gemini-live"}}`), out: make(chan []byte, 1)}
	err = NewAnthropicRuntime(store).ProxyWebSocket(context.Background(), fmt.Sprint(account.ID), "/ws/google.ai.generativelanguage.v1beta.GenerativeService.BidiGenerateContent", nil, peer)
	require.NoError(t, err)
	select {
	case response := <-peer.out:
		assert.Contains(t, string(response), `"audio/pcm"`)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Gemini Live response")
	}
}

func TestYunwuConstrainedLiveUsesOnlyEphemeralCredential(t *testing.T) {
	store := newTestStorage(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "auth_tokens/short-lived", r.URL.Query().Get("access_token"))
		assert.Empty(t, r.Header.Get("Authorization"))
		conn, err := coderws.Accept(w, r, nil)
		require.NoError(t, err)
		_, _, err = conn.Read(r.Context())
		require.NoError(t, err)
		_ = conn.Close(coderws.StatusNormalClosure, "done")
	}))
	defer server.Close()
	account, err := store.DB().Account.Create().
		SetName("yunwu-constrained-live").SetPlatform("yunwu").SetType("api_key").SetStatus("active").
		SetCredentials(map[string]any{"api_key": "long-lived-must-not-pass", "base_url": server.URL}).Save(context.Background())
	require.NoError(t, err)
	err = NewAnthropicRuntime(store).ProxyWebSocket(context.Background(), fmt.Sprint(account.ID), "/ws/google.ai.generativelanguage.v1beta.GenerativeService.BidiGenerateContentConstrained?access_token=auth_tokens%2Fshort-lived", nil, &testLivePeer{first: []byte(`{"setup":{}}`)})
	require.NoError(t, err)
}

func TestYunwuGeminiLiveQueryCredentialIsRedactedFromDialError(t *testing.T) {
	store := newTestStorage(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "no websocket", http.StatusBadGateway) }))
	server.Close()
	account, err := store.DB().Account.Create().
		SetName("yunwu-live-query").SetPlatform("yunwu").SetType("api_key").SetStatus("active").
		SetExtra(map[string]any{"yunwu_live_auth": "query"}).
		SetCredentials(map[string]any{"api_key": "secret-live-key", "base_url": server.URL}).Save(context.Background())
	require.NoError(t, err)
	err = NewAnthropicRuntime(store).ProxyWebSocket(context.Background(), fmt.Sprint(account.ID), "/ws/live", nil, &testLivePeer{})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "secret-live-key")
	assert.NotContains(t, err.Error(), "?key=")
}

func TestResponsesSSETransformerDirect(t *testing.T) {
	var buf bytes.Buffer
	tr := newResponsesSSETransformer(&buf)

	input := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hello world\"}}]}\n\ndata: [DONE]\n\n")
	_, err := tr.Write(input)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "response.output_text.delta")
	assert.Contains(t, out, "Hello world")
	assert.Contains(t, out, "response.completed")
}

func TestConstructMessagesFromResponsesPayload(t *testing.T) {
	payload := map[string]any{
		"instructions": "Be concise.",
		"input": []any{
			map[string]any{
				"type": "message",
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_text", "text": "Что такое телеграм?"},
				},
			},
		},
	}
	msgs := constructMessagesFromResponsesPayload(payload)
	require.Len(t, msgs, 2)
	assert.Equal(t, "system", msgs[0]["role"])
	assert.Equal(t, "Be concise.", msgs[0]["content"])
	assert.Equal(t, "user", msgs[1]["role"])
	assert.Equal(t, "Что такое телеграм?", msgs[1]["content"])
}

func TestSafeResponseBodyRedactsCredentialsAndCapsLength(t *testing.T) {
	body := []byte(`{"error":{"message":"bad key","api_key":"secret-value","nested":{"authorization":"Bearer secret"}}}`)
	safe := SafeResponseBody(body)
	assert.Contains(t, safe, `"api_key":"[redacted]"`)
	assert.Contains(t, safe, `"authorization":"[redacted]"`)
	assert.NotContains(t, safe, "secret-value")
	assert.NotContains(t, safe, "Bearer secret")

	assert.Len(t, SafeResponseBody(bytes.Repeat([]byte("x"), 9*1024)), 8*1024)
}

func TestExtractTokensSupportsGeminiAndInteractionsUsage(t *testing.T) {
	input, output := extractTokens([]byte(`{"usageMetadata":{"promptTokenCount":258,"candidatesTokenCount":20,"thoughtsTokenCount":7}}`))
	assert.Equal(t, int64(258), input)
	assert.Equal(t, int64(27), output)
	input, output = extractTokens([]byte(`{"usage":{"total_input_tokens":50,"total_output_tokens":10,"total_thought_tokens":22}}`))
	assert.Equal(t, int64(50), input)
	assert.Equal(t, int64(32), output)
}

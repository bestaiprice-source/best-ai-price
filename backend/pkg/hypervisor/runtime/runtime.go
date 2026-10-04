package runtime

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	dbaccount "github.com/Wei-Shaw/sub2api/ent/account"
	"github.com/Wei-Shaw/sub2api/internal/credentials"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
	coderws "github.com/coder/websocket"
	"github.com/google/uuid"
)

type ResponseMetadata struct {
	InputTokens  int64
	OutputTokens int64
	StatusCode   int
}

type Runtime interface {
	Execute(ctx context.Context, nodeID string, reqPayload []byte, stream io.Writer) (*ResponseMetadata, error)
}

type WSMessageType int

const (
	WSMessageText WSMessageType = iota + 1
	WSMessageBinary
)

type WebSocketPeer interface {
	Read(context.Context) (WSMessageType, []byte, error)
	Write(context.Context, WSMessageType, []byte) error
}

type WebSocketRuntime interface {
	ProxyWebSocket(context.Context, string, string, http.Header, WebSocketPeer) error
}

type upstreamRequestOverride struct {
	Method      string
	Path        string
	ContentType string
	Headers     http.Header
}

// UpstreamRequest describes the vendor-relative request selected by a gateway.
// It intentionally contains no origin or credentials.
type UpstreamRequest struct {
	Method      string
	Path        string
	ContentType string
	Headers     http.Header
}

type upstreamRequestOverrideKey struct{}

// WithUpstreamRequest selects a vendor-relative endpoint for protocol-specific
// APIs such as Gemini generateContent and asynchronous media operations.
// Only absolute-path URLs are accepted by Execute; origins always come from the
// selected account and therefore cannot be supplied by clients.
func WithUpstreamRequest(ctx context.Context, method, path, contentType string) context.Context {
	return context.WithValue(ctx, upstreamRequestOverrideKey{}, upstreamRequestOverride{Method: method, Path: path, ContentType: contentType})
}

// WithUpstreamRequestHeaders adds protocol headers required by resumable media
// APIs. Authentication headers are deliberately ignored by Execute.
func WithUpstreamRequestHeaders(ctx context.Context, headers http.Header) context.Context {
	override, ok := ctx.Value(upstreamRequestOverrideKey{}).(upstreamRequestOverride)
	if !ok {
		return ctx
	}
	override.Headers = headers.Clone()
	return context.WithValue(ctx, upstreamRequestOverrideKey{}, override)
}

func UpstreamRequestFromContext(ctx context.Context) (UpstreamRequest, bool) {
	override, ok := ctx.Value(upstreamRequestOverrideKey{}).(upstreamRequestOverride)
	if !ok {
		return UpstreamRequest{}, false
	}
	return UpstreamRequest{Method: override.Method, Path: override.Path, ContentType: override.ContentType, Headers: override.Headers.Clone()}, true
}

// MockRuntime implements Runtime for simulating upstream responses and testing failures
type MockRuntime struct {
	store                *storage.Storage
	SuccessDelay         time.Duration
	SlowStreamDelay      time.Duration
	Force429             bool
	ForceTimeout         bool
	ForceDisconnect      bool
	ForceBrokenSSE       bool
	ForceToolUse         bool
	ForceInvalidResponse bool
	CustomInputTokens    int64
	CustomOutputTokens   int64
}

func NewMockRuntime(store *storage.Storage) *MockRuntime {
	return &MockRuntime{store: store}
}

func (m *MockRuntime) Execute(ctx context.Context, nodeID string, reqPayload []byte, stream io.Writer) (*ResponseMetadata, error) {
	if m.Force429 {
		return nil, errors.New("upstream error 429 Too Many Requests")
	}

	if m.ForceTimeout {
		select {
		case <-time.After(10 * time.Second):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if m.ForceDisconnect {
		_, _ = stream.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_mock\",\"role\":\"assistant\"}}\n\n"))
		return nil, errors.New("upstream connection closed abruptly")
	}

	if m.ForceInvalidResponse {
		_, _ = stream.Write([]byte("this is not a valid upstream response"))
		return nil, errors.New("invalid upstream response")
	}

	if m.SuccessDelay > 0 {
		time.Sleep(m.SuccessDelay)
	}

	platform := "anthropic"
	if m.store != nil {
		var acc *ent.Account
		var err error
		dbClient := m.store.DB()
		if id, parseErr := strconv.ParseInt(nodeID, 10, 64); parseErr == nil {
			acc, err = dbClient.Account.Get(ctx, id)
		}
		if err != nil || acc == nil {
			acc, err = dbClient.Account.Query().Where(dbaccount.Name(nodeID)).Only(ctx)
		}
		if err == nil && acc != nil {
			platform = acc.Platform
		}
	}

	if platform == "openai" {
		if m.ForceBrokenSSE {
			_, _ = stream.Write([]byte("data: {\"choices\":[{\"delta\":\n\n"))
			return nil, errors.New("broken SSE stream")
		}
		if m.ForceToolUse {
			events := []string{
				"data: {\"id\":\"chatcmpl-mock\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_mock\",\"type\":\"function\",\"function\":{\"name\":\"mock_tool\",\"arguments\":\"{}\"}}]},\"finish_reason\":null}]}\n\n",
				"data: {\"id\":\"chatcmpl-mock\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n",
				"data: [DONE]\n\n",
			}
			return m.writeMockEvents(ctx, stream, events)
		}
		events := []string{
			"data: {\"id\":\"chatcmpl-mock\",\"object\":\"chat.completion.chunk\",\"created\":1677858342,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"\"},\"finish_reason\":null}]}\n\n",
			"data: {\"id\":\"chatcmpl-mock\",\"object\":\"chat.completion.chunk\",\"created\":1677858342,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello \"},\"finish_reason\":null}]}\n\n",
			"data: {\"id\":\"chatcmpl-mock\",\"object\":\"chat.completion.chunk\",\"created\":1677858342,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"from \"},\"finish_reason\":null}]}\n\n",
			"data: {\"id\":\"chatcmpl-mock\",\"object\":\"chat.completion.chunk\",\"created\":1677858342,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"OpenAI Mock!\"},\"finish_reason\":null}]}\n\n",
			"data: {\"id\":\"chatcmpl-mock\",\"object\":\"chat.completion.chunk\",\"created\":1677858342,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n",
			"data: [DONE]\n\n",
		}
		return m.writeMockEvents(ctx, stream, events)
	}

	if m.ForceBrokenSSE {
		_, _ = stream.Write([]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\"\n\n"))
		return nil, errors.New("broken SSE stream")
	}
	if m.ForceToolUse {
		events := []string{
			"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_mock_tool\",\"role\":\"assistant\",\"content\":[]}}\n\n",
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_mock\",\"name\":\"mock_tool\",\"input\":{}}}\n\n",
			"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n",
			"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"},\"usage\":{\"input_tokens\":15,\"output_tokens\":10}}\n\n",
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
		}
		return m.writeMockEvents(ctx, stream, events)
	}

	inTokens := int64(15)
	if m.CustomInputTokens > 0 {
		inTokens = m.CustomInputTokens
	}
	outTokens := int64(10)
	if m.CustomOutputTokens > 0 {
		outTokens = m.CustomOutputTokens
	}

	// Simulating standard Anthropic SSE stream
	events := []string{
		"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_mock\",\"role\":\"assistant\",\"content\":[]}}\n\n",
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello \"}}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"from \"}}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Mock!\"}}\n\n",
		"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n",
		fmt.Sprintf("event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"input_tokens\":%d,\"output_tokens\":%d}}\n\n", inTokens, outTokens),
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
	}

	return m.writeMockEvents(ctx, stream, events)
}

func (m *MockRuntime) writeMockEvents(ctx context.Context, stream io.Writer, events []string) (*ResponseMetadata, error) {
	for _, ev := range events {
		if m.SlowStreamDelay > 0 {
			timer := time.NewTimer(m.SlowStreamDelay)
			select {
			case <-timer.C:
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return nil, ctx.Err()
			}
		}
		_, err := stream.Write([]byte(ev))
		if err != nil {
			return nil, err
		}
		if flusher, ok := stream.(http.Flusher); ok {
			flusher.Flush()
		}
	}

	return &ResponseMetadata{
		InputTokens:  15,
		OutputTokens: 10,
		StatusCode:   200,
	}, nil
}

// AnthropicRuntime implements the real upstream communication with Claude API
type AnthropicRuntime struct {
	store     *storage.Storage
	decryptor credentials.Cipher
}

func NewAnthropicRuntime(store *storage.Storage, decryptors ...credentials.Cipher) *AnthropicRuntime {
	var decryptor credentials.Cipher
	if len(decryptors) > 0 {
		decryptor = decryptors[0]
	}
	return &AnthropicRuntime{store: store, decryptor: decryptor}
}

// ProxyWebSocket transparently relays Gemini Live frames. The upstream origin
// and credentials are always resolved from the selected Yunwu account.
func (r *AnthropicRuntime) ProxyWebSocket(ctx context.Context, nodeID, path string, headers http.Header, downstream WebSocketPeer) error {
	if path == "" || !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.Contains(path, "://") {
		return fmt.Errorf("invalid upstream websocket path")
	}
	id, err := strconv.ParseInt(nodeID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid Yunwu node ID: %w", err)
	}
	acc, err := r.store.DB().Account.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("resolve Yunwu websocket account: %w", err)
	}
	creds, err := credentials.DecryptMap(acc.Credentials, r.decryptor)
	if err != nil {
		return fmt.Errorf("decrypt Yunwu websocket credentials: %w", err)
	}
	apiKey := ""
	for _, key := range []string{"access_token", "api_key", "token", "key"} {
		if value, ok := creds[key].(string); ok && strings.TrimSpace(value) != "" {
			apiKey = strings.TrimSpace(value)
			break
		}
	}
	if apiKey == "" {
		return fmt.Errorf("empty api_key credentials for node %s", nodeID)
	}
	baseURL, _ := creds["base_url"].(string)
	origin, err := upstreamOrigin(baseURL)
	if err != nil {
		return err
	}
	u, err := url.Parse(origin + path)
	if err != nil {
		return err
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	dialHeaders := make(http.Header)
	liveAuth, _ := acc.Extra["yunwu_live_auth"].(string)
	// Constrained Live sessions authenticate with the short-lived token in the
	// URL. Never add the long-lived node credential to those requests.
	if u.Query().Get("access_token") != "" {
		// Ephemeral token already present.
	} else if strings.EqualFold(strings.TrimSpace(liveAuth), "query") {
		query := u.Query()
		query.Set("key", apiKey)
		u.RawQuery = query.Encode()
	} else {
		dialHeaders.Set("Authorization", "Bearer "+apiKey)
	}
	var subprotocols []string
	for _, protocol := range strings.Split(headers.Get("Sec-WebSocket-Protocol"), ",") {
		if protocol = strings.TrimSpace(protocol); protocol != "" {
			subprotocols = append(subprotocols, protocol)
		}
	}
	upstream, _, err := coderws.Dial(ctx, u.String(), &coderws.DialOptions{HTTPHeader: dialHeaders, Subprotocols: subprotocols, CompressionMode: coderws.CompressionContextTakeover})
	if err != nil {
		// Dial errors can contain the URL; never expose query-auth credentials.
		return fmt.Errorf("dial Yunwu Gemini Live websocket failed")
	}
	defer upstream.CloseNow()
	upstream.SetReadLimit(64 << 20)

	relayCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	errs := make(chan error, 2)
	go func() {
		for {
			msgType, payload, readErr := downstream.Read(relayCtx)
			if readErr != nil {
				errs <- readErr
				return
			}
			coderType := coderws.MessageText
			if msgType == WSMessageBinary {
				coderType = coderws.MessageBinary
			}
			if writeErr := upstream.Write(relayCtx, coderType, payload); writeErr != nil {
				errs <- writeErr
				return
			}
		}
	}()
	go func() {
		for {
			msgType, payload, readErr := upstream.Read(relayCtx)
			if readErr != nil {
				errs <- readErr
				return
			}
			downType := WSMessageText
			if msgType == coderws.MessageBinary {
				downType = WSMessageBinary
			}
			if writeErr := downstream.Write(relayCtx, downType, payload); writeErr != nil {
				errs <- writeErr
				return
			}
		}
	}()
	err = <-errs
	cancel()
	if coderws.CloseStatus(err) == coderws.StatusNormalClosure || coderws.CloseStatus(err) == coderws.StatusGoingAway || errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func (r *AnthropicRuntime) Execute(ctx context.Context, nodeID string, reqPayload []byte, stream io.Writer) (*ResponseMetadata, error) {
	dbClient := r.store.DB()

	// 1. Resolve Account from database using nodeID (either ID or Name)
	var acc *ent.Account
	var err error

	if id, parseErr := strconv.ParseInt(nodeID, 10, 64); parseErr == nil {
		acc, err = dbClient.Account.Get(ctx, id)
	}
	if err != nil || acc == nil {
		acc, err = dbClient.Account.Query().
			Where(dbaccount.Name(nodeID)).
			Only(ctx)
		if err != nil {
			return nil, fmt.Errorf("resolve account for node %s failed: %w", nodeID, err)
		}
	}

	accountCredentials, err := credentials.DecryptMap(acc.Credentials, r.decryptor)
	if err != nil {
		return nil, fmt.Errorf("decrypt credentials for node %s: %w", nodeID, err)
	}
	if r.decryptor != nil && credentials.NeedsEncryption(acc.Credentials) {
		encrypted, encryptErr := credentials.EncryptMap(acc.Credentials, r.decryptor)
		if encryptErr != nil {
			return nil, fmt.Errorf("migrate credentials for node %s: %w", nodeID, encryptErr)
		}
		if _, updateErr := dbClient.Account.UpdateOneID(acc.ID).SetCredentials(encrypted).Save(ctx); updateErr != nil {
			return nil, fmt.Errorf("persist encrypted credentials for node %s: %w", nodeID, updateErr)
		}
	}

	// Extract API Key / Token from Credentials.  API-key accounts occasionally
	// retain an old access_token after being edited in the admin UI; that token
	// must never shadow the configured api_key.  OAuth and setup-token accounts
	// intentionally keep access_token first.
	var apiKey string
	keysToTry := []string{"api_key", "key", "token", "access_token", "setup_token", "session_key"}
	if acc.Type == "oauth" || acc.Type == "setup-token" {
		keysToTry = []string{"access_token", "setup_token", "api_key", "key", "token", "session_key"}
	}
	for _, k := range keysToTry {
		if val, ok := accountCredentials[k].(string); ok && strings.TrimSpace(val) != "" {
			apiKey = strings.TrimSpace(val)
			break
		}
	}
	if apiKey == "" {
		return nil, fmt.Errorf("empty api_key credentials for node %s", nodeID)
	}

	// 3. Resolve Proxy if configured
	var proxyFunc func(*http.Request) (*url.URL, error)
	if acc.ProxyID != nil && *acc.ProxyID > 0 {
		prx, err := dbClient.Proxy.Get(ctx, *acc.ProxyID)
		if err == nil && prx != nil && prx.Status == "active" {
			var proxyStr string
			if prx.Username != nil && prx.Password != nil && *prx.Username != "" {
				proxyStr = fmt.Sprintf("%s://%s:%s@%s:%d", prx.Protocol, *prx.Username, *prx.Password, prx.Host, prx.Port)
			} else {
				proxyStr = fmt.Sprintf("%s://%s:%d", prx.Protocol, prx.Host, prx.Port)
			}
			proxyURL, parseURLErr := url.Parse(proxyStr)
			if parseURLErr == nil {
				proxyFunc = http.ProxyURL(proxyURL)
			}
		}
	}

	// 4. Create HTTP Request
	customURL, _ := accountCredentials["base_url"].(string)
	if customURL == "" && acc.Platform == "yunwu" {
		customURL = defaultBaseURLForYunwu(acc.Extra)
	}

	isChatCompletions := false
	if acc.Platform == "openai" {
		isChatCompletions = true
	} else if acc.Platform == "yunwu" {
		var bodyMap map[string]any
		if json.Unmarshal(reqPayload, &bodyMap) == nil {
			modelStr, _ := bodyMap["model"].(string)
			modelLower := strings.ToLower(strings.TrimSpace(modelStr))
			if idx := strings.Index(modelLower, "/"); idx != -1 {
				modelLower = modelLower[idx+1:]
			}
			protocol, _ := acc.Extra["yunwu_protocol"].(string)
			protocol = strings.ToLower(strings.TrimSpace(protocol))
			if protocol == "openai" || protocol == "responses" || protocol == "gemini" ||
				(protocol == "" && (strings.HasPrefix(modelLower, "gemini-") ||
					strings.HasPrefix(modelLower, "imagen-") ||
					strings.HasPrefix(modelLower, "veo-") ||
					strings.HasPrefix(modelLower, "lyria-") ||
					strings.HasPrefix(modelLower, "chirp-") ||
					strings.HasPrefix(modelLower, "glm-") ||
					strings.HasPrefix(modelLower, "deepseek-") ||
					strings.HasPrefix(modelLower, "qwen-") ||
					strings.HasPrefix(modelLower, "gpt-") ||
					strings.HasPrefix(modelLower, "o1-") ||
					strings.HasPrefix(modelLower, "o3-") ||
					strings.HasPrefix(modelLower, "text-") ||
					bodyMap["input"] != nil ||
					bodyMap["instructions"] != nil)) {
				isChatCompletions = true
			}
		}
	}

	apiURL := resolveUpstreamAPIURL(customURL, acc.Platform, isChatCompletions)
	requestMethod := http.MethodPost
	requestContentType := "application/json"
	_, hasUpstreamOverride := ctx.Value(upstreamRequestOverrideKey{}).(upstreamRequestOverride)
	if override, ok := ctx.Value(upstreamRequestOverrideKey{}).(upstreamRequestOverride); ok {
		if override.Path == "" || !strings.HasPrefix(override.Path, "/") || strings.HasPrefix(override.Path, "//") || strings.Contains(override.Path, "://") {
			return nil, fmt.Errorf("invalid upstream relative path")
		}
		method := strings.ToUpper(strings.TrimSpace(override.Method))
		if method != http.MethodGet && method != http.MethodPost && method != http.MethodDelete {
			return nil, fmt.Errorf("unsupported upstream method %q", override.Method)
		}
		origin, originErr := upstreamOrigin(customURL)
		if originErr != nil {
			return nil, originErr
		}
		apiURL = origin + override.Path
		requestMethod = method
		if strings.TrimSpace(override.ContentType) != "" {
			requestContentType = override.ContentType
		}
	}

	isOAuth := acc.Type == "oauth"
	isOAuthOrSetupToken := acc.Type == "oauth" || acc.Type == "setup-token" || strings.HasPrefix(apiKey, "sk-ant-oat") || strings.HasPrefix(apiKey, "sk-oat")
	if isOAuth && acc.Platform == "openai" && customURL == "" {
		apiURL = "https://chatgpt.com/backend-api/codex/responses"
	} else if acc.Platform == "anthropic" && isOAuthOrSetupToken {
		if !strings.Contains(apiURL, "beta=true") {
			if strings.Contains(apiURL, "?") {
				apiURL += "&beta=true"
			} else {
				apiURL += "?beta=true"
			}
		}
	}

	var payloadMap struct {
		Stream bool `json:"stream"`
	}
	isStream := false
	if json.Unmarshal(reqPayload, &payloadMap) == nil {
		isStream = payloadMap.Stream
	}

	var bodyMap map[string]any
	// Explicit endpoint overrides are transparent protocol tunnels. Applying
	// chat-schema conversion here would corrupt native Gemini and media bodies.
	if !hasUpstreamOverride && json.Unmarshal(reqPayload, &bodyMap) == nil {
		modified := false

		targetIsResponsesAPI := strings.Contains(apiURL, "/responses")

		if targetIsResponsesAPI {
			if _, hasMsgs := bodyMap["messages"]; hasMsgs {
				if _, hasInput := bodyMap["input"]; !hasInput {
					bodyMap["input"] = constructInputFromMessagesPayload(bodyMap)
				}
				delete(bodyMap, "messages")
				modified = true
			}
			if isOAuth {
				bodyMap["store"] = false
				modified = true
			}
		} else {
			msgs, hasMsgs := bodyMap["messages"]
			if !hasMsgs || msgs == nil {
				bodyMap["messages"] = constructMessagesFromResponsesPayload(bodyMap)
				modified = true
			} else if msgArr, ok := msgs.([]any); ok && len(msgArr) == 0 {
				bodyMap["messages"] = constructMessagesFromResponsesPayload(bodyMap)
				modified = true
			}
		}

		if !isChatCompletions || acc.Platform == "anthropic" || strings.HasSuffix(apiURL, "/messages") {
			val, hasMax := bodyMap["max_tokens"]
			if !hasMax || val == nil {
				if mct, ok := bodyMap["max_completion_tokens"]; ok && mct != nil {
					bodyMap["max_tokens"] = mct
					delete(bodyMap, "max_completion_tokens")
					modified = true
				} else {
					bodyMap["max_tokens"] = 4096
					modified = true
				}
			} else if num, ok := val.(float64); ok && num <= 0 {
				bodyMap["max_tokens"] = 4096
				modified = true
			}

			// Translate tool_choice from string or OpenAI object to Anthropic object
			if tc, ok := bodyMap["tool_choice"]; ok && tc != nil {
				if tcStr, ok := tc.(string); ok {
					if tcStr == "auto" {
						bodyMap["tool_choice"] = map[string]any{"type": "auto"}
						modified = true
					} else if tcStr == "required" {
						bodyMap["tool_choice"] = map[string]any{"type": "any"}
						modified = true
					} else if tcStr == "none" {
						delete(bodyMap, "tool_choice")
						modified = true
					}
				} else if tcMap, ok := tc.(map[string]any); ok {
					if typ, ok := tcMap["type"].(string); ok && typ == "function" {
						if fn, ok := tcMap["function"].(map[string]any); ok {
							if name, ok := fn["name"].(string); ok {
								bodyMap["tool_choice"] = map[string]any{
									"type": "tool",
									"name": name,
								}
								modified = true
							}
						}
					}
				}
			}

			if acc.Platform == "anthropic" && isOAuthOrSetupToken {
				if ensureClaudeCodeSystemPrompt(bodyMap) {
					modified = true
				}
				if ensureClaudeCodeMetadata(bodyMap) {
					modified = true
				}
			}
		}

		if modified {
			if newPayload, err := json.Marshal(bodyMap); err == nil {
				reqPayload = newPayload
			}
		}
	}

	var requestBody io.Reader = bytes.NewReader(reqPayload)
	if requestMethod == http.MethodGet || requestMethod == http.MethodDelete {
		requestBody = nil
	}
	req, err := http.NewRequestWithContext(ctx, requestMethod, apiURL, requestBody)
	if err != nil {
		return nil, err
	}

	if isChatCompletions || acc.Platform == "openai" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", requestContentType)
		if isOAuth && acc.Platform == "openai" {
			req.Host = "chatgpt.com"
			if isStream {
				req.Header.Set("accept", "text/event-stream")
			}
			if chatgptAccID, ok := accountCredentials["chatgpt_account_id"].(string); ok && chatgptAccID != "" {
				req.Header.Set("chatgpt-account-id", chatgptAccID)
			}
		}
	} else if acc.Platform == "yunwu" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("Content-Type", requestContentType)
		if strings.HasSuffix(apiURL, "/messages") {
			req.Header.Set("anthropic-version", "2023-06-01")
			req.Header.Set("anthropic-beta", claude.DefaultBetaHeader)
		}
		if isStream {
			req.Header.Set("accept", "text/event-stream")
		} else {
			req.Header.Set("accept", "application/json")
		}
	} else {
		// Use the exact Claude Code headers used by the admin account test.
		// Keeping these paths aligned is important for OAuth/setup-token accounts,
		// whose upstream policy can depend on the declared client protocol.
		for name, value := range claude.DefaultHeaders {
			req.Header.Set(name, value)
		}
		if isOAuthOrSetupToken {
			req.Header.Set("Authorization", "Bearer "+apiKey)
			req.Header.Set("anthropic-beta", claude.DefaultBetaHeader)
		} else {
			req.Header.Set("x-api-key", apiKey)
			req.Header.Set("anthropic-beta", claude.APIKeyBetaHeader)
		}
		req.Header.Set("anthropic-version", "2023-06-01")
		req.Header.Set("content-type", requestContentType)
		if isStream {
			req.Header.Set("accept", "text/event-stream")
		} else {
			req.Header.Set("accept", "application/json")
		}
	}
	if override, ok := ctx.Value(upstreamRequestOverrideKey{}).(upstreamRequestOverride); ok {
		for _, name := range []string{"Accept", "Range", "X-Goog-Upload-Command", "X-Goog-Upload-Header-Content-Length", "X-Goog-Upload-Header-Content-Type", "X-Goog-Upload-Offset", "X-Goog-Upload-Protocol"} {
			if len(override.Headers.Values(name)) > 0 {
				req.Header.Del(name)
			}
			for _, value := range override.Headers.Values(name) {
				req.Header.Add(name, value)
			}
		}
	}

	// 5. Send request
	transport := &http.Transport{
		Proxy: proxyFunc,
	}
	client := &http.Client{
		Transport: transport,
	}

	slog.Info("[Runtime] Sending upstream HTTP request",
		"node_id", nodeID,
		"platform", acc.Platform,
		"url", apiURL,
		"is_stream", isStream,
	)

	reqStart := time.Now()
	resp, err := client.Do(req)
	latencyMs := time.Since(reqStart).Milliseconds()

	if err != nil {
		slog.Error("[Runtime] Upstream HTTP request failed",
			"node_id", nodeID,
			"latency_ms", latencyMs,
			"error", err,
		)
		return nil, err
	}
	defer resp.Body.Close()

	// Extract Anthropic ratelimit headers and update account.Extra & SessionWindowEnd in DB
	if acc != nil && resp.Header != nil {
		extraUpdates := make(map[string]any, 4)

		util5hStr := resp.Header.Get("anthropic-ratelimit-unified-5h-utilization")
		if util5hStr == "" {
			util5hStr = resp.Header.Get("anthropic-ratelimit-5h-utilization")
		}
		if util5hStr == "" {
			util5hStr = resp.Header.Get("anthropic-ratelimit-requests-utilization")
		}
		if util5hStr == "" {
			util5hStr = resp.Header.Get("anthropic-ratelimit-tokens-utilization")
		}
		if util5hStr != "" {
			if util, parseErr := strconv.ParseFloat(util5hStr, 64); parseErr == nil {
				extraUpdates["session_window_utilization"] = util
				extraUpdates["passive_usage_5h_utilization"] = util
				slog.Debug("[Runtime] Parsed 5h ratelimit utilization", "node_id", nodeID, "utilization", util)
			}
		}

		util7dStr := resp.Header.Get("anthropic-ratelimit-unified-7d-utilization")
		if util7dStr == "" {
			util7dStr = resp.Header.Get("anthropic-ratelimit-7d-utilization")
		}
		if util7dStr != "" {
			if util, parseErr := strconv.ParseFloat(util7dStr, 64); parseErr == nil {
				extraUpdates["passive_usage_7d_utilization"] = util
				slog.Debug("[Runtime] Parsed 7d ratelimit utilization", "node_id", nodeID, "utilization", util)
			}
		}

		reset5hStr := resp.Header.Get("anthropic-ratelimit-unified-5h-reset")
		if reset5hStr == "" {
			reset5hStr = resp.Header.Get("anthropic-ratelimit-5h-reset")
		}
		if reset5hStr != "" {
			if ts, parseErr := strconv.ParseInt(reset5hStr, 10, 64); parseErr == nil {
				if ts > 1e11 {
					ts = ts / 1000
				}
				sessionWindowEnd := time.Unix(ts, 0)
				_ = dbClient.Account.UpdateOne(acc).
					SetSessionWindowEnd(sessionWindowEnd).
					SetSessionWindowStatus("allowed").
					Exec(ctx)
			}
		}

		if len(extraUpdates) > 0 {
			extraUpdates["passive_usage_sampled_at"] = time.Now().UTC().Format(time.RFC3339)
			existingExtra := acc.Extra
			if existingExtra == nil {
				existingExtra = make(map[string]any)
			}
			for k, v := range extraUpdates {
				existingExtra[k] = v
			}
			_ = dbClient.Account.UpdateOne(acc).
				SetExtra(existingExtra).
				Exec(ctx)
			slog.Info("[Runtime] Saved ratelimit metadata to account.Extra", "node_id", nodeID)
		}
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		bodyBytes, _ := io.ReadAll(resp.Body)
		body := SafeResponseBody(bodyBytes)
		if acc.Platform == "yunwu" {
			slog.Warn("[Runtime] Yunwu upstream returned an error response",
				"node_id", nodeID,
				"status_code", resp.StatusCode,
				"upstream_request_id", upstreamRequestID(resp.Header),
				"response_body", body,
			)
		}
		return nil, fmt.Errorf("upstream error status %d: %s", resp.StatusCode, body)
	}

	// Preserve media and async-operation response metadata. The gateway writer
	// implements this interface, while tests and buffered callers remain valid.
	if headerWriter, ok := stream.(interface{ Header() http.Header }); ok {
		copyUpstreamResponseHeaders(headerWriter.Header(), resp.Header)
	}
	if statusWriter, ok := stream.(interface{ WriteHeader(int) }); ok && resp.StatusCode != http.StatusOK {
		statusWriter.WriteHeader(resp.StatusCode)
	}

	isResponsesAPI := false
	var checkMap map[string]any
	if json.Unmarshal(reqPayload, &checkMap) == nil {
		if checkMap["input"] != nil || checkMap["instructions"] != nil {
			isResponsesAPI = true
		}
	}

	// 6. Stream response and track end of stream for token usage
	var outStream io.Writer = stream
	if !hasUpstreamOverride && isResponsesAPI && !strings.Contains(apiURL, "/responses") && isStream {
		outStream = newResponsesSSETransformer(stream)
	}

	var endBuf bytes.Buffer
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			_, writeErr := outStream.Write(buf[:n])
			if writeErr != nil {
				return nil, writeErr
			}
			if flusher, ok := stream.(http.Flusher); ok {
				flusher.Flush()
			}

			// Keep last 2048 bytes for token extraction
			if endBuf.Len() > 2048 {
				truncated := endBuf.Bytes()[endBuf.Len()-2048:]
				endBuf.Reset()
				endBuf.Write(truncated)
			}
			endBuf.Write(buf[:n])
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
	}

	if !hasUpstreamOverride && isChatCompletions && isStream {
		_, _ = stream.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_completed\"}}\n\n"))
		if flusher, ok := stream.(http.Flusher); ok {
			flusher.Flush()
		}
	}

	input, output := extractTokens(endBuf.Bytes())

	slog.Info("[Runtime] Upstream request completed",
		"node_id", nodeID,
		"status_code", resp.StatusCode,
		"latency_ms", latencyMs,
		"input_tokens", input,
		"output_tokens", output,
	)

	return &ResponseMetadata{
		StatusCode:   resp.StatusCode,
		InputTokens:  input,
		OutputTokens: output,
	}, nil
}

// SafeResponseBody returns a bounded version of an upstream response suitable
// for diagnostics. It removes common credential fields before the value is
// written to logs or error events.
func SafeResponseBody(body []byte) string {
	const maxBytes = 8 * 1024
	if len(body) > maxBytes {
		body = body[:maxBytes]
	}
	var value any
	if json.Unmarshal(body, &value) == nil {
		redactResponseValue(value)
		if encoded, err := json.Marshal(value); err == nil {
			return string(encoded)
		}
	}
	return string(body)
}

func redactResponseValue(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "authorization") || strings.Contains(lower, "api_key") ||
				strings.Contains(lower, "token") || strings.Contains(lower, "secret") ||
				strings.Contains(lower, "password") || strings.Contains(lower, "credential") {
				typed[key] = "[redacted]"
				continue
			}
			redactResponseValue(child)
		}
	case []any:
		for _, child := range typed {
			redactResponseValue(child)
		}
	}
}

func upstreamRequestID(headers http.Header) string {
	for _, name := range []string{"X-Request-ID", "Request-ID", "OpenAI-Request-ID"} {
		if value := headers.Get(name); value != "" {
			return value
		}
	}
	return ""
}

func upstreamOrigin(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		raw = "https://api.openlux.ai"
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid upstream base URL")
	}
	return u.Scheme + "://" + u.Host, nil
}

func copyUpstreamResponseHeaders(dst, src http.Header) {
	for _, name := range []string{
		"Content-Type", "Content-Length", "Content-Disposition", "Content-Encoding",
		"Cache-Control", "ETag", "Last-Modified", "Location", "Retry-After",
		"X-Request-ID", "OpenAI-Request-ID",
		"X-Goog-Upload-URL", "X-Goog-Upload-Status", "X-Goog-Upload-Command",
		"X-Goog-Upload-Offset", "X-Goog-Upload-Chunk-Granularity",
	} {
		if values := src.Values(name); len(values) > 0 {
			dst.Del(name)
			for _, value := range values {
				dst.Add(name, value)
			}
		}
	}
}

type responsesSSETransformer struct {
	writer     io.Writer
	buf        []byte
	headerSent bool
	accContent strings.Builder
}

func newResponsesSSETransformer(w io.Writer) *responsesSSETransformer {
	return &responsesSSETransformer{writer: w}
}

func (t *responsesSSETransformer) ensureHeaderSent() {
	if t.headerSent {
		return
	}
	t.headerSent = true
	preamble := `data: {"type":"response.created","response":{"id":"resp_yunwu_1","status":"in_progress"}}` + "\n\n" +
		`data: {"type":"response.output_item.added","output_index":0,"item":{"id":"item_yunwu_1","type":"message","status":"in_progress","role":"assistant","content":[]}}` + "\n\n" +
		`data: {"type":"response.content_part.added","output_index":0,"content_index":0,"part":{"type":"text","text":""}}` + "\n\n"
	_, _ = t.writer.Write([]byte(preamble))
	if flusher, ok := t.writer.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (t *responsesSSETransformer) Write(p []byte) (n int, err error) {
	t.buf = append(t.buf, p...)
	for {
		idx := bytes.IndexByte(t.buf, '\n')
		if idx == -1 {
			break
		}
		line := bytes.TrimSpace(t.buf[:idx])
		t.buf = t.buf[idx+1:]

		if len(line) == 0 {
			continue
		}

		if bytes.HasPrefix(line, []byte("data: ")) {
			dataPayload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data: ")))
			if bytes.Equal(dataPayload, []byte("[DONE]")) {
				t.ensureHeaderSent()
				fullText := t.accContent.String()

				doneEvt := map[string]any{
					"type":          "response.output_text.done",
					"output_index":  0,
					"content_index": 0,
					"text":          fullText,
				}
				if b, err := json.Marshal(doneEvt); err == nil {
					_, _ = t.writer.Write([]byte("data: " + string(b) + "\n\n"))
				}

				itemDoneEvt := map[string]any{
					"type":         "response.output_item.done",
					"output_index": 0,
					"item": map[string]any{
						"id":     "item_yunwu_1",
						"type":   "message",
						"status": "completed",
						"role":   "assistant",
						"content": []map[string]any{
							{
								"type": "text",
								"text": fullText,
							},
						},
					},
				}
				if b, err := json.Marshal(itemDoneEvt); err == nil {
					_, _ = t.writer.Write([]byte("data: " + string(b) + "\n\n"))
				}

				completedEvt := map[string]any{
					"type": "response.completed",
					"response": map[string]any{
						"id":     "resp_yunwu_1",
						"status": "completed",
					},
				}
				if b, err := json.Marshal(completedEvt); err == nil {
					_, _ = t.writer.Write([]byte("data: " + string(b) + "\n\n"))
				}
			} else {
				var eventMap map[string]any
				if json.Unmarshal(dataPayload, &eventMap) == nil {
					if _, hasType := eventMap["type"]; hasType {
						_, _ = t.writer.Write(append(line, []byte("\n\n")...))
					} else if choices, ok := eventMap["choices"].([]any); ok && len(choices) > 0 {
						if choiceMap, ok := choices[0].(map[string]any); ok {
							if delta, ok := choiceMap["delta"].(map[string]any); ok {
								if content, ok := delta["content"].(string); ok && content != "" {
									t.ensureHeaderSent()
									t.accContent.WriteString(content)
									deltaEvt := map[string]any{
										"type":          "response.output_text.delta",
										"output_index":  0,
										"content_index": 0,
										"delta":         content,
									}
									if bytesEvt, err := json.Marshal(deltaEvt); err == nil {
										_, _ = t.writer.Write([]byte("data: " + string(bytesEvt) + "\n\n"))
									}
								}
							}
						}
					} else {
						_, _ = t.writer.Write(append(line, []byte("\n\n")...))
					}
				} else {
					_, _ = t.writer.Write(append(line, []byte("\n\n")...))
				}
			}
			if flusher, ok := t.writer.(http.Flusher); ok {
				flusher.Flush()
			}
		} else {
			_, _ = t.writer.Write(append(line, []byte("\n\n")...))
		}
	}
	return len(p), nil
}

func extractTokens(data []byte) (int64, int64) {
	input := firstJSONInteger(data, "total_input_tokens", "promptTokenCount", "prompt_token_count", "prompt_tokens", "input_tokens")
	output := firstJSONInteger(data, "total_output_tokens", "candidatesTokenCount", "candidates_token_count", "completion_tokens", "output_tokens")
	thoughts := firstJSONInteger(data, "total_thought_tokens", "thoughtsTokenCount", "thoughts_token_count")
	return input, output + thoughts
}

func firstJSONInteger(data []byte, keys ...string) int64 {
	text := string(data)
	for _, key := range keys {
		needle := `"` + key + `"`
		search := text
		var found int64
		matched := false
		for {
			idx := strings.Index(search, needle)
			if idx < 0 {
				break
			}
			after := search[idx+len(needle):]
			colon := strings.IndexByte(after, ':')
			if colon < 0 {
				break
			}
			after = strings.TrimLeft(after[colon+1:], " \t\r\n")
			var value int64
			if _, err := fmt.Sscanf(after, "%d", &value); err == nil {
				found, matched = value, true
			}
			search = after
		}
		if matched {
			return found
		}
	}
	return 0
}

func constructInputFromMessagesPayload(bodyMap map[string]any) []map[string]any {
	var input []map[string]any
	msgs, ok := bodyMap["messages"].([]any)
	if !ok {
		return input
	}
	for _, msg := range msgs {
		msgMap, ok := msg.(map[string]any)
		if !ok {
			continue
		}
		role, _ := msgMap["role"].(string)
		if role == "" {
			role = "user"
		}
		contentStr, _ := msgMap["content"].(string)
		if contentStr != "" {
			input = append(input, map[string]any{
				"role": role,
				"content": []map[string]any{
					{
						"type": "input_text",
						"text": contentStr,
					},
				},
			})
		}
	}
	return input
}

func resolveUpstreamAPIURL(customURL string, platform string, isChatCompletions bool) string {
	customURL = strings.TrimSpace(customURL)
	if customURL == "" {
		if platform == "openai" || isChatCompletions {
			return "https://api.openai.com/v1/chat/completions"
		}
		return "https://api.anthropic.com/v1/messages"
	}

	customURL = strings.TrimSuffix(customURL, "/")

	// If full endpoint URL is already provided
	if strings.HasSuffix(customURL, "/v1/messages") || strings.HasSuffix(customURL, "/messages") ||
		strings.HasSuffix(customURL, "/v1/chat/completions") || strings.HasSuffix(customURL, "/chat/completions") {
		return customURL
	}

	if platform == "openai" || isChatCompletions {
		if strings.HasSuffix(customURL, "/v1") {
			return customURL + "/chat/completions"
		}
		return customURL + "/v1/chat/completions"
	}

	// Anthropic / Yunwu Messages endpoint
	if strings.HasSuffix(customURL, "/v1") {
		return customURL + "/messages"
	}
	return customURL + "/v1/messages"
}

func defaultBaseURLForYunwu(extra map[string]any) string {
	provider := "openlux"
	if extra != nil {
		if p, ok := extra["provider"].(string); ok && strings.TrimSpace(p) != "" {
			provider = strings.ToLower(strings.TrimSpace(p))
		}
	}
	switch provider {
	case "portdan", "portdan.com":
		return "https://portdan.com/v1"
	case "api-top", "api-top.com", "top-api":
		return "https://api-top.com/v1"
	case "yuanqiaoyun", "yuanqiaoyun.com":
		return "https://yuanqiaoyun.com/v1"
	case "x-llm", "x-llm.net", "xllm":
		return "https://x-llm.net/v1"
	default:
		return "https://api.openlux.ai/v1"
	}
}

func constructMessagesFromResponsesPayload(bodyMap map[string]any) []map[string]any {
	var msgs []map[string]any

	if inst, ok := bodyMap["instructions"].(string); ok && strings.TrimSpace(inst) != "" {
		msgs = append(msgs, map[string]any{
			"role":    "system",
			"content": inst,
		})
	}

	inputVal := bodyMap["input"]
	if inputVal != nil {
		switch v := inputVal.(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				msgs = append(msgs, map[string]any{
					"role":    "user",
					"content": v,
				})
			}
		case []any:
			for _, item := range v {
				switch itemObj := item.(type) {
				case string:
					if strings.TrimSpace(itemObj) != "" {
						msgs = append(msgs, map[string]any{
							"role":    "user",
							"content": itemObj,
						})
					}
				case map[string]any:
					role, _ := itemObj["role"].(string)
					if role == "" {
						role = "user"
					}
					if contentStr, ok := itemObj["content"].(string); ok && strings.TrimSpace(contentStr) != "" {
						msgs = append(msgs, map[string]any{
							"role":    role,
							"content": contentStr,
						})
					} else if contentArr, ok := itemObj["content"].([]any); ok {
						var textParts []string
						for _, part := range contentArr {
							if partStr, ok := part.(string); ok && strings.TrimSpace(partStr) != "" {
								textParts = append(textParts, partStr)
							} else if partObj, ok := part.(map[string]any); ok {
								if t, ok := partObj["text"].(string); ok && strings.TrimSpace(t) != "" {
									textParts = append(textParts, t)
								}
							}
						}
						if len(textParts) > 0 {
							msgs = append(msgs, map[string]any{
								"role":    role,
								"content": strings.Join(textParts, "\n"),
							})
						}
					} else if textVal, ok := itemObj["text"].(string); ok && strings.TrimSpace(textVal) != "" {
						msgs = append(msgs, map[string]any{
							"role":    role,
							"content": textVal,
						})
					}
				}
			}
		}
	}

	if len(msgs) == 0 || (len(msgs) == 1 && msgs[0]["role"] == "system") {
		if promptStr, ok := bodyMap["prompt"].(string); ok && strings.TrimSpace(promptStr) != "" {
			msgs = append(msgs, map[string]any{
				"role":    "user",
				"content": promptStr,
			})
		}
	}

	if len(msgs) == 0 {
		msgs = []map[string]any{
			{"role": "user", "content": "Hello"},
		}
	}

	return msgs
}

func ensureClaudeCodeSystemPrompt(bodyMap map[string]any) bool {
	systemVal, hasSystem := bodyMap["system"]
	if !hasSystem || systemVal == nil {
		bodyMap["system"] = []map[string]any{
			{
				"type": "text",
				"text": claude.ClaudeCodeSystemPrompt,
			},
		}
		return true
	}
	switch s := systemVal.(type) {
	case string:
		if strings.TrimSpace(s) == "" {
			bodyMap["system"] = []map[string]any{
				{
					"type": "text",
					"text": claude.ClaudeCodeSystemPrompt,
				},
			}
			return true
		}
		if !strings.Contains(s, "You are Claude Code") {
			bodyMap["system"] = []map[string]any{
				{
					"type": "text",
					"text": claude.ClaudeCodeSystemPrompt,
				},
				{
					"type": "text",
					"text": s,
				},
			}
			return true
		}
	case []any:
		hasClaudeCode := false
		for _, item := range s {
			if m, ok := item.(map[string]any); ok {
				if t, ok := m["text"].(string); ok && strings.Contains(t, "You are Claude Code") {
					hasClaudeCode = true
					break
				}
			}
		}
		if !hasClaudeCode {
			newSystem := make([]any, 0, len(s)+1)
			newSystem = append(newSystem, map[string]any{
				"type": "text",
				"text": claude.ClaudeCodeSystemPrompt,
			})
			newSystem = append(newSystem, s...)
			bodyMap["system"] = newSystem
			return true
		}
	}
	return false
}

func ensureClaudeCodeMetadata(bodyMap map[string]any) bool {
	if metaVal, ok := bodyMap["metadata"]; ok && metaVal != nil {
		if metaMap, ok := metaVal.(map[string]any); ok {
			if uid, ok := metaMap["user_id"].(string); !ok || strings.TrimSpace(uid) == "" {
				b := make([]byte, 32)
				_, _ = rand.Read(b)
				metaMap["user_id"] = "user_" + hex.EncodeToString(b) + "_session_" + uuid.New().String()
				return true
			}
			return false
		}
	}
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	bodyMap["metadata"] = map[string]any{
		"user_id": "user_" + hex.EncodeToString(b) + "_session_" + uuid.New().String(),
	}
	return true
}


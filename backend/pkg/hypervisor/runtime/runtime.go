package runtime

import (
	"bytes"
	"context"
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
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
)

type ResponseMetadata struct {
	InputTokens  int64
	OutputTokens int64
	StatusCode   int
}

type Runtime interface {
	Execute(ctx context.Context, nodeID string, reqPayload []byte, stream io.Writer) (*ResponseMetadata, error)
}

// MockRuntime implements Runtime for simulating upstream responses and testing failures
type MockRuntime struct {
	store           *storage.Storage
	SuccessDelay    time.Duration
	SlowStreamDelay time.Duration
	Force429        bool
	ForceTimeout    bool
	ForceDisconnect bool
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
		events := []string{
			"data: {\"id\":\"chatcmpl-mock\",\"object\":\"chat.completion.chunk\",\"created\":1677858342,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"\"},\"finish_reason\":null}]}\n\n",
			"data: {\"id\":\"chatcmpl-mock\",\"object\":\"chat.completion.chunk\",\"created\":1677858342,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello \"},\"finish_reason\":null}]}\n\n",
			"data: {\"id\":\"chatcmpl-mock\",\"object\":\"chat.completion.chunk\",\"created\":1677858342,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"from \"},\"finish_reason\":null}]}\n\n",
			"data: {\"id\":\"chatcmpl-mock\",\"object\":\"chat.completion.chunk\",\"created\":1677858342,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"OpenAI Mock!\"},\"finish_reason\":null}]}\n\n",
			"data: {\"id\":\"chatcmpl-mock\",\"object\":\"chat.completion.chunk\",\"created\":1677858342,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n",
			"data: [DONE]\n\n",
		}
		for _, ev := range events {
			if m.SlowStreamDelay > 0 {
				time.Sleep(m.SlowStreamDelay)
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

	// Simulating standard Anthropic SSE stream
	events := []string{
		"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_mock\",\"role\":\"assistant\",\"content\":[]}}\n\n",
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello \"}}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"from \"}}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Mock!\"}}\n\n",
		"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n",
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"input_tokens\":15,\"output_tokens\":10}}\n\n",
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
	}

	for _, ev := range events {
		if m.SlowStreamDelay > 0 {
			time.Sleep(m.SlowStreamDelay)
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
	store *storage.Storage
}

func NewAnthropicRuntime(store *storage.Storage) *AnthropicRuntime {
	return &AnthropicRuntime{store: store}
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

	// Extract API Key / Token from Credentials
	var apiKey string
	keysToTry := []string{"access_token", "api_key", "token", "setup_token", "key", "session_key"}
	for _, k := range keysToTry {
		if val, ok := acc.Credentials[k].(string); ok && strings.TrimSpace(val) != "" {
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
	customURL, _ := acc.Credentials["base_url"].(string)

	isChatCompletions := false
	if acc.Platform == "openai" {
		isChatCompletions = true
	} else if acc.Platform == "yunwu" {
		var bodyMap map[string]any
		if json.Unmarshal(reqPayload, &bodyMap) == nil {
			modelStr, _ := bodyMap["model"].(string)
			modelLower := strings.ToLower(modelStr)
			if strings.HasPrefix(modelLower, "gpt-") ||
				strings.HasPrefix(modelLower, "o1-") ||
				strings.HasPrefix(modelLower, "o3-") ||
				strings.HasPrefix(modelLower, "text-") ||
				bodyMap["input"] != nil ||
				bodyMap["instructions"] != nil {
				isChatCompletions = true
			}
		}
	}

	apiURL := resolveUpstreamAPIURL(customURL, acc.Platform, isChatCompletions)

	var payloadMap struct {
		Stream bool `json:"stream"`
	}
	isStream := false
	if json.Unmarshal(reqPayload, &payloadMap) == nil {
		isStream = payloadMap.Stream
	}

	var bodyMap map[string]any
	if json.Unmarshal(reqPayload, &bodyMap) == nil {
		modified := false

		msgs, hasMsgs := bodyMap["messages"]
		if !hasMsgs || msgs == nil {
			bodyMap["messages"] = constructMessagesFromResponsesPayload(bodyMap)
			modified = true
		} else if msgArr, ok := msgs.([]any); ok && len(msgArr) == 0 {
			bodyMap["messages"] = constructMessagesFromResponsesPayload(bodyMap)
			modified = true
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
		}

		if modified {
			if newPayload, err := json.Marshal(bodyMap); err == nil {
				reqPayload = newPayload
			}
		}
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(reqPayload))
	if err != nil {
		return nil, err
	}

	if isChatCompletions || acc.Platform == "openai" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")
	} else {
		isOAuthOrSetupToken := acc.Type == "oauth" || acc.Type == "setup-token" || strings.HasPrefix(apiKey, "sk-ant-oat") || strings.HasPrefix(apiKey, "sk-oat")
		if isOAuthOrSetupToken {
			req.Header.Set("Authorization", "Bearer "+apiKey)
			req.Header.Set("User-Agent", "Claude-Code/0.2.29")
			req.Header.Set("anthropic-beta", "claude-code-20250219,oauth-2025-04-20,prompt-caching-2024-07-31")
		} else {
			req.Header.Set("x-api-key", apiKey)
			req.Header.Set("Authorization", "Bearer "+apiKey)
			req.Header.Set("User-Agent", "Claude-Code/0.2.29")
		}
		req.Header.Set("anthropic-version", "2023-06-01")
		req.Header.Set("content-type", "application/json")
		if isStream {
			req.Header.Set("accept", "text/event-stream")
		} else {
			req.Header.Set("accept", "application/json")
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

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("upstream error status %d: %s", resp.StatusCode, string(bodyBytes))
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
	if isResponsesAPI && isStream {
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

	if isChatCompletions && isStream {
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
	str := string(data)
	input := int64(0)
	output := int64(0)

	if idx := strings.Index(str, `"input_tokens"`); idx != -1 {
		sub := str[idx:]
		if colIdx := strings.Index(sub, ":"); colIdx != -1 {
			sub = sub[colIdx+1:]
			var val int64
			if _, scanErr := fmt.Sscanf(sub, "%d", &val); scanErr == nil {
				input = val
			}
		}
	}
	if idx := strings.Index(str, `"output_tokens"`); idx != -1 {
		sub := str[idx:]
		if colIdx := strings.Index(sub, ":"); colIdx != -1 {
			sub = sub[colIdx+1:]
			var val int64
			if _, scanErr := fmt.Sscanf(sub, "%d", &val); scanErr == nil {
				output = val
			}
		}
	}
	return input, output
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

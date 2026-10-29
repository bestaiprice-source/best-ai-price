package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/gemini"
	"github.com/Wei-Shaw/sub2api/internal/pkg/googleapi"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/server/gatewayctx"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	hyperruntime "github.com/Wei-Shaw/sub2api/pkg/hypervisor/runtime"
	"github.com/google/uuid"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// geminiCLITmpDirRegex 用于从 Gemini CLI 请求体中提取 tmp 目录的哈希值
// 匹配格式: /Users/xxx/.gemini/tmp/[64位十六进制哈希]
var geminiCLITmpDirRegex = regexp.MustCompile(`/\.gemini/tmp/([A-Fa-f0-9]{64})`)

type hypervisorGatewayWriter struct{ ctx gatewayctx.GatewayContext }

func (w hypervisorGatewayWriter) Write(p []byte) (int, error) { return w.ctx.WriteBytes(0, p) }
func (w hypervisorGatewayWriter) Header() http.Header         { return w.ctx.Header() }
func (w hypervisorGatewayWriter) WriteHeader(status int)      { w.ctx.SetStatus(status) }
func (w hypervisorGatewayWriter) Flush()                      { _ = w.ctx.Flush() }

type yunwuLivePeer struct{ conn gatewayctx.WebSocketConn }

type bufferedGatewayWriter struct {
	header http.Header
	status int
	buf    bytes.Buffer
}

func (w *bufferedGatewayWriter) Header() http.Header         { return w.header }
func (w *bufferedGatewayWriter) Write(p []byte) (int, error) { return w.buf.Write(p) }
func (w *bufferedGatewayWriter) WriteHeader(status int)      { w.status = status }
func (w *bufferedGatewayWriter) Flush()                      {}

func (p yunwuLivePeer) Read(ctx context.Context) (hyperruntime.WSMessageType, []byte, error) {
	messageType, payload, err := p.conn.Read(ctx)
	if messageType == gatewayctx.MessageBinary {
		return hyperruntime.WSMessageBinary, payload, err
	}
	return hyperruntime.WSMessageText, payload, err
}

func (p yunwuLivePeer) Write(ctx context.Context, messageType hyperruntime.WSMessageType, payload []byte) error {
	targetType := gatewayctx.MessageText
	if messageType == hyperruntime.WSMessageBinary {
		targetType = gatewayctx.MessageBinary
	}
	return p.conn.Write(ctx, targetType, payload)
}

// GeminiLive proxies the bidirectional Gemini Live API, including realtime
// audio/video input and audio responses, through a balance-aware Yunwu node.
func (h *GatewayHandler) GeminiLive(c *gin.Context) {
	h.GeminiLiveGateway(gatewayctx.FromGin(c))
}

func (h *GatewayHandler) GeminiLiveGateway(c gatewayctx.GatewayContext) {
	if !strings.EqualFold(strings.TrimSpace(c.HeaderValue("Upgrade")), "websocket") {
		googleErrorContext(c, http.StatusUpgradeRequired, "WebSocket upgrade required")
		return
	}
	apiKey, ok := middleware.GetAPIKeyFromGatewayContext(c)
	if !ok || apiKey == nil || !apiKeyIsYunwu(apiKey) {
		googleErrorContext(c, http.StatusBadRequest, "A Yunwu group is required for Gemini Live")
		return
	}
	subject, ok := middleware.GetAuthSubjectFromGatewayContext(c)
	if !ok {
		googleErrorContext(c, http.StatusUnauthorized, "Invalid API key")
		return
	}
	if h.hypervisorEngine == nil || h.hypervisorEngine.YunwuEngine() == nil {
		googleErrorContext(c, http.StatusServiceUnavailable, "Yunwu hypervisor is not initialized")
		return
	}
	var subprotocols []string
	for _, protocol := range strings.Split(c.HeaderValue("Sec-WebSocket-Protocol"), ",") {
		if protocol = strings.TrimSpace(protocol); protocol != "" {
			subprotocols = append(subprotocols, protocol)
		}
	}
	conn, err := c.AcceptWebSocket(gatewayctx.WebSocketAcceptOptions{CompressionEnabled: true, Subprotocols: subprotocols})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	path := c.Path()
	if path != "/ws/google.ai.generativelanguage.v1beta.GenerativeService.BidiGenerateContent" &&
		path != "/ws/google.ai.generativelanguage.v1alpha.GenerativeService.BidiGenerateContent" &&
		path != "/ws/google.ai.generativelanguage.v1alpha.GenerativeService.BidiGenerateMusic" &&
		path != "/ws/google.ai.generativelanguage.v1beta.GenerativeService.BidiGenerateContentConstrained" {
		return
	}
	query := c.Request().URL.Query()
	query.Del("key")
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	err = h.hypervisorEngine.YunwuEngine().HandleLive(c.Context(), strconv.FormatInt(subject.UserID, 10), c.HeaderValue("X-Conversation-ID"), path, c.Header(), yunwuLivePeer{conn: conn})
	if err != nil {
		slog.Warn("Gemini Live session ended", "owner_id", subject.UserID, "error", err)
	}
}

// GeminiEphemeralLive accepts the opaque token returned by GeminiAuthToken.
// It intentionally does not use normal API-key middleware: the one-use token
// is itself the credential and maps back to its owner and issuing Yunwu node.
func (h *GatewayHandler) GeminiEphemeralLive(c *gin.Context) {
	h.GeminiEphemeralLiveGateway(gatewayctx.FromGin(c))
}

func (h *GatewayHandler) GeminiEphemeralLiveGateway(c gatewayctx.GatewayContext) {
	if !strings.EqualFold(strings.TrimSpace(c.HeaderValue("Upgrade")), "websocket") {
		googleErrorContext(c, http.StatusUpgradeRequired, "WebSocket upgrade required")
		return
	}
	if h.hypervisorEngine == nil || h.hypervisorEngine.YunwuEngine() == nil {
		googleErrorContext(c, http.StatusServiceUnavailable, "Yunwu hypervisor is not initialized")
		return
	}
	token := strings.TrimSpace(c.QueryValue("access_token"))
	if token == "" {
		if authorization := strings.TrimSpace(c.HeaderValue("Authorization")); strings.HasPrefix(authorization, "Token ") {
			token = strings.TrimSpace(strings.TrimPrefix(authorization, "Token "))
		}
	}
	conn, err := c.AcceptWebSocket(gatewayctx.WebSocketAcceptOptions{CompressionEnabled: true})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	path := "/ws/google.ai.generativelanguage.v1beta.GenerativeService.BidiGenerateContentConstrained"
	err = h.hypervisorEngine.YunwuEngine().HandleEphemeralLive(c.Context(), token, c.HeaderValue("X-Conversation-ID"), path, c.Header(), yunwuLivePeer{conn: conn})
	if err != nil {
		slog.Warn("constrained Gemini Live session ended", "error", err)
	}
}

// GeminiAuthToken provisions a vendor token on a Yunwu node, replaces it with
// an opaque gateway token, and preserves the public Gemini response schema.
func (h *GatewayHandler) GeminiAuthToken(c *gin.Context) {
	h.GeminiAuthTokenGateway(gatewayctx.FromGin(c))
}

func (h *GatewayHandler) GeminiAuthTokenGateway(c gatewayctx.GatewayContext) {
	apiKey, ok := middleware.GetAPIKeyFromGatewayContext(c)
	if !ok || apiKey == nil || !apiKeyIsYunwu(apiKey) {
		googleErrorContext(c, http.StatusBadRequest, "A Yunwu group is required for Gemini auth tokens")
		return
	}
	subject, ok := middleware.GetAuthSubjectFromGatewayContext(c)
	if !ok || h.hypervisorEngine == nil || h.hypervisorEngine.YunwuEngine() == nil {
		googleErrorContext(c, http.StatusUnauthorized, "Invalid API key")
		return
	}
	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request())
	if err != nil {
		googleErrorContext(c, http.StatusBadRequest, "Failed to read auth token request")
		return
	}
	var request struct {
		Uses                 int64  `json:"uses"`
		NewSessionExpireTime string `json:"newSessionExpireTime"`
	}
	_ = json.Unmarshal(body, &request)
	ttl := time.Minute
	if expiry, parseErr := time.Parse(time.RFC3339Nano, request.NewSessionExpireTime); parseErr == nil {
		ttl = time.Until(expiry)
		if ttl <= 0 || ttl > 20*time.Hour {
			googleErrorContext(c, http.StatusBadRequest, "newSessionExpireTime must be in the future and no more than 20 hours")
			return
		}
	} else if request.NewSessionExpireTime != "" {
		googleErrorContext(c, http.StatusBadRequest, "Invalid newSessionExpireTime")
		return
	}
	writer := &bufferedGatewayWriter{header: make(http.Header), status: http.StatusOK}
	requestCtx := hyperruntime.WithUpstreamRequest(c.Context(), http.MethodPost, "/v1beta/auth_tokens", "application/json")
	_, err = h.hypervisorEngine.HandleRequest(requestCtx, strconv.FormatInt(subject.UserID, 10), c.HeaderValue("X-Conversation-ID"), body, writer, service.PlatformYunwu)
	if err != nil {
		googleErrorContext(c, http.StatusBadGateway, err.Error())
		return
	}
	var response map[string]any
	if json.Unmarshal(writer.buf.Bytes(), &response) != nil {
		googleErrorContext(c, http.StatusBadGateway, "Invalid auth token response")
		return
	}
	upstreamToken, _ := response["name"].(string)
	gatewayToken, err := h.hypervisorEngine.YunwuEngine().ExchangeEphemeralToken(c.Context(), strconv.FormatInt(subject.UserID, 10), upstreamToken, ttl, request.Uses)
	if err != nil {
		googleErrorContext(c, http.StatusBadGateway, "Failed to secure ephemeral token")
		return
	}
	response["name"] = gatewayToken
	for key, values := range writer.header {
		if strings.EqualFold(key, "Content-Length") || strings.EqualFold(key, "Transfer-Encoding") {
			continue
		}
		if len(values) > 0 {
			c.SetHeader(key, values[0])
		}
	}
	c.WriteJSON(writer.status, response)
}

func (h *GatewayHandler) GeminiInteraction(c *gin.Context) {
	h.GeminiInteractionGateway(gatewayctx.FromGin(c))
}

func (h *GatewayHandler) GeminiV1BetaResource(c *gin.Context) {
	h.GeminiV1BetaResourceGateway(gatewayctx.FromGin(c))
}

// GeminiV1BetaResource transparently forwards the remaining stateful Gemini
// resources used by batch generation, caching, generated media and File Search.
func (h *GatewayHandler) GeminiV1BetaResourceGateway(c gatewayctx.GatewayContext) {
	apiKey, ok := middleware.GetAPIKeyFromGatewayContext(c)
	if !ok || apiKey == nil || !apiKeyIsYunwu(apiKey) {
		googleErrorContext(c, http.StatusBadRequest, "A Yunwu group is required for this Gemini resource")
		return
	}
	subject, ok := middleware.GetAuthSubjectFromGatewayContext(c)
	if !ok || h.hypervisorEngine == nil || h.hypervisorEngine.YunwuEngine() == nil {
		googleErrorContext(c, http.StatusUnauthorized, "Invalid API key")
		return
	}
	path := c.Path()
	allowed := false
	for _, prefix := range []string{"/v1beta/batches", "/v1beta/cachedContents", "/v1beta/generatedFiles", "/v1beta/fileSearchStores", "/v1beta/corpora", "/v1beta/tunedModels", "/v1beta/dynamic", "/upload/v1beta/fileSearchStores"} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") || strings.HasPrefix(path, prefix+":") {
			allowed = true
			break
		}
	}
	if !allowed {
		googleErrorContext(c, http.StatusBadRequest, "Invalid Gemini resource path")
		return
	}
	method := c.Method()
	if method != http.MethodGet && method != http.MethodPost && method != http.MethodDelete && method != http.MethodPatch && method != http.MethodPut {
		googleErrorContext(c, http.StatusMethodNotAllowed, "Unsupported Gemini resource method")
		return
	}
	body := []byte(`{}`)
	if method == http.MethodPost || method == http.MethodPatch || method == http.MethodPut {
		requestBody, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request())
		if err != nil {
			googleErrorContext(c, http.StatusBadRequest, "Failed to read Gemini resource body")
			return
		}
		if len(requestBody) > 0 {
			body = requestBody
		}
	}
	if query := yunwuGeminiUpstreamQuery(c, nil); query != "" {
		path += "?" + query
	}
	contentType := c.HeaderValue("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	requestCtx := hyperruntime.WithUpstreamRequest(c.Context(), method, path, contentType)
	requestCtx = hyperruntime.WithUpstreamRequestHeaders(requestCtx, c.Header())
	_, err := h.hypervisorEngine.HandleRequest(requestCtx, strconv.FormatInt(subject.UserID, 10), c.HeaderValue("X-Conversation-ID"), body, hypervisorGatewayWriter{ctx: c}, service.PlatformYunwu)
	if err != nil {
		googleErrorContext(c, http.StatusBadGateway, err.Error())
	}
}

// GeminiInteractionGateway transparently serves the current multimodal
// Interactions API, including stored/background interactions and SSE output.
func (h *GatewayHandler) GeminiInteractionGateway(c gatewayctx.GatewayContext) {
	apiKey, ok := middleware.GetAPIKeyFromGatewayContext(c)
	if !ok || apiKey == nil || !apiKeyIsYunwu(apiKey) {
		googleErrorContext(c, http.StatusBadRequest, "A Yunwu group is required for Gemini Interactions")
		return
	}
	subject, ok := middleware.GetAuthSubjectFromGatewayContext(c)
	if !ok {
		googleErrorContext(c, http.StatusUnauthorized, "Invalid API key")
		return
	}
	if h.hypervisorEngine == nil || h.hypervisorEngine.YunwuEngine() == nil {
		googleErrorContext(c, http.StatusServiceUnavailable, "Yunwu hypervisor is not initialized")
		return
	}
	path := c.Path()
	if !strings.HasPrefix(path, "/v1beta/interactions") && !strings.HasPrefix(path, "/v1/interactions") {
		googleErrorContext(c, http.StatusBadRequest, "Invalid Gemini interaction path")
		return
	}
	method := c.Method()
	if method != http.MethodPost && method != http.MethodGet && method != http.MethodDelete {
		googleErrorContext(c, http.StatusMethodNotAllowed, "Unsupported interaction method")
		return
	}
	body := []byte(`{}`)
	if method == http.MethodPost {
		requestBody, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request())
		if err != nil {
			googleErrorContext(c, http.StatusBadRequest, "Failed to read interaction body")
			return
		}
		if len(requestBody) > 0 {
			body = requestBody
		}
	}
	query := yunwuGeminiUpstreamQuery(c, nil)
	if query != "" {
		path += "?" + query
	}
	if c.QueryValue("stream") == "true" || bytes.Contains(body, []byte(`"stream":true`)) || bytes.Contains(body, []byte(`"stream": true`)) {
		c.SetHeader("Content-Type", "text/event-stream")
		c.SetHeader("Cache-Control", "no-cache, no-transform")
	}
	requestCtx := hyperruntime.WithUpstreamRequest(c.Context(), method, path, "application/json")
	_, err := h.hypervisorEngine.HandleRequest(requestCtx, strconv.FormatInt(subject.UserID, 10), c.HeaderValue("X-Conversation-ID"), body, hypervisorGatewayWriter{ctx: c}, service.PlatformYunwu)
	if err != nil {
		googleErrorContext(c, http.StatusBadGateway, err.Error())
	}
}

// GeminiV1BetaListModels proxies:
// GET /v1beta/models
func (h *GatewayHandler) GeminiV1BetaListModels(c *gin.Context) {
	h.GeminiV1BetaListModelsGateway(gatewayctx.FromGin(c))
}

func (h *GatewayHandler) GeminiV1BetaListModelsGateway(transportCtx gatewayctx.GatewayContext) {
	apiKey, ok := middleware.GetAPIKeyFromGatewayContext(transportCtx)
	if !ok || apiKey == nil {
		googleErrorContext(transportCtx, http.StatusUnauthorized, "Invalid API key")
		return
	}
	// 检查平台：优先使用强制平台（/antigravity 路由），否则要求 gemini 分组
	forcePlatform, hasForcePlatform := middleware.GetForcePlatformFromGatewayContext(transportCtx)
	isYunwu := apiKeyIsYunwu(apiKey)
	if !hasForcePlatform && !isYunwu && (apiKey.Group == nil || apiKey.Group.Platform != service.PlatformGemini) {
		googleErrorContext(transportCtx, http.StatusBadRequest, "API key group platform is not gemini or yunwu")
		return
	}

	// 强制 antigravity 模式：返回 antigravity 支持的模型列表
	if forcePlatform == service.PlatformAntigravity {
		fallback := antigravity.FallbackGeminiModelsList()
		fallback.Models = filterAntigravityGeminiModelsForAPIKey(apiKey, fallback.Models)
		transportCtx.WriteJSON(http.StatusOK, fallback)
		return
	}
	if !hasForcePlatform && isYunwu {
		h.forwardYunwuGeminiGET(transportCtx, "/v1beta/models")
		return
	}

	_, account, err := selectGeminiAIStudioAccountAcrossAPIKeyGroups(
		transportCtx.Context(),
		apiKey,
		h.geminiCompatService.SelectAccountForAIStudioEndpoints,
	)
	if err != nil {
		// 没有 gemini 账户，检查是否有 antigravity 账户可用
		hasAntigravity, _ := h.geminiCompatService.HasAntigravityAccounts(transportCtx.Context(), apiKey.GroupID)
		if hasAntigravity {
			// antigravity 账户使用静态模型列表
			fallback := gemini.FallbackModelsList()
			fallback.Models = filterGeminiModelsForAPIKey(apiKey, fallback.Models)
			transportCtx.WriteJSON(http.StatusOK, fallback)
			return
		}
		googleErrorContext(transportCtx, http.StatusServiceUnavailable, "No available Gemini accounts: "+err.Error())
		return
	}

	res, err := h.geminiCompatService.ForwardAIStudioGET(transportCtx.Context(), account, "/v1beta/models")
	if err != nil {
		googleErrorContext(transportCtx, http.StatusBadGateway, err.Error())
		return
	}
	if shouldFallbackGeminiModels(res) {
		fallback := gemini.FallbackModelsList()
		fallback.Models = filterGeminiModelsForAPIKey(apiKey, fallback.Models)
		transportCtx.WriteJSON(http.StatusOK, fallback)
		return
	}
	if apiKey != nil && len(apiKey.AllowedModels) > 0 {
		var payload gemini.ModelsListResponse
		if err := json.Unmarshal(res.Body, &payload); err == nil {
			payload.Models = filterGeminiModelsForAPIKey(apiKey, payload.Models)
			if body, marshalErr := json.Marshal(payload); marshalErr == nil {
				res.Body = body
			}
		}
	}
	writeUpstreamResponseContext(transportCtx, res)
}

// GeminiV1BetaGetModel proxies:
// GET /v1beta/models/{model}
func (h *GatewayHandler) GeminiV1BetaGetModel(c *gin.Context) {
	h.GeminiV1BetaGetModelGateway(gatewayctx.FromGin(c))
}

func (h *GatewayHandler) GeminiV1BetaGetModelGateway(transportCtx gatewayctx.GatewayContext) {
	apiKey, ok := middleware.GetAPIKeyFromGatewayContext(transportCtx)
	if !ok || apiKey == nil {
		googleErrorContext(transportCtx, http.StatusUnauthorized, "Invalid API key")
		return
	}
	// 检查平台：优先使用强制平台（/antigravity 路由），否则要求 gemini 分组
	forcePlatform, hasForcePlatform := middleware.GetForcePlatformFromGatewayContext(transportCtx)
	isYunwu := apiKeyIsYunwu(apiKey)
	if !hasForcePlatform && !isYunwu && (apiKey.Group == nil || apiKey.Group.Platform != service.PlatformGemini) {
		googleErrorContext(transportCtx, http.StatusBadRequest, "API key group platform is not gemini or yunwu")
		return
	}

	modelName := strings.TrimSpace(transportCtx.PathParam("model"))
	if modelName == "" {
		googleErrorContext(transportCtx, http.StatusBadRequest, "Missing model in URL")
		return
	}
	if !apiKeyAllowsRequestedModel(apiKey, modelName) {
		googleErrorContext(transportCtx, http.StatusBadRequest, apiKeyModelNotAllowedMessage(modelName))
		return
	}

	// 强制 antigravity 模式：返回 antigravity 模型信息
	if forcePlatform == service.PlatformAntigravity {
		transportCtx.WriteJSON(http.StatusOK, antigravity.FallbackGeminiModel(modelName))
		return
	}
	if !hasForcePlatform && isYunwu {
		h.forwardYunwuGeminiGET(transportCtx, "/v1beta/models/"+url.PathEscape(modelName))
		return
	}

	_, account, err := selectGeminiAIStudioAccountAcrossAPIKeyGroups(
		transportCtx.Context(),
		apiKey,
		h.geminiCompatService.SelectAccountForAIStudioEndpoints,
	)
	if err != nil {
		// 没有 gemini 账户，检查是否有 antigravity 账户可用
		hasAntigravity, _ := h.geminiCompatService.HasAntigravityAccounts(transportCtx.Context(), apiKey.GroupID)
		if hasAntigravity {
			// antigravity 账户使用静态模型信息
			transportCtx.WriteJSON(http.StatusOK, gemini.FallbackModel(modelName))
			return
		}
		googleErrorContext(transportCtx, http.StatusServiceUnavailable, "No available Gemini accounts: "+err.Error())
		return
	}

	res, err := h.geminiCompatService.ForwardAIStudioGET(transportCtx.Context(), account, "/v1beta/models/"+modelName)
	if err != nil {
		googleErrorContext(transportCtx, http.StatusBadGateway, err.Error())
		return
	}
	if shouldFallbackGeminiModels(res) {
		transportCtx.WriteJSON(http.StatusOK, gemini.FallbackModel(modelName))
		return
	}
	writeUpstreamResponseContext(transportCtx, res)
}

func (h *GatewayHandler) forwardYunwuGeminiGET(c gatewayctx.GatewayContext, path string) {
	if h.hypervisorEngine == nil || h.hypervisorEngine.YunwuEngine() == nil {
		googleErrorContext(c, http.StatusServiceUnavailable, "Yunwu hypervisor is not initialized")
		return
	}
	subject, ok := middleware.GetAuthSubjectFromGatewayContext(c)
	if !ok {
		googleErrorContext(c, http.StatusUnauthorized, "Invalid API key")
		return
	}
	if query := yunwuGeminiUpstreamQuery(c, nil); query != "" {
		path += "?" + query
	}
	requestCtx := hyperruntime.WithUpstreamRequest(c.Context(), http.MethodGet, path, "application/json")
	_, err := h.hypervisorEngine.HandleRequest(requestCtx, strconv.FormatInt(subject.UserID, 10), c.HeaderValue("X-Conversation-ID"), []byte(`{}`), hypervisorGatewayWriter{ctx: c}, service.PlatformYunwu)
	if err != nil {
		googleErrorContext(c, http.StatusBadGateway, err.Error())
	}
}

// GeminiV1BetaModels proxies Gemini native REST endpoints like:
// POST /v1beta/models/{model}:generateContent
// POST /v1beta/models/{model}:streamGenerateContent?alt=sse
func (h *GatewayHandler) GeminiV1BetaModels(c *gin.Context) {
	h.GeminiV1BetaModelsGateway(gatewayctx.FromGin(c))
}

// GeminiV1BetaOperation proxies polling/cancel/delete of long-running Gemini
// media operations (for example Veo video generation) through a Yunwu group.
func (h *GatewayHandler) GeminiV1BetaOperation(c *gin.Context) {
	h.GeminiV1BetaOperationGateway(gatewayctx.FromGin(c))
}

func (h *GatewayHandler) GeminiV1BetaOperationGateway(c gatewayctx.GatewayContext) {
	apiKey, ok := middleware.GetAPIKeyFromGatewayContext(c)
	if !ok || apiKey == nil || !apiKeyIsYunwu(apiKey) {
		googleErrorContext(c, http.StatusBadRequest, "A Yunwu group is required for Gemini media operations")
		return
	}
	if h.hypervisorEngine == nil || h.hypervisorEngine.YunwuEngine() == nil {
		googleErrorContext(c, http.StatusServiceUnavailable, "Yunwu hypervisor is not initialized")
		return
	}
	subject, ok := middleware.GetAuthSubjectFromGatewayContext(c)
	if !ok {
		googleErrorContext(c, http.StatusUnauthorized, "Invalid API key")
		return
	}
	path := c.Path()
	if !strings.HasPrefix(path, "/v1beta/operations/") {
		googleErrorContext(c, http.StatusBadRequest, "Invalid Gemini operation path")
		return
	}
	if query := yunwuGeminiUpstreamQuery(c, nil); query != "" {
		path += "?" + query
	}
	method := c.Method()
	if method != http.MethodGet && method != http.MethodDelete && method != http.MethodPost {
		googleErrorContext(c, http.StatusMethodNotAllowed, "Unsupported operation method")
		return
	}
	body := []byte(`{}`)
	if method == http.MethodPost {
		if requestBody, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request()); err == nil && len(requestBody) > 0 {
			body = requestBody
		}
	}
	requestCtx := hyperruntime.WithUpstreamRequest(c.Context(), method, path, "application/json")
	_, err := h.hypervisorEngine.HandleRequest(requestCtx, strconv.FormatInt(subject.UserID, 10), c.HeaderValue("X-Conversation-ID"), body, hypervisorGatewayWriter{ctx: c}, service.PlatformYunwu)
	if err != nil {
		googleErrorContext(c, http.StatusBadGateway, err.Error())
	}
}

// GeminiV1BetaFile proxies Gemini Files API upload/list/get/delete calls. This
// enables large image, audio, and video inputs without embedding base64 data in
// every generateContent request.
func (h *GatewayHandler) GeminiV1BetaFile(c *gin.Context) {
	h.GeminiV1BetaFileGateway(gatewayctx.FromGin(c))
}

func (h *GatewayHandler) GeminiV1BetaFileGateway(c gatewayctx.GatewayContext) {
	apiKey, ok := middleware.GetAPIKeyFromGatewayContext(c)
	if !ok || apiKey == nil || !apiKeyIsYunwu(apiKey) {
		googleErrorContext(c, http.StatusBadRequest, "A Yunwu group is required for Gemini Files API")
		return
	}
	if h.hypervisorEngine == nil || h.hypervisorEngine.YunwuEngine() == nil {
		googleErrorContext(c, http.StatusServiceUnavailable, "Yunwu hypervisor is not initialized")
		return
	}
	subject, ok := middleware.GetAuthSubjectFromGatewayContext(c)
	if !ok {
		googleErrorContext(c, http.StatusUnauthorized, "Invalid API key")
		return
	}
	path := c.Path()
	if !strings.HasPrefix(path, "/v1beta/files") && path != "/upload/v1beta/files" {
		googleErrorContext(c, http.StatusBadRequest, "Invalid Gemini file path")
		return
	}
	if query := yunwuGeminiUpstreamQuery(c, nil); query != "" {
		path += "?" + query
	}
	body := []byte(`{}`)
	if c.Method() == http.MethodPost {
		requestBody, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request())
		if err != nil {
			googleErrorContext(c, http.StatusBadRequest, "Failed to read upload body")
			return
		}
		body = requestBody
	}
	contentType := c.HeaderValue("Content-Type")
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/json"
	}
	requestCtx := hyperruntime.WithUpstreamRequest(c.Context(), c.Method(), path, contentType)
	requestCtx = hyperruntime.WithUpstreamRequestHeaders(requestCtx, c.Header())
	_, err := h.hypervisorEngine.HandleRequest(requestCtx, strconv.FormatInt(subject.UserID, 10), c.HeaderValue("X-Conversation-ID"), body, hypervisorGatewayWriter{ctx: c}, service.PlatformYunwu)
	if err != nil {
		googleErrorContext(c, http.StatusBadGateway, err.Error())
	}
}

func (h *GatewayHandler) GeminiV1BetaModelsGateway(transportCtx gatewayctx.GatewayContext) {
	apiKey, ok := middleware.GetAPIKeyFromGatewayContext(transportCtx)
	if !ok || apiKey == nil {
		googleErrorContext(transportCtx, http.StatusUnauthorized, "Invalid API key")
		return
	}
	authSubject, ok := middleware.GetAuthSubjectFromGatewayContext(transportCtx)
	if !ok {
		googleErrorContext(transportCtx, http.StatusInternalServerError, "User context not found")
		return
	}
	reqLog := requestLoggerContext(
		transportCtx,
		"handler.gemini_v1beta.models",
		zap.Int64("user_id", authSubject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
	)
	requestStart := time.Now()

	// 检查平台：Yunwu groups use the isolated balance-aware hypervisor while
	// preserving the native Gemini request/response schema byte-for-byte.
	useYunwu := false
	if !middleware.HasForcePlatformContext(transportCtx) {
		isYunwu := apiKeyIsYunwu(apiKey)
		if !isYunwu && (apiKey.Group == nil || apiKey.Group.Platform != service.PlatformGemini) {
			googleErrorContext(transportCtx, http.StatusBadRequest, "API key group platform is not gemini or yunwu")
			return
		}
		useYunwu = isYunwu
	}

	modelName, action, err := parseGeminiModelAction(strings.TrimPrefix(transportCtx.PathParam("modelAction"), "/"))
	if err != nil {
		googleErrorContext(transportCtx, http.StatusNotFound, err.Error())
		return
	}
	if !apiKeyAllowsRequestedModel(apiKey, modelName) {
		googleErrorContext(transportCtx, http.StatusBadRequest, apiKeyModelNotAllowedMessage(modelName))
		return
	}

	// The Yunwu dispatch happens before the shared validation block, so the
	// per-window spend limits must be enforced here explicitly.
	if err := checkHypervisorRequestGuards(transportCtx.Context(), h.billingCacheService, apiKey, modelName); err != nil {
		status, _, message := hypervisorGuardStatus(err)
		googleErrorContext(transportCtx, status, message)
		return
	}

	stream := action == "streamGenerateContent"
	reqLog = reqLog.With(zap.String("model", modelName), zap.String("action", action), zap.Bool("stream", stream))

	body, err := pkghttputil.ReadRequestBodyWithPrealloc(transportCtx.Request())
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			googleErrorContext(transportCtx, http.StatusRequestEntityTooLarge, buildBodyTooLargeMessage(maxErr.Limit))
			return
		}
		googleErrorContext(transportCtx, http.StatusBadRequest, "Failed to read request body")
		return
	}
	if len(body) == 0 {
		googleErrorContext(transportCtx, http.StatusBadRequest, "Request body is empty")
		return
	}

	setOpsRequestContextGateway(transportCtx, modelName, stream, body)
	setOpsEndpointContextGateway(transportCtx, "", int16(service.RequestTypeFromLegacy(stream, false)))

	if useYunwu {
		if h.hypervisorEngine == nil || h.hypervisorEngine.YunwuEngine() == nil {
			googleErrorContext(transportCtx, http.StatusServiceUnavailable, "Yunwu hypervisor is not initialized")
			return
		}
		if stream {
			transportCtx.SetHeader("Content-Type", "text/event-stream")
			transportCtx.SetHeader("Cache-Control", "no-cache, no-transform")
		} else {
			transportCtx.SetHeader("Content-Type", "application/json")
		}
		path := "/v1beta/models/" + url.PathEscape(modelName) + ":" + action
		updates := map[string]string{}
		if stream {
			updates["alt"] = "sse"
		}
		if query := yunwuGeminiUpstreamQuery(transportCtx, updates); query != "" {
			path += "?" + query
		}
		requestCtx := hyperruntime.WithUpstreamRequest(transportCtx.Context(), http.MethodPost, path, "application/json")
		requestCtx = service.WithYunwuModelRouting(requestCtx, apiKey.Group, modelName)
		conversationID := transportCtx.HeaderValue("X-Conversation-ID")
		meta, forwardErr := h.hypervisorEngine.HandleRequest(requestCtx, strconv.FormatInt(authSubject.UserID, 10), conversationID, body, hypervisorGatewayWriter{ctx: transportCtx}, service.PlatformYunwu)
		if forwardErr != nil {
			googleErrorContext(transportCtx, http.StatusBadGateway, forwardErr.Error())
			return
		}
		// Bill the end user for the completed Unofficial-provider request.
		if meta != nil && apiKey.User != nil {
			sub, _ := middleware.GetSubscriptionFromGatewayContext(transportCtx)
			// Billing must survive the client closing the connection after the
			// body was flushed (see detachedBillingContext).
			billingCtx, cancelBilling := detachedBillingContext(transportCtx.Context())
			defer cancelBilling()
			if meta.MediaCostMicroUSD > 0 {
				if billedErr := h.gatewayService.RecordYunwuMediaUsage(billingCtx, &service.RecordYunwuMediaUsageInput{
					APIKey:               apiKey,
					User:                 apiKey.User,
					AccountID:            meta.NodeID,
					Subscription:         sub,
					Model:                modelName,
					MediaKind:            meta.MediaUsageUnit,
					Units:                meta.MediaUnits,
					UpstreamCostMicroUSD: meta.MediaCostMicroUSD,
					UserAgent:            transportCtx.HeaderValue("User-Agent"),
					APIKeyService:        h.apiKeyService,
				}); billedErr != nil {
					reqLog.Error("gemini.yunwu_media_billing_failed", zap.Error(billedErr))
				}
			} else if meta.InputTokens > 0 || meta.OutputTokens > 0 {
				result := service.BuildYunwuForwardResult("", modelName, meta.InputTokens, meta.OutputTokens, stream, requestStart)
				if billedErr := h.gatewayService.RecordYunwuUsage(billingCtx, &service.RecordYunwuUsageInput{
					Result:        result,
					APIKey:        apiKey,
					User:          apiKey.User,
					AccountID:     meta.NodeID,
					Subscription:  sub,
					UserAgent:     transportCtx.HeaderValue("User-Agent"),
					APIKeyService: h.apiKeyService,
				}); billedErr != nil {
					reqLog.Error("gemini.yunwu_billing_failed", zap.Error(billedErr))
				}
			}
		}
		return
	}

	// Get subscription (may be nil)
	subscription, _ := middleware.GetSubscriptionFromGatewayContext(transportCtx)
	service.SetOpsLatencyMsContext(transportCtx, service.OpsAuthLatencyMsKey, time.Since(requestStart).Milliseconds())
	routingStart := time.Now()

	// For Gemini native API, do not send Claude-style ping frames.
	geminiConcurrency := NewConcurrencyHelper(h.concurrencyHelper.concurrencyService, SSEPingFormatNone, 0)

	// 0) wait queue check
	maxWait := service.CalculateMaxWait(authSubject.Concurrency)
	canWait, err := geminiConcurrency.IncrementWaitCount(transportCtx.Context(), authSubject.UserID, maxWait)
	waitCounted := false
	if err != nil {
		reqLog.Warn("gemini.user_wait_counter_increment_failed", zap.Error(err))
	} else if !canWait {
		reqLog.Info("gemini.user_wait_queue_full", zap.Int("max_wait", maxWait))
		googleErrorContext(transportCtx, http.StatusTooManyRequests, "Too many pending requests, please retry later")
		return
	}
	if err == nil && canWait {
		waitCounted = true
	}
	defer func() {
		if waitCounted {
			geminiConcurrency.DecrementWaitCount(transportCtx.Context(), authSubject.UserID)
		}
	}()

	// 1) user concurrency slot
	streamStarted := false
	if h.errorPassthroughService != nil {
		service.BindErrorPassthroughServiceContext(transportCtx, h.errorPassthroughService)
	}
	userReleaseFunc, err := geminiConcurrency.AcquireUserSlotWithWaitContext(transportCtx, authSubject.UserID, authSubject.Concurrency, stream, &streamStarted)
	if err != nil {
		reqLog.Warn("gemini.user_slot_acquire_failed", zap.Error(err))
		googleErrorContext(transportCtx, http.StatusTooManyRequests, err.Error())
		return
	}
	if waitCounted {
		geminiConcurrency.DecrementWaitCount(transportCtx.Context(), authSubject.UserID)
		waitCounted = false
	}
	// 确保请求取消时也会释放槽位，避免长连接被动中断造成泄漏
	userReleaseFunc = wrapReleaseOnDone(transportCtx.Context(), userReleaseFunc)
	if userReleaseFunc != nil {
		defer userReleaseFunc()
	}

	// 2) billing eligibility check (after wait)
	if err := h.billingCacheService.CheckBillingEligibility(transportCtx.Context(), apiKey.User, apiKey, apiKey.Group, subscription); err != nil {
		reqLog.Info("gemini.billing_eligibility_check_failed", zap.Error(err))
		status, _, message := billingErrorDetails(err)
		googleErrorContext(transportCtx, status, message)
		return
	}

	// 3) select account (sticky session based on request body)
	// 优先使用 Gemini CLI 的会话标识（privileged-user-id + tmp 目录哈希）
	sessionHash := extractGeminiCLISessionHashContext(transportCtx, body)
	if sessionHash == "" {
		// Fallback: 使用通用的会话哈希生成逻辑（适用于其他客户端）
		parsedReq, _ := service.ParseGatewayRequest(body, domain.PlatformGemini)
		if parsedReq != nil {
			parsedReq.SessionContext = buildGatewaySessionContextContext(transportCtx, apiKey.ID)
		}
		sessionHash = h.gatewayService.GenerateSessionHash(parsedReq)
	}
	sessionKey := sessionHash
	if sessionHash != "" {
		sessionKey = "gemini:" + sessionHash
	}

	// 查询粘性会话绑定的账号 ID（用于检测账号切换）
	var sessionBoundAccountID int64
	if sessionKey != "" {
		sessionBoundAccountID, _ = h.gatewayService.GetCachedSessionAccountID(transportCtx.Context(), apiKey.GroupID, sessionKey)
		if sessionBoundAccountID > 0 {
			prefetchedGroupID := int64(0)
			if apiKey.GroupID != nil {
				prefetchedGroupID = *apiKey.GroupID
			}
			ctx := service.WithPrefetchedStickySession(transportCtx.Context(), sessionBoundAccountID, prefetchedGroupID, h.metadataBridgeEnabled())
			if req := transportCtx.Request(); req != nil {
				transportCtx.SetRequest(req.WithContext(ctx))
			}
		}
	}

	// === Gemini 内容摘要会话 Fallback 逻辑 ===
	// 当原有会话标识无效时（sessionBoundAccountID == 0），尝试基于内容摘要链匹配
	var geminiDigestChain string
	var geminiPrefixHash string
	var geminiSessionUUID string
	var matchedDigestChain string
	useDigestFallback := sessionBoundAccountID == 0

	if useDigestFallback {
		// 解析 Gemini 请求体
		var geminiReq antigravity.GeminiRequest
		if err := json.Unmarshal(body, &geminiReq); err == nil && len(geminiReq.Contents) > 0 {
			// 生成摘要链
			geminiDigestChain = service.BuildGeminiDigestChain(&geminiReq)
			if geminiDigestChain != "" {
				// 生成前缀 hash
				userAgent := strings.TrimSpace(transportCtx.HeaderValue("User-Agent"))
				clientIP := strings.TrimSpace(transportCtx.ClientIP())
				platform := ""
				if apiKey.Group != nil {
					platform = apiKey.Group.Platform
				}
				geminiPrefixHash = service.GenerateGeminiPrefixHash(
					authSubject.UserID,
					apiKey.ID,
					clientIP,
					userAgent,
					platform,
					modelName,
				)

				// 查找会话
				foundUUID, foundAccountID, foundMatchedChain, found := h.gatewayService.FindGeminiSession(
					transportCtx.Context(),
					derefGroupID(apiKey.GroupID),
					geminiPrefixHash,
					geminiDigestChain,
				)
				if found {
					matchedDigestChain = foundMatchedChain
					sessionBoundAccountID = foundAccountID
					geminiSessionUUID = foundUUID
					reqLog.Info("gemini.digest_fallback_matched",
						zap.String("session_uuid_prefix", safeShortPrefix(foundUUID, 8)),
						zap.Int64("account_id", foundAccountID),
						zap.String("digest_chain", truncateDigestChain(geminiDigestChain)),
					)

					// 关键：如果原 sessionKey 为空，使用 prefixHash + uuid 作为 sessionKey
					// 这样 SelectAccountWithLoadAwareness 的粘性会话逻辑会优先使用匹配到的账号
					if sessionKey == "" {
						sessionKey = service.GenerateGeminiDigestSessionKey(geminiPrefixHash, foundUUID)
					}
					_ = h.gatewayService.BindStickySession(transportCtx.Context(), apiKey.GroupID, sessionKey, foundAccountID)
				} else {
					// 生成新的会话 UUID
					geminiSessionUUID = uuid.New().String()
					// 为新会话也生成 sessionKey（用于后续请求的粘性会话）
					if sessionKey == "" {
						sessionKey = service.GenerateGeminiDigestSessionKey(geminiPrefixHash, geminiSessionUUID)
					}
				}
			}
		}
	}

	// 判断是否真的绑定了粘性会话：有 sessionKey 且已经绑定到某个账号
	hasBoundSession := sessionKey != "" && sessionBoundAccountID > 0
	cleanedForUnknownBinding := false

	fs := NewFailoverState(h.maxAccountSwitchesGemini, hasBoundSession)

	// 单账号分组提前设置 SingleAccountRetry 标记，让 Service 层首次 503 就不设模型限流标记。
	// 避免单账号分组收到 503 (MODEL_CAPACITY_EXHAUSTED) 时设 29s 限流，导致后续请求连续快速失败。
	if h.gatewayService.IsSingleAntigravityAccountGroup(transportCtx.Context(), apiKey.GroupID) {
		ctx := service.WithSingleAccountRetry(transportCtx.Context(), true, h.metadataBridgeEnabled())
		if req := transportCtx.Request(); req != nil {
			transportCtx.SetRequest(req.WithContext(ctx))
		}
	}

	for {
		groupSelection, err := selectGatewayAPIKeyGroup(
			transportCtx.Context(),
			apiKey,
			sessionKey,
			modelName,
			fs.FailedAccountIDs,
			"",
			h.gatewayService.SelectAccountWithLoadAwareness,
		) // Gemini 不使用会话限制
		if err != nil {
			if len(fs.FailedAccountIDs) == 0 {
				googleErrorContext(transportCtx, http.StatusServiceUnavailable, "No available Gemini accounts: "+err.Error())
				return
			}
			action := fs.HandleSelectionExhausted(transportCtx.Context())
			switch action {
			case FailoverContinue:
				ctx := service.WithSingleAccountRetry(transportCtx.Context(), true, h.metadataBridgeEnabled())
				if req := transportCtx.Request(); req != nil {
					transportCtx.SetRequest(req.WithContext(ctx))
				}
				continue
			case FailoverCanceled:
				return
			default: // FailoverExhausted
				h.handleGeminiFailoverExhaustedContext(transportCtx, fs.LastFailoverErr)
				return
			}
		}
		selectedAPIKey := groupSelection.APIKey
		selection := groupSelection.Selection
		account := selection.Account
		setOpsSelectedAccountGateway(transportCtx, account.ID, account.Platform)

		// 检测账号切换：如果粘性会话绑定的账号与当前选择的账号不同，清除 thoughtSignature
		// 注意：Gemini 原生 API 的 thoughtSignature 与具体上游账号强相关；跨账号透传会导致 400。
		if sessionBoundAccountID > 0 && sessionBoundAccountID != account.ID {
			reqLog.Info("gemini.sticky_session_account_switched",
				zap.Int64("from_account_id", sessionBoundAccountID),
				zap.Int64("to_account_id", account.ID),
				zap.Bool("clean_thought_signature", true),
			)
			body = service.CleanGeminiNativeThoughtSignatures(body)
			sessionBoundAccountID = account.ID
		} else if sessionKey != "" && sessionBoundAccountID == 0 && !cleanedForUnknownBinding && bytes.Contains(body, []byte(`"thoughtSignature"`)) {
			// 无缓存绑定但请求里已有 thoughtSignature：常见于缓存丢失/TTL 过期后，客户端继续携带旧签名。
			// 为避免第一次转发就 400，这里做一次确定性清理，让新账号重新生成签名链路。
			reqLog.Info("gemini.sticky_session_binding_missing",
				zap.Bool("clean_thought_signature", true),
			)
			body = service.CleanGeminiNativeThoughtSignatures(body)
			cleanedForUnknownBinding = true
			sessionBoundAccountID = account.ID
		} else if sessionBoundAccountID == 0 {
			// 记录本次请求中首次选择到的账号，便于同一请求内 failover 时检测切换。
			sessionBoundAccountID = account.ID
		}

		// 4) account concurrency slot
		accountReleaseFunc := selection.ReleaseFunc
		if !selection.Acquired {
			if selection.WaitPlan == nil {
				googleErrorContext(transportCtx, http.StatusServiceUnavailable, "No available Gemini accounts")
				return
			}
			accountWaitCounted := false
			canWait, err := geminiConcurrency.IncrementAccountWaitCount(transportCtx.Context(), account.ID, selection.WaitPlan.MaxWaiting)
			if err != nil {
				reqLog.Warn("gemini.account_wait_counter_increment_failed", zap.Int64("account_id", account.ID), zap.Error(err))
			} else if !canWait {
				reqLog.Info("gemini.account_wait_queue_full",
					zap.Int64("account_id", account.ID),
					zap.Int("max_waiting", selection.WaitPlan.MaxWaiting),
				)
				googleErrorContext(transportCtx, http.StatusTooManyRequests, "Too many pending requests, please retry later")
				return
			}
			if err == nil && canWait {
				accountWaitCounted = true
			}
			defer func() {
				if accountWaitCounted {
					geminiConcurrency.DecrementAccountWaitCount(transportCtx.Context(), account.ID)
				}
			}()

			accountReleaseFunc, err = geminiConcurrency.AcquireAccountSlotWithWaitTimeoutContext(
				transportCtx,
				account.ID,
				selection.WaitPlan.MaxConcurrency,
				selection.WaitPlan.Timeout,
				stream,
				&streamStarted,
			)
			if err != nil {
				reqLog.Warn("gemini.account_slot_acquire_failed", zap.Int64("account_id", account.ID), zap.Error(err))
				googleErrorContext(transportCtx, http.StatusTooManyRequests, err.Error())
				return
			}
			if accountWaitCounted {
				geminiConcurrency.DecrementAccountWaitCount(transportCtx.Context(), account.ID)
				accountWaitCounted = false
			}
			if err := h.gatewayService.BindStickySession(transportCtx.Context(), selectedAPIKey.GroupID, sessionKey, account.ID); err != nil {
				reqLog.Warn("gemini.bind_sticky_session_failed", zap.Int64("account_id", account.ID), zap.Error(err))
			}
		}
		// 账号槽位/等待计数需要在超时或断开时安全回收
		accountReleaseFunc = wrapReleaseOnDone(transportCtx.Context(), accountReleaseFunc)

		// 5) forward (根据平台分流)
		var result *service.ForwardResult
		requestCtx := transportCtx.Context()
		if fs.SwitchCount > 0 {
			requestCtx = service.WithAccountSwitchCount(requestCtx, fs.SwitchCount, h.metadataBridgeEnabled())
		}
		service.SetOpsLatencyMsContext(transportCtx, service.OpsRoutingLatencyMsKey, time.Since(routingStart).Milliseconds())
		forwardStart := time.Now()
		if account.Platform == service.PlatformAntigravity && account.Type != service.AccountTypeAPIKey {
			result, err = h.antigravityGatewayService.ForwardGeminiContext(requestCtx, transportCtx, account, modelName, action, stream, body, hasBoundSession)
		} else {
			result, err = h.geminiCompatService.ForwardNativeContext(requestCtx, transportCtx, account, modelName, action, stream, body)
		}
		forwardDurationMs := time.Since(forwardStart).Milliseconds()
		if accountReleaseFunc != nil {
			accountReleaseFunc()
		}
		service.SetOpsLatencyMsContext(transportCtx, service.OpsResponseLatencyMsKey, forwardDurationMs)
		if err == nil && result != nil && result.FirstTokenMs != nil {
			service.SetOpsLatencyMsContext(transportCtx, service.OpsTimeToFirstTokenMsKey, int64(*result.FirstTokenMs))
		}
		if err != nil {
			var failoverErr *service.UpstreamFailoverError
			if errors.As(err, &failoverErr) {
				h.gatewayService.ReportGatewayAccountScheduleResult(account.ID, false, nil)
				h.gatewayService.RecordGatewayAccountSwitch(account.ID)
				failoverAction := fs.HandleFailoverError(transportCtx.Context(), h.gatewayService, account.ID, account.Platform, failoverErr)
				switch failoverAction {
				case FailoverContinue:
					continue
				case FailoverExhausted:
					h.handleGeminiFailoverExhaustedContext(transportCtx, fs.LastFailoverErr)
					return
				case FailoverCanceled:
					return
				}
			}
			h.gatewayService.ReportGatewayAccountScheduleResult(account.ID, false, nil)
			// ForwardNative already wrote the response
			reqLog.Error("gemini.forward_failed", zap.Int64("account_id", account.ID), zap.Error(err))
			return
		}
		if result != nil && result.FirstTokenMs != nil {
			h.gatewayService.ReportGatewayAccountScheduleResult(account.ID, true, result.FirstTokenMs)
		} else {
			h.gatewayService.ReportGatewayAccountScheduleResult(account.ID, true, nil)
		}

		// 捕获请求信息（用于异步记录，避免在 goroutine 中访问 gin.Context）
		userAgent := strings.TrimSpace(transportCtx.HeaderValue("User-Agent"))
		clientIP := strings.TrimSpace(transportCtx.ClientIP())

		// 保存 Gemini 内容摘要会话（用于 Fallback 匹配）
		if useDigestFallback && geminiDigestChain != "" && geminiPrefixHash != "" {
			if err := h.gatewayService.SaveGeminiSession(
				transportCtx.Context(),
				derefGroupID(selectedAPIKey.GroupID),
				geminiPrefixHash,
				geminiDigestChain,
				geminiSessionUUID,
				account.ID,
				matchedDigestChain,
			); err != nil {
				reqLog.Warn("gemini.digest_session_save_failed", zap.Int64("account_id", account.ID), zap.Error(err))
			}
		}

		// 使用量记录通过有界 worker 池提交，避免请求热路径创建无界 goroutine。
		requestPayloadHash := service.HashUsageRequestPayload(body)
		inboundEndpoint := GetInboundEndpointContext(transportCtx)
		upstreamEndpoint := GetUpstreamEndpointContext(transportCtx, account.Platform)
		h.submitUsageRecordTask(func(ctx context.Context) {
			if err := h.gatewayService.RecordUsageWithLongContext(ctx, &service.RecordUsageLongContextInput{
				Result:                result,
				APIKey:                selectedAPIKey,
				User:                  selectedAPIKey.User,
				Account:               account,
				Subscription:          subscription,
				InboundEndpoint:       inboundEndpoint,
				UpstreamEndpoint:      upstreamEndpoint,
				UserAgent:             userAgent,
				IPAddress:             clientIP,
				RequestPayloadHash:    requestPayloadHash,
				LongContextThreshold:  200000, // Gemini 200K 阈值
				LongContextMultiplier: 2.0,    // 超出部分双倍计费
				ForceCacheBilling:     fs.ForceCacheBilling,
				APIKeyService:         h.apiKeyService,
			}); err != nil {
				logger.L().With(
					zap.String("component", "handler.gemini_v1beta.models"),
					zap.Int64("user_id", authSubject.UserID),
					zap.Int64("api_key_id", selectedAPIKey.ID),
					zap.Any("group_id", selectedAPIKey.GroupID),
					zap.String("model", modelName),
					zap.Int64("account_id", account.ID),
				).Error("gemini.record_usage_failed", zap.Error(err))
			}
		})
		reqLog.Debug("gemini.request_completed",
			zap.Int64("account_id", account.ID),
			zap.Int("switch_count", fs.SwitchCount),
		)
		return
	}
}

func yunwuGeminiUpstreamQuery(c gatewayctx.GatewayContext, updates map[string]string) string {
	query := c.Request().URL.Query()
	// The downstream service key authenticates this gateway and must never be
	// disclosed to a Yunwu vendor. Upstream authentication uses node credentials.
	query.Del("key")
	for key, value := range updates {
		query.Set(key, value)
	}
	return query.Encode()
}

func parseGeminiModelAction(rest string) (model string, action string, err error) {
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return "", "", &pathParseError{"missing path"}
	}

	// Standard: {model}:{action}
	if i := strings.Index(rest, ":"); i > 0 && i < len(rest)-1 {
		return rest[:i], rest[i+1:], nil
	}

	// Fallback: {model}/{action}
	if i := strings.Index(rest, "/"); i > 0 && i < len(rest)-1 {
		return rest[:i], rest[i+1:], nil
	}

	return "", "", &pathParseError{"invalid model action path"}
}

func (h *GatewayHandler) handleGeminiFailoverExhausted(c *gin.Context, failoverErr *service.UpstreamFailoverError) {
	h.handleGeminiFailoverExhaustedContext(gatewayctx.FromGin(c), failoverErr)
}

func (h *GatewayHandler) handleGeminiFailoverExhaustedContext(c gatewayctx.GatewayContext, failoverErr *service.UpstreamFailoverError) {
	if failoverErr == nil {
		googleErrorContext(c, http.StatusBadGateway, "Upstream request failed")
		return
	}

	statusCode := failoverErr.StatusCode
	responseBody := failoverErr.ResponseBody

	// 先检查透传规则
	if h.errorPassthroughService != nil && len(responseBody) > 0 {
		if rule := h.errorPassthroughService.MatchRule(service.PlatformGemini, statusCode, responseBody); rule != nil {
			// 确定响应状态码
			respCode := statusCode
			if !rule.PassthroughCode && rule.ResponseCode != nil {
				respCode = *rule.ResponseCode
			}

			// 确定响应消息
			msg := service.ExtractUpstreamErrorMessage(responseBody)
			if !rule.PassthroughBody && rule.CustomMessage != nil {
				msg = *rule.CustomMessage
			}

			if rule.SkipMonitoring {
				c.SetValue(service.OpsSkipPassthroughKey, true)
			}

			googleErrorContext(c, respCode, msg)
			return
		}
	}

	// 记录原始上游状态码，以便 ops 错误日志捕获真实的上游错误
	upstreamMsg := service.ExtractUpstreamErrorMessage(responseBody)
	service.SetOpsUpstreamErrorContext(c, statusCode, upstreamMsg, "")

	// 使用默认的错误映射
	status, message := mapGeminiUpstreamError(statusCode)
	googleErrorContext(c, status, message)
}

func mapGeminiUpstreamError(statusCode int) (int, string) {
	switch statusCode {
	case 401:
		return http.StatusBadGateway, "Upstream authentication failed, please contact administrator"
	case 403:
		return http.StatusBadGateway, "Upstream access forbidden, please contact administrator"
	case 429:
		return http.StatusTooManyRequests, "Upstream rate limit exceeded, please retry later"
	case 529:
		return http.StatusServiceUnavailable, "Upstream service overloaded, please retry later"
	case 500, 502, 503, 504:
		return http.StatusBadGateway, "Upstream service temporarily unavailable"
	default:
		return http.StatusBadGateway, "Upstream request failed"
	}
}

type pathParseError struct{ msg string }

func (e *pathParseError) Error() string { return e.msg }

func googleError(c *gin.Context, status int, message string) {
	googleErrorContext(gatewayctx.FromGin(c), status, message)
}

func googleErrorContext(c gatewayctx.GatewayContext, status int, message string) {
	if c == nil {
		return
	}
	c.WriteJSON(status, gin.H{
		"error": gin.H{
			"code":    status,
			"message": message,
			"status":  googleapi.HTTPStatusToGoogleStatus(status),
		},
	})
}

func writeUpstreamResponse(c *gin.Context, res *service.UpstreamHTTPResult) {
	writeUpstreamResponseContext(gatewayctx.FromGin(c), res)
}

func writeUpstreamResponseContext(c gatewayctx.GatewayContext, res *service.UpstreamHTTPResult) {
	if res == nil {
		googleErrorContext(c, http.StatusBadGateway, "Empty upstream response")
		return
	}
	for k, vv := range res.Headers {
		// Avoid overriding content-length and hop-by-hop headers.
		if strings.EqualFold(k, "Content-Length") || strings.EqualFold(k, "Transfer-Encoding") || strings.EqualFold(k, "Connection") {
			continue
		}
		for _, v := range vv {
			c.Header().Add(k, v)
		}
	}
	contentType := res.Headers.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	c.SetHeader("Content-Type", contentType)
	_, _ = c.WriteBytes(res.StatusCode, res.Body)
}

func shouldFallbackGeminiModels(res *service.UpstreamHTTPResult) bool {
	if res == nil {
		return true
	}
	if res.StatusCode != http.StatusUnauthorized && res.StatusCode != http.StatusForbidden {
		return false
	}
	if strings.Contains(strings.ToLower(res.Headers.Get("Www-Authenticate")), "insufficient_scope") {
		return true
	}
	if strings.Contains(strings.ToLower(string(res.Body)), "insufficient authentication scopes") {
		return true
	}
	if strings.Contains(strings.ToLower(string(res.Body)), "access_token_scope_insufficient") {
		return true
	}
	return false
}

// extractGeminiCLISessionHash 从 Gemini CLI 请求中提取会话标识。
// 组合 x-gemini-api-privileged-user-id header 和请求体中的 tmp 目录哈希。
//
// 会话标识生成策略：
//  1. 从请求体中提取 tmp 目录哈希（64位十六进制）
//  2. 从 header 中提取 privileged-user-id（UUID）
//  3. 组合两者生成 SHA256 哈希作为最终的会话标识
//
// 如果找不到 tmp 目录哈希，返回空字符串（不使用粘性会话）。
//
// extractGeminiCLISessionHash extracts session identifier from Gemini CLI requests.
// Combines x-gemini-api-privileged-user-id header with tmp directory hash from request body.
func extractGeminiCLISessionHash(c *gin.Context, body []byte) string {
	return extractGeminiCLISessionHashContext(gatewayctx.FromGin(c), body)
}

func extractGeminiCLISessionHashContext(c gatewayctx.GatewayContext, body []byte) string {
	// 1. 从请求体中提取 tmp 目录哈希
	match := geminiCLITmpDirRegex.FindSubmatch(body)
	if len(match) < 2 {
		return "" // 没有找到 tmp 目录，不使用粘性会话
	}
	tmpDirHash := string(match[1])

	// 2. 提取 privileged-user-id
	privilegedUserID := strings.TrimSpace(c.HeaderValue("x-gemini-api-privileged-user-id"))

	// 3. 组合生成最终的 session hash
	if privilegedUserID != "" {
		// 组合两个标识符：privileged-user-id + tmp 目录哈希
		combined := privilegedUserID + ":" + tmpDirHash
		hash := sha256.Sum256([]byte(combined))
		return hex.EncodeToString(hash[:])
	}

	// 如果没有 privileged-user-id，直接使用 tmp 目录哈希
	return tmpDirHash
}

// truncateDigestChain 截断摘要链用于日志显示
func truncateDigestChain(chain string) string {
	if len(chain) <= 50 {
		return chain
	}
	return chain[:50] + "..."
}

// safeShortPrefix 返回字符串前 n 个字符；长度不足时返回原字符串。
// 用于日志展示，避免切片越界。
func safeShortPrefix(value string, n int) string {
	if n <= 0 || len(value) <= n {
		return value
	}
	return value[:n]
}

// derefGroupID 安全解引用 *int64，nil 返回 0
func derefGroupID(groupID *int64) int64 {
	if groupID == nil {
		return 0
	}
	return *groupID
}

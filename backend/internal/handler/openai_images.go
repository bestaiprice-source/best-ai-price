package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/server/gatewayctx"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	hyperruntime "github.com/Wei-Shaw/sub2api/pkg/hypervisor/runtime"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"go.uber.org/zap"
)

// Images handles OpenAI Images API requests.
// POST /v1/images/generations
// POST /v1/images/edits
func (h *OpenAIGatewayHandler) Images(c *gin.Context) {
	h.ImagesGateway(gatewayctx.FromGin(c))
}

func (h *OpenAIGatewayHandler) ImagesGateway(c gatewayctx.GatewayContext) {
	streamStarted := false
	defer h.recoverResponsesPanicContext(c, &streamStarted)

	requestStart := time.Now()

	apiKey, ok := middleware2.GetAPIKeyFromGatewayContext(c)
	if !ok {
		h.errorResponseGateway(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}

	subject, ok := middleware2.GetAuthSubjectFromGatewayContext(c)
	if !ok {
		h.errorResponseGateway(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}

	reqLog := requestLoggerContext(
		c,
		"handler.openai_gateway.images",
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
	)
	if !h.ensureResponsesDependenciesGateway(c, reqLog) {
		return
	}

	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request())
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			h.errorResponseGateway(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
			return
		}
		h.errorResponseGateway(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}
	if len(body) == 0 {
		h.errorResponseGateway(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return
	}

	if apiKey.Group != nil && apiKey.Group.Platform == service.PlatformYunwu {
		if h.hypervisorEngine == nil || h.hypervisorEngine.YunwuEngine() == nil {
			h.errorResponseGateway(c, http.StatusServiceUnavailable, "api_error", "Yunwu hypervisor is not initialized")
			return
		}
		path := c.Path()
		if path != "/v1/images/generations" && path != "/images/generations" && path != "/v1/images/edits" && path != "/images/edits" {
			h.errorResponseGateway(c, http.StatusNotFound, "invalid_request_error", "unsupported Yunwu image endpoint")
			return
		}
		if !strings.HasPrefix(path, "/v1/") {
			path = "/v1" + path
		}
		contentType := c.HeaderValue("Content-Type")
		if strings.TrimSpace(contentType) == "" {
			contentType = "application/json"
		}
		if geminiBody, geminiModel, ok, err := buildYunwuGeminiImageGenerationRequest(body, contentType, path); err != nil {
			h.errorResponseGateway(c, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		} else if ok {
			requestCtx := hyperruntime.WithUpstreamRequest(c.Context(), http.MethodPost, "/v1beta/models/"+url.PathEscape(geminiModel)+":generateContent", "application/json")
			var upstream bufferedHypervisorResponseWriter
			_, forwardErr := h.hypervisorEngine.HandleRequest(requestCtx, strconv.FormatInt(subject.UserID, 10), c.HeaderValue("X-Conversation-ID"), geminiBody, &upstream, service.PlatformYunwu)
			if forwardErr != nil {
				h.errorResponseGateway(c, http.StatusBadGateway, "api_error", forwardErr.Error())
				return
			}
			response, err := openAIImagesResponseFromGemini(upstream.Bytes())
			if err != nil {
				h.errorResponseGateway(c, http.StatusBadGateway, "api_error", err.Error())
				return
			}
			c.SetHeader("Content-Type", "application/json")
			c.WriteJSON(http.StatusOK, response)
			return
		}
		forwardBody, err := normalizeYunwuOpenAIImagesRequest(body, contentType)
		if err != nil {
			h.errorResponseGateway(c, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
		if rawQuery := c.Request().URL.RawQuery; rawQuery != "" {
			path += "?" + rawQuery
		}
		requestCtx := hyperruntime.WithUpstreamRequest(c.Context(), http.MethodPost, path, contentType)
		conversationID := c.HeaderValue("X-Conversation-ID")
		_, forwardErr := h.hypervisorEngine.HandleRequest(requestCtx, strconv.FormatInt(subject.UserID, 10), conversationID, forwardBody, hypervisorGatewayWriter{ctx: c}, service.PlatformYunwu)
		if forwardErr != nil {
			h.errorResponseGateway(c, http.StatusBadGateway, "api_error", forwardErr.Error())
		}
		return
	}

	if isMultipartImagesContentType(c.HeaderValue("Content-Type")) {
		setOpsRequestContextGateway(c, "", false, nil)
	} else {
		setOpsRequestContextGateway(c, "", false, body)
	}

	parsed, err := h.gatewayService.ParseOpenAIImagesRequestContext(c, body)
	if err != nil {
		h.errorResponseGateway(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	reqLog = reqLog.With(
		zap.String("model", parsed.Model),
		zap.Bool("stream", parsed.Stream),
		zap.Bool("multipart", parsed.Multipart),
		zap.String("capability", string(parsed.RequiredCapability)),
	)
	if !apiKeyAllowsRequestedModel(apiKey, parsed.Model) {
		h.errorResponseGateway(c, http.StatusBadRequest, "invalid_request_error", apiKeyModelNotAllowedMessage(parsed.Model))
		return
	}

	if parsed.Multipart {
		setOpsRequestContextGateway(c, parsed.Model, parsed.Stream, nil)
	} else {
		setOpsRequestContextGateway(c, parsed.Model, parsed.Stream, body)
	}
	setOpsEndpointContextGateway(c, "", int16(service.RequestTypeFromLegacy(parsed.Stream, false)))

	if h.errorPassthroughService != nil {
		service.BindErrorPassthroughServiceContext(c, h.errorPassthroughService)
	}

	subscription, _ := middleware2.GetSubscriptionFromGatewayContext(c)
	service.SetOpsLatencyMsContext(c, service.OpsAuthLatencyMsKey, time.Since(requestStart).Milliseconds())
	routingStart := time.Now()

	userReleaseFunc, acquired := h.acquireResponsesUserSlotContext(c, subject.UserID, subject.Concurrency, parsed.Stream, &streamStarted, reqLog)
	if !acquired {
		return
	}
	if userReleaseFunc != nil {
		defer userReleaseFunc()
	}

	if err := h.billingCacheService.CheckBillingEligibility(c.Context(), apiKey.User, apiKey, apiKey.Group, subscription); err != nil {
		reqLog.Info("openai.images.billing_eligibility_check_failed", zap.Error(err))
		status, code, message := billingErrorDetails(err)
		h.handleStreamingAwareErrorContext(c, status, code, message, streamStarted)
		return
	}

	sessionHash := h.gatewayService.GenerateSessionHashContext(c, body)
	if parsed.Multipart {
		sessionHash = h.gatewayService.GenerateSessionHashWithFallbackContext(c, nil, parsed.StickySessionSeed())
	}

	maxAccountSwitches := h.maxAccountSwitches
	switchCount := 0
	failedAccountIDs := make(map[int64]struct{})
	sameAccountRetryCount := make(map[int64]int)
	var lastFailoverErr *service.UpstreamFailoverError

	for {
		reqLog.Debug("openai.images.account_selecting", zap.Int("excluded_account_count", len(failedAccountIDs)))
		groupSelection, err := selectOpenAIAPIKeyGroup(
			c.Context(),
			apiKey,
			"",
			sessionHash,
			parsed.Model,
			failedAccountIDs,
			service.OpenAIUpstreamTransportHTTPSSE,
			h.gatewayService.SelectAccountWithScheduler,
		)
		var (
			selection      *service.AccountSelectionResult
			selectedAPIKey = apiKey
		)
		if err == nil && groupSelection != nil {
			selection = groupSelection.Selection
			selectedAPIKey = groupSelection.APIKey
		}
		if err != nil {
			reqLog.Warn("openai.images.account_select_failed", zap.Error(err), zap.Int("excluded_account_count", len(failedAccountIDs)))
			if lastFailoverErr != nil {
				h.handleFailoverExhaustedContext(c, lastFailoverErr, streamStarted)
			} else {
				h.handleStreamingAwareErrorContext(c, http.StatusServiceUnavailable, "api_error", "No available compatible accounts", streamStarted)
			}
			return
		}
		if selection == nil || selection.Account == nil {
			h.handleStreamingAwareErrorContext(c, http.StatusServiceUnavailable, "api_error", "No available compatible accounts", streamStarted)
			return
		}

		account := selection.Account
		if !account.SupportsOpenAIImageCapability(parsed.RequiredCapability) {
			failedAccountIDs[account.ID] = struct{}{}
			reqLog.Info("openai.images.account_incompatible",
				zap.Int64("account_id", account.ID),
				zap.String("account_type", account.Type),
				zap.String("capability", string(parsed.RequiredCapability)),
			)
			continue
		}

		sessionHash = ensureOpenAIPoolModeSessionHash(sessionHash, account)
		reqLog.Debug("openai.images.account_selected", zap.Int64("account_id", account.ID), zap.String("account_name", account.Name))
		setOpsSelectedAccountGateway(c, account.ID, account.Platform)

		accountReleaseFunc, acquired := h.acquireResponsesAccountSlotContext(c, selectedAPIKey.GroupID, sessionHash, selection, parsed.Stream, &streamStarted, reqLog)
		if !acquired {
			return
		}

		service.SetOpsLatencyMsContext(c, service.OpsRoutingLatencyMsKey, time.Since(routingStart).Milliseconds())
		forwardStart := time.Now()
		result, err := h.gatewayService.ForwardImagesContext(c.Context(), c, account, body, parsed, "")
		forwardDurationMs := time.Since(forwardStart).Milliseconds()
		if accountReleaseFunc != nil {
			accountReleaseFunc()
		}
		upstreamLatencyMs, _ := getContextInt64Gateway(c, service.OpsUpstreamLatencyMsKey)
		responseLatencyMs := forwardDurationMs
		if upstreamLatencyMs > 0 && forwardDurationMs > upstreamLatencyMs {
			responseLatencyMs = forwardDurationMs - upstreamLatencyMs
		}
		service.SetOpsLatencyMsContext(c, service.OpsResponseLatencyMsKey, responseLatencyMs)
		if err == nil && result != nil && result.FirstTokenMs != nil {
			service.SetOpsLatencyMsContext(c, service.OpsTimeToFirstTokenMsKey, int64(*result.FirstTokenMs))
		}
		if err != nil {
			var failoverErr *service.UpstreamFailoverError
			if errors.As(err, &failoverErr) {
				h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, false, nil)
				if failoverErr.RetryableOnSameAccount {
					retryLimit := account.GetPoolModeRetryCount()
					if sameAccountRetryCount[account.ID] < retryLimit {
						sameAccountRetryCount[account.ID]++
						reqLog.Warn("openai.images.pool_mode_same_account_retry",
							zap.Int64("account_id", account.ID),
							zap.Int("upstream_status", failoverErr.StatusCode),
							zap.Int("retry_limit", retryLimit),
							zap.Int("retry_count", sameAccountRetryCount[account.ID]),
						)
						select {
						case <-c.Context().Done():
							return
						case <-time.After(sameAccountRetryDelay):
						}
						continue
					}
				}
				h.gatewayService.RecordOpenAIAccountSwitch()
				failedAccountIDs[account.ID] = struct{}{}
				lastFailoverErr = failoverErr
				if switchCount >= maxAccountSwitches {
					h.handleFailoverExhaustedContext(c, failoverErr, streamStarted)
					return
				}
				switchCount++
				reqLog.Warn("openai.images.upstream_failover_switching",
					zap.Int64("account_id", account.ID),
					zap.Int("upstream_status", failoverErr.StatusCode),
					zap.Int("switch_count", switchCount),
					zap.Int("max_switches", maxAccountSwitches),
				)
				continue
			}

			h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, false, nil)
			wroteFallback := h.ensureForwardErrorResponseContext(c, streamStarted)
			fields := []zap.Field{
				zap.Int64("account_id", account.ID),
				zap.Bool("fallback_error_response_written", wroteFallback),
				zap.Error(err),
			}
			if shouldLogOpenAIForwardFailureAsWarnContext(c, wroteFallback) {
				reqLog.Warn("openai.images.forward_failed", fields...)
			} else {
				reqLog.Error("openai.images.forward_failed", fields...)
			}
			return
		}

		h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, true, result.FirstTokenMs)

		userAgent := c.HeaderValue("User-Agent")
		clientIP := ip.GetClientIPContext(c)
		requestPayloadHash := service.HashUsageRequestPayload(body)
		inboundEndpoint := GetInboundEndpointContext(c)
		upstreamEndpoint := GetUpstreamEndpointContext(c, account.Platform)

		h.submitUsageRecordTask(func(taskCtx context.Context) {
			_ = h.gatewayService.RecordUsage(taskCtx, &service.OpenAIRecordUsageInput{
				Result:             result,
				APIKey:             selectedAPIKey,
				User:               selectedAPIKey.User,
				Account:            account,
				Subscription:       subscription,
				InboundEndpoint:    inboundEndpoint,
				UpstreamEndpoint:   upstreamEndpoint,
				UserAgent:          userAgent,
				IPAddress:          clientIP,
				RequestPayloadHash: requestPayloadHash,
				APIKeyService:      h.apiKeyService,
			})
		})
		return
	}
}

type bufferedHypervisorResponseWriter struct {
	bytes.Buffer
	header http.Header
	status int
}

func (w *bufferedHypervisorResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *bufferedHypervisorResponseWriter) WriteHeader(status int) { w.status = status }
func (w *bufferedHypervisorResponseWriter) Flush()                 {}

// buildYunwuGeminiImageGenerationRequest converts the portable OpenAI Images
// generation payload into Gemini's native generateContent schema. It is kept
// deliberately narrow: editing and arbitrary Imagen options remain vendor
// passthrough requests, while Gemini image models get a reliable common path.
func buildYunwuGeminiImageGenerationRequest(body []byte, contentType, path string) ([]byte, string, bool, error) {
	if !strings.Contains(path, "/images/generations") {
		return nil, "", false, nil
	}
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err != nil || !strings.EqualFold(mediaType, "application/json") || !gjson.ValidBytes(body) {
		return nil, "", false, nil
	}
	model := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	modelLower := strings.ToLower(model)
	if !strings.HasPrefix(modelLower, "gemini-") || !strings.Contains(modelLower, "image") {
		return nil, "", false, nil
	}
	prompt := strings.TrimSpace(gjson.GetBytes(body, "prompt").String())
	if prompt == "" {
		return nil, "", true, errors.New("prompt is required")
	}
	if n := gjson.GetBytes(body, "n"); n.Exists() && n.Int() > 1 {
		return nil, "", true, errors.New("Gemini image generation supports n=1")
	}
	generationConfig := map[string]any{"responseModalities": []string{"IMAGE"}}
	if imageSize := geminiImageSizeFromOpenAIImageSize(gjson.GetBytes(body, "size").String()); imageSize != "" {
		generationConfig["imageConfig"] = map[string]any{"imageSize": imageSize}
	}
	payload := map[string]any{
		"contents": []any{map[string]any{
			"role":  "user",
			"parts": []any{map[string]any{"text": prompt}},
		}},
		"generationConfig": generationConfig,
	}
	nativeBody, err := json.Marshal(payload)
	return nativeBody, model, true, err
}

func geminiImageSizeFromOpenAIImageSize(size string) string {
	switch strings.ToLower(strings.TrimSpace(size)) {
	case "1024x1024":
		return "1K"
	case "1536x1024", "1024x1536", "1792x1024", "1024x1792", "":
		return "2K"
	case "2048x2048", "2048x1536", "1536x2048":
		return "4K"
	default:
		return ""
	}
}

func openAIImagesResponseFromGemini(body []byte) (map[string]any, error) {
	if !gjson.ValidBytes(body) {
		return nil, errors.New("Gemini image generation returned an invalid response")
	}
	data := make([]map[string]any, 0)
	for _, candidate := range gjson.GetBytes(body, "candidates").Array() {
		for _, part := range candidate.Get("content.parts").Array() {
			inlineData := part.Get("inlineData")
			if !inlineData.Exists() {
				inlineData = part.Get("inline_data")
			}
			encoded := strings.TrimSpace(inlineData.Get("data").String())
			if encoded == "" {
				continue
			}
			data = append(data, map[string]any{"b64_json": encoded})
		}
	}
	if len(data) == 0 {
		return nil, errors.New("Gemini image generation returned no image data")
	}
	return map[string]any{"created": time.Now().Unix(), "data": data}, nil
}

// normalizeYunwuOpenAIImagesRequest applies the small compatibility layer
// needed by OpenAI-compatible Yunwu media providers.  It deliberately leaves
// non-GPT image models and multipart edit uploads unchanged: those providers
// often have vendor-specific options which must be forwarded verbatim.
func normalizeYunwuOpenAIImagesRequest(body []byte, contentType string) ([]byte, error) {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err != nil || !strings.EqualFold(mediaType, "application/json") {
		return body, nil
	}
	if !gjson.ValidBytes(body) {
		return nil, errors.New("failed to parse request body")
	}

	// Image edits can carry their source image in multipart form, while both
	// OpenAI Images endpoints require a prompt.
	if strings.TrimSpace(gjson.GetBytes(body, "prompt").String()) == "" {
		return nil, errors.New("prompt is required")
	}

	model := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if model == "" {
		model = "gpt-image-1"
		body, err = sjson.SetBytes(body, "model", model)
		if err != nil {
			return nil, err
		}
	}
	if !strings.HasPrefix(strings.ToLower(model), "gpt-image-") {
		return body, nil
	}

	// Some OpenAI-compatible Yunwu vendors reject response_format even though
	// it is optional in the Images API. Omitting it retains the vendor default
	// (normally URL or b64_json) and avoids rejecting an otherwise valid image
	// generation request.
	if gjson.GetBytes(body, "response_format").Exists() {
		body, err = sjson.DeleteBytes(body, "response_format")
		if err != nil {
			return nil, err
		}
	}
	return body, nil
}

// YunwuMedia transparently proxies OpenAI-compatible audio and asynchronous
// video APIs while retaining Yunwu balance scheduling and reservations.
func (h *OpenAIGatewayHandler) YunwuMedia(c *gin.Context) {
	h.YunwuMediaGateway(gatewayctx.FromGin(c))
}

func (h *OpenAIGatewayHandler) YunwuMediaGateway(c gatewayctx.GatewayContext) {
	apiKey, ok := middleware2.GetAPIKeyFromGatewayContext(c)
	if !ok || apiKey == nil || apiKey.Group == nil || apiKey.Group.Platform != service.PlatformYunwu {
		h.errorResponseGateway(c, http.StatusBadRequest, "invalid_request_error", "A Yunwu group is required for this media endpoint")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromGatewayContext(c)
	if !ok {
		h.errorResponseGateway(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	if h.hypervisorEngine == nil || h.hypervisorEngine.YunwuEngine() == nil {
		h.errorResponseGateway(c, http.StatusServiceUnavailable, "api_error", "Yunwu hypervisor is not initialized")
		return
	}
	path := c.Path()
	allowed := strings.HasPrefix(path, "/v1/audio/") || strings.HasPrefix(path, "/v1/videos") || strings.HasPrefix(path, "/v1/video/")
	if !allowed {
		h.errorResponseGateway(c, http.StatusNotFound, "invalid_request_error", "Unsupported Yunwu media endpoint")
		return
	}
	if rawQuery := c.Request().URL.RawQuery; rawQuery != "" {
		path += "?" + rawQuery
	}
	body := []byte(`{}`)
	if c.Method() == http.MethodPost {
		requestBody, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request())
		if err != nil {
			h.errorResponseGateway(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
			return
		}
		if len(requestBody) > 0 {
			body = requestBody
		}
	}
	contentType := c.HeaderValue("Content-Type")
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/json"
	}
	requestCtx := hyperruntime.WithUpstreamRequest(c.Context(), c.Method(), path, contentType)
	_, err := h.hypervisorEngine.HandleRequest(requestCtx, strconv.FormatInt(subject.UserID, 10), c.HeaderValue("X-Conversation-ID"), body, hypervisorGatewayWriter{ctx: c}, service.PlatformYunwu)
	if err != nil {
		h.errorResponseGateway(c, http.StatusBadGateway, "api_error", err.Error())
	}
}

func isMultipartImagesContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	return err == nil && strings.EqualFold(mediaType, "multipart/form-data")
}

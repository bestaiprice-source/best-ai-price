package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/server/gatewayctx"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/yunwu"

	"github.com/gin-gonic/gin"
)

// Speed test tuning.
//
// The request is deliberately tiny (max_tokens 1, one short user message) and it
// goes through the REAL engine path, so the number it reports is the latency of
// an actual completion rather than a TCP handshake. Output is discarded — only
// the timings are returned — and the (negligible) token usage is billed through
// the same path as any other yunwu request, so the test cannot be used as free
// generation.
const (
	speedTestMaxTokens = 1
	speedTestPrompt    = "ping"
	// speedTestTimeout bounds the whole probe. The engine's own request timeout
	// is 45s, so a hung provider surfaces as a timeout error instead of a
	// request that never returns.
	speedTestTimeout = 60 * time.Second
)

type speedTestRequest struct {
	Model string `json:"model"`
}

type speedTestResponse struct {
	Model        string `json:"model"`
	TTFTMs       int64  `json:"ttft_ms"`
	TotalMs      int64  `json:"total_ms"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	NodeID       string `json:"node_id"`
}

// speedTestWriter is a timing sink: it stamps the first byte the engine writes
// (time to first token) and throws the payload away.
type speedTestWriter struct {
	startedAt time.Time
	firstByte time.Time
}

func (w *speedTestWriter) Write(p []byte) (int, error) {
	if w.firstByte.IsZero() && len(p) > 0 {
		w.firstByte = time.Now()
	}
	return len(p), nil
}

// ttftMs is the time to first byte, or the total elapsed time when the engine
// produced nothing (a request that still answered but streamed no chunk).
func (w *speedTestWriter) ttftMs(finished time.Time) int64 {
	if w.firstByte.IsZero() {
		return finished.Sub(w.startedAt).Milliseconds()
	}
	return w.firstByte.Sub(w.startedAt).Milliseconds()
}

// ModelSpeedTest POST /api/v1/usage/speed-test
func (h *GatewayHandler) ModelSpeedTest(c *gin.Context) {
	h.ModelSpeedTestGateway(gatewayctx.FromGin(c))
}

// ModelSpeedTestGateway runs one minimal real completion for the requested model
// and reports how long it took.
//
// It deliberately reuses the normal request path (the hypervisor engine plus the
// standard yunwu billing call) instead of inventing a second upstream probe: a
// separate path would report numbers the user's real traffic never sees, and a
// second billing implementation would drift from the charged price.
func (h *GatewayHandler) ModelSpeedTestGateway(c gatewayctx.GatewayContext) {
	subject, ok := middleware2.GetAuthSubjectFromGatewayContext(c)
	if !ok {
		h.errorResponseGateway(c, http.StatusUnauthorized, "invalid_request_error", "User not authenticated")
		return
	}

	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request())
	if err != nil {
		h.errorResponseGateway(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}
	var req speedTestRequest
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			h.errorResponseGateway(c, http.StatusBadRequest, "invalid_request_error", "Invalid JSON body")
			return
		}
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		h.errorResponseGateway(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}

	ctx := c.Request().Context()

	user, err := h.userService.GetByID(ctx, subject.UserID)
	if err != nil || user == nil {
		h.errorResponseGateway(c, http.StatusInternalServerError, "api_error", "Failed to load the account")
		return
	}

	// The request is routed and billed like the user's own traffic, which means
	// it needs one of their API keys: it carries the group (routing scope) and
	// receives the quota/rate-limit accounting. Creating a key is step one of the
	// dashboard anyway, so this is a clear setup error rather than a server fault.
	apiKey, err := h.firstActiveAPIKey(ctx, subject.UserID)
	if err != nil {
		h.errorResponseGateway(c, http.StatusInternalServerError, "api_error", "Failed to load an API key")
		return
	}
	if apiKey == nil {
		h.errorResponseGateway(c, http.StatusBadRequest, "invalid_request_error",
			"Create an API key first — the speed test runs a real request on your own key")
		return
	}

	payload, err := buildSpeedTestPayload(model)
	if err != nil {
		h.errorResponseGateway(c, http.StatusInternalServerError, "api_error", "Failed to build the test request")
		return
	}

	probeCtx, cancel := context.WithTimeout(ctx, speedTestTimeout)
	defer cancel()
	// Same routing context the live path uses, so an explicit admin Primary/Backup
	// rule is honoured by the speed test too.
	probeCtx = service.WithYunwuModelRouting(probeCtx, apiKey.Group, model)

	writer := &speedTestWriter{startedAt: time.Now()}
	ownerID := fmt.Sprintf("%d", subject.UserID)
	conversationID := fmt.Sprintf("speedtest-%d", time.Now().UnixNano())

	meta, err := h.hypervisorEngine.HandleRequest(probeCtx, ownerID, conversationID, payload, writer, "yunwu")
	finished := time.Now()
	if err != nil {
		status, errType := speedTestErrorStatus(err)
		h.errorResponseGateway(c, status, errType, err.Error())
		return
	}

	resp := speedTestResponse{
		Model:   model,
		TTFTMs:  writer.ttftMs(finished),
		TotalMs: finished.Sub(writer.startedAt).Milliseconds(),
	}
	if meta != nil {
		resp.NodeID = meta.NodeID
		resp.InputTokens = meta.InputTokens
		resp.OutputTokens = meta.OutputTokens
	}

	// Bill the tokens the probe actually consumed on the same detached context as
	// every other post-response yunwu charge.
	if meta != nil && (meta.InputTokens > 0 || meta.OutputTokens > 0) {
		result := service.BuildYunwuForwardResult("", model, meta.InputTokens, meta.OutputTokens, true, writer.startedAt)
		billingCtx, cancelBilling := detachedBillingContext(ctx)
		defer cancelBilling()
		subscription, _ := middleware2.GetSubscriptionFromGatewayContext(c)
		if billedErr := h.gatewayService.RecordYunwuUsage(billingCtx, &service.RecordYunwuUsageInput{
			Result:        result,
			APIKey:        apiKey,
			User:          user,
			AccountID:     meta.NodeID,
			Subscription:  subscription,
			UserAgent:     "speed-test",
			APIKeyService: h.apiKeyService,
		}); billedErr != nil {
			// The probe already succeeded; a billing hiccup must not fail the
			// measurement the user asked for.
			slog.Error("[ModelSpeedTest] billing failed", "error", billedErr, "model", model, "user_id", user.ID)
		}
	}

	c.WriteJSON(http.StatusOK, resp)
}

// buildSpeedTestPayload is the payload the engine actually receives: one short
// user message, max_tokens 1. Kept as its own function so the contract is pinned
// by a test rather than buried in the handler.
func buildSpeedTestPayload(model string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"model":      model,
		"max_tokens": speedTestMaxTokens,
		"stream":     true,
		"messages": []map[string]any{
			{"role": "user", "content": speedTestPrompt},
		},
	})
}

// speedTestErrorStatus maps an engine failure onto the response the caller sees.
//
// "no pricing" / "no compatible nodes" / "tier not configured" are configuration
// answers about the REQUEST (this model cannot be served at all) and come back as
// 400; anything else — an upstream failure, a timeout, a broken stream — is a
// gateway-side problem and comes back as 502.
func speedTestErrorStatus(err error) (int, string) {
	if err == nil {
		return http.StatusBadGateway, "api_error"
	}
	if errors.Is(err, yunwu.ErrNoPricingAvailable) ||
		errors.Is(err, yunwu.ErrNoCompatibleNodes) ||
		errors.Is(err, yunwu.ErrTierNotConfigured) {
		return http.StatusBadRequest, "invalid_request_error"
	}
	return http.StatusBadGateway, "api_error"
}

// firstActiveAPIKey returns the user's first active API key, or nil when they
// have none.
func (h *GatewayHandler) firstActiveAPIKey(ctx context.Context, userID int64) (*service.APIKey, error) {
	if h.apiKeyService == nil {
		return nil, nil
	}
	keys, _, err := h.apiKeyService.List(ctx, userID, pagination.PaginationParams{Page: 1, PageSize: 1}, service.APIKeyListFilters{
		Status: "active",
	})
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, nil
	}
	key := keys[0]
	return &key, nil
}

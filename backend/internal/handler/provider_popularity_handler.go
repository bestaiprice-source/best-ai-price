package handler

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/gatewayctx"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type ProviderPopularityHandler struct {
	popularityService *service.ProviderPopularityService
}

func NewProviderPopularityHandler(popularityService *service.ProviderPopularityService) *ProviderPopularityHandler {
	return &ProviderPopularityHandler{
		popularityService: popularityService,
	}
}

// RecordClick handles click tracking with anti-abuse protection
// POST /api/v1/providers/:provider/click
func (h *ProviderPopularityHandler) RecordClick(c *gin.Context) {
	h.RecordClickGateway(gatewayctx.FromGin(c))
}

func (h *ProviderPopularityHandler) RecordClickGateway(c gatewayctx.GatewayContext) {
	provider := c.PathParam("provider")
	if strings.TrimSpace(provider) == "" {
		response.ErrorContext(commentGatewayResponder{ctx: c}, http.StatusBadRequest, "provider is required")
		return
	}

	clientIP := ip.GetClientIPContext(c)
	if clientIP == "" {
		if xff := c.HeaderValue("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			clientIP = strings.TrimSpace(parts[0])
		} else if xrip := c.HeaderValue("X-Real-IP"); xrip != "" {
			clientIP = strings.TrimSpace(xrip)
		}
	}

	var currentUserID int64
	if subject, ok := middleware2.GetAuthSubjectFromGatewayContext(c); ok {
		currentUserID = subject.UserID
	}

	userAgent := c.HeaderValue("User-Agent")

	res, err := h.popularityService.RecordClick(c.Request().Context(), service.RecordClickInput{
		Provider:  provider,
		ClientIP:  clientIP,
		UserID:    currentUserID,
		UserAgent: userAgent,
	})
	if err != nil {
		response.ErrorFromContext(commentGatewayResponder{ctx: c}, err)
		return
	}

	response.SuccessContext(commentGatewayResponder{ctx: c}, res)
}

// GetPopularity handles getting popularity stats for a single provider
// GET /api/v1/providers/:provider/popularity
func (h *ProviderPopularityHandler) GetPopularity(c *gin.Context) {
	h.GetPopularityGateway(gatewayctx.FromGin(c))
}

func (h *ProviderPopularityHandler) GetPopularityGateway(c gatewayctx.GatewayContext) {
	provider := c.PathParam("provider")
	if strings.TrimSpace(provider) == "" {
		response.ErrorContext(commentGatewayResponder{ctx: c}, http.StatusBadRequest, "provider is required")
		return
	}

	res, err := h.popularityService.GetPopularity(c.Request().Context(), provider)
	if err != nil {
		response.ErrorFromContext(commentGatewayResponder{ctx: c}, err)
		return
	}

	response.SuccessContext(commentGatewayResponder{ctx: c}, res)
}

// ListPopularity handles listing all provider popularities ranked
// GET /api/v1/providers/popularity
func (h *ProviderPopularityHandler) ListPopularity(c *gin.Context) {
	h.ListPopularityGateway(gatewayctx.FromGin(c))
}

func (h *ProviderPopularityHandler) ListPopularityGateway(c gatewayctx.GatewayContext) {
	list, err := h.popularityService.ListPopularity(c.Request().Context())
	if err != nil {
		response.ErrorFromContext(commentGatewayResponder{ctx: c}, err)
		return
	}

	response.SuccessContext(commentGatewayResponder{ctx: c}, gin.H{
		"providers": list,
	})
}

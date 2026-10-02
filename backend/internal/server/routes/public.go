package routes

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/gatewayctx"
	"github.com/gin-gonic/gin"
)

// ExecutablePublicRoutes registers public executable routes for gnet
func ExecutablePublicRoutes(h *handler.Handlers) []gatewayctx.RouteDef {
	if h == nil || h.Gateway == nil {
		return nil
	}
	mw := []string{
		"request_logger",
		"cors",
		"security_headers",
		"client_request_id",
	}
	return []gatewayctx.RouteDef{
		{Method: http.MethodGet, Path: "/api/v1/models", Handler: h.Gateway.PublicModelsPricingGateway, Middleware: mw},
		{Method: http.MethodGet, Path: "/models", Handler: h.Gateway.PublicModelsPricingGateway, Middleware: mw},
	}
}

// RegisterPublicRoutes registers public open endpoints
func RegisterPublicRoutes(v1 *gin.RouterGroup, h *handler.Handlers) {
	if h != nil && h.Gateway != nil {
		v1.GET("/models", h.Gateway.PublicModelsPricing)
	}
	if h != nil && h.AdminVendor != nil {
		public := v1.Group("/public")
		// We reuse the existing GetVendors handler since it returns the full list
		public.GET("/vendors", h.AdminVendor.GetVendors)
	}
}

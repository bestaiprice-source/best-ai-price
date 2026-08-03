package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/gin-gonic/gin"
)

// RegisterPublicRoutes registers public open endpoints
func RegisterPublicRoutes(v1 *gin.RouterGroup, h *handler.Handlers) {
	if h != nil && h.AdminVendor != nil {
		public := v1.Group("/public")
		// We reuse the existing GetVendors handler since it returns the full list
		public.GET("/vendors", h.AdminVendor.GetVendors)
	}
}

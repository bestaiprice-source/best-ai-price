package routes

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/gatewayctx"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"

	"github.com/gin-gonic/gin"
)

func ExecutableSupportRoutes(h *handler.Handlers) []gatewayctx.RouteDef {
	if h == nil || h.Support == nil {
		return nil
	}
	mw := []string{"request_logger", "cors", "security_headers", "client_request_id", "jwt_auth", "backend_mode_user_guard"}
	return []gatewayctx.RouteDef{
		{Method: http.MethodPost, Path: "/api/v1/support/messages", Handler: h.Support.SendGateway, Middleware: mw},
		{Method: http.MethodGet, Path: "/api/v1/support/messages", Handler: h.Support.ListGateway, Middleware: mw},
		{Method: http.MethodPost, Path: "/api/v1/support/messages/read", Handler: h.Support.MarkReadGateway, Middleware: mw},
	}
}

func RegisterSupportRoutes(rg *gin.RouterGroup, h *handler.Handlers, jwtAuth middleware.JWTAuthMiddleware) {
	if h == nil || h.Support == nil {
		return
	}
	g := rg.Group("/support")
	if jwtAuth != nil {
		g.Use(gin.HandlerFunc(jwtAuth))
	}
	g.POST("/messages", h.Support.Send)
	g.GET("/messages", h.Support.List)
	g.POST("/messages/read", h.Support.MarkRead)
}

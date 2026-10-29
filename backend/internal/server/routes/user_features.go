package routes

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/gatewayctx"

	"github.com/gin-gonic/gin"
)

func executableUserFeatureRoutes(h *handler.Handlers) []gatewayctx.RouteDef {
	if h == nil {
		return nil
	}
	mw := []string{
		"request_logger",
		"cors",
		"security_headers",
		"client_request_id",
		"jwt_auth",
		"backend_mode_user_guard",
	}
	out := make([]gatewayctx.RouteDef, 0, 6)
	if h.User != nil {
		out = append(out,
			gatewayctx.RouteDef{
				Method:     http.MethodGet,
				Path:       "/api/v1/user/aff",
				Handler:    h.User.GetAffiliateGateway,
				Middleware: mw,
			},
			gatewayctx.RouteDef{
				Method:     http.MethodPost,
				Path:       "/api/v1/user/aff/transfer",
				Handler:    h.User.TransferAffiliateQuotaGateway,
				Middleware: mw,
			},
		)
	}
	if h.AvailableChannel != nil {
		out = append(out, gatewayctx.RouteDef{
			Method:     http.MethodGet,
			Path:       "/api/v1/channels/available",
			Handler:    h.AvailableChannel.ListGateway,
			Middleware: mw,
		})
	}
	if h.ChannelMonitor != nil {
		out = append(out,
			gatewayctx.RouteDef{
				Method:     http.MethodGet,
				Path:       "/api/v1/channel-monitors",
				Handler:    h.ChannelMonitor.ListGateway,
				Middleware: mw,
			},
			gatewayctx.RouteDef{
				Method:     http.MethodGet,
				Path:       "/api/v1/channel-monitors/:id/status",
				Handler:    h.ChannelMonitor.GetStatusGateway,
				Middleware: mw,
			},
			gatewayctx.RouteDef{
				Method:     http.MethodGet,
				Path:       "/api/v1/model-health",
				Handler:    h.ChannelMonitor.ModelHealthGateway,
				Middleware: mw,
			},
		)
	}
	return out
}

func registerUserFeatureRoutes(authenticated *gin.RouterGroup, h *handler.Handlers) {
	if authenticated == nil || h == nil {
		return
	}
	if h.User != nil {
		user := authenticated.Group("/user")
		{
			user.GET("/aff", h.User.GetAffiliate)
			user.POST("/aff/transfer", h.User.TransferAffiliateQuota)
		}
	}
	if h.AvailableChannel != nil {
		authenticated.GET("/channels/available", h.AvailableChannel.List)
	}
	if h.ChannelMonitor != nil {
		monitors := authenticated.Group("/channel-monitors")
		{
			monitors.GET("", h.ChannelMonitor.List)
			monitors.GET("/:id/status", h.ChannelMonitor.GetStatus)
		}
		// 扁平「模型 → 健康」read model（仪表盘健康块 / 模型目录卡片共用）。
		// 与 /channel-monitors 平级注册，避免与 :id 路由产生歧义。
		authenticated.GET("/model-health", h.ChannelMonitor.ModelHealth)
	}
}

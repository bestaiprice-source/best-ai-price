package routes

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/gatewayctx"

	"github.com/gin-gonic/gin"
)

func ExecutableProviderPopularityRoutes(h *handler.Handlers) []gatewayctx.RouteDef {
	if h == nil || h.ProviderPopularity == nil {
		return nil
	}
	out := make([]gatewayctx.RouteDef, 0, 4)
	out = append(out,
		// Public (optional auth) - Record click with anti-abuse protection
		gatewayctx.RouteDef{
			Method:     http.MethodPost,
			Path:       "/api/v1/providers/:provider/click",
			Handler:    h.ProviderPopularity.RecordClickGateway,
			Middleware: []string{"request_logger", "cors", "security_headers", "client_request_id"},
		},
		// Public - Get popularity rankings for all providers
		gatewayctx.RouteDef{
			Method:     http.MethodGet,
			Path:       "/api/v1/providers/popularity",
			Handler:    h.ProviderPopularity.ListPopularityGateway,
			Middleware: []string{"request_logger", "cors", "security_headers", "client_request_id"},
		},
		// Public - Get popularity stats for a specific provider
		gatewayctx.RouteDef{
			Method:     http.MethodGet,
			Path:       "/api/v1/providers/:provider/popularity",
			Handler:    h.ProviderPopularity.GetPopularityGateway,
			Middleware: []string{"request_logger", "cors", "security_headers", "client_request_id"},
		},
	)
	return out
}

func RegisterProviderPopularityRoutes(rg *gin.RouterGroup, h *handler.Handlers) {
	if h == nil || h.ProviderPopularity == nil {
		return
	}

	rg.POST("/providers/:provider/click", h.ProviderPopularity.RecordClick)
	rg.GET("/providers/popularity", h.ProviderPopularity.ListPopularity)
	rg.GET("/providers/:provider/popularity", h.ProviderPopularity.GetPopularity)
}

package routes

import (
	"net/http"
	"os"

	"github.com/Wei-Shaw/sub2api/internal/server/gatewayctx"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	middlewareRequestLogger = "request_logger"
	middlewareSecurity      = "security_headers"
	middlewareCORS          = "cors"
)

// RegisterCommonRoutes 注册通用路由（健康检查、状态等）
func RegisterCommonRoutes(r gin.IRoutes) {
	// 健康检查
	r.GET("/health", gatewayctx.AdaptGinHandler(commonHealth))

	// Claude Code 遥测日志（忽略，直接返回200）
	r.POST("/api/event_logging/batch", gatewayctx.AdaptGinHandler(commonEventLoggingBatch))

	// Setup status endpoint (always returns needs_setup: false in normal mode)
	// This is used by the frontend to detect when the service has restarted after setup
	r.GET("/setup/status", gatewayctx.AdaptGinHandler(commonSetupStatus))

	// Parser providers.json static endpoint (supports /data/providers.json, /api/data/providers.json, and /api/v1/data/providers.json)
	r.GET("/data/providers.json", gatewayctx.AdaptGinHandler(commonProvidersJSON))
	r.GET("/api/data/providers.json", gatewayctx.AdaptGinHandler(commonProvidersJSON))
	r.GET("/api/v1/data/providers.json", gatewayctx.AdaptGinHandler(commonProvidersJSON))

	// Parser models.json static endpoint (supports /data/models.json, /api/data/models.json, and /api/v1/data/models.json)
	r.GET("/data/models.json", gatewayctx.AdaptGinHandler(commonModelsJSON))
	r.GET("/api/data/models.json", gatewayctx.AdaptGinHandler(commonModelsJSON))
	r.GET("/api/v1/data/models.json", gatewayctx.AdaptGinHandler(commonModelsJSON))

	// Parser api_descriptions.json static endpoint (supports /data/api_descriptions.json, /api/data/api_descriptions.json, and /api/v1/data/api_descriptions.json)
	r.GET("/data/api_descriptions.json", gatewayctx.AdaptGinHandler(commonApiDescriptionsJSON))
	r.GET("/api/data/api_descriptions.json", gatewayctx.AdaptGinHandler(commonApiDescriptionsJSON))
	r.GET("/api/v1/data/api_descriptions.json", gatewayctx.AdaptGinHandler(commonApiDescriptionsJSON))

	// Parser llm_rankings.json static endpoint (supports /data/llm_rankings.json, /api/data/llm_rankings.json, and /api/v1/data/llm_rankings.json)
	r.GET("/data/llm_rankings.json", gatewayctx.AdaptGinHandler(commonLlmRankingsJSON))
	r.GET("/api/data/llm_rankings.json", gatewayctx.AdaptGinHandler(commonLlmRankingsJSON))
	r.GET("/api/v1/data/llm_rankings.json", gatewayctx.AdaptGinHandler(commonLlmRankingsJSON))

	// Parser image_generation_rankings.json static endpoint
	r.GET("/data/image_generation_rankings.json", gatewayctx.AdaptGinHandler(commonImageGenerationRankingsJSON))
	r.GET("/api/data/image_generation_rankings.json", gatewayctx.AdaptGinHandler(commonImageGenerationRankingsJSON))
	r.GET("/api/v1/data/image_generation_rankings.json", gatewayctx.AdaptGinHandler(commonImageGenerationRankingsJSON))

	// Parser video_generation_rankings.json static endpoint
	r.GET("/data/video_generation_rankings.json", gatewayctx.AdaptGinHandler(commonVideoGenerationRankingsJSON))
	r.GET("/api/data/video_generation_rankings.json", gatewayctx.AdaptGinHandler(commonVideoGenerationRankingsJSON))
	r.GET("/api/v1/data/video_generation_rankings.json", gatewayctx.AdaptGinHandler(commonVideoGenerationRankingsJSON))

	// Parser text_to_speech_rankings.json static endpoint
	r.GET("/data/text_to_speech_rankings.json", gatewayctx.AdaptGinHandler(commonTextToSpeechRankingsJSON))
	r.GET("/api/data/text_to_speech_rankings.json", gatewayctx.AdaptGinHandler(commonTextToSpeechRankingsJSON))
	r.GET("/api/v1/data/text_to_speech_rankings.json", gatewayctx.AdaptGinHandler(commonTextToSpeechRankingsJSON))

	// Parser speech_to_text_rankings.json static endpoint
	r.GET("/data/speech_to_text_rankings.json", gatewayctx.AdaptGinHandler(commonSpeechToTextRankingsJSON))
	r.GET("/api/data/speech_to_text_rankings.json", gatewayctx.AdaptGinHandler(commonSpeechToTextRankingsJSON))
	r.GET("/api/v1/data/speech_to_text_rankings.json", gatewayctx.AdaptGinHandler(commonSpeechToTextRankingsJSON))

	// Parser provider_descriptions.json / providers_description.json static endpoints
	r.GET("/data/provider_descriptions.json", gatewayctx.AdaptGinHandler(commonProviderDescriptionsJSON))
	r.GET("/api/data/provider_descriptions.json", gatewayctx.AdaptGinHandler(commonProviderDescriptionsJSON))
	r.GET("/api/v1/data/provider_descriptions.json", gatewayctx.AdaptGinHandler(commonProviderDescriptionsJSON))
	r.GET("/data/providers_description.json", gatewayctx.AdaptGinHandler(commonProviderDescriptionsJSON))
	r.GET("/api/data/providers_description.json", gatewayctx.AdaptGinHandler(commonProviderDescriptionsJSON))
	r.GET("/api/v1/data/providers_description.json", gatewayctx.AdaptGinHandler(commonProviderDescriptionsJSON))

	// Parser run_stats.json static endpoints
	r.GET("/data/run_stats.json", gatewayctx.AdaptGinHandler(commonRunStatsJSON))
	r.GET("/api/data/run_stats.json", gatewayctx.AdaptGinHandler(commonRunStatsJSON))
	r.GET("/api/v1/data/run_stats.json", gatewayctx.AdaptGinHandler(commonRunStatsJSON))
}

func ExecutableCommonRoutes() []gatewayctx.RouteDef {
	return []gatewayctx.RouteDef{
		{
			Method:     http.MethodGet,
			Path:       "/health",
			Handler:    commonHealth,
			Middleware: []string{middlewareRequestLogger, middlewareCORS, middlewareSecurity},
		},
		{
			Method:     http.MethodPost,
			Path:       "/api/event_logging/batch",
			Handler:    commonEventLoggingBatch,
			Middleware: []string{middlewareRequestLogger, middlewareCORS, middlewareSecurity},
		},
		{
			Method:     http.MethodGet,
			Path:       "/setup/status",
			Handler:    commonSetupStatus,
			Middleware: []string{middlewareRequestLogger, middlewareCORS, middlewareSecurity},
		},
		{
			Method:     http.MethodGet,
			Path:       "/data/providers.json",
			Handler:    commonProvidersJSON,
			Middleware: []string{middlewareRequestLogger, middlewareCORS, middlewareSecurity},
		},
		{
			Method:     http.MethodGet,
			Path:       "/api/data/providers.json",
			Handler:    commonProvidersJSON,
			Middleware: []string{middlewareRequestLogger, middlewareCORS, middlewareSecurity},
		},
		{
			Method:     http.MethodGet,
			Path:       "/api/v1/data/providers.json",
			Handler:    commonProvidersJSON,
			Middleware: []string{middlewareRequestLogger, middlewareCORS, middlewareSecurity},
		},
		{
			Method:     http.MethodGet,
			Path:       "/data/models.json",
			Handler:    commonModelsJSON,
			Middleware: []string{middlewareRequestLogger, middlewareCORS, middlewareSecurity},
		},
		{
			Method:     http.MethodGet,
			Path:       "/api/data/models.json",
			Handler:    commonModelsJSON,
			Middleware: []string{middlewareRequestLogger, middlewareCORS, middlewareSecurity},
		},
		{
			Method:     http.MethodGet,
			Path:       "/api/v1/data/models.json",
			Handler:    commonModelsJSON,
			Middleware: []string{middlewareRequestLogger, middlewareCORS, middlewareSecurity},
		},
		{
			Method:     http.MethodGet,
			Path:       "/data/api_descriptions.json",
			Handler:    commonApiDescriptionsJSON,
			Middleware: []string{middlewareRequestLogger, middlewareCORS, middlewareSecurity},
		},
		{
			Method:     http.MethodGet,
			Path:       "/api/data/api_descriptions.json",
			Handler:    commonApiDescriptionsJSON,
			Middleware: []string{middlewareRequestLogger, middlewareCORS, middlewareSecurity},
		},
		{
			Method:     http.MethodGet,
			Path:       "/api/v1/data/api_descriptions.json",
			Handler:    commonApiDescriptionsJSON,
			Middleware: []string{middlewareRequestLogger, middlewareCORS, middlewareSecurity},
		},
		{
			Method:     http.MethodGet,
			Path:       "/data/llm_rankings.json",
			Handler:    commonLlmRankingsJSON,
			Middleware: []string{middlewareRequestLogger, middlewareCORS, middlewareSecurity},
		},
		{
			Method:     http.MethodGet,
			Path:       "/api/data/llm_rankings.json",
			Handler:    commonLlmRankingsJSON,
			Middleware: []string{middlewareRequestLogger, middlewareCORS, middlewareSecurity},
		},
		{
			Method:     http.MethodGet,
			Path:       "/api/v1/data/llm_rankings.json",
			Handler:    commonLlmRankingsJSON,
			Middleware: []string{middlewareRequestLogger, middlewareCORS, middlewareSecurity},
		},
		{
			Method:     http.MethodGet,
			Path:       "/data/image_generation_rankings.json",
			Handler:    commonImageGenerationRankingsJSON,
			Middleware: []string{middlewareRequestLogger, middlewareCORS, middlewareSecurity},
		},
		{
			Method:     http.MethodGet,
			Path:       "/api/data/image_generation_rankings.json",
			Handler:    commonImageGenerationRankingsJSON,
			Middleware: []string{middlewareRequestLogger, middlewareCORS, middlewareSecurity},
		},
		{
			Method:     http.MethodGet,
			Path:       "/api/v1/data/image_generation_rankings.json",
			Handler:    commonImageGenerationRankingsJSON,
			Middleware: []string{middlewareRequestLogger, middlewareCORS, middlewareSecurity},
		},
		{
			Method:     http.MethodGet,
			Path:       "/data/video_generation_rankings.json",
			Handler:    commonVideoGenerationRankingsJSON,
			Middleware: []string{middlewareRequestLogger, middlewareCORS, middlewareSecurity},
		},
		{
			Method:     http.MethodGet,
			Path:       "/api/data/video_generation_rankings.json",
			Handler:    commonVideoGenerationRankingsJSON,
			Middleware: []string{middlewareRequestLogger, middlewareCORS, middlewareSecurity},
		},
		{
			Method:     http.MethodGet,
			Path:       "/api/v1/data/video_generation_rankings.json",
			Handler:    commonVideoGenerationRankingsJSON,
			Middleware: []string{middlewareRequestLogger, middlewareCORS, middlewareSecurity},
		},
		{
			Method:     http.MethodGet,
			Path:       "/data/text_to_speech_rankings.json",
			Handler:    commonTextToSpeechRankingsJSON,
			Middleware: []string{middlewareRequestLogger, middlewareCORS, middlewareSecurity},
		},
		{
			Method:     http.MethodGet,
			Path:       "/api/data/text_to_speech_rankings.json",
			Handler:    commonTextToSpeechRankingsJSON,
			Middleware: []string{middlewareRequestLogger, middlewareCORS, middlewareSecurity},
		},
		{
			Method:     http.MethodGet,
			Path:       "/api/v1/data/text_to_speech_rankings.json",
			Handler:    commonTextToSpeechRankingsJSON,
			Middleware: []string{middlewareRequestLogger, middlewareCORS, middlewareSecurity},
		},
		{
			Method:     http.MethodGet,
			Path:       "/data/speech_to_text_rankings.json",
			Handler:    commonSpeechToTextRankingsJSON,
			Middleware: []string{middlewareRequestLogger, middlewareCORS, middlewareSecurity},
		},
		{
			Method:     http.MethodGet,
			Path:       "/api/data/speech_to_text_rankings.json",
			Handler:    commonSpeechToTextRankingsJSON,
			Middleware: []string{middlewareRequestLogger, middlewareCORS, middlewareSecurity},
		},
		{
			Method:     http.MethodGet,
			Path:       "/api/v1/data/speech_to_text_rankings.json",
			Handler:    commonSpeechToTextRankingsJSON,
			Middleware: []string{middlewareRequestLogger, middlewareCORS, middlewareSecurity},
		},
		{
			Method:     http.MethodGet,
			Path:       "/data/provider_descriptions.json",
			Handler:    commonProviderDescriptionsJSON,
			Middleware: []string{middlewareRequestLogger, middlewareCORS, middlewareSecurity},
		},
		{
			Method:     http.MethodGet,
			Path:       "/api/data/provider_descriptions.json",
			Handler:    commonProviderDescriptionsJSON,
			Middleware: []string{middlewareRequestLogger, middlewareCORS, middlewareSecurity},
		},
		{
			Method:     http.MethodGet,
			Path:       "/api/v1/data/provider_descriptions.json",
			Handler:    commonProviderDescriptionsJSON,
			Middleware: []string{middlewareRequestLogger, middlewareCORS, middlewareSecurity},
		},
		{
			Method:     http.MethodGet,
			Path:       "/data/providers_description.json",
			Handler:    commonProviderDescriptionsJSON,
			Middleware: []string{middlewareRequestLogger, middlewareCORS, middlewareSecurity},
		},
		{
			Method:     http.MethodGet,
			Path:       "/api/data/providers_description.json",
			Handler:    commonProviderDescriptionsJSON,
			Middleware: []string{middlewareRequestLogger, middlewareCORS, middlewareSecurity},
		},
		{
			Method:     http.MethodGet,
			Path:       "/api/v1/data/providers_description.json",
			Handler:    commonProviderDescriptionsJSON,
			Middleware: []string{middlewareRequestLogger, middlewareCORS, middlewareSecurity},
		},
		{
			Method:     http.MethodGet,
			Path:       "/data/run_stats.json",
			Handler:    commonRunStatsJSON,
			Middleware: []string{middlewareRequestLogger, middlewareCORS, middlewareSecurity},
		},
		{
			Method:     http.MethodGet,
			Path:       "/api/data/run_stats.json",
			Handler:    commonRunStatsJSON,
			Middleware: []string{middlewareRequestLogger, middlewareCORS, middlewareSecurity},
		},
		{
			Method:     http.MethodGet,
			Path:       "/api/v1/data/run_stats.json",
			Handler:    commonRunStatsJSON,
			Middleware: []string{middlewareRequestLogger, middlewareCORS, middlewareSecurity},
		},
	}
}

func commonHealth(c gatewayctx.GatewayContext) {
	c.WriteJSON(http.StatusOK, map[string]any{"status": "ok"})
}

func commonEventLoggingBatch(c gatewayctx.GatewayContext) {
	c.SetStatus(http.StatusOK)
}

func commonSetupStatus(c gatewayctx.GatewayContext) {
	c.WriteJSON(http.StatusOK, map[string]any{
		"code": 0,
		"data": map[string]any{
			"needs_setup": false,
			"step":        "completed",
		},
	})
}

func commonProvidersJSON(c gatewayctx.GatewayContext) {
	serveStaticDataJSON(c, "providers.json")
}

func commonModelsJSON(c gatewayctx.GatewayContext) {
	serveStaticDataJSON(c, "models.json")
}

func commonApiDescriptionsJSON(c gatewayctx.GatewayContext) {
	serveStaticDataJSON(c, "api_descriptions.json")
}

func serveStaticDataJSON(c gatewayctx.GatewayContext, filename string) {
	paths := service.LocateParserDataFile(filename)
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			c.SetHeader("Cache-Control", "public, max-age=300")
			c.SetHeader("Access-Control-Allow-Origin", "*")
			if err := c.ServeFile(p); err == nil {
				return
			}
		}
	}
	c.WriteJSON(http.StatusNotFound, map[string]any{"error": filename + " not found"})
}

func commonLlmRankingsJSON(c gatewayctx.GatewayContext) {
	serveStaticDataJSON(c, "llm_rankings.json")
}

func commonImageGenerationRankingsJSON(c gatewayctx.GatewayContext) {
	serveStaticDataJSON(c, "image_generation_rankings.json")
}

func commonVideoGenerationRankingsJSON(c gatewayctx.GatewayContext) {
	serveStaticDataJSON(c, "video_generation_rankings.json")
}

func commonTextToSpeechRankingsJSON(c gatewayctx.GatewayContext) {
	serveStaticDataJSON(c, "text_to_speech_rankings.json")
}

func commonSpeechToTextRankingsJSON(c gatewayctx.GatewayContext) {
	serveStaticDataJSON(c, "speech_to_text_rankings.json")
}

func commonProviderDescriptionsJSON(c gatewayctx.GatewayContext) {
	for _, fname := range []string{"providers_description.json", "provider_descriptions.json"} {
		paths := service.LocateParserDataFile(fname)
		for _, p := range paths {
			if _, err := os.Stat(p); err == nil {
				c.SetHeader("Cache-Control", "public, max-age=300")
				c.SetHeader("Access-Control-Allow-Origin", "*")
				if err := c.ServeFile(p); err == nil {
					return
				}
			}
		}
	}
	c.WriteJSON(http.StatusNotFound, map[string]any{"error": "providers_description.json not found"})
}

func commonRunStatsJSON(c gatewayctx.GatewayContext) {
	paths := service.LocateParserDataFile("run_stats.json")
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			c.SetHeader("Cache-Control", "public, max-age=60")
			c.SetHeader("Access-Control-Allow-Origin", "*")
			if err := c.ServeFile(p); err == nil {
				return
			}
		}
	}
	c.WriteJSON(http.StatusNotFound, map[string]any{"error": "run_stats.json not found"})
}


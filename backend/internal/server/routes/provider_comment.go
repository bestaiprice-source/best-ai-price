package routes

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/gatewayctx"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"

	"github.com/gin-gonic/gin"
)

func ExecutableProviderCommentRoutes(h *handler.Handlers) []gatewayctx.RouteDef {
	if h == nil || h.ProviderComment == nil {
		return nil
	}
	out := make([]gatewayctx.RouteDef, 0, 8)
	out = append(out,
		// Public (optional auth) - list provider reviews & summary
		gatewayctx.RouteDef{
			Method:     http.MethodGet,
			Path:       "/api/v1/providers/:provider/comments",
			Handler:    h.ProviderComment.ListProviderCommentsGateway,
			Middleware: []string{"request_logger", "cors", "security_headers", "client_request_id"},
		},
		// Public (optional auth) - list replies for a comment
		gatewayctx.RouteDef{
			Method:     http.MethodGet,
			Path:       "/api/v1/comments/:id/replies",
			Handler:    h.ProviderComment.ListRepliesGateway,
			Middleware: []string{"request_logger", "cors", "security_headers", "client_request_id"},
		},
		// Authenticated - Create provider review
		gatewayctx.RouteDef{
			Method:     http.MethodPost,
			Path:       "/api/v1/providers/:provider/comments",
			Handler:    h.ProviderComment.CreateCommentGateway,
			Middleware: []string{"request_logger", "cors", "security_headers", "client_request_id", "jwt_auth", "backend_mode_user_guard"},
		},
		// Authenticated - Reply to comment
		gatewayctx.RouteDef{
			Method:     http.MethodPost,
			Path:       "/api/v1/comments/:id/reply",
			Handler:    h.ProviderComment.ReplyCommentGateway,
			Middleware: []string{"request_logger", "cors", "security_headers", "client_request_id", "jwt_auth", "backend_mode_user_guard"},
		},
		// Authenticated - Like / Dislike / Cancel reaction
		gatewayctx.RouteDef{
			Method:     http.MethodPost,
			Path:       "/api/v1/comments/:id/vote",
			Handler:    h.ProviderComment.VoteCommentGateway,
			Middleware: []string{"request_logger", "cors", "security_headers", "client_request_id", "jwt_auth", "backend_mode_user_guard"},
		},
		// Authenticated - Delete comment (returns forbidden per rules: only reactions can be deleted)
		gatewayctx.RouteDef{
			Method:     http.MethodDelete,
			Path:       "/api/v1/comments/:id",
			Handler:    h.ProviderComment.DeleteCommentGateway,
			Middleware: []string{"request_logger", "cors", "security_headers", "client_request_id", "jwt_auth", "backend_mode_user_guard"},
		},
		// Admin - Moderation list
		gatewayctx.RouteDef{
			Method:     http.MethodGet,
			Path:       "/api/v1/admin/provider-comments",
			Handler:    h.ProviderComment.AdminListCommentsGateway,
			Middleware: []string{"request_logger", "cors", "security_headers", "client_request_id", "admin_auth"},
		},
		// Admin - Moderation set status (active / hidden)
		gatewayctx.RouteDef{
			Method:     http.MethodPut,
			Path:       "/api/v1/admin/provider-comments/:id/status",
			Handler:    h.ProviderComment.AdminSetStatusGateway,
			Middleware: []string{"request_logger", "cors", "security_headers", "client_request_id", "admin_auth"},
		},
	)
	return out
}

func RegisterProviderCommentRoutes(rg *gin.RouterGroup, h *handler.Handlers, jwtAuth middleware.JWTAuthMiddleware, adminAuth middleware.AdminAuthMiddleware) {
	if h == nil || h.ProviderComment == nil {
		return
	}

	// Public routes
	rg.GET("/providers/:provider/comments", h.ProviderComment.ListProviderComments)
	rg.GET("/comments/:id/replies", h.ProviderComment.ListReplies)

	// User authenticated routes
	userGroup := rg.Group("")
	if jwtAuth != nil {
		userGroup.Use(gin.HandlerFunc(jwtAuth))
	}
	userGroup.POST("/providers/:provider/comments", h.ProviderComment.CreateComment)
	userGroup.POST("/comments/:id/reply", h.ProviderComment.ReplyComment)
	userGroup.POST("/comments/:id/vote", h.ProviderComment.VoteComment)
	userGroup.DELETE("/comments/:id", h.ProviderComment.DeleteComment)

	// Admin routes
	adminGroup := rg.Group("/admin/provider-comments")
	if adminAuth != nil {
		adminGroup.Use(gin.HandlerFunc(adminAuth))
	}
	adminGroup.GET("", h.ProviderComment.AdminListComments)
	adminGroup.PUT("/:id/status", h.ProviderComment.AdminSetStatus)
}

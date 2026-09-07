package routes

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/gatewayctx"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"

	"github.com/gin-gonic/gin"
)

func ExecutableForumRoutes(h *handler.Handlers) []gatewayctx.RouteDef {
	if h == nil || h.Forum == nil {
		return nil
	}
	out := make([]gatewayctx.RouteDef, 0, 16)
	out = append(out,
		// Topics - Public (optional auth)
		gatewayctx.RouteDef{
			Method:     http.MethodGet,
			Path:       "/api/v1/forum/topics",
			Handler:    h.Forum.ListTopicsGateway,
			Middleware: []string{"request_logger", "cors", "security_headers", "client_request_id"},
		},
		gatewayctx.RouteDef{
			Method:     http.MethodGet,
			Path:       "/api/v1/forum/topics/:id",
			Handler:    h.Forum.GetTopicGateway,
			Middleware: []string{"request_logger", "cors", "security_headers", "client_request_id"},
		},
		// Topics - Authenticated
		gatewayctx.RouteDef{
			Method:     http.MethodPost,
			Path:       "/api/v1/forum/topics",
			Handler:    h.Forum.CreateTopicGateway,
			Middleware: []string{"request_logger", "cors", "security_headers", "client_request_id", "jwt_auth", "backend_mode_user_guard"},
		},
		gatewayctx.RouteDef{
			Method:     http.MethodPost,
			Path:       "/api/v1/forum/topics/:id/vote",
			Handler:    h.Forum.VoteTopicGateway,
			Middleware: []string{"request_logger", "cors", "security_headers", "client_request_id", "jwt_auth", "backend_mode_user_guard"},
		},
		gatewayctx.RouteDef{
			Method:     http.MethodDelete,
			Path:       "/api/v1/forum/topics/:id",
			Handler:    h.Forum.DeleteTopicGateway,
			Middleware: []string{"request_logger", "cors", "security_headers", "client_request_id", "jwt_auth", "backend_mode_user_guard"},
		},
		// Topics - Admin
		gatewayctx.RouteDef{
			Method:     http.MethodGet,
			Path:       "/api/v1/admin/forum/topics",
			Handler:    h.Forum.AdminListTopicsGateway,
			Middleware: []string{"request_logger", "cors", "security_headers", "client_request_id", "admin_auth"},
		},
		gatewayctx.RouteDef{
			Method:     http.MethodPut,
			Path:       "/api/v1/admin/forum/topics/:id/status",
			Handler:    h.Forum.AdminSetTopicStatusGateway,
			Middleware: []string{"request_logger", "cors", "security_headers", "client_request_id", "admin_auth"},
		},

		// Comments - Public (optional auth)
		gatewayctx.RouteDef{
			Method:     http.MethodGet,
			Path:       "/api/v1/forum/topics/:id/comments",
			Handler:    h.Forum.ListCommentsGateway,
			Middleware: []string{"request_logger", "cors", "security_headers", "client_request_id"},
		},
		gatewayctx.RouteDef{
			Method:     http.MethodGet,
			Path:       "/api/v1/forum/comments/:id/replies",
			Handler:    h.Forum.ListCommentRepliesGateway,
			Middleware: []string{"request_logger", "cors", "security_headers", "client_request_id"},
		},
		// Comments - Authenticated
		gatewayctx.RouteDef{
			Method:     http.MethodPost,
			Path:       "/api/v1/forum/topics/:id/comments",
			Handler:    h.Forum.CreateCommentGateway,
			Middleware: []string{"request_logger", "cors", "security_headers", "client_request_id", "jwt_auth", "backend_mode_user_guard"},
		},
		gatewayctx.RouteDef{
			Method:     http.MethodPost,
			Path:       "/api/v1/forum/comments/:id/reply",
			Handler:    h.Forum.ReplyCommentGateway,
			Middleware: []string{"request_logger", "cors", "security_headers", "client_request_id", "jwt_auth", "backend_mode_user_guard"},
		},
		gatewayctx.RouteDef{
			Method:     http.MethodPost,
			Path:       "/api/v1/forum/comments/:id/vote",
			Handler:    h.Forum.VoteCommentGateway,
			Middleware: []string{"request_logger", "cors", "security_headers", "client_request_id", "jwt_auth", "backend_mode_user_guard"},
		},
		gatewayctx.RouteDef{
			Method:     http.MethodDelete,
			Path:       "/api/v1/forum/comments/:id",
			Handler:    h.Forum.DeleteCommentGateway,
			Middleware: []string{"request_logger", "cors", "security_headers", "client_request_id", "jwt_auth", "backend_mode_user_guard"},
		},
		// Comments - Admin
		gatewayctx.RouteDef{
			Method:     http.MethodGet,
			Path:       "/api/v1/admin/forum/comments",
			Handler:    h.Forum.AdminListCommentsGateway,
			Middleware: []string{"request_logger", "cors", "security_headers", "client_request_id", "admin_auth"},
		},
		gatewayctx.RouteDef{
			Method:     http.MethodPut,
			Path:       "/api/v1/admin/forum/comments/:id/status",
			Handler:    h.Forum.AdminSetCommentStatusGateway,
			Middleware: []string{"request_logger", "cors", "security_headers", "client_request_id", "admin_auth"},
		},
	)
	return out
}

func RegisterForumRoutes(rg *gin.RouterGroup, h *handler.Handlers, jwtAuth middleware.JWTAuthMiddleware, adminAuth middleware.AdminAuthMiddleware) {
	if h == nil || h.Forum == nil {
		return
	}

	// Public routes
	rg.GET("/forum/topics", h.Forum.ListTopics)
	rg.GET("/forum/topics/:id", h.Forum.GetTopic)
	rg.GET("/forum/topics/:id/comments", h.Forum.ListComments)
	rg.GET("/forum/comments/:id/replies", h.Forum.ListCommentReplies)

	// User authenticated routes
	userGroup := rg.Group("")
	if jwtAuth != nil {
		userGroup.Use(gin.HandlerFunc(jwtAuth))
	}
	userGroup.POST("/forum/topics", h.Forum.CreateTopic)
	userGroup.POST("/forum/topics/:id/vote", h.Forum.VoteTopic)
	userGroup.DELETE("/forum/topics/:id", h.Forum.DeleteTopic)

	userGroup.POST("/forum/topics/:id/comments", h.Forum.CreateComment)
	userGroup.POST("/forum/comments/:id/reply", h.Forum.ReplyComment)
	userGroup.POST("/forum/comments/:id/vote", h.Forum.VoteComment)
	userGroup.DELETE("/forum/comments/:id", h.Forum.DeleteComment)

	// Admin routes
	adminGroup := rg.Group("/admin/forum")
	if adminAuth != nil {
		adminGroup.Use(gin.HandlerFunc(adminAuth))
	}
	adminGroup.GET("/topics", h.Forum.AdminListTopics)
	adminGroup.PUT("/topics/:id/status", h.Forum.AdminSetTopicStatus)
	adminGroup.GET("/comments", h.Forum.AdminListComments)
	adminGroup.PUT("/comments/:id/status", h.Forum.AdminSetCommentStatus)
}

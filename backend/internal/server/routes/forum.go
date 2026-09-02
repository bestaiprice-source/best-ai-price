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
	out := make([]gatewayctx.RouteDef, 0, 8)
	out = append(out,
		// Public (optional auth) - list forum posts
		gatewayctx.RouteDef{
			Method:     http.MethodGet,
			Path:       "/api/v1/forum/posts",
			Handler:    h.Forum.ListPostsGateway,
			Middleware: []string{"request_logger", "cors", "security_headers", "client_request_id"},
		},
		// Public (optional auth) - list replies for a post
		gatewayctx.RouteDef{
			Method:     http.MethodGet,
			Path:       "/api/v1/forum/posts/:id/replies",
			Handler:    h.Forum.ListRepliesGateway,
			Middleware: []string{"request_logger", "cors", "security_headers", "client_request_id"},
		},
		// Authenticated - Create root forum post
		gatewayctx.RouteDef{
			Method:     http.MethodPost,
			Path:       "/api/v1/forum/posts",
			Handler:    h.Forum.CreatePostGateway,
			Middleware: []string{"request_logger", "cors", "security_headers", "client_request_id", "jwt_auth", "backend_mode_user_guard"},
		},
		// Authenticated - Reply to a post
		gatewayctx.RouteDef{
			Method:     http.MethodPost,
			Path:       "/api/v1/forum/posts/:id/reply",
			Handler:    h.Forum.ReplyPostGateway,
			Middleware: []string{"request_logger", "cors", "security_headers", "client_request_id", "jwt_auth", "backend_mode_user_guard"},
		},
		// Authenticated - Like / Dislike / Cancel reaction
		gatewayctx.RouteDef{
			Method:     http.MethodPost,
			Path:       "/api/v1/forum/posts/:id/vote",
			Handler:    h.Forum.VotePostGateway,
			Middleware: []string{"request_logger", "cors", "security_headers", "client_request_id", "jwt_auth", "backend_mode_user_guard"},
		},
		// Authenticated - Delete post (forbidden per rules)
		gatewayctx.RouteDef{
			Method:     http.MethodDelete,
			Path:       "/api/v1/forum/posts/:id",
			Handler:    h.Forum.DeletePostGateway,
			Middleware: []string{"request_logger", "cors", "security_headers", "client_request_id", "jwt_auth", "backend_mode_user_guard"},
		},
		// Admin - Moderation list
		gatewayctx.RouteDef{
			Method:     http.MethodGet,
			Path:       "/api/v1/admin/forum/posts",
			Handler:    h.Forum.AdminListPostsGateway,
			Middleware: []string{"request_logger", "cors", "security_headers", "client_request_id", "admin_auth"},
		},
		// Admin - Moderation set status (active / hidden)
		gatewayctx.RouteDef{
			Method:     http.MethodPut,
			Path:       "/api/v1/admin/forum/posts/:id/status",
			Handler:    h.Forum.AdminSetStatusGateway,
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
	rg.GET("/forum/posts", h.Forum.ListPosts)
	rg.GET("/forum/posts/:id/replies", h.Forum.ListReplies)

	// User authenticated routes
	userGroup := rg.Group("")
	if jwtAuth != nil {
		userGroup.Use(gin.HandlerFunc(jwtAuth))
	}
	userGroup.POST("/forum/posts", h.Forum.CreatePost)
	userGroup.POST("/forum/posts/:id/reply", h.Forum.ReplyPost)
	userGroup.POST("/forum/posts/:id/vote", h.Forum.VotePost)
	userGroup.DELETE("/forum/posts/:id", h.Forum.DeletePost)

	// Admin routes
	adminGroup := rg.Group("/admin/forum/posts")
	if adminAuth != nil {
		adminGroup.Use(gin.HandlerFunc(adminAuth))
	}
	adminGroup.GET("", h.Forum.AdminListPosts)
	adminGroup.PUT("/:id/status", h.Forum.AdminSetStatus)
}

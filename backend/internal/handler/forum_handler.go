package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/gatewayctx"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type ForumHandler struct {
	forumService *service.ForumService
}

type forumGatewayResponder struct {
	ctx gatewayctx.GatewayContext
}

func (g forumGatewayResponder) Request() *http.Request {
	if g.ctx == nil {
		return nil
	}
	return g.ctx.Request()
}

func (g forumGatewayResponder) WriteJSON(status int, payload any) {
	if g.ctx == nil {
		return
	}
	g.ctx.WriteJSON(status, payload)
}

func NewForumHandler(forumService *service.ForumService) *ForumHandler {
	return &ForumHandler{
		forumService: forumService,
	}
}

type CreateForumPostRequest struct {
	Content  string `json:"content" binding:"required"`
	ParentID *int64 `json:"parent_id,omitempty"`
}

type ReplyForumPostRequest struct {
	Content string `json:"content" binding:"required"`
}

type VoteForumPostRequest struct {
	VoteType string `json:"vote_type" binding:"required"` // "like", "dislike", "none"
}

type AdminSetForumPostStatusRequest struct {
	Status string `json:"status" binding:"required"` // "active", "hidden"
}

// ListPosts handles public/authenticated listing of root forum posts
// GET /api/v1/forum/posts
func (h *ForumHandler) ListPosts(c *gin.Context) {
	h.ListPostsGateway(gatewayctx.FromGin(c))
}

func (h *ForumHandler) ListPostsGateway(c gatewayctx.GatewayContext) {
	page, _ := strconv.Atoi(c.QueryValue("page"))
	if page <= 0 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(c.QueryValue("page_size"))
	if pageSize <= 0 {
		pageSize = 20
	}
	sort := c.QueryValue("sort")

	var currentUserID int64
	if subject, ok := middleware2.GetAuthSubjectFromGatewayContext(c); ok {
		currentUserID = subject.UserID
	}

	query := service.ForumPostListQuery{
		Sort:     sort,
		Page:     page,
		PageSize: pageSize,
		UserID:   currentUserID,
	}

	posts, pagResult, err := h.forumService.ListPosts(c.Request().Context(), query)
	if err != nil {
		response.ErrorFromContext(forumGatewayResponder{ctx: c}, err)
		return
	}

	response.SuccessContext(forumGatewayResponder{ctx: c}, gin.H{
		"posts":      posts,
		"pagination": pagResult,
	})
}

// ListReplies handles listing replies for a specific post
// GET /api/v1/forum/posts/:id/replies
func (h *ForumHandler) ListReplies(c *gin.Context) {
	h.ListRepliesGateway(gatewayctx.FromGin(c))
}

func (h *ForumHandler) ListRepliesGateway(c gatewayctx.GatewayContext) {
	idStr := c.PathParam("id")
	parentID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.ErrorContext(forumGatewayResponder{ctx: c}, http.StatusBadRequest, "invalid post id")
		return
	}

	page, _ := strconv.Atoi(c.QueryValue("page"))
	if page <= 0 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(c.QueryValue("page_size"))
	if pageSize <= 0 {
		pageSize = 20
	}

	var currentUserID int64
	if subject, ok := middleware2.GetAuthSubjectFromGatewayContext(c); ok {
		currentUserID = subject.UserID
	}

	replies, pagResult, err := h.forumService.ListReplies(c.Request().Context(), parentID, page, pageSize, currentUserID)
	if err != nil {
		response.ErrorFromContext(forumGatewayResponder{ctx: c}, err)
		return
	}

	response.SuccessContext(forumGatewayResponder{ctx: c}, gin.H{
		"post_id":    parentID,
		"replies":    replies,
		"pagination": pagResult,
	})
}

// CreatePost handles creating a root forum post
// POST /api/v1/forum/posts
func (h *ForumHandler) CreatePost(c *gin.Context) {
	h.CreatePostGateway(gatewayctx.FromGin(c))
}

func (h *ForumHandler) CreatePostGateway(c gatewayctx.GatewayContext) {
	subject, ok := middleware2.GetAuthSubjectFromGatewayContext(c)
	if !ok {
		response.ErrorContext(forumGatewayResponder{ctx: c}, http.StatusUnauthorized, "User not authenticated")
		return
	}

	var req CreateForumPostRequest
	if err := c.BindJSON(&req); err != nil {
		response.ErrorContext(forumGatewayResponder{ctx: c}, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	post, err := h.forumService.CreatePost(c.Request().Context(), service.CreateForumPostInput{
		UserID:   subject.UserID,
		ParentID: req.ParentID,
		Content:  req.Content,
	})
	if err != nil {
		response.ErrorFromContext(forumGatewayResponder{ctx: c}, err)
		return
	}

	response.SuccessContext(forumGatewayResponder{ctx: c}, post)
}

// ReplyPost handles posting a reply directly to a post
// POST /api/v1/forum/posts/:id/reply
func (h *ForumHandler) ReplyPost(c *gin.Context) {
	h.ReplyPostGateway(gatewayctx.FromGin(c))
}

func (h *ForumHandler) ReplyPostGateway(c gatewayctx.GatewayContext) {
	subject, ok := middleware2.GetAuthSubjectFromGatewayContext(c)
	if !ok {
		response.ErrorContext(forumGatewayResponder{ctx: c}, http.StatusUnauthorized, "User not authenticated")
		return
	}

	idStr := c.PathParam("id")
	parentID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.ErrorContext(forumGatewayResponder{ctx: c}, http.StatusBadRequest, "invalid post id")
		return
	}

	var req ReplyForumPostRequest
	if err := c.BindJSON(&req); err != nil {
		response.ErrorContext(forumGatewayResponder{ctx: c}, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	post, err := h.forumService.CreatePost(c.Request().Context(), service.CreateForumPostInput{
		UserID:   subject.UserID,
		ParentID: &parentID,
		Content:  req.Content,
	})
	if err != nil {
		response.ErrorFromContext(forumGatewayResponder{ctx: c}, err)
		return
	}

	response.SuccessContext(forumGatewayResponder{ctx: c}, post)
}

// VotePost handles adding, changing, or removing a like/dislike reaction on a post
// POST /api/v1/forum/posts/:id/vote
func (h *ForumHandler) VotePost(c *gin.Context) {
	h.VotePostGateway(gatewayctx.FromGin(c))
}

func (h *ForumHandler) VotePostGateway(c gatewayctx.GatewayContext) {
	subject, ok := middleware2.GetAuthSubjectFromGatewayContext(c)
	if !ok {
		response.ErrorContext(forumGatewayResponder{ctx: c}, http.StatusUnauthorized, "User not authenticated")
		return
	}

	idStr := c.PathParam("id")
	postID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.ErrorContext(forumGatewayResponder{ctx: c}, http.StatusBadRequest, "invalid post id")
		return
	}

	var req VoteForumPostRequest
	if err := c.BindJSON(&req); err != nil {
		response.ErrorContext(forumGatewayResponder{ctx: c}, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	res, err := h.forumService.VotePost(c.Request().Context(), service.VoteForumPostInput{
		PostID:   postID,
		UserID:   subject.UserID,
		VoteType: req.VoteType,
	})
	if err != nil {
		response.ErrorFromContext(forumGatewayResponder{ctx: c}, err)
		return
	}

	response.SuccessContext(forumGatewayResponder{ctx: c}, res)
}

// DeletePost rejects post deletion as per rules (only reactions can be deleted)
// DELETE /api/v1/forum/posts/:id
func (h *ForumHandler) DeletePost(c *gin.Context) {
	h.DeletePostGateway(gatewayctx.FromGin(c))
}

func (h *ForumHandler) DeletePostGateway(c gatewayctx.GatewayContext) {
	response.ErrorFromContext(forumGatewayResponder{ctx: c}, domain.ErrCannotDeleteForumPost)
}

// AdminListPosts handles admin listing of all forum posts with moderation filters
// GET /api/v1/admin/forum/posts
func (h *ForumHandler) AdminListPosts(c *gin.Context) {
	h.AdminListPostsGateway(gatewayctx.FromGin(c))
}

func (h *ForumHandler) AdminListPostsGateway(c gatewayctx.GatewayContext) {
	page, _ := strconv.Atoi(c.QueryValue("page"))
	pageSize, _ := strconv.Atoi(c.QueryValue("page_size"))
	status := strings.TrimSpace(c.QueryValue("status"))

	posts, pagResult, err := h.forumService.AdminListPosts(c.Request().Context(), status, page, pageSize)
	if err != nil {
		response.ErrorFromContext(forumGatewayResponder{ctx: c}, err)
		return
	}

	response.SuccessContext(forumGatewayResponder{ctx: c}, gin.H{
		"posts":      posts,
		"pagination": pagResult,
	})
}

// AdminSetStatus handles updating status (active/hidden) for moderation
// PUT /api/v1/admin/forum/posts/:id/status
func (h *ForumHandler) AdminSetStatus(c *gin.Context) {
	h.AdminSetStatusGateway(gatewayctx.FromGin(c))
}

func (h *ForumHandler) AdminSetStatusGateway(c gatewayctx.GatewayContext) {
	idStr := c.PathParam("id")
	postID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.ErrorContext(forumGatewayResponder{ctx: c}, http.StatusBadRequest, "invalid post id")
		return
	}

	var req AdminSetForumPostStatusRequest
	if err := c.BindJSON(&req); err != nil {
		response.ErrorContext(forumGatewayResponder{ctx: c}, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	if err := h.forumService.AdminSetStatus(c.Request().Context(), postID, req.Status); err != nil {
		response.ErrorFromContext(forumGatewayResponder{ctx: c}, err)
		return
	}

	response.SuccessContext(forumGatewayResponder{ctx: c}, gin.H{"status": req.Status})
}

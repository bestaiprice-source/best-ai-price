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

type ProviderCommentHandler struct {
	commentService *service.ProviderCommentService
}

type commentGatewayResponder struct {
	ctx gatewayctx.GatewayContext
}

func (g commentGatewayResponder) Request() *http.Request {
	if g.ctx == nil {
		return nil
	}
	return g.ctx.Request()
}

func (g commentGatewayResponder) WriteJSON(status int, payload any) {
	if g.ctx == nil {
		return
	}
	g.ctx.WriteJSON(status, payload)
}

func NewProviderCommentHandler(commentService *service.ProviderCommentService) *ProviderCommentHandler {
	return &ProviderCommentHandler{
		commentService: commentService,
	}
}

type CreateCommentRequest struct {
	Content   string `json:"content" binding:"required"`
	Sentiment string `json:"sentiment"` // "positive", "negative" (defaults to "positive")
	ParentID  *int64 `json:"parent_id,omitempty"`
}

type ReplyCommentRequest struct {
	Content string `json:"content" binding:"required"`
}

type VoteCommentRequest struct {
	VoteType string `json:"vote_type" binding:"required"` // "like", "dislike", "none"
}

type AdminSetCommentStatusRequest struct {
	Status string `json:"status" binding:"required"` // "active", "hidden"
}

// ListProviderComments handles public/authenticated listing of root provider reviews
// GET /api/v1/providers/:provider/comments
func (h *ProviderCommentHandler) ListProviderComments(c *gin.Context) {
	h.ListProviderCommentsGateway(gatewayctx.FromGin(c))
}

func (h *ProviderCommentHandler) ListProviderCommentsGateway(c gatewayctx.GatewayContext) {
	provider := c.PathParam("provider")
	if provider == "" {
		response.ErrorContext(commentGatewayResponder{ctx: c}, http.StatusBadRequest, "provider is required")
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
	sentiment := c.QueryValue("sentiment")
	sort := c.QueryValue("sort")

	var currentUserID int64
	if subject, ok := middleware2.GetAuthSubjectFromGatewayContext(c); ok {
		currentUserID = subject.UserID
	}

	query := service.CommentListQuery{
		Provider:  provider,
		Sentiment: sentiment,
		Sort:      sort,
		Page:      page,
		PageSize:  pageSize,
		UserID:    currentUserID,
	}

	comments, pagResult, summary, err := h.commentService.ListComments(c.Request().Context(), query)
	if err != nil {
		response.ErrorFromContext(commentGatewayResponder{ctx: c}, err)
		return
	}

	response.SuccessContext(commentGatewayResponder{ctx: c}, gin.H{
		"provider":   provider,
		"summary":    summary,
		"comments":   comments,
		"pagination": pagResult,
	})
}

// ListReplies handles listing replies for a specific comment
// GET /api/v1/comments/:id/replies
func (h *ProviderCommentHandler) ListReplies(c *gin.Context) {
	h.ListRepliesGateway(gatewayctx.FromGin(c))
}

func (h *ProviderCommentHandler) ListRepliesGateway(c gatewayctx.GatewayContext) {
	idStr := c.PathParam("id")
	parentID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.ErrorContext(commentGatewayResponder{ctx: c}, http.StatusBadRequest, "invalid comment id")
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

	replies, pagResult, err := h.commentService.ListReplies(c.Request().Context(), parentID, page, pageSize, currentUserID)
	if err != nil {
		response.ErrorFromContext(commentGatewayResponder{ctx: c}, err)
		return
	}

	response.SuccessContext(commentGatewayResponder{ctx: c}, gin.H{
		"comment_id": parentID,
		"replies":    replies,
		"pagination": pagResult,
	})
}

// CreateComment handles creating a review for a provider
// POST /api/v1/providers/:provider/comments
func (h *ProviderCommentHandler) CreateComment(c *gin.Context) {
	h.CreateCommentGateway(gatewayctx.FromGin(c))
}

func (h *ProviderCommentHandler) CreateCommentGateway(c gatewayctx.GatewayContext) {
	subject, ok := middleware2.GetAuthSubjectFromGatewayContext(c)
	if !ok {
		response.ErrorContext(commentGatewayResponder{ctx: c}, http.StatusUnauthorized, "User not authenticated")
		return
	}

	provider := c.PathParam("provider")
	if provider == "" {
		response.ErrorContext(commentGatewayResponder{ctx: c}, http.StatusBadRequest, "provider is required")
		return
	}

	var req CreateCommentRequest
	if err := c.BindJSON(&req); err != nil {
		response.ErrorContext(commentGatewayResponder{ctx: c}, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	comment, err := h.commentService.CreateComment(c.Request().Context(), service.CreateCommentInput{
		Provider:  provider,
		UserID:    subject.UserID,
		ParentID:  req.ParentID,
		Content:   req.Content,
		Sentiment: req.Sentiment,
	})
	if err != nil {
		response.ErrorFromContext(commentGatewayResponder{ctx: c}, err)
		return
	}

	response.SuccessContext(commentGatewayResponder{ctx: c}, comment)
}

// ReplyComment handles posting a reply directly to a comment
// POST /api/v1/comments/:id/reply
func (h *ProviderCommentHandler) ReplyComment(c *gin.Context) {
	h.ReplyCommentGateway(gatewayctx.FromGin(c))
}

func (h *ProviderCommentHandler) ReplyCommentGateway(c gatewayctx.GatewayContext) {
	subject, ok := middleware2.GetAuthSubjectFromGatewayContext(c)
	if !ok {
		response.ErrorContext(commentGatewayResponder{ctx: c}, http.StatusUnauthorized, "User not authenticated")
		return
	}

	idStr := c.PathParam("id")
	parentID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.ErrorContext(commentGatewayResponder{ctx: c}, http.StatusBadRequest, "invalid comment id")
		return
	}

	var req ReplyCommentRequest
	if err := c.BindJSON(&req); err != nil {
		response.ErrorContext(commentGatewayResponder{ctx: c}, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	comment, err := h.commentService.CreateComment(c.Request().Context(), service.CreateCommentInput{
		UserID:   subject.UserID,
		ParentID: &parentID,
		Content:  req.Content,
	})
	if err != nil {
		response.ErrorFromContext(commentGatewayResponder{ctx: c}, err)
		return
	}

	response.SuccessContext(commentGatewayResponder{ctx: c}, comment)
}

// VoteComment handles adding, changing, or removing a like/dislike reaction
// POST /api/v1/comments/:id/vote
func (h *ProviderCommentHandler) VoteComment(c *gin.Context) {
	h.VoteCommentGateway(gatewayctx.FromGin(c))
}

func (h *ProviderCommentHandler) VoteCommentGateway(c gatewayctx.GatewayContext) {
	subject, ok := middleware2.GetAuthSubjectFromGatewayContext(c)
	if !ok {
		response.ErrorContext(commentGatewayResponder{ctx: c}, http.StatusUnauthorized, "User not authenticated")
		return
	}

	idStr := c.PathParam("id")
	commentID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.ErrorContext(commentGatewayResponder{ctx: c}, http.StatusBadRequest, "invalid comment id")
		return
	}

	var req VoteCommentRequest
	if err := c.BindJSON(&req); err != nil {
		response.ErrorContext(commentGatewayResponder{ctx: c}, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	res, err := h.commentService.VoteComment(c.Request().Context(), service.VoteCommentInput{
		CommentID: commentID,
		UserID:    subject.UserID,
		VoteType:  req.VoteType,
	})
	if err != nil {
		response.ErrorFromContext(commentGatewayResponder{ctx: c}, err)
		return
	}

	response.SuccessContext(commentGatewayResponder{ctx: c}, res)
}

// DeleteComment rejects comment deletion as per rules (only reactions can be deleted)
// DELETE /api/v1/comments/:id
func (h *ProviderCommentHandler) DeleteComment(c *gin.Context) {
	h.DeleteCommentGateway(gatewayctx.FromGin(c))
}

func (h *ProviderCommentHandler) DeleteCommentGateway(c gatewayctx.GatewayContext) {
	response.ErrorFromContext(commentGatewayResponder{ctx: c}, domain.ErrCannotDeleteComment)
}

// AdminListComments handles admin listing of all comments with moderation filters
// GET /api/v1/admin/provider-comments
func (h *ProviderCommentHandler) AdminListComments(c *gin.Context) {
	h.AdminListCommentsGateway(gatewayctx.FromGin(c))
}

func (h *ProviderCommentHandler) AdminListCommentsGateway(c gatewayctx.GatewayContext) {
	page, _ := strconv.Atoi(c.QueryValue("page"))
	pageSize, _ := strconv.Atoi(c.QueryValue("page_size"))
	provider := strings.TrimSpace(c.QueryValue("provider"))
	status := strings.TrimSpace(c.QueryValue("status"))
	sentiment := strings.TrimSpace(c.QueryValue("sentiment"))

	comments, pagResult, err := h.commentService.AdminListComments(c.Request().Context(), provider, status, sentiment, page, pageSize)
	if err != nil {
		response.ErrorFromContext(commentGatewayResponder{ctx: c}, err)
		return
	}

	response.SuccessContext(commentGatewayResponder{ctx: c}, gin.H{
		"comments":   comments,
		"pagination": pagResult,
	})
}

// AdminSetStatus handles updating status (active/hidden) for moderation
// PUT /api/v1/admin/provider-comments/:id/status
func (h *ProviderCommentHandler) AdminSetStatus(c *gin.Context) {
	h.AdminSetStatusGateway(gatewayctx.FromGin(c))
}

func (h *ProviderCommentHandler) AdminSetStatusGateway(c gatewayctx.GatewayContext) {
	idStr := c.PathParam("id")
	commentID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.ErrorContext(commentGatewayResponder{ctx: c}, http.StatusBadRequest, "invalid comment id")
		return
	}

	var req AdminSetCommentStatusRequest
	if err := c.BindJSON(&req); err != nil {
		response.ErrorContext(commentGatewayResponder{ctx: c}, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	if err := h.commentService.AdminSetStatus(c.Request().Context(), commentID, req.Status); err != nil {
		response.ErrorFromContext(commentGatewayResponder{ctx: c}, err)
		return
	}

	response.SuccessContext(commentGatewayResponder{ctx: c}, gin.H{"status": req.Status})
}

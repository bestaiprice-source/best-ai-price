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

// -----------------------------------------------------------------------------
// Request Payloads
// -----------------------------------------------------------------------------

type CreateForumTopicRequest struct {
	Title   string `json:"title" binding:"required"`
	Content string `json:"content" binding:"required"`
}

type VoteForumTopicRequest struct {
	VoteType string `json:"vote_type" binding:"required"` // "like", "dislike", "none"
}

type AdminSetForumTopicStatusRequest struct {
	Status string `json:"status" binding:"required"` // "active", "hidden"
}

type CreateForumCommentRequest struct {
	Content  string `json:"content" binding:"required"`
	ParentID *int64 `json:"parent_id,omitempty"`
}

type ReplyForumCommentRequest struct {
	Content string `json:"content" binding:"required"`
}

type VoteForumCommentRequest struct {
	VoteType string `json:"vote_type" binding:"required"` // "like", "dislike", "none"
}

type AdminSetForumCommentStatusRequest struct {
	Status string `json:"status" binding:"required"` // "active", "hidden"
}

// -----------------------------------------------------------------------------
// Topics Handlers
// -----------------------------------------------------------------------------

// ListTopics handles public/authenticated listing of topics
// GET /api/v1/forum/topics
func (h *ForumHandler) ListTopics(c *gin.Context) {
	h.ListTopicsGateway(gatewayctx.FromGin(c))
}

func (h *ForumHandler) ListTopicsGateway(c gatewayctx.GatewayContext) {
	page, _ := strconv.Atoi(c.QueryValue("page"))
	if page <= 0 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(c.QueryValue("page_size"))
	if pageSize <= 0 {
		pageSize = 20
	}
	sort := c.QueryValue("sort")
	search := strings.TrimSpace(c.QueryValue("search"))

	var currentUserID int64
	if subject, ok := middleware2.GetAuthSubjectFromGatewayContext(c); ok {
		currentUserID = subject.UserID
	}

	query := service.ForumTopicListQuery{
		Search:   search,
		Sort:     sort,
		Page:     page,
		PageSize: pageSize,
		UserID:   currentUserID,
	}

	topics, pagResult, err := h.forumService.ListTopics(c.Request().Context(), query)
	if err != nil {
		response.ErrorFromContext(forumGatewayResponder{ctx: c}, err)
		return
	}

	response.SuccessContext(forumGatewayResponder{ctx: c}, gin.H{
		"topics":     topics,
		"pagination": pagResult,
	})
}

// CreateTopic handles creating a forum topic
// POST /api/v1/forum/topics
func (h *ForumHandler) CreateTopic(c *gin.Context) {
	h.CreateTopicGateway(gatewayctx.FromGin(c))
}

func (h *ForumHandler) CreateTopicGateway(c gatewayctx.GatewayContext) {
	subject, ok := middleware2.GetAuthSubjectFromGatewayContext(c)
	if !ok {
		response.ErrorContext(forumGatewayResponder{ctx: c}, http.StatusUnauthorized, "User not authenticated")
		return
	}

	var req CreateForumTopicRequest
	if err := c.BindJSON(&req); err != nil {
		response.ErrorContext(forumGatewayResponder{ctx: c}, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	topic, err := h.forumService.CreateTopic(c.Request().Context(), service.CreateForumTopicInput{
		UserID:  subject.UserID,
		Title:   req.Title,
		Content: req.Content,
	})
	if err != nil {
		response.ErrorFromContext(forumGatewayResponder{ctx: c}, err)
		return
	}

	response.SuccessContext(forumGatewayResponder{ctx: c}, topic)
}

// GetTopic handles getting a topic by ID
// GET /api/v1/forum/topics/:id
func (h *ForumHandler) GetTopic(c *gin.Context) {
	h.GetTopicGateway(gatewayctx.FromGin(c))
}

func (h *ForumHandler) GetTopicGateway(c gatewayctx.GatewayContext) {
	idStr := c.PathParam("id")
	topicID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.ErrorContext(forumGatewayResponder{ctx: c}, http.StatusBadRequest, "invalid topic id")
		return
	}

	var currentUserID int64
	if subject, ok := middleware2.GetAuthSubjectFromGatewayContext(c); ok {
		currentUserID = subject.UserID
	}

	topic, err := h.forumService.GetTopic(c.Request().Context(), topicID, currentUserID)
	if err != nil {
		response.ErrorFromContext(forumGatewayResponder{ctx: c}, err)
		return
	}

	response.SuccessContext(forumGatewayResponder{ctx: c}, topic)
}

// VoteTopic handles voting on a topic
// POST /api/v1/forum/topics/:id/vote
func (h *ForumHandler) VoteTopic(c *gin.Context) {
	h.VoteTopicGateway(gatewayctx.FromGin(c))
}

func (h *ForumHandler) VoteTopicGateway(c gatewayctx.GatewayContext) {
	subject, ok := middleware2.GetAuthSubjectFromGatewayContext(c)
	if !ok {
		response.ErrorContext(forumGatewayResponder{ctx: c}, http.StatusUnauthorized, "User not authenticated")
		return
	}

	idStr := c.PathParam("id")
	topicID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.ErrorContext(forumGatewayResponder{ctx: c}, http.StatusBadRequest, "invalid topic id")
		return
	}

	var req VoteForumTopicRequest
	if err := c.BindJSON(&req); err != nil {
		response.ErrorContext(forumGatewayResponder{ctx: c}, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	res, err := h.forumService.VoteTopic(c.Request().Context(), service.VoteForumTopicInput{
		TopicID:  topicID,
		UserID:   subject.UserID,
		VoteType: req.VoteType,
	})
	if err != nil {
		response.ErrorFromContext(forumGatewayResponder{ctx: c}, err)
		return
	}

	response.SuccessContext(forumGatewayResponder{ctx: c}, res)
}

// DeleteTopic rejects topic deletion as per rules
// DELETE /api/v1/forum/topics/:id
func (h *ForumHandler) DeleteTopic(c *gin.Context) {
	h.DeleteTopicGateway(gatewayctx.FromGin(c))
}

func (h *ForumHandler) DeleteTopicGateway(c gatewayctx.GatewayContext) {
	response.ErrorFromContext(forumGatewayResponder{ctx: c}, domain.ErrCannotDeleteForumPost)
}

// AdminListTopics handles admin listing of topics
// GET /api/v1/admin/forum/topics
func (h *ForumHandler) AdminListTopics(c *gin.Context) {
	h.AdminListTopicsGateway(gatewayctx.FromGin(c))
}

func (h *ForumHandler) AdminListTopicsGateway(c gatewayctx.GatewayContext) {
	page, _ := strconv.Atoi(c.QueryValue("page"))
	pageSize, _ := strconv.Atoi(c.QueryValue("page_size"))
	status := strings.TrimSpace(c.QueryValue("status"))

	topics, pagResult, err := h.forumService.AdminListTopics(c.Request().Context(), status, page, pageSize)
	if err != nil {
		response.ErrorFromContext(forumGatewayResponder{ctx: c}, err)
		return
	}

	response.SuccessContext(forumGatewayResponder{ctx: c}, gin.H{
		"topics":     topics,
		"pagination": pagResult,
	})
}

// AdminSetTopicStatus handles admin setting topic status
// PUT /api/v1/admin/forum/topics/:id/status
func (h *ForumHandler) AdminSetTopicStatus(c *gin.Context) {
	h.AdminSetTopicStatusGateway(gatewayctx.FromGin(c))
}

func (h *ForumHandler) AdminSetTopicStatusGateway(c gatewayctx.GatewayContext) {
	idStr := c.PathParam("id")
	topicID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.ErrorContext(forumGatewayResponder{ctx: c}, http.StatusBadRequest, "invalid topic id")
		return
	}

	var req AdminSetForumTopicStatusRequest
	if err := c.BindJSON(&req); err != nil {
		response.ErrorContext(forumGatewayResponder{ctx: c}, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	if err := h.forumService.AdminSetTopicStatus(c.Request().Context(), topicID, req.Status); err != nil {
		response.ErrorFromContext(forumGatewayResponder{ctx: c}, err)
		return
	}

	response.SuccessContext(forumGatewayResponder{ctx: c}, gin.H{"status": req.Status})
}

// -----------------------------------------------------------------------------
// Comments Handlers
// -----------------------------------------------------------------------------

// ListComments handles listing root comments for a topic
// GET /api/v1/forum/topics/:id/comments
func (h *ForumHandler) ListComments(c *gin.Context) {
	h.ListCommentsGateway(gatewayctx.FromGin(c))
}

func (h *ForumHandler) ListCommentsGateway(c gatewayctx.GatewayContext) {
	idStr := c.PathParam("id")
	topicID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.ErrorContext(forumGatewayResponder{ctx: c}, http.StatusBadRequest, "invalid topic id")
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
	sort := c.QueryValue("sort")

	var currentUserID int64
	if subject, ok := middleware2.GetAuthSubjectFromGatewayContext(c); ok {
		currentUserID = subject.UserID
	}

	query := service.ForumCommentListQuery{
		TopicID:  topicID,
		Sort:     sort,
		Page:     page,
		PageSize: pageSize,
		UserID:   currentUserID,
	}

	comments, pagResult, err := h.forumService.ListCommentsByTopic(c.Request().Context(), query)
	if err != nil {
		response.ErrorFromContext(forumGatewayResponder{ctx: c}, err)
		return
	}

	response.SuccessContext(forumGatewayResponder{ctx: c}, gin.H{
		"topic_id":   topicID,
		"comments":   comments,
		"pagination": pagResult,
	})
}

// CreateComment handles creating a comment for a topic
// POST /api/v1/forum/topics/:id/comments
func (h *ForumHandler) CreateComment(c *gin.Context) {
	h.CreateCommentGateway(gatewayctx.FromGin(c))
}

func (h *ForumHandler) CreateCommentGateway(c gatewayctx.GatewayContext) {
	subject, ok := middleware2.GetAuthSubjectFromGatewayContext(c)
	if !ok {
		response.ErrorContext(forumGatewayResponder{ctx: c}, http.StatusUnauthorized, "User not authenticated")
		return
	}

	idStr := c.PathParam("id")
	topicID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.ErrorContext(forumGatewayResponder{ctx: c}, http.StatusBadRequest, "invalid topic id")
		return
	}

	var req CreateForumCommentRequest
	if err := c.BindJSON(&req); err != nil {
		response.ErrorContext(forumGatewayResponder{ctx: c}, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	comment, err := h.forumService.CreateComment(c.Request().Context(), service.CreateForumCommentInput{
		TopicID:  topicID,
		UserID:   subject.UserID,
		ParentID: req.ParentID,
		Content:  req.Content,
	})
	if err != nil {
		response.ErrorFromContext(forumGatewayResponder{ctx: c}, err)
		return
	}

	response.SuccessContext(forumGatewayResponder{ctx: c}, comment)
}

// ListCommentReplies handles listing replies to a specific comment
// GET /api/v1/forum/comments/:id/replies
func (h *ForumHandler) ListCommentReplies(c *gin.Context) {
	h.ListCommentRepliesGateway(gatewayctx.FromGin(c))
}

func (h *ForumHandler) ListCommentRepliesGateway(c gatewayctx.GatewayContext) {
	idStr := c.PathParam("id")
	parentID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.ErrorContext(forumGatewayResponder{ctx: c}, http.StatusBadRequest, "invalid comment id")
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

	replies, pagResult, err := h.forumService.ListCommentReplies(c.Request().Context(), parentID, page, pageSize, currentUserID)
	if err != nil {
		response.ErrorFromContext(forumGatewayResponder{ctx: c}, err)
		return
	}

	response.SuccessContext(forumGatewayResponder{ctx: c}, gin.H{
		"comment_id": parentID,
		"replies":    replies,
		"pagination": pagResult,
	})
}

// ReplyComment handles posting a reply directly to a comment
// POST /api/v1/forum/comments/:id/reply
func (h *ForumHandler) ReplyComment(c *gin.Context) {
	h.ReplyCommentGateway(gatewayctx.FromGin(c))
}

func (h *ForumHandler) ReplyCommentGateway(c gatewayctx.GatewayContext) {
	subject, ok := middleware2.GetAuthSubjectFromGatewayContext(c)
	if !ok {
		response.ErrorContext(forumGatewayResponder{ctx: c}, http.StatusUnauthorized, "User not authenticated")
		return
	}

	idStr := c.PathParam("id")
	parentID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.ErrorContext(forumGatewayResponder{ctx: c}, http.StatusBadRequest, "invalid comment id")
		return
	}

	var req ReplyForumCommentRequest
	if err := c.BindJSON(&req); err != nil {
		response.ErrorContext(forumGatewayResponder{ctx: c}, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	// We need topic_id from parent comment
	parent, err := h.forumService.GetCommentByID(c.Request().Context(), parentID)
	if err != nil {
		response.ErrorFromContext(forumGatewayResponder{ctx: c}, err)
		return
	}

	comment, err := h.forumService.CreateComment(c.Request().Context(), service.CreateForumCommentInput{
		TopicID:  parent.TopicID,
		UserID:   subject.UserID,
		ParentID: &parentID,
		Content:  req.Content,
	})
	if err != nil {
		response.ErrorFromContext(forumGatewayResponder{ctx: c}, err)
		return
	}

	response.SuccessContext(forumGatewayResponder{ctx: c}, comment)
}

// VoteComment handles voting on a comment
// POST /api/v1/forum/comments/:id/vote
func (h *ForumHandler) VoteComment(c *gin.Context) {
	h.VoteCommentGateway(gatewayctx.FromGin(c))
}

func (h *ForumHandler) VoteCommentGateway(c gatewayctx.GatewayContext) {
	subject, ok := middleware2.GetAuthSubjectFromGatewayContext(c)
	if !ok {
		response.ErrorContext(forumGatewayResponder{ctx: c}, http.StatusUnauthorized, "User not authenticated")
		return
	}

	idStr := c.PathParam("id")
	commentID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.ErrorContext(forumGatewayResponder{ctx: c}, http.StatusBadRequest, "invalid comment id")
		return
	}

	var req VoteForumCommentRequest
	if err := c.BindJSON(&req); err != nil {
		response.ErrorContext(forumGatewayResponder{ctx: c}, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	res, err := h.forumService.VoteComment(c.Request().Context(), service.VoteForumCommentInput{
		CommentID: commentID,
		UserID:    subject.UserID,
		VoteType:  req.VoteType,
	})
	if err != nil {
		response.ErrorFromContext(forumGatewayResponder{ctx: c}, err)
		return
	}

	response.SuccessContext(forumGatewayResponder{ctx: c}, res)
}

// DeleteComment rejects comment deletion as per rules
// DELETE /api/v1/forum/comments/:id
func (h *ForumHandler) DeleteComment(c *gin.Context) {
	h.DeleteCommentGateway(gatewayctx.FromGin(c))
}

func (h *ForumHandler) DeleteCommentGateway(c gatewayctx.GatewayContext) {
	response.ErrorFromContext(forumGatewayResponder{ctx: c}, domain.ErrCannotDeleteForumPost)
}

// AdminListComments handles admin listing of comments
// GET /api/v1/admin/forum/comments
func (h *ForumHandler) AdminListComments(c *gin.Context) {
	h.AdminListCommentsGateway(gatewayctx.FromGin(c))
}

func (h *ForumHandler) AdminListCommentsGateway(c gatewayctx.GatewayContext) {
	page, _ := strconv.Atoi(c.QueryValue("page"))
	pageSize, _ := strconv.Atoi(c.QueryValue("page_size"))
	status := strings.TrimSpace(c.QueryValue("status"))

	comments, pagResult, err := h.forumService.AdminListComments(c.Request().Context(), status, page, pageSize)
	if err != nil {
		response.ErrorFromContext(forumGatewayResponder{ctx: c}, err)
		return
	}

	response.SuccessContext(forumGatewayResponder{ctx: c}, gin.H{
		"comments":   comments,
		"pagination": pagResult,
	})
}

// AdminSetCommentStatus handles admin setting comment status
// PUT /api/v1/admin/forum/comments/:id/status
func (h *ForumHandler) AdminSetCommentStatus(c *gin.Context) {
	h.AdminSetCommentStatusGateway(gatewayctx.FromGin(c))
}

func (h *ForumHandler) AdminSetCommentStatusGateway(c gatewayctx.GatewayContext) {
	idStr := c.PathParam("id")
	commentID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.ErrorContext(forumGatewayResponder{ctx: c}, http.StatusBadRequest, "invalid comment id")
		return
	}

	var req AdminSetForumCommentStatusRequest
	if err := c.BindJSON(&req); err != nil {
		response.ErrorContext(forumGatewayResponder{ctx: c}, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	if err := h.forumService.AdminSetCommentStatus(c.Request().Context(), commentID, req.Status); err != nil {
		response.ErrorFromContext(forumGatewayResponder{ctx: c}, err)
		return
	}

	response.SuccessContext(forumGatewayResponder{ctx: c}, gin.H{"status": req.Status})
}

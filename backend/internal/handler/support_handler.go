package handler

import (
	"errors"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/gatewayctx"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// SupportHandler serves the two-way support chat (user side). Admin replies
// arrive through the Telegram bot, not over HTTP.
type SupportHandler struct {
	supportService *service.SupportService
}

func NewSupportHandler(supportService *service.SupportService) *SupportHandler {
	return &SupportHandler{supportService: supportService}
}

type SendSupportMessageRequest struct {
	Message   string `json:"message" binding:"required"`
	RequestID string `json:"request_id"`
	Model     string `json:"model"`
	Page      string `json:"page"`
	// Topic selects the Telegram headline ("manual_invoice" for an invoice
	// request); unknown values fall back to a normal support message.
	Topic string `json:"topic"`
	// Amount is the requested invoice / top-up amount, shown in the alert.
	Amount string `json:"amount"`
}

// Send stores the user's message and forwards it to the admin Telegram bot.
// POST /api/v1/support/messages
func (h *SupportHandler) Send(c *gin.Context) { h.SendGateway(gatewayctx.FromGin(c)) }

func (h *SupportHandler) SendGateway(c gatewayctx.GatewayContext) {
	r := forumGatewayResponder{ctx: c}
	subject, ok := middleware2.GetAuthSubjectFromGatewayContext(c)
	if !ok {
		response.ErrorContext(r, http.StatusUnauthorized, "User not authenticated")
		return
	}
	var req SendSupportMessageRequest
	if err := c.BindJSON(&req); err != nil {
		response.ErrorContext(r, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}
	res, err := h.supportService.SendUserMessage(c.Request().Context(), subject.UserID, service.SupportSendInput{
		Message: req.Message, RequestID: req.RequestID, Model: req.Model, Page: req.Page,
		Topic: req.Topic, Amount: req.Amount,
	})
	switch {
	case errors.Is(err, service.ErrSupportEmptyMessage):
		response.ErrorContext(r, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrSupportRateLimited):
		response.ErrorContext(r, http.StatusTooManyRequests, err.Error())
	case err != nil:
		response.ErrorFromContext(r, err)
	default:
		response.SuccessContext(r, res)
	}
}

// List returns the caller's own support conversation.
// GET /api/v1/support/messages
func (h *SupportHandler) List(c *gin.Context) { h.ListGateway(gatewayctx.FromGin(c)) }

func (h *SupportHandler) ListGateway(c gatewayctx.GatewayContext) {
	r := forumGatewayResponder{ctx: c}
	subject, ok := middleware2.GetAuthSubjectFromGatewayContext(c)
	if !ok {
		response.ErrorContext(r, http.StatusUnauthorized, "User not authenticated")
		return
	}
	thread, err := h.supportService.GetThread(c.Request().Context(), subject.UserID)
	if err != nil {
		response.ErrorFromContext(r, err)
		return
	}
	response.SuccessContext(r, thread)
}

// MarkRead marks the caller's admin replies as read.
// POST /api/v1/support/messages/read
func (h *SupportHandler) MarkRead(c *gin.Context) { h.MarkReadGateway(gatewayctx.FromGin(c)) }

func (h *SupportHandler) MarkReadGateway(c gatewayctx.GatewayContext) {
	r := forumGatewayResponder{ctx: c}
	subject, ok := middleware2.GetAuthSubjectFromGatewayContext(c)
	if !ok {
		response.ErrorContext(r, http.StatusUnauthorized, "User not authenticated")
		return
	}
	if err := h.supportService.MarkRead(c.Request().Context(), subject.UserID); err != nil {
		response.ErrorFromContext(r, err)
		return
	}
	response.SuccessContext(r, gin.H{"ok": true})
}

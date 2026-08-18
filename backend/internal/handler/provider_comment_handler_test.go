package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockCommentRepoForHandler struct {
	comments map[int64]*service.ProviderComment
	nextID   int64
}

func (m *mockCommentRepoForHandler) CreateComment(ctx context.Context, c *service.ProviderComment) error {
	c.ID = m.nextID
	m.nextID++
	if c.ParentID != nil && *c.ParentID > 0 {
		c.Provider = "yunwu"
	}
	m.comments[c.ID] = c
	return nil
}

func (m *mockCommentRepoForHandler) GetByID(ctx context.Context, id int64) (*service.ProviderComment, error) {
	c, ok := m.comments[id]
	if !ok {
		return nil, service.ErrCommentNotFound
	}
	return c, nil
}

func (m *mockCommentRepoForHandler) ListProviderComments(ctx context.Context, query service.CommentListQuery) ([]service.ProviderComment, *pagination.PaginationResult, error) {
	var list []service.ProviderComment
	for _, c := range m.comments {
		if c.Provider == query.Provider && c.ParentID == nil && c.Status == service.CommentStatusActive {
			list = append(list, *c)
		}
	}
	return list, &pagination.PaginationResult{Total: int64(len(list)), Page: query.Page, PageSize: query.PageSize}, nil
}

func (m *mockCommentRepoForHandler) ListReplies(ctx context.Context, parentID int64, page, pageSize int, currentUserID int64) ([]service.ProviderComment, *pagination.PaginationResult, error) {
	var list []service.ProviderComment
	for _, c := range m.comments {
		if c.ParentID != nil && *c.ParentID == parentID && c.Status == service.CommentStatusActive {
			list = append(list, *c)
		}
	}
	return list, &pagination.PaginationResult{Total: int64(len(list)), Page: page, PageSize: pageSize}, nil
}

func (m *mockCommentRepoForHandler) GetProviderSummary(ctx context.Context, provider string) (*service.ProviderCommentSummary, error) {
	return &service.ProviderCommentSummary{Provider: provider, TotalComments: len(m.comments), PositiveCount: len(m.comments), PositiveRatio: 100.0}, nil
}

func (m *mockCommentRepoForHandler) IncrementReplyCount(ctx context.Context, commentID int64) error {
	if c, ok := m.comments[commentID]; ok {
		c.ReplyCount++
	}
	return nil
}

func (m *mockCommentRepoForHandler) SetVote(ctx context.Context, commentID, userID int64, voteType string) (*service.VoteResult, error) {
	return &service.VoteResult{CommentID: commentID, LikeCount: 1, DislikeCount: 0, UserVote: voteType}, nil
}

func (m *mockCommentRepoForHandler) GetUserVotes(ctx context.Context, userID int64, commentIDs []int64) (map[int64]string, error) {
	return make(map[int64]string), nil
}

func (m *mockCommentRepoForHandler) AdminListComments(ctx context.Context, provider, status, sentiment string, page, pageSize int) ([]service.ProviderComment, *pagination.PaginationResult, error) {
	var list []service.ProviderComment
	for _, c := range m.comments {
		list = append(list, *c)
	}
	return list, &pagination.PaginationResult{Total: int64(len(list)), Page: page, PageSize: pageSize}, nil
}

func (m *mockCommentRepoForHandler) AdminSetStatus(ctx context.Context, id int64, status string) error {
	if c, ok := m.comments[id]; ok {
		c.Status = status
	}
	return nil
}

func setupTestCommentRouter(h *handler.ProviderCommentHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	r.GET("/api/v1/providers/:provider/comments", h.ListProviderComments)
	r.GET("/api/v1/comments/:id/replies", h.ListReplies)

	// Simulated auth group
	authGroup := r.Group("")
	authGroup.Use(func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{
				UserID: 42,
			})
		}
		c.Next()
	})

	authGroup.POST("/api/v1/providers/:provider/comments", h.CreateComment)
	authGroup.POST("/api/v1/comments/:id/reply", h.ReplyComment)
	authGroup.POST("/api/v1/comments/:id/vote", h.VoteComment)
	authGroup.DELETE("/api/v1/comments/:id", h.DeleteComment)

	adminGroup := r.Group("/api/v1/admin/provider-comments")
	adminGroup.GET("", h.AdminListComments)
	adminGroup.PUT("/:id/status", h.AdminSetStatus)

	return r
}

func TestProviderCommentHandler(t *testing.T) {
	repo := &mockCommentRepoForHandler{comments: make(map[int64]*service.ProviderComment), nextID: 1}
	svc := service.NewProviderCommentService(repo)
	h := handler.NewProviderCommentHandler(svc)
	router := setupTestCommentRouter(h)

	t.Run("Create comment unauthorized", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"content": "Awesome!"})
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/providers/yunwu/comments", bytes.NewReader(body))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Create comment authorized", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"content": "Great provider!", "sentiment": "positive"})
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/providers/yunwu/comments", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer test_token")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		var resp struct {
			Code int                     `json:"code"`
			Data service.ProviderComment `json:"data"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "Great provider!", resp.Data.Content)
		assert.Equal(t, "yunwu", resp.Data.Provider)
		assert.Equal(t, "positive", resp.Data.Sentiment)
	})

	t.Run("Reply to comment authorized", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"content": "Nice review!"})
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/comments/1/reply", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer test_token")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("List comments and replies public", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/providers/yunwu/comments", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		reqReplies, _ := http.NewRequest(http.MethodGet, "/api/v1/comments/1/replies", nil)
		wReplies := httptest.NewRecorder()
		router.ServeHTTP(wReplies, reqReplies)
		assert.Equal(t, http.StatusOK, wReplies.Code)
	})

	t.Run("Vote on comment", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"vote_type": "like"})
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/comments/1/vote", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer test_token")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Delete comment is forbidden", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, "/api/v1/comments/1", nil)
		req.Header.Set("Authorization", "Bearer test_token")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("Admin operations", func(t *testing.T) {
		reqList, _ := http.NewRequest(http.MethodGet, "/api/v1/admin/provider-comments", nil)
		wList := httptest.NewRecorder()
		router.ServeHTTP(wList, reqList)
		assert.Equal(t, http.StatusOK, wList.Code)

		statusBody, _ := json.Marshal(map[string]string{"status": "hidden"})
		reqStatus, _ := http.NewRequest(http.MethodPut, "/api/v1/admin/provider-comments/1/status", bytes.NewReader(statusBody))
		reqStatus.Header.Set("Content-Type", "application/json")
		wStatus := httptest.NewRecorder()
		router.ServeHTTP(wStatus, reqStatus)
		assert.Equal(t, http.StatusOK, wStatus.Code)
	})
}

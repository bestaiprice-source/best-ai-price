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
)

type mockForumRepoForHandler struct {
	topics        map[int64]*service.ForumTopic
	comments      map[int64]*service.ForumComment
	nextTopicID   int64
	nextCommentID int64
}

func (m *mockForumRepoForHandler) CreateTopic(ctx context.Context, t *service.ForumTopic) error {
	t.ID = m.nextTopicID
	m.nextTopicID++
	m.topics[t.ID] = t
	return nil
}

func (m *mockForumRepoForHandler) GetTopicByID(ctx context.Context, id int64) (*service.ForumTopic, error) {
	t, ok := m.topics[id]
	if !ok {
		return nil, service.ErrForumTopicNotFound
	}
	return t, nil
}

func (m *mockForumRepoForHandler) ListTopics(ctx context.Context, query service.ForumTopicListQuery) ([]service.ForumTopic, *pagination.PaginationResult, error) {
	var list []service.ForumTopic
	for _, t := range m.topics {
		if t.Status == service.ForumTopicStatusActive {
			list = append(list, *t)
		}
	}
	return list, &pagination.PaginationResult{Total: int64(len(list)), Page: query.Page, PageSize: query.PageSize}, nil
}

func (m *mockForumRepoForHandler) SetTopicVote(ctx context.Context, topicID, userID int64, voteType string) (*service.ForumVoteResult, error) {
	return &service.ForumVoteResult{ID: topicID, LikeCount: 1, DislikeCount: 0, UserVote: voteType}, nil
}

func (m *mockForumRepoForHandler) GetTopicUserVotes(ctx context.Context, userID int64, topicIDs []int64) (map[int64]string, error) {
	return make(map[int64]string), nil
}

func (m *mockForumRepoForHandler) AdminListTopics(ctx context.Context, status string, page, pageSize int) ([]service.ForumTopic, *pagination.PaginationResult, error) {
	var list []service.ForumTopic
	for _, t := range m.topics {
		list = append(list, *t)
	}
	return list, &pagination.PaginationResult{Total: int64(len(list)), Page: page, PageSize: pageSize}, nil
}

func (m *mockForumRepoForHandler) AdminSetTopicStatus(ctx context.Context, id int64, status string) error {
	if t, ok := m.topics[id]; ok {
		t.Status = status
	}
	return nil
}

func (m *mockForumRepoForHandler) CreateComment(ctx context.Context, c *service.ForumComment) error {
	c.ID = m.nextCommentID
	m.nextCommentID++
	m.comments[c.ID] = c
	return nil
}

func (m *mockForumRepoForHandler) GetCommentByID(ctx context.Context, id int64) (*service.ForumComment, error) {
	c, ok := m.comments[id]
	if !ok {
		return nil, service.ErrForumCommentNotFound
	}
	return c, nil
}

func (m *mockForumRepoForHandler) ListCommentsByTopic(ctx context.Context, query service.ForumCommentListQuery) ([]service.ForumComment, *pagination.PaginationResult, error) {
	var list []service.ForumComment
	for _, c := range m.comments {
		if c.TopicID == query.TopicID && c.ParentID == nil && c.Status == service.ForumCommentStatusActive {
			list = append(list, *c)
		}
	}
	return list, &pagination.PaginationResult{Total: int64(len(list)), Page: query.Page, PageSize: query.PageSize}, nil
}

func (m *mockForumRepoForHandler) ListCommentReplies(ctx context.Context, parentID int64, page, pageSize int, currentUserID int64) ([]service.ForumComment, *pagination.PaginationResult, error) {
	var list []service.ForumComment
	for _, c := range m.comments {
		if c.ParentID != nil && *c.ParentID == parentID && c.Status == service.ForumCommentStatusActive {
			list = append(list, *c)
		}
	}
	return list, &pagination.PaginationResult{Total: int64(len(list)), Page: page, PageSize: pageSize}, nil
}

func (m *mockForumRepoForHandler) IncrementTopicCommentCount(ctx context.Context, topicID int64) error {
	if t, ok := m.topics[topicID]; ok {
		t.CommentCount++
	}
	return nil
}

func (m *mockForumRepoForHandler) IncrementCommentReplyCount(ctx context.Context, commentID int64) error {
	if c, ok := m.comments[commentID]; ok {
		c.ReplyCount++
	}
	return nil
}

func (m *mockForumRepoForHandler) SetCommentVote(ctx context.Context, commentID, userID int64, voteType string) (*service.ForumVoteResult, error) {
	return &service.ForumVoteResult{ID: commentID, LikeCount: 1, DislikeCount: 0, UserVote: voteType}, nil
}

func (m *mockForumRepoForHandler) GetCommentUserVotes(ctx context.Context, userID int64, commentIDs []int64) (map[int64]string, error) {
	return make(map[int64]string), nil
}

func (m *mockForumRepoForHandler) AdminListComments(ctx context.Context, status string, page, pageSize int) ([]service.ForumComment, *pagination.PaginationResult, error) {
	var list []service.ForumComment
	for _, c := range m.comments {
		list = append(list, *c)
	}
	return list, &pagination.PaginationResult{Total: int64(len(list)), Page: page, PageSize: pageSize}, nil
}

func (m *mockForumRepoForHandler) AdminSetCommentStatus(ctx context.Context, id int64, status string) error {
	if c, ok := m.comments[id]; ok {
		c.Status = status
	}
	return nil
}

func setupTestForumRouter(h *handler.ForumHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	// Public
	r.GET("/api/v1/forum/topics", h.ListTopics)
	r.GET("/api/v1/forum/topics/:id", h.GetTopic)
	r.GET("/api/v1/forum/topics/:id/comments", h.ListComments)
	r.GET("/api/v1/forum/comments/:id/replies", h.ListCommentReplies)

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

	authGroup.POST("/api/v1/forum/topics", h.CreateTopic)
	authGroup.POST("/api/v1/forum/topics/:id/vote", h.VoteTopic)
	authGroup.DELETE("/api/v1/forum/topics/:id", h.DeleteTopic)

	authGroup.POST("/api/v1/forum/topics/:id/comments", h.CreateComment)
	authGroup.POST("/api/v1/forum/comments/:id/reply", h.ReplyComment)
	authGroup.POST("/api/v1/forum/comments/:id/vote", h.VoteComment)
	authGroup.DELETE("/api/v1/forum/comments/:id", h.DeleteComment)

	// Admin
	adminGroup := r.Group("/api/v1/admin/forum")
	adminGroup.GET("/topics", h.AdminListTopics)
	adminGroup.PUT("/topics/:id/status", h.AdminSetTopicStatus)
	adminGroup.GET("/comments", h.AdminListComments)
	adminGroup.PUT("/comments/:id/status", h.AdminSetCommentStatus)

	return r
}

func TestForumHandler(t *testing.T) {
	repo := &mockForumRepoForHandler{
		topics:        make(map[int64]*service.ForumTopic),
		comments:      make(map[int64]*service.ForumComment),
		nextTopicID:   1,
		nextCommentID: 1,
	}
	svc := service.NewForumService(repo)
	h := handler.NewForumHandler(svc)
	router := setupTestForumRouter(h)

	// Seed topic
	_ = repo.CreateTopic(context.Background(), &service.ForumTopic{
		UserID:  42,
		Title:   "Seed Topic",
		Content: "Seed Content",
		Status:  service.ForumTopicStatusActive,
	})

	t.Run("Create topic unauthorized", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"title": "Test", "content": "Awesome!"})
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/forum/topics", bytes.NewReader(body))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Create topic authorized", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"title": "Test", "content": "Awesome!"})
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/forum/topics", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer test-token")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("List topics", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/forum/topics?page=1&page_size=10", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Get topic by ID", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/forum/topics/1", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Vote topic authorized", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"vote_type": "like"})
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/forum/topics/1/vote", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer test-token")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Delete topic forbidden", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, "/api/v1/forum/topics/1", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("Create comment authorized", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"content": "First Comment!"})
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/forum/topics/1/comments", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer test-token")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("List comments", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/forum/topics/1/comments?page=1&page_size=10", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Reply comment authorized", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"content": "Reply to comment 1"})
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/forum/comments/1/reply", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer test-token")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("List replies", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/forum/comments/1/replies?page=1&page_size=10", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Vote comment authorized", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"vote_type": "like"})
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/forum/comments/1/vote", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer test-token")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Delete comment forbidden", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, "/api/v1/forum/comments/1", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("Admin list topics & set status", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/admin/forum/topics?page=1&page_size=10", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		body, _ := json.Marshal(map[string]string{"status": "hidden"})
		req, _ = http.NewRequest(http.MethodPut, "/api/v1/admin/forum/topics/1/status", bytes.NewReader(body))
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Admin list comments & set status", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/admin/forum/comments?page=1&page_size=10", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		body, _ := json.Marshal(map[string]string{"status": "hidden"})
		req, _ = http.NewRequest(http.MethodPut, "/api/v1/admin/forum/comments/1/status", bytes.NewReader(body))
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Invalid parameters and payloads", func(t *testing.T) {
		// Invalid JSON create topic
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/forum/topics", bytes.NewReader([]byte("invalid json")))
		req.Header.Set("Authorization", "Bearer test-token")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)

		// Get topic invalid id
		req, _ = http.NewRequest(http.MethodGet, "/api/v1/forum/topics/abc", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)

		// Vote topic invalid id
		req, _ = http.NewRequest(http.MethodPost, "/api/v1/forum/topics/abc/vote", bytes.NewReader([]byte("{}")))
		req.Header.Set("Authorization", "Bearer test-token")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)

		// Create comment invalid topic id
		req, _ = http.NewRequest(http.MethodPost, "/api/v1/forum/topics/abc/comments", bytes.NewReader([]byte(`{"content":"test"}`)))
		req.Header.Set("Authorization", "Bearer test-token")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)

		// List comments invalid topic id
		req, _ = http.NewRequest(http.MethodGet, "/api/v1/forum/topics/abc/comments", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)

		// Reply comment invalid id
		req, _ = http.NewRequest(http.MethodPost, "/api/v1/forum/comments/abc/reply", bytes.NewReader([]byte(`{"content":"test"}`)))
		req.Header.Set("Authorization", "Bearer test-token")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)

		// List replies invalid id
		req, _ = http.NewRequest(http.MethodGet, "/api/v1/forum/comments/abc/replies", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)

		// Vote comment invalid id
		req, _ = http.NewRequest(http.MethodPost, "/api/v1/forum/comments/abc/vote", bytes.NewReader([]byte("{}")))
		req.Header.Set("Authorization", "Bearer test-token")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)

		// Admin set topic status invalid id
		req, _ = http.NewRequest(http.MethodPut, "/api/v1/admin/forum/topics/abc/status", bytes.NewReader([]byte(`{"status":"hidden"}`)))
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)

		// Admin set comment status invalid id
		req, _ = http.NewRequest(http.MethodPut, "/api/v1/admin/forum/comments/abc/status", bytes.NewReader([]byte(`{"status":"hidden"}`)))
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

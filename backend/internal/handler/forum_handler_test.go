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

type mockForumRepoForHandler struct {
	posts  map[int64]*service.ForumPost
	nextID int64
}

func (m *mockForumRepoForHandler) CreatePost(ctx context.Context, p *service.ForumPost) error {
	p.ID = m.nextID
	m.nextID++
	m.posts[p.ID] = p
	return nil
}

func (m *mockForumRepoForHandler) GetByID(ctx context.Context, id int64) (*service.ForumPost, error) {
	p, ok := m.posts[id]
	if !ok {
		return nil, service.ErrForumPostNotFound
	}
	return p, nil
}

func (m *mockForumRepoForHandler) ListPosts(ctx context.Context, query service.ForumPostListQuery) ([]service.ForumPost, *pagination.PaginationResult, error) {
	var list []service.ForumPost
	for _, p := range m.posts {
		if p.ParentID == nil && p.Status == service.ForumPostStatusActive {
			list = append(list, *p)
		}
	}
	return list, &pagination.PaginationResult{Total: int64(len(list)), Page: query.Page, PageSize: query.PageSize}, nil
}

func (m *mockForumRepoForHandler) ListReplies(ctx context.Context, parentID int64, page, pageSize int, currentUserID int64) ([]service.ForumPost, *pagination.PaginationResult, error) {
	var list []service.ForumPost
	for _, p := range m.posts {
		if p.ParentID != nil && *p.ParentID == parentID && p.Status == service.ForumPostStatusActive {
			list = append(list, *p)
		}
	}
	return list, &pagination.PaginationResult{Total: int64(len(list)), Page: page, PageSize: pageSize}, nil
}

func (m *mockForumRepoForHandler) IncrementReplyCount(ctx context.Context, postID int64) error {
	if p, ok := m.posts[postID]; ok {
		p.ReplyCount++
	}
	return nil
}

func (m *mockForumRepoForHandler) SetVote(ctx context.Context, postID, userID int64, voteType string) (*service.ForumVoteResult, error) {
	return &service.ForumVoteResult{PostID: postID, LikeCount: 1, DislikeCount: 0, UserVote: voteType}, nil
}

func (m *mockForumRepoForHandler) GetUserVotes(ctx context.Context, userID int64, postIDs []int64) (map[int64]string, error) {
	return make(map[int64]string), nil
}

func (m *mockForumRepoForHandler) AdminListPosts(ctx context.Context, status string, page, pageSize int) ([]service.ForumPost, *pagination.PaginationResult, error) {
	var list []service.ForumPost
	for _, p := range m.posts {
		list = append(list, *p)
	}
	return list, &pagination.PaginationResult{Total: int64(len(list)), Page: page, PageSize: pageSize}, nil
}

func (m *mockForumRepoForHandler) AdminSetStatus(ctx context.Context, id int64, status string) error {
	if p, ok := m.posts[id]; ok {
		p.Status = status
	}
	return nil
}

func setupTestForumRouter(h *handler.ForumHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	r.GET("/api/v1/forum/posts", h.ListPosts)
	r.GET("/api/v1/forum/posts/:id/replies", h.ListReplies)

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

	authGroup.POST("/api/v1/forum/posts", h.CreatePost)
	authGroup.POST("/api/v1/forum/posts/:id/reply", h.ReplyPost)
	authGroup.POST("/api/v1/forum/posts/:id/vote", h.VotePost)
	authGroup.DELETE("/api/v1/forum/posts/:id", h.DeletePost)

	adminGroup := r.Group("/api/v1/admin/forum/posts")
	adminGroup.GET("", h.AdminListPosts)
	adminGroup.PUT("/:id/status", h.AdminSetStatus)

	return r
}

func TestForumHandler(t *testing.T) {
	repo := &mockForumRepoForHandler{posts: make(map[int64]*service.ForumPost), nextID: 1}
	svc := service.NewForumService(repo)
	h := handler.NewForumHandler(svc)
	router := setupTestForumRouter(h)

	t.Run("Create post unauthorized", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"content": "Awesome post!"})
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/forum/posts", bytes.NewReader(body))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Create post authorized", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"content": "Awesome post!"})
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/forum/posts", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer test-token")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, float64(0), resp["code"])
	})

	t.Run("Create post invalid request", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/forum/posts", bytes.NewReader([]byte("not json")))
		req.Header.Set("Authorization", "Bearer test-token")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("List posts", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/forum/posts?page=1&page_size=10", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Reply post unauthorized", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"content": "I reply!"})
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/forum/posts/1/reply", bytes.NewReader(body))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Reply post authorized", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"content": "I reply!"})
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/forum/posts/1/reply", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer test-token")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Reply post invalid id", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"content": "I reply!"})
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/forum/posts/invalid/reply", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer test-token")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("List posts with auth and sorting", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/forum/posts?page=1&page_size=10&sort=top_liked", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("List replies with auth", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/forum/posts/1/replies?page=1&page_size=10", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("List replies invalid post id", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/forum/posts/invalid/replies", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Vote post authorized", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"vote_type": "like"})
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/forum/posts/1/vote", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer test-token")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Vote post unauthorized", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"vote_type": "like"})
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/forum/posts/1/vote", bytes.NewReader(body))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Vote post invalid id", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"vote_type": "like"})
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/forum/posts/invalid/vote", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer test-token")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Delete post forbidden", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, "/api/v1/forum/posts/1", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("Admin list posts", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/admin/forum/posts?page=1&page_size=10", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Admin set status", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"status": "hidden"})
		req, _ := http.NewRequest(http.MethodPut, "/api/v1/admin/forum/posts/1/status", bytes.NewReader(body))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Admin set status invalid id", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"status": "hidden"})
		req, _ := http.NewRequest(http.MethodPut, "/api/v1/admin/forum/posts/invalid/status", bytes.NewReader(body))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

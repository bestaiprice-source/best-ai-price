package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockPopularityRepoForHandler struct {
	counts map[string]int64
	times  map[string]time.Time
}

func (m *mockPopularityRepoForHandler) IncrementClick(ctx context.Context, provider string) (*service.ProviderPopularity, error) {
	m.counts[provider]++
	now := time.Now()
	m.times[provider] = now
	return &service.ProviderPopularity{
		Provider:      provider,
		ClickCount:    m.counts[provider],
		LastClickedAt: &now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}

func (m *mockPopularityRepoForHandler) GetByProvider(ctx context.Context, provider string) (*service.ProviderPopularity, error) {
	cnt := m.counts[provider]
	t := m.times[provider]
	var lastClicked *time.Time
	if !t.IsZero() {
		lastClicked = &t
	}
	return &service.ProviderPopularity{
		Provider:      provider,
		ClickCount:    cnt,
		LastClickedAt: lastClicked,
	}, nil
}

func (m *mockPopularityRepoForHandler) ListPopularity(ctx context.Context) ([]service.ProviderPopularityItem, error) {
	var items []service.ProviderPopularityItem
	rank := 1
	for p, cnt := range m.counts {
		t := m.times[p]
		var lastClicked *time.Time
		if !t.IsZero() {
			lastClicked = &t
		}
		items = append(items, service.ProviderPopularityItem{
			Provider:      p,
			ClickCount:    cnt,
			Rank:          rank,
			LastClickedAt: lastClicked,
		})
		rank++
	}
	return items, nil
}

func setupTestPopularityRouter(h *handler.ProviderPopularityHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	r.POST("/api/v1/providers/:provider/click", h.RecordClick)
	r.GET("/api/v1/providers/:provider/popularity", h.GetPopularity)
	r.GET("/api/v1/providers/popularity", h.ListPopularity)

	return r
}

func TestProviderPopularityHandler(t *testing.T) {
	repo := &mockPopularityRepoForHandler{
		counts: make(map[string]int64),
		times:  make(map[string]time.Time),
	}
	svc := service.NewProviderPopularityService(repo, nil)
	h := handler.NewProviderPopularityHandler(svc)
	router := setupTestPopularityRouter(h)

	t.Run("Record click success", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/providers/openai/click", nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 Chrome/120.0.0.0")
		req.Header.Set("X-Forwarded-For", "198.51.100.1")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp struct {
			Code int                       `json:"code"`
			Data service.RecordClickResult `json:"data"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "openai", resp.Data.Provider)
		assert.Equal(t, int64(1), resp.Data.ClickCount)
		assert.True(t, resp.Data.Counted)
	})

	t.Run("Record click cooldown deduplication", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/providers/openai/click", nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 Chrome/120.0.0.0")
		req.Header.Set("X-Forwarded-For", "198.51.100.1")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp struct {
			Code int                       `json:"code"`
			Data service.RecordClickResult `json:"data"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "openai", resp.Data.Provider)
		assert.Equal(t, int64(1), resp.Data.ClickCount)
		assert.False(t, resp.Data.Counted)
		assert.True(t, resp.Data.Cooldown)
	})

	t.Run("Record click bot rejected", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/providers/claude/click", nil)
		req.Header.Set("User-Agent", "curl/7.88.1")
		req.Header.Set("X-Forwarded-For", "198.51.100.2")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("Get single provider popularity", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/providers/openai/popularity", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp struct {
			Code int                        `json:"code"`
			Data service.ProviderPopularity `json:"data"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "openai", resp.Data.Provider)
		assert.Equal(t, int64(1), resp.Data.ClickCount)
	})

	t.Run("List provider popularity", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/providers/popularity", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp struct {
			Code int `json:"code"`
			Data struct {
				Providers []service.ProviderPopularityItem `json:"providers"`
			} `json:"data"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Len(t, resp.Data.Providers, 1)
		assert.Equal(t, "openai", resp.Data.Providers[0].Provider)
	})
}

package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRegisterCommonRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	RegisterCommonRoutes(router)

	t.Run("health", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		require.JSONEq(t, `{"status":"ok"}`, rec.Body.String())
	})

	t.Run("event logging batch", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/event_logging/batch", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("setup status", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/setup/status", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		require.JSONEq(t, `{"code":0,"data":{"needs_setup":false,"step":"completed"}}`, rec.Body.String())
	})

	t.Run("providers json not found default", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/data/providers.json", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		require.Equal(t, http.StatusNotFound, rec.Code)
		require.JSONEq(t, `{"error":"providers.json not found"}`, rec.Body.String())
	})

	t.Run("models json not found default", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/data/models.json", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		require.Equal(t, http.StatusNotFound, rec.Code)
		require.JSONEq(t, `{"error":"models.json not found"}`, rec.Body.String())
	})

	t.Run("api descriptions json not found default", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/data/api_descriptions.json", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		require.Equal(t, http.StatusNotFound, rec.Code)
		require.JSONEq(t, `{"error":"api_descriptions.json not found"}`, rec.Body.String())
	})

	t.Run("provider descriptions json not found default", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/data/provider_descriptions.json", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		require.Equal(t, http.StatusNotFound, rec.Code)
		require.JSONEq(t, `{"error":"provider_descriptions.json not found"}`, rec.Body.String())
	})

	t.Run("executable common routes completeness", func(t *testing.T) {
		routes := ExecutableCommonRoutes()
		require.NotEmpty(t, routes)
		paths := make(map[string]bool)
		for _, r := range routes {
			paths[r.Path] = true
		}
		require.True(t, paths["/health"])
		require.True(t, paths["/setup/status"])
		require.True(t, paths["/data/providers.json"])
		require.True(t, paths["/data/models.json"])
		require.True(t, paths["/data/api_descriptions.json"])
		require.True(t, paths["/api/data/api_descriptions.json"])
		require.True(t, paths["/api/v1/data/api_descriptions.json"])
		require.True(t, paths["/data/provider_descriptions.json"])
		require.True(t, paths["/api/data/provider_descriptions.json"])
		require.True(t, paths["/api/v1/data/provider_descriptions.json"])
	})
}

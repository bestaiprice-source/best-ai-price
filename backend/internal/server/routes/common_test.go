package routes

import (
	"net/http"
	"net/http/httptest"
	"os"
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

	t.Run("llm rankings json not found default", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/data/llm_rankings.json", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		require.Equal(t, http.StatusNotFound, rec.Code)
		require.JSONEq(t, `{"error":"llm_rankings.json not found"}`, rec.Body.String())

		req2 := httptest.NewRequest(http.MethodGet, "/api/data/llm_rankings.json", nil)
		rec2 := httptest.NewRecorder()
		router.ServeHTTP(rec2, req2)

		require.Equal(t, http.StatusNotFound, rec2.Code)
		require.JSONEq(t, `{"error":"llm_rankings.json not found"}`, rec2.Body.String())

		req3 := httptest.NewRequest(http.MethodGet, "/api/v1/data/llm_rankings.json", nil)
		rec3 := httptest.NewRecorder()
		router.ServeHTTP(rec3, req3)

		require.Equal(t, http.StatusNotFound, rec3.Code)
		require.JSONEq(t, `{"error":"llm_rankings.json not found"}`, rec3.Body.String())
	})

	t.Run("provider descriptions json not found default", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/data/provider_descriptions.json", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		require.Equal(t, http.StatusNotFound, rec.Code)
		require.JSONEq(t, `{"error":"providers_description.json not found"}`, rec.Body.String())

		req2 := httptest.NewRequest(http.MethodGet, "/data/providers_description.json", nil)
		rec2 := httptest.NewRecorder()
		router.ServeHTTP(rec2, req2)

		require.Equal(t, http.StatusNotFound, rec2.Code)
		require.JSONEq(t, `{"error":"providers_description.json not found"}`, rec2.Body.String())
	})

	t.Run("llm rankings json serves file when exists", func(t *testing.T) {
		err := os.MkdirAll("./data", 0755)
		require.NoError(t, err)
		filePath := "./data/llm_rankings.json"
		testJSON := `{"rankings":[{"rank":1,"model":"gpt-4o"}]}`
		err = os.WriteFile(filePath, []byte(testJSON), 0644)
		require.NoError(t, err)
		defer os.Remove(filePath)

		for _, p := range []string{"/data/llm_rankings.json", "/api/data/llm_rankings.json", "/api/v1/data/llm_rankings.json"} {
			req := httptest.NewRequest(http.MethodGet, p, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			require.Equal(t, "public, max-age=300", rec.Header().Get("Cache-Control"))
			require.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
			require.JSONEq(t, testJSON, rec.Body.String())
		}
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
		require.True(t, paths["/data/llm_rankings.json"])
		require.True(t, paths["/api/data/llm_rankings.json"])
		require.True(t, paths["/api/v1/data/llm_rankings.json"])
		require.True(t, paths["/data/provider_descriptions.json"])
		require.True(t, paths["/api/data/provider_descriptions.json"])
		require.True(t, paths["/api/v1/data/provider_descriptions.json"])
		require.True(t, paths["/data/providers_description.json"])
		require.True(t, paths["/api/data/providers_description.json"])
		require.True(t, paths["/api/v1/data/providers_description.json"])
	})
}

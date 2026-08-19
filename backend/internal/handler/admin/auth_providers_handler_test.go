package admin

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAuthProvidersHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "auth_providers.json")
	svc := service.NewAuthProvidersServiceWithPaths([]string{filePath})
	h := NewAuthProvidersHandler(svc)

	r := gin.New()
	r.POST("/api/v1/admin/auth-providers", h.SaveAuthProviders)
	r.GET("/api/v1/admin/auth-providers", h.GetAuthProviders)

	// 1. Initial GET should return 404
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/auth-providers", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusNotFound, rec.Code)
	}

	// 2. POST invalid JSON should return 400
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth-providers", bytes.NewBufferString("invalid json"))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	}

	// 3. POST valid JSON should return 200
	validPayload := `{"providers":[{"id":"anthropic","name":"Anthropic"}]}`
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth-providers", bytes.NewBufferString(validPayload))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	}

	// 4. GET should now return 200 with saved JSON data
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/auth-providers", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Body.String(), "anthropic")
	}
}

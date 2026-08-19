package routes

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAuthProvidersRoutes_SecurityAndFunctionality(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 1. Verify that auth_providers.json is NOT in common public routes
	commonRoutes := ExecutableCommonRoutes()
	for _, r := range commonRoutes {
		require.NotContains(t, r.Path, "auth_providers.json", "auth_providers.json must not be publicly exposed in common routes")
	}

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "auth_providers.json")
	authProvidersSvc := service.NewAuthProvidersServiceWithPaths([]string{filePath})
	authProvidersHandler := admin.NewAuthProvidersHandler(authProvidersSvc)

	h := &handler.Handlers{
		Admin: &handler.AdminHandlers{
			AuthProviders: authProvidersHandler,
		},
	}

	// 2. Verify routes presence in executableAdminFeatureRoutes
	execRoutes := executableAdminFeatureRoutes(h)
	require.NotEmpty(t, execRoutes)
	paths := make(map[string]bool)
	for _, r := range execRoutes {
		paths[r.Method+" "+r.Path] = true
	}
	require.True(t, paths["POST /api/v1/admin/auth-providers"])
	require.True(t, paths["PUT /api/v1/admin/auth-providers"])
	require.True(t, paths["GET /api/v1/admin/auth-providers"])
	require.True(t, paths["POST /api/v1/admin/data/auth-providers"])
	require.True(t, paths["POST /api/v1/admin/data/auth_providers.json"])
	require.True(t, paths["POST /data/auth_providers.json"])

	// Setup Router with mock AdminAuth middleware
	mockAdminAuth := middleware.AdminAuthMiddleware(func(c *gin.Context) {
		apiKey := c.GetHeader("x-api-key")
		if apiKey != "valid-admin-key" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	})

	r := gin.New()
	v1 := r.Group("/api/v1")
	RegisterAdminRoutes(v1, h, mockAdminAuth)

	// Add direct endpoints
	r.POST("/data/auth_providers.json", gin.HandlerFunc(mockAdminAuth), h.Admin.AuthProviders.SaveAuthProviders)
	r.PUT("/data/auth_providers.json", gin.HandlerFunc(mockAdminAuth), h.Admin.AuthProviders.SaveAuthProviders)

	// 3. Unauthenticated request to /api/v1/admin/data/auth-providers should fail with 401
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/data/auth-providers", bytes.NewBufferString(`{"providers":[]}`))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
	}

	// 4. Unauthenticated request to direct /data/auth_providers.json should fail with 401
	{
		req := httptest.NewRequest(http.MethodPost, "/data/auth_providers.json", bytes.NewBufferString(`{"providers":[]}`))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
	}

	// 5. Authenticated request with invalid JSON should return 400
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/data/auth-providers", bytes.NewBufferString(`invalid json`))
		req.Header.Set("x-api-key", "valid-admin-key")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	}

	// 6. Authenticated request with valid JSON should return 200 and save file
	validPayload := `{"providers":[{"id":"google","name":"Google"}]}`
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/data/auth-providers", bytes.NewBufferString(validPayload))
		req.Header.Set("x-api-key", "valid-admin-key")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	}

	// 7. Authenticated GET to /api/v1/admin/data/auth-providers should return 200 with content
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/data/auth-providers", nil)
		req.Header.Set("x-api-key", "valid-admin-key")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Body.String(), "google")
	}

	// 8. Direct POST to /data/auth_providers.json with valid admin key should update content
	updatedPayload := `{"providers":[{"id":"github","name":"GitHub"}]}`
	{
		req := httptest.NewRequest(http.MethodPost, "/data/auth_providers.json", bytes.NewBufferString(updatedPayload))
		req.Header.Set("x-api-key", "valid-admin-key")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	}

	// 9. Authenticated GET to verify updated content
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/data/auth-providers", nil)
		req.Header.Set("x-api-key", "valid-admin-key")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Body.String(), "github")
	}
}

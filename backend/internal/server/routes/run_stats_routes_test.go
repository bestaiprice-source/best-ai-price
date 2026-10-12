package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRunStatsRoutes_CommonAndAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "run_stats.json")

	content := `{
  "last_sync_at": "2026-10-12T12:00:00Z",
  "totals": {
    "models": 99,
    "providers": 12,
    "auth_providers": 3,
    "offers": 320
  },
  "current_run": null,
  "last_full_run": null,
  "last_change": null,
  "history": []
}`
	err := os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(t, err)

	svc := service.NewRunStatsServiceWithPaths([]string{filePath})
	h := admin.NewRunStatsHandler(svc)

	handlers := &handler.Handlers{
		Admin: &handler.AdminHandlers{
			RunStats: h,
		},
	}

	// 1. Verify executableAdminFeatureRoutes contains run-stats
	routes := executableAdminFeatureRoutes(handlers)
	var hasAdminRunStats bool
	for _, r := range routes {
		if r.Path == "/api/v1/admin/parser/run-stats" && r.Method == http.MethodGet {
			hasAdminRunStats = true
			break
		}
	}
	require.True(t, hasAdminRunStats, "expected /api/v1/admin/parser/run-stats in ExecutableAdminFeatureRoutes")

	// 2. Test Gin router integration
	r := gin.New()
	adminGroup := r.Group("/api/v1/admin")
	registerAdminFeatureRoutes(adminGroup, handlers)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/parser/run-stats", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int                    `json:"code"`
		Data service.RunStatsResult `json:"data"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.True(t, resp.Data.FileExists)
	require.Equal(t, 99, resp.Data.Totals.Models)
	require.Equal(t, 12, resp.Data.Totals.Providers)

	// 3. Verify that run_stats.json is NOT in common public routes (admin only)
	commonRoutes := ExecutableCommonRoutes()
	for _, cr := range commonRoutes {
		require.NotContains(t, cr.Path, "run_stats.json", "run_stats.json must not be publicly exposed in common routes")
	}
}

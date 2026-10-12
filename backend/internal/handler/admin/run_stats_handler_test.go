package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRunStatsHandler_GetRunStats(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "run_stats.json")

	content := `{
  "last_sync_at": "2026-10-12T12:00:00Z",
  "totals": {
    "models": 50,
    "providers": 8,
    "auth_providers": 2,
    "offers": 200
  },
  "current_run": null,
  "last_full_run": null,
  "last_change": null,
  "history": []
}`
	err := os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(t, err)

	svc := service.NewRunStatsServiceWithPaths([]string{filePath})
	h := NewRunStatsHandler(svc)

	r := gin.New()
	r.GET("/api/v1/admin/parser/run-stats", h.GetRunStats)
	r.GET("/data/run_stats.json", h.GetRunStatsRaw)

	// Test GET /api/v1/admin/parser/run-stats
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
	require.Equal(t, 50, resp.Data.Totals.Models)
	require.Equal(t, 8, resp.Data.Totals.Providers)

	// Test GET /data/run_stats.json raw
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/data/run_stats.json", nil)
	r.ServeHTTP(w2, req2)

	require.Equal(t, http.StatusOK, w2.Code)
	require.Equal(t, "application/json; charset=utf-8", w2.Header().Get("Content-Type"))
	require.Contains(t, w2.Body.String(), `"models": 50`)
}

func TestRunStatsHandler_GetRunStats_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := service.NewRunStatsServiceWithPaths([]string{"/non/existent/path/run_stats.json"})
	h := NewRunStatsHandler(svc)

	r := gin.New()
	r.GET("/api/v1/admin/parser/run-stats", h.GetRunStats)
	r.GET("/data/run_stats.json", h.GetRunStatsRaw)

	// GetRunStats returns 200 with FileExists: false
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/parser/run-stats", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int                    `json:"code"`
		Data service.RunStatsResult `json:"data"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.False(t, resp.Data.FileExists)

	// Raw endpoint returns 404 when file not found
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/data/run_stats.json", nil)
	r.ServeHTTP(w2, req2)

	require.Equal(t, http.StatusNotFound, w2.Code)
}

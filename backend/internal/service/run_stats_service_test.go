package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRunStatsService_GetStats_ExistingFile(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "run_stats.json")

	content := `{
  "last_sync_at": "2026-10-12T12:00:00Z",
  "totals": {
    "models": 120,
    "providers": 15,
    "auth_providers": 3,
    "offers": 450
  },
  "current_run": {
    "status": "running",
    "started_at": "2026-10-12T12:00:00Z",
    "updated_at": "2026-10-12T12:05:00Z",
    "providers_done": 5,
    "providers_total": 15,
    "prices_extracted": 150,
    "errors_count": 0
  },
  "last_full_run": {
    "started_at": "2026-10-12T10:00:00Z",
    "finished_at": "2026-10-12T10:15:00Z",
    "duration_seconds": 900.5,
    "success": true,
    "providers_found": 15,
    "providers_unique": 15,
    "pricing_pages_found": 14,
    "prices_extracted": 450,
    "prices_published": 440,
    "records_needing_review": 2,
    "errors_count": 0
  },
  "last_change": {
    "at": "2026-10-12T12:00:00Z",
    "baseline": false,
    "in_progress": false,
    "new_models": [
      {
        "canonical_model_id": "claude-3-7-sonnet",
        "display_name": "Claude 3.7 Sonnet",
        "providers": ["openlux.ai"],
        "via_auth_provider": false
      }
    ],
    "new_providers": [
      {
        "provider_domain": "openlux.ai",
        "provider_name": "OpenLux",
        "requires_auth": false
      }
    ],
    "removed_models": ["legacy-model"],
    "removed_providers": ["dead-provider.com"],
    "new_offers_count": 10
  },
  "history": []
}`

	err := os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(t, err)

	svc := NewRunStatsServiceWithPaths([]string{filePath})

	res, err := svc.GetStats()
	require.NoError(t, err)
	require.NotNil(t, res)
	require.True(t, res.FileExists)
	require.Equal(t, filePath, res.SourcePath)
	require.NotNil(t, res.FileModTime)
	require.Equal(t, 120, res.Totals.Models)
	require.Equal(t, 15, res.Totals.Providers)
	require.Equal(t, 3, res.Totals.AuthProviders)
	require.Equal(t, 450, res.Totals.Offers)
	require.NotNil(t, res.CurrentRun)
	require.Equal(t, "running", res.CurrentRun.Status)
	require.NotNil(t, res.LastFullRun)
	require.True(t, res.LastFullRun.Success)
	require.NotNil(t, res.LastChange)
	require.Len(t, res.LastChange.NewModels, 1)
	require.Equal(t, "claude-3-7-sonnet", res.LastChange.NewModels[0].CanonicalModelID)
	require.Len(t, res.LastChange.NewProviders, 1)
	require.Equal(t, "openlux.ai", res.LastChange.NewProviders[0].ProviderDomain)
	require.Len(t, res.LastChange.RemovedModels, 1)
	require.Equal(t, "legacy-model", res.LastChange.RemovedModels[0])
	require.Equal(t, 10, res.LastChange.NewOffersCount)

	raw, path, modTime, err := svc.ReadRaw()
	require.NoError(t, err)
	require.Equal(t, []byte(content), raw)
	require.Equal(t, filePath, path)
	require.False(t, modTime.IsZero())
}

func TestRunStatsService_GetStats_NonExistingFile(t *testing.T) {
	tempDir := t.TempDir()
	nonExistentPath := filepath.Join(tempDir, "does_not_exist.json")

	svc := NewRunStatsServiceWithPaths([]string{nonExistentPath})

	res, err := svc.GetStats()
	require.NoError(t, err)
	require.NotNil(t, res)
	require.False(t, res.FileExists)
	require.Equal(t, 0, res.Totals.Models)

	_, _, _, err = svc.ReadRaw()
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestRunStatsService_GetStats_CorruptedFile(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "corrupted.json")
	err := os.WriteFile(filePath, []byte("{ invalid json"), 0644)
	require.NoError(t, err)

	svc := NewRunStatsServiceWithPaths([]string{filePath})

	res, err := svc.GetStats()
	require.Error(t, err)
	require.Nil(t, res)
}

func TestRunStatsService_DefaultPaths(t *testing.T) {
	svc := NewRunStatsService()
	paths := svc.getPaths()
	require.NotEmpty(t, paths)
	require.Contains(t, paths, "/app/data/parser/run_stats.json")
	require.Contains(t, paths, filepath.Clean("./data/run_stats.json"))
}

func TestRunStatsService_MultipleFilesNewestWins(t *testing.T) {
	tempDir := t.TempDir()
	oldFile := filepath.Join(tempDir, "old.json")
	newFile := filepath.Join(tempDir, "new.json")

	err := os.WriteFile(oldFile, []byte(`{"totals":{"models":10}}`), 0644)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	err = os.WriteFile(newFile, []byte(`{"totals":{"models":25}}`), 0644)
	require.NoError(t, err)

	svc := NewRunStatsServiceWithPaths([]string{oldFile, newFile})

	res, err := svc.GetStats()
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, 25, res.Totals.Models)
	require.Equal(t, newFile, res.SourcePath)
}

func TestRunStatsService_AdjacentToProvidersWins(t *testing.T) {
	tempDir := t.TempDir()
	dirWithoutProviders := filepath.Join(tempDir, "stray")
	dirWithProviders := filepath.Join(tempDir, "parser_data")

	require.NoError(t, os.MkdirAll(dirWithoutProviders, 0755))
	require.NoError(t, os.MkdirAll(dirWithProviders, 0755))

	// Write run_stats in dirWithProviders (older) and providers.json
	realStats := filepath.Join(dirWithProviders, "run_stats.json")
	require.NoError(t, os.WriteFile(realStats, []byte(`{"totals":{"models":88}}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dirWithProviders, "providers.json"), []byte(`[]`), 0644))

	time.Sleep(10 * time.Millisecond)

	// Write stray run_stats in dirWithoutProviders (newer, but stray)
	strayStats := filepath.Join(dirWithoutProviders, "run_stats.json")
	require.NoError(t, os.WriteFile(strayStats, []byte(`{"totals":{"models":999}}`), 0644))

	// Save original DefaultParserDirs and restore
	origDirs := DefaultParserDirs
	defer func() { DefaultParserDirs = origDirs }()
	DefaultParserDirs = []string{dirWithoutProviders, dirWithProviders}

	svc := NewRunStatsService()
	res, err := svc.GetStats()
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, 88, res.Totals.Models, "Should prefer run_stats.json in directory containing providers.json")
	require.Equal(t, realStats, res.SourcePath)
}

func TestLocateParserDataFile_EnvOverrides(t *testing.T) {
	tempDir := t.TempDir()
	customDir := filepath.Join(tempDir, "custom")
	require.NoError(t, os.MkdirAll(customDir, 0755))

	t.Setenv("PARSER_DATA_DIR", customDir)
	candidates := LocateParserDataFile("run_stats.json")
	require.NotEmpty(t, candidates)
	require.Equal(t, filepath.Join(customDir, "run_stats.json"), candidates[0])

	t.Setenv("PROVIDERS_PATH", filepath.Join(customDir, "sub", "providers.json"))
	candidates2 := LocateParserDataFile("run_stats.json")
	require.Contains(t, candidates2, filepath.Join(customDir, "sub", "run_stats.json"))
}

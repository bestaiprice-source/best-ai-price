package service

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DefaultRunStatsPaths defines candidate paths for run_stats.json,
// covering host deployments, Docker containers, and standard paths.
var DefaultRunStatsPaths = []string{
	"/root/bestai_prices_parser/public/data/run_stats.json",
	"/root/bestai_prices_parser/data/run_stats.json",
	"/app/data/parser/run_stats.json",
	"/srv/parser-data/data/run_stats.json",
	"/srv/parser-data/run_stats.json",
	"/app/data/run_stats.json",
	"./data/run_stats.json",
	"../data/run_stats.json",
	"./data/parser/run_stats.json",
	"/opt/sub2api/data/parser/run_stats.json",
	"/opt/sub2api/data/run_stats.json",
}

type RunStatsTotals struct {
	Models        int `json:"models"`
	Providers     int `json:"providers"`
	AuthProviders int `json:"auth_providers"`
	Offers        int `json:"offers"`
}

type RunStatsCurrentRun struct {
	Status          string `json:"status"`
	StartedAt       string `json:"started_at"`
	UpdatedAt       string `json:"updated_at"`
	ProvidersDone   int    `json:"providers_done"`
	ProvidersTotal  int    `json:"providers_total"`
	PricesExtracted int    `json:"prices_extracted"`
	ErrorsCount     int    `json:"errors_count"`
}

type RunStatsFullRun struct {
	StartedAt            string  `json:"started_at"`
	FinishedAt           string  `json:"finished_at"`
	DurationSeconds      float64 `json:"duration_seconds"`
	Success              bool    `json:"success"`
	ProvidersFound       int     `json:"providers_found"`
	ProvidersUnique      int     `json:"providers_unique"`
	PricingPagesFound    int     `json:"pricing_pages_found"`
	PricesExtracted      int     `json:"prices_extracted"`
	PricesPublished      int     `json:"prices_published"`
	RecordsNeedingReview int     `json:"records_needing_review"`
	ErrorsCount          int     `json:"errors_count"`
}

type RunStatsNewModel struct {
	CanonicalModelID string   `json:"canonical_model_id"`
	DisplayName      string   `json:"display_name"`
	Providers        []string `json:"providers"`
	ViaAuthProvider  bool     `json:"via_auth_provider"`
}

type RunStatsNewProvider struct {
	ProviderDomain string `json:"provider_domain"`
	ProviderName   string `json:"provider_name"`
	RequiresAuth   bool   `json:"requires_auth"`
}

type RunStatsChange struct {
	At               *string               `json:"at"`
	Baseline         bool                  `json:"baseline"`
	InProgress       bool                  `json:"in_progress"`
	NewModels        []RunStatsNewModel    `json:"new_models"`
	NewProviders     []RunStatsNewProvider `json:"new_providers"`
	RemovedModels    []string              `json:"removed_models"`
	RemovedProviders []string              `json:"removed_providers"`
	NewOffersCount   int                   `json:"new_offers_count"`
}

type RunStatsSnapshot struct {
	Models    []string `json:"models"`
	Providers []string `json:"providers"`
	Offers    any      `json:"offers"`
}

type RunStatsData struct {
	LastSyncAt  *string             `json:"last_sync_at"`
	Totals      RunStatsTotals      `json:"totals"`
	LastChange  *RunStatsChange     `json:"last_change"`
	History     []RunStatsChange    `json:"history"`
	CurrentRun  *RunStatsCurrentRun `json:"current_run"`
	LastFullRun *RunStatsFullRun    `json:"last_full_run"`
	Snapshot    *RunStatsSnapshot   `json:"snapshot"`
}

type RunStatsResult struct {
	RunStatsData
	SourcePath     string            `json:"source_path"`
	FileExists     bool              `json:"file_exists"`
	FileModTime    *time.Time        `json:"file_mod_time,omitempty"`
	CandidatePaths []string          `json:"candidate_paths,omitempty"`
	CheckedErrors  map[string]string `json:"checked_errors,omitempty"`
}

// RunStatsService provides access to parser run_stats.json for the admin panel.
type RunStatsService struct {
	mu          sync.RWMutex
	customPaths []string
}

func NewRunStatsService() *RunStatsService {
	return &RunStatsService{}
}

func NewRunStatsServiceWithPaths(paths []string) *RunStatsService {
	return &RunStatsService{
		customPaths: paths,
	}
}

// FindActiveProvidersDir locates the directory where providers.json currently exists on disk.
func FindActiveProvidersDir() string {
	candidateProvidersPaths := []string{
		"/app/data/parser/providers.json",
		"/srv/parser-data/data/providers.json",
		"/srv/parser-data/providers.json",
		"/root/bestai_prices_parser/public/data/providers.json",
		"/root/bestai_prices_parser/data/providers.json",
		"/app/data/providers.json",
		"./data/providers.json",
		"../data/providers.json",
		"/home/cucum/dev/bestai_prices_parser/public/data/providers.json",
	}
	if envDir := os.Getenv("PARSER_DATA_DIR"); envDir != "" {
		candidateProvidersPaths = append([]string{filepath.Join(envDir, "providers.json")}, candidateProvidersPaths...)
	}
	if envProv := os.Getenv("PROVIDERS_PATH"); envProv != "" {
		candidateProvidersPaths = append([]string{envProv}, candidateProvidersPaths...)
	}
	for _, p := range candidateProvidersPaths {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return filepath.Dir(p)
		}
	}
	return ""
}

func (s *RunStatsService) getPaths() []string {
	if len(s.customPaths) > 0 {
		return s.customPaths
	}
	var paths []string

	if envPath := os.Getenv("RUN_STATS_PATH"); envPath != "" {
		paths = append(paths, envPath)
	}
	if envDir := os.Getenv("PARSER_DATA_DIR"); envDir != "" {
		paths = append(paths, filepath.Join(envDir, "run_stats.json"))
	}
	if envProv := os.Getenv("PROVIDERS_PATH"); envProv != "" {
		paths = append(paths, filepath.Join(filepath.Dir(envProv), "run_stats.json"))
	}

	// 1. Top priority: wherever providers.json is located on the system!
	if provDir := FindActiveProvidersDir(); provDir != "" {
		paths = append(paths,
			filepath.Join(provDir, "run_stats.json"),
			filepath.Join(provDir, "run-stats.json"),
			filepath.Join(provDir, "parser_stats.json"),
			filepath.Join(provDir, "stats.json"),
			filepath.Join(provDir, "..", "run_stats.json"),
			filepath.Join(provDir, "data", "run_stats.json"),
		)
	}

	paths = append(paths, DefaultRunStatsPaths...)
	paths = append(paths,
		"/root/bestai_prices_parser/run_stats.json",
		"/root/bestai_prices_parser/public/run_stats.json",
		"/app/data/parser/data/run_stats.json",
		"/app/data/parser/run-stats.json",
		"/app/data/run-stats.json",
	)

	if homeDir, err := os.UserHomeDir(); err == nil && homeDir != "" {
		paths = append(paths,
			filepath.Join(homeDir, "dev/bestai_prices_parser/public/data/run_stats.json"),
			filepath.Join(homeDir, "bestai_prices_parser/public/data/run_stats.json"),
			filepath.Join(homeDir, "bestai_prices_parser/data/run_stats.json"),
		)
	}

	paths = append(paths,
		"/home/cucum/dev/bestai_prices_parser/public/data/run_stats.json",
	)

	return deduplicatePaths(paths)
}

func deduplicatePaths(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	var result []string
	for _, p := range paths {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		result = append(result, p)
	}
	return result
}

// ReadRawWithDiagnostics searches candidate locations and records any access errors for troubleshooting.
func (s *RunStatsService) ReadRawWithDiagnostics() ([]byte, string, time.Time, []string, map[string]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	paths := s.getPaths()
	var latestData []byte
	var latestPath string
	var latestModTime time.Time
	found := false
	checkedErrors := make(map[string]string)

	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			checkedErrors[p] = err.Error()
			continue
		}
		if info.IsDir() {
			checkedErrors[p] = "target is a directory, not a file"
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			checkedErrors[p] = err.Error()
			continue
		}
		checkedErrors[p] = "ok"
		if !found || info.ModTime().After(latestModTime) {
			latestData = data
			latestPath = p
			latestModTime = info.ModTime()
			found = true
		}
	}

	if !found {
		slog.Warn("[RunStatsService] run_stats.json not found in any candidate path",
			"checked_paths", paths,
			"errors", checkedErrors,
		)
		return nil, "", time.Time{}, paths, checkedErrors, os.ErrNotExist
	}

	slog.Info("[RunStatsService] Successfully located and loaded run_stats.json",
		"source_path", latestPath,
		"mod_time", latestModTime,
	)
	return latestData, latestPath, latestModTime, paths, checkedErrors, nil
}

// ReadRaw searches candidate locations and returns the newest content among existing files.
func (s *RunStatsService) ReadRaw() ([]byte, string, time.Time, error) {
	data, path, modTime, _, _, err := s.ReadRawWithDiagnostics()
	return data, path, modTime, err
}

// GetStats returns the parsed run stats or a graceful result with full diagnostics.
func (s *RunStatsService) GetStats() (*RunStatsResult, error) {
	data, path, modTime, paths, checkedErrors, err := s.ReadRawWithDiagnostics()
	if err != nil {
		if os.IsNotExist(err) {
			return &RunStatsResult{
				FileExists:     false,
				CandidatePaths: paths,
				CheckedErrors:  checkedErrors,
			}, nil
		}
		return nil, err
	}

	var parsed RunStatsData
	if err := json.Unmarshal(data, &parsed); err != nil {
		slog.Error("[RunStatsService] Failed to parse run_stats.json JSON", "path", path, "error", err)
		return nil, err
	}

	result := &RunStatsResult{
		RunStatsData:   parsed,
		SourcePath:     path,
		FileExists:     true,
		FileModTime:    &modTime,
		CandidatePaths: paths,
		CheckedErrors:  checkedErrors,
	}
	return result, nil
}

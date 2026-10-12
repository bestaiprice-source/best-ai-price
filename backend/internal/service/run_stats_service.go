package service

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// DefaultRunStatsPaths defines candidate paths for run_stats.json,
// matching the exact locations where providers.json is located.
var DefaultRunStatsPaths = []string{
	"/app/data/parser/run_stats.json",
	"/root/bestai_prices_parser/public/data/run_stats.json",
	"/srv/parser-data/data/run_stats.json",
	"/app/data/run_stats.json",
	"./data/run_stats.json",
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
	SourcePath  string     `json:"source_path"`
	FileExists  bool       `json:"file_exists"`
	FileModTime *time.Time `json:"file_mod_time,omitempty"`
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

func (s *RunStatsService) getPaths() []string {
	if len(s.customPaths) > 0 {
		return s.customPaths
	}
	return DefaultRunStatsPaths
}

// ReadRaw searches candidate locations and returns the newest content among existing files.
func (s *RunStatsService) ReadRaw() ([]byte, string, time.Time, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	paths := s.getPaths()
	var latestData []byte
	var latestPath string
	var latestModTime time.Time
	found := false

	for _, p := range paths {
		info, err := os.Stat(p)
		if err == nil && !info.IsDir() {
			data, err := os.ReadFile(p)
			if err == nil {
				if !found || info.ModTime().After(latestModTime) {
					latestData = data
					latestPath = p
					latestModTime = info.ModTime()
					found = true
				}
			}
		}
	}

	if !found {
		return nil, "", time.Time{}, os.ErrNotExist
	}

	return latestData, latestPath, latestModTime, nil
}

// GetStats returns the parsed run stats or a graceful empty result
func (s *RunStatsService) GetStats() (*RunStatsResult, error) {
	data, path, modTime, err := s.ReadRaw()
	if err != nil {
		if os.IsNotExist(err) {
			return &RunStatsResult{
				FileExists: false,
			}, nil
		}
		return nil, err
	}

	var parsed RunStatsData
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}

	result := &RunStatsResult{
		RunStatsData: parsed,
		SourcePath:   path,
		FileExists:   true,
		FileModTime:  &modTime,
	}
	return result, nil
}

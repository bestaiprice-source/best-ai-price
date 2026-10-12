package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DefaultParserDirs defines directories searched for parser data files (providers.json, run_stats.json, etc.)
var DefaultParserDirs = []string{
	"/app/data/parser",
	"/app/data",
	"./data",
	"../data",
	"/data",
	"/srv/parser-data/data",
	"/root/bestai_prices_parser/public/data",
	"./deploy/data/parser",
	"./deploy/data",
}

// LocateParserDataFile returns candidate filepaths for a given parser file (e.g. "run_stats.json" or "providers.json").
// As required by architecture, run_stats.json is located next to providers.json in the parser data directory.
// Candidates in directories where providers.json actually exists are prioritized first.
func LocateParserDataFile(filename string) []string {
	var candidates []string

	// Specific override env vars
	if filename == "run_stats.json" {
		if envRunStats := os.Getenv("RUN_STATS_PATH"); envRunStats != "" {
			candidates = append(candidates, envRunStats)
		}
	}
	if envParserDir := os.Getenv("PARSER_DATA_DIR"); envParserDir != "" {
		candidates = append(candidates, filepath.Join(envParserDir, filename))
	}
	if envProviders := os.Getenv("PROVIDERS_PATH"); envProviders != "" {
		candidates = append(candidates, filepath.Join(filepath.Dir(envProviders), filename))
	}

	// 1. Probe for directories where providers.json actually exists:
	// run_stats.json and other parser data files lie next to providers.json.
	for _, dir := range DefaultParserDirs {
		provPath := filepath.Join(dir, "providers.json")
		if info, err := os.Stat(provPath); err == nil && !info.IsDir() {
			candidates = append(candidates, filepath.Join(dir, filename))
		}
	}

	// 2. Default candidate paths across all parser dirs as fallbacks
	for _, dir := range DefaultParserDirs {
		candidates = append(candidates, filepath.Join(dir, filename))
	}

	return deduplicatePaths(candidates)
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

var DefaultRunStatsPaths = []string{
	"/app/data/parser/run_stats.json",
	"/app/data/run_stats.json",
	"./data/run_stats.json",
	"../data/run_stats.json",
	"/data/run_stats.json",
	"/srv/parser-data/data/run_stats.json",
	"/root/bestai_prices_parser/public/data/run_stats.json",
	"./deploy/data/parser/run_stats.json",
	"./deploy/data/run_stats.json",
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

// RunStatsService provides access to parser run_stats.json
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
	return LocateParserDataFile("run_stats.json")
}

// readNewestFrom reads the newest file from an explicit slice of candidate paths
func readNewestFrom(paths []string) ([]byte, string, time.Time, error) {
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

// ReadRaw searches candidate locations and returns the newest content.
// Per system requirements, run_stats.json is located next to providers.json;
// candidate files in directories where providers.json actually exists are strictly prioritized.
func (s *RunStatsService) ReadRaw() ([]byte, string, time.Time, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.customPaths) > 0 {
		return readNewestFrom(s.customPaths)
	}

	paths := s.getPaths()
	var bestData []byte
	var bestPath string
	var bestModTime time.Time
	foundAdjacent := false

	for _, p := range paths {
		dir := filepath.Dir(p)
		provPath := filepath.Join(dir, "providers.json")
		hasProviders := false
		if provInfo, err := os.Stat(provPath); err == nil && !provInfo.IsDir() {
			hasProviders = true
		}

		info, err := os.Stat(p)
		if err == nil && !info.IsDir() {
			data, err := os.ReadFile(p)
			if err == nil {
				if hasProviders {
					if !foundAdjacent || info.ModTime().After(bestModTime) {
						bestData = data
						bestPath = p
						bestModTime = info.ModTime()
						foundAdjacent = true
					}
				} else if !foundAdjacent {
					if bestPath == "" || info.ModTime().After(bestModTime) {
						bestData = data
						bestPath = p
						bestModTime = info.ModTime()
					}
				}
			}
		}
	}

	if bestPath == "" {
		return nil, "", time.Time{}, os.ErrNotExist
	}

	return bestData, bestPath, bestModTime, nil
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

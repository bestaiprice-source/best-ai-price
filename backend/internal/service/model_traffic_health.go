package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Model health data sources.
//
// `probe` is an independent measurement (a channel monitor issues its own
// request), `traffic` is derived from the requests users actually made. Probes
// win when both exist for a model: they measure the upstream even when nobody
// is calling it.
const (
	ModelHealthSourceProbe   = "probe"
	ModelHealthSourceTraffic = "traffic"
)

const (
	// trafficModelMinRequests is the smallest sample we publish an uptime for.
	// One failure out of two requests is not 50% availability, it is noise.
	trafficModelMinRequests = 5
	// trafficModelDegradedAtOrBelowPct / trafficModelFailedAtOrBelowPct map a
	// measured success ratio onto the same vocabulary the probes use, so one
	// legend covers both sources. Inclusive on purpose: a 100-request sample with
	// one failure is 99.00% and must read as "something is failing", not as
	// healthy.
	trafficModelDegradedAtOrBelowPct = 99.0
	trafficModelFailedAtOrBelowPct   = 90.0
	// defaultModelTrafficCacheTTL bounds how often the aggregates are recomputed:
	// the underlying tables are large and the page is opened often.
	defaultModelTrafficCacheTTL = 60 * time.Second
)

// ModelTrafficStatRow is one raw per-model aggregate from the repository.
type ModelTrafficStatRow struct {
	Model           string
	Successes       int64
	Failures        int64
	AvgDurationMs   *float64
	AvgFirstTokenMs *float64
}

// ModelTrafficHealthRepository aggregates logged traffic per model.
type ModelTrafficHealthRepository interface {
	ModelTrafficStats(ctx context.Context, since time.Time) ([]ModelTrafficStatRow, error)
}

// ModelTrafficHealthService turns logged traffic into the same per-model health
// read model the probes produce.
//
// It exists because uptime per model is a user-facing promise: deriving it from
// real requests costs nothing upstream (unlike a probe, which spends a real
// generation per model) and measures exactly what a user experiences. The
// trade-off is honest: a model nobody calls has no uptime, and the catalog
// simply shows no badge for it.
type ModelTrafficHealthService struct {
	repo ModelTrafficHealthRepository
	ttl  time.Duration
	now  func() time.Time

	mu     sync.Mutex
	cache  map[int]modelTrafficCacheEntry
	failed map[int]bool
}

type modelTrafficCacheEntry struct {
	entries   []ModelHealthEntry
	expiresAt time.Time
}

// NewModelTrafficHealthService constructs the traffic-health service.
func NewModelTrafficHealthService(repo ModelTrafficHealthRepository) *ModelTrafficHealthService {
	return &ModelTrafficHealthService{
		repo:   repo,
		ttl:    defaultModelTrafficCacheTTL,
		now:    time.Now,
		cache:  make(map[int]modelTrafficCacheEntry),
		failed: make(map[int]bool),
	}
}

// ListTrafficModelHealth returns one entry per model with enough traffic in the
// window. Models below the sample threshold are omitted rather than published
// with a meaningless percentage.
func (s *ModelTrafficHealthService) ListTrafficModelHealth(ctx context.Context, windowDays int) ([]ModelHealthEntry, error) {
	if s == nil || s.repo == nil {
		return nil, nil
	}
	if windowDays <= 0 {
		windowDays = ModelHealthWindowDays
	}

	s.mu.Lock()
	if entry, ok := s.cache[windowDays]; ok && !s.now().After(entry.expiresAt) {
		cached := cloneModelHealthEntries(entry.entries)
		s.mu.Unlock()
		return cached, nil
	}
	// A failing aggregate must not be retried on every page view.
	if s.failed[windowDays] && s.now().Before(s.cache[windowDays].expiresAt) {
		s.mu.Unlock()
		return nil, nil
	}
	s.mu.Unlock()

	since := s.now().UTC().Add(-time.Duration(windowDays) * 24 * time.Hour)
	rows, err := s.repo.ModelTrafficStats(ctx, since)
	if err != nil {
		s.mu.Lock()
		s.failed[windowDays] = true
		s.cache[windowDays] = modelTrafficCacheEntry{expiresAt: s.now().Add(s.ttl)}
		s.mu.Unlock()
		return nil, fmt.Errorf("model traffic stats: %w", err)
	}

	entries := buildTrafficModelHealth(rows)

	s.mu.Lock()
	s.failed[windowDays] = false
	s.cache[windowDays] = modelTrafficCacheEntry{entries: cloneModelHealthEntries(entries), expiresAt: s.now().Add(s.ttl)}
	s.mu.Unlock()

	return entries, nil
}

// buildTrafficModelHealth folds raw ids onto canonical models and applies the
// sample/status rules.
//
// The same model arrives under several raw ids (`gpt-5.2`, `openai/gpt-5.2`,
// a dated snapshot), so counts are summed per canonical id and the busiest raw
// id becomes the display name.
func buildTrafficModelHealth(rows []ModelTrafficStatRow) []ModelHealthEntry {
	type aggregate struct {
		displayModel string
		bestRequests int64
		successes    int64
		failures     int64
		latencySum   float64
		latencyCount int64
	}

	byCanonical := make(map[string]*aggregate, len(rows))
	for _, row := range rows {
		model := strings.TrimSpace(row.Model)
		if model == "" {
			continue
		}
		key := ToCanonicalModelID(model)
		if key == "" {
			key = strings.ToLower(model)
		}
		agg, ok := byCanonical[key]
		if !ok {
			agg = &aggregate{}
			byCanonical[key] = agg
		}
		requests := row.Successes + row.Failures
		if requests > agg.bestRequests {
			agg.bestRequests = requests
			agg.displayModel = model
		}
		agg.successes += row.Successes
		agg.failures += row.Failures
		if latency := trafficLatencyMs(row); latency != nil && requests > 0 {
			// Weight by traffic so a raw id with two requests cannot skew the
			// model's average latency.
			agg.latencySum += float64(*latency) * float64(requests)
			agg.latencyCount += requests
		}
	}

	out := make([]ModelHealthEntry, 0, len(byCanonical))
	for key, agg := range byCanonical {
		total := agg.successes + agg.failures
		if total < trafficModelMinRequests {
			continue
		}
		var avgLatencyMs *int
		if agg.latencyCount > 0 {
			value := int(agg.latencySum/float64(agg.latencyCount) + 0.5)
			avgLatencyMs = &value
		}
		status, pct := classifyTrafficModelStatus(total, agg.failures, avgLatencyMs)
		out = append(out, ModelHealthEntry{
			Model:           agg.displayModel,
			Candidates:      ModelNameCandidates(agg.displayModel, nil),
			Status:          status,
			AvailabilityPct: pct,
			AvgLatencyMs:    avgLatencyMs,
			Source:          ModelHealthSourceTraffic,
			Successes:       agg.successes,
			Failures:        agg.failures,
			CanonicalModel:  key,
		})
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].Model < out[j].Model })
	return out
}

// trafficLatencyMs prefers the full response time and falls back to the time to
// first token, so a streaming-only model still contributes a latency signal.
func trafficLatencyMs(row ModelTrafficStatRow) *int {
	value := row.AvgDurationMs
	if value == nil {
		value = row.AvgFirstTokenMs
	}
	if value == nil || *value <= 0 {
		return nil
	}
	ms := int(*value + 0.5)
	return &ms
}

// classifyTrafficModelStatus maps a measured success ratio (and latency) onto the
// monitor status vocabulary: operational / degraded / failed.
//
// High latency is `degraded`, matching the probe rule, because a model that
// answers everything in 12 seconds is not "operational" in the sense the legend
// promises.
func classifyTrafficModelStatus(total, failures int64, avgLatencyMs *int) (string, *float64) {
	if total <= 0 {
		return MonitorStatusError, nil
	}
	pct := roundAvailabilityPct(float64(total-failures) / float64(total) * 100)
	switch {
	case pct <= trafficModelFailedAtOrBelowPct:
		return MonitorStatusFailed, &pct
	case pct <= trafficModelDegradedAtOrBelowPct:
		return MonitorStatusDegraded, &pct
	case avgLatencyMs != nil && *avgLatencyMs >= int(monitorDegradedThreshold/time.Millisecond):
		return MonitorStatusDegraded, &pct
	default:
		return MonitorStatusOperational, &pct
	}
}

// roundAvailabilityPct keeps two decimals so the badge reads like the provider
// consoles (`99.44%`).
func roundAvailabilityPct(value float64) float64 {
	if value < 0 {
		value = 0
	}
	if value > 100 {
		value = 100
	}
	return float64(int(value*100+0.5)) / 100
}

// MergeModelHealth combines probe entries (authoritative) with traffic entries.
//
// A model covered by a monitor is never reported twice: the probe measured the
// upstream itself, so its row wins even if it disagrees with the traffic. Every
// other model is filled in from traffic, which is how the catalog gets an uptime
// without paying for a probe per model.
func MergeModelHealth(probe, traffic []ModelHealthEntry) []ModelHealthEntry {
	out := make([]ModelHealthEntry, 0, len(probe)+len(traffic))
	covered := make(map[string]bool, len(probe))
	for _, entry := range probe {
		out = append(out, entry)
		for _, key := range modelHealthKeys(entry) {
			covered[key] = true
		}
	}
	for _, entry := range traffic {
		shadowed := false
		for _, key := range modelHealthKeys(entry) {
			if covered[key] {
				shadowed = true
				break
			}
		}
		if shadowed {
			continue
		}
		out = append(out, entry)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Model < out[j].Model })
	return out
}

// modelHealthKeys are the lowercased keys a row can be matched on: its own
// candidates plus the canonical form of its display name.
func modelHealthKeys(entry ModelHealthEntry) []string {
	keys := make([]string, 0, len(entry.Candidates)+2)
	add := func(raw string) {
		value := strings.ToLower(strings.TrimSpace(raw))
		if value == "" {
			return
		}
		keys = append(keys, value)
		if canonical := ToCanonicalModelID(value); canonical != "" {
			keys = append(keys, canonical)
		}
	}
	for _, candidate := range entry.Candidates {
		add(candidate)
	}
	add(entry.Model)
	add(entry.CanonicalModel)
	return keys
}

// cloneModelHealthEntries copies a slice so a cached value is never mutated by
// callers (the handler sets JSON-only fields on its own copies).
func cloneModelHealthEntries(entries []ModelHealthEntry) []ModelHealthEntry {
	if len(entries) == 0 {
		return entries
	}
	out := make([]ModelHealthEntry, len(entries))
	copy(out, entries)
	return out
}

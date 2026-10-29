//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type modelTrafficRepoStub struct {
	rows  []ModelTrafficStatRow
	err   error
	calls int
}

func (s *modelTrafficRepoStub) ModelTrafficStats(context.Context, time.Time) ([]ModelTrafficStatRow, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.rows, nil
}

func floatPtr(v float64) *float64 { return &v }

func TestClassifyTrafficModelStatus(t *testing.T) {
	degradedLatency := int(monitorDegradedThreshold/time.Millisecond) + 500
	fastLatency := 200

	tests := []struct {
		name      string
		total     int64
		failures  int64
		latency   *int
		wantState string
		wantPct   float64
	}{
		{
			// The provider-console number users expect to see.
			name: "clean sample is operational", total: 360, failures: 2, latency: &fastLatency,
			wantState: MonitorStatusOperational, wantPct: 99.44,
		},
		{
			name: "one failure in a hundred is degraded", total: 100, failures: 1, latency: &fastLatency,
			wantState: MonitorStatusDegraded, wantPct: 99,
		},
		{
			name: "every tenth failing is failed", total: 100, failures: 10, latency: &fastLatency,
			wantState: MonitorStatusFailed, wantPct: 90,
		},
		{
			// Slow but successful must not read as "Normal": the legend promises
			// that green means usable.
			name: "slow sample is degraded", total: 50, failures: 0, latency: &degradedLatency,
			wantState: MonitorStatusDegraded, wantPct: 100,
		},
		{
			name: "no sample yields no percentage", total: 0, failures: 0, latency: nil,
			wantState: MonitorStatusError, wantPct: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, pct := classifyTrafficModelStatus(tt.total, tt.failures, tt.latency)
			assert.Equal(t, tt.wantState, status)
			if tt.wantPct < 0 {
				assert.Nil(t, pct)
				return
			}
			require.NotNil(t, pct)
			assert.Equal(t, tt.wantPct, *pct)
		})
	}
}

func TestBuildTrafficModelHealthFoldsRawIDsOntoOneModel(t *testing.T) {
	entries := buildTrafficModelHealth([]ModelTrafficStatRow{
		{Model: "gpt-5.2", Successes: 18, Failures: 1, AvgDurationMs: floatPtr(1200)},
		// The same model, logged under its canonical id: counts must add up
		// instead of producing two rows for one model.
		{Model: "openai/gpt-5.2", Successes: 21, Failures: 0, AvgDurationMs: floatPtr(800)},
		// Below the sample threshold: never published with a fake percentage.
		{Model: "claude-sonnet-5", Successes: 2, Failures: 0},
	})

	require.Len(t, entries, 1)
	entry := entries[0]
	assert.Equal(t, "openai/gpt-5.2", entry.Model)
	assert.Equal(t, int64(39), entry.Successes)
	assert.Equal(t, int64(1), entry.Failures)
	assert.Equal(t, ModelHealthSourceTraffic, entry.Source)
	require.NotNil(t, entry.AvailabilityPct)
	assert.Equal(t, 97.5, *entry.AvailabilityPct)
	assert.Equal(t, MonitorStatusDegraded, entry.Status)
	// The display name keeps its gateway-produced normalization keys so the
	// catalog can join on them.
	assert.Contains(t, entry.Candidates, "gpt-5.2")
	assert.Equal(t, "openai/gpt-5.2", entry.CanonicalModel)
}

func TestBuildTrafficModelHealthFallsBackToFirstTokenLatency(t *testing.T) {
	entries := buildTrafficModelHealth([]ModelTrafficStatRow{
		{Model: "kimi-k2.7-code", Successes: 10, AvgFirstTokenMs: floatPtr(4200)},
	})

	require.Len(t, entries, 1)
	require.NotNil(t, entries[0].AvgLatencyMs)
	assert.Equal(t, 4200, *entries[0].AvgLatencyMs)
}

func TestMergeModelHealthProbeWinsOverTraffic(t *testing.T) {
	pct := 99.9
	probe := []ModelHealthEntry{{
		Model:           "claude-sonnet-5-20250929",
		Candidates:      ModelNameCandidates("claude-sonnet-5-20250929", nil),
		Status:          MonitorStatusOperational,
		AvailabilityPct: &pct,
		Source:          ModelHealthSourceProbe,
	}}
	trafficPct := 91.0
	traffic := []ModelHealthEntry{
		{
			// Same model measured by a probe: the probe row must win, not be
			// duplicated next to a traffic row.
			Model:           "claude-sonnet-5",
			Candidates:      ModelNameCandidates("claude-sonnet-5", nil),
			Status:          MonitorStatusFailed,
			AvailabilityPct: &trafficPct,
			Source:          ModelHealthSourceTraffic,
		},
		{
			Model:      "gpt-5.2",
			Candidates: ModelNameCandidates("gpt-5.2", nil),
			Status:     MonitorStatusOperational,
			Source:     ModelHealthSourceTraffic,
		},
	}

	merged := MergeModelHealth(probe, traffic)

	require.Len(t, merged, 2)
	assert.Equal(t, "claude-sonnet-5-20250929", merged[0].Model)
	assert.Equal(t, ModelHealthSourceProbe, merged[0].Source)
	assert.Equal(t, "gpt-5.2", merged[1].Model)
	assert.Equal(t, ModelHealthSourceTraffic, merged[1].Source)
}

func TestModelTrafficHealthServiceCachesAndSurvivesFailure(t *testing.T) {
	repo := &modelTrafficRepoStub{rows: []ModelTrafficStatRow{
		{Model: "gpt-5.2", Successes: 10, Failures: 0},
	}}
	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	svc := NewModelTrafficHealthService(repo)
	svc.now = func() time.Time { return now }

	first, err := svc.ListTrafficModelHealth(context.Background(), 7)
	require.NoError(t, err)
	require.Len(t, first, 1)
	assert.Equal(t, 1, repo.calls)

	// Second read is served from the cache (the tables behind it are large).
	second, err := svc.ListTrafficModelHealth(context.Background(), 7)
	require.NoError(t, err)
	require.Len(t, second, 1)
	assert.Equal(t, 1, repo.calls)

	// The same window is a separate cache entry.
	_, err = svc.ListTrafficModelHealth(context.Background(), 30)
	require.NoError(t, err)
	assert.Equal(t, 2, repo.calls)

	// Past the TTL the aggregate is recomputed.
	now = now.Add(2 * defaultModelTrafficCacheTTL)
	_, err = svc.ListTrafficModelHealth(context.Background(), 7)
	require.NoError(t, err)
	assert.Equal(t, 3, repo.calls)

	// A failing aggregate must not be retried on every page view, and must not
	// fail the request either.
	repo.err = errors.New("db down")
	now = now.Add(2 * defaultModelTrafficCacheTTL)
	_, err = svc.ListTrafficModelHealth(context.Background(), 15)
	require.Error(t, err)
	callsAfterFailure := repo.calls
	_, err = svc.ListTrafficModelHealth(context.Background(), 15)
	require.NoError(t, err)
	assert.Equal(t, callsAfterFailure, repo.calls)
}

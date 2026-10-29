package service

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// stubChannelMonitorRepo 只实现本文件需要的行为，其余方法保持零值/空实现，
// 以便测试 read model 的展开、维护窗口与状态覆盖逻辑而不触碰数据库。
type stubChannelMonitorRepo struct {
	monitors []*ChannelMonitor
	latest   map[int64][]*ChannelMonitorLatest
	avail    map[int64][]*ChannelMonitorAvailability
	updated  *ChannelMonitor
	getErr   error
}

func (s *stubChannelMonitorRepo) Create(context.Context, *ChannelMonitor) error { return nil }
func (s *stubChannelMonitorRepo) GetByID(_ context.Context, id int64) (*ChannelMonitor, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	for _, m := range s.monitors {
		if m.ID == id {
			// 与真实仓储一致：APIKey 仍为密文（此处为空，decryptInPlace 会直接返回）。
			clone := *m
			return &clone, nil
		}
	}
	return nil, ErrChannelMonitorNotFound
}
func (s *stubChannelMonitorRepo) Update(_ context.Context, m *ChannelMonitor) error {
	clone := *m
	s.updated = &clone
	return nil
}
func (s *stubChannelMonitorRepo) Delete(context.Context, int64) error { return nil }
func (s *stubChannelMonitorRepo) List(context.Context, ChannelMonitorListParams) ([]*ChannelMonitor, int64, error) {
	return s.monitors, int64(len(s.monitors)), nil
}
func (s *stubChannelMonitorRepo) ListEnabled(context.Context) ([]*ChannelMonitor, error) {
	return s.monitors, nil
}
func (s *stubChannelMonitorRepo) MarkChecked(context.Context, int64, time.Time) error { return nil }
func (s *stubChannelMonitorRepo) InsertHistoryBatch(context.Context, []*ChannelMonitorHistoryRow) error {
	return nil
}
func (s *stubChannelMonitorRepo) DeleteHistoryBefore(context.Context, time.Time) (int64, error) {
	return 0, nil
}
func (s *stubChannelMonitorRepo) ListHistory(context.Context, int64, string, int) ([]*ChannelMonitorHistoryEntry, error) {
	return nil, nil
}
func (s *stubChannelMonitorRepo) ListLatestPerModel(context.Context, int64) ([]*ChannelMonitorLatest, error) {
	return nil, nil
}
func (s *stubChannelMonitorRepo) ComputeAvailability(context.Context, int64, int) ([]*ChannelMonitorAvailability, error) {
	return nil, nil
}
func (s *stubChannelMonitorRepo) ListLatestForMonitorIDs(context.Context, []int64) (map[int64][]*ChannelMonitorLatest, error) {
	return s.latest, nil
}
func (s *stubChannelMonitorRepo) ComputeAvailabilityForMonitors(context.Context, []int64, int) (map[int64][]*ChannelMonitorAvailability, error) {
	return s.avail, nil
}
func (s *stubChannelMonitorRepo) ListRecentHistoryForMonitors(context.Context, []int64, map[int64]string, int) (map[int64][]*ChannelMonitorHistoryEntry, error) {
	return nil, nil
}
func (s *stubChannelMonitorRepo) UpsertDailyRollupsFor(context.Context, time.Time) (int64, error) {
	return 0, nil
}
func (s *stubChannelMonitorRepo) DeleteRollupsBefore(context.Context, time.Time) (int64, error) {
	return 0, nil
}
func (s *stubChannelMonitorRepo) LoadAggregationWatermark(context.Context) (*time.Time, error) {
	return nil, nil
}
func (s *stubChannelMonitorRepo) UpdateAggregationWatermark(context.Context, time.Time) error {
	return nil
}

func healthIntPtr(v int) *int { return &v }

func TestListModelHealthExpandsEveryModelOfEveryMonitor(t *testing.T) {
	repo := &stubChannelMonitorRepo{
		monitors: []*ChannelMonitor{{
			ID:           7,
			Name:         "OpenLux Claude",
			Provider:     "anthropic",
			PrimaryModel: "claude-3-7-sonnet-20250219",
			ExtraModels:  []string{"claude-3-5-haiku", "claude-3-7-sonnet-20250219"},
		}},
		latest: map[int64][]*ChannelMonitorLatest{
			7: {{Model: "claude-3-7-sonnet-20250219", Status: MonitorStatusOperational}},
		},
		avail: map[int64][]*ChannelMonitorAvailability{
			7: {{Model: "claude-3-7-sonnet-20250219", AvailabilityPct: 99.44, AvgLatencyMs: healthIntPtr(812)}},
		},
	}
	svc := NewChannelMonitorService(repo, nil)

	entries, err := svc.ListModelHealth(context.Background(), 0)
	if err != nil {
		t.Fatalf("ListModelHealth: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 models (primary + one distinct extra, de-duplicated), got %d", len(entries))
	}

	primary := entries[0]
	if primary.Model != "claude-3-5-haiku" {
		// Sorted by model name inside the monitor.
		t.Fatalf("unexpected first entry order: %s", primary.Model)
	}
	if primary.Status != "" || primary.AvailabilityPct != nil {
		t.Fatalf("model without history must stay empty, got status=%q availability=%v", primary.Status, primary.AvailabilityPct)
	}
	if len(primary.Candidates) == 0 {
		t.Fatal("every entry must carry its normalization keys for the catalog join")
	}

	sonnet := entries[1]
	if sonnet.Status != MonitorStatusOperational {
		t.Fatalf("expected the latest probe status, got %q", sonnet.Status)
	}
	if sonnet.AvailabilityPct == nil || *sonnet.AvailabilityPct != 99.44 {
		t.Fatalf("expected 7d availability 99.44, got %v", sonnet.AvailabilityPct)
	}
	if sonnet.AvgLatencyMs == nil || *sonnet.AvgLatencyMs != 812 {
		t.Fatalf("expected avg latency 812, got %v", sonnet.AvgLatencyMs)
	}
	if sonnet.Provider != "anthropic" || sonnet.MonitorID != 7 || sonnet.MonitorName != "OpenLux Claude" {
		t.Fatalf("monitor attribution missing: %+v", sonnet)
	}

	// The join key the catalog relies on: the monitored upstream id must
	// normalize onto the catalog's canonical slug.
	joined := false
	for _, c := range sonnet.Candidates {
		if c == "claude-3-7-sonnet" {
			joined = true
		}
	}
	if !joined {
		t.Fatalf("candidates must include the date-stripped slug used by the catalog, got %v", sonnet.Candidates)
	}
}

func TestListModelHealthReportsMaintenanceAndDropsAvailability(t *testing.T) {
	until := time.Now().UTC().Add(2 * time.Hour)
	repo := &stubChannelMonitorRepo{
		monitors: []*ChannelMonitor{{
			ID:               3,
			PrimaryModel:     "gpt-5.2",
			MaintenanceUntil: &until,
			MaintenanceNote:  "scheduled upstream migration",
		}},
		latest: map[int64][]*ChannelMonitorLatest{
			3: {{Model: "gpt-5.2", Status: MonitorStatusFailed}},
		},
		avail: map[int64][]*ChannelMonitorAvailability{
			3: {{Model: "gpt-5.2", AvailabilityPct: 42.5}},
		},
	}
	svc := NewChannelMonitorService(repo, nil)

	entries, err := svc.ListModelHealth(context.Background(), ModelHealthWindowDays)
	if err != nil {
		t.Fatalf("ListModelHealth: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Status != MonitorStatusMaintenance {
		t.Fatalf("planned maintenance must replace the probe status, got %q", entries[0].Status)
	}
	if entries[0].AvailabilityPct != nil {
		t.Fatalf("a maintenance window must not report a misleading uptime, got %v", *entries[0].AvailabilityPct)
	}
	if entries[0].MaintenanceNote != "scheduled upstream migration" {
		t.Fatalf("maintenance note must survive, got %q", entries[0].MaintenanceNote)
	}
}

func TestListModelHealthOnExpiredWindowKeepsProbeStatus(t *testing.T) {
	until := time.Now().UTC().Add(-time.Minute)
	repo := &stubChannelMonitorRepo{
		monitors: []*ChannelMonitor{{ID: 4, PrimaryModel: "gpt-5.2", MaintenanceUntil: &until}},
		latest: map[int64][]*ChannelMonitorLatest{
			4: {{Model: "gpt-5.2", Status: MonitorStatusOperational}},
		},
	}
	svc := NewChannelMonitorService(repo, nil)

	entries, err := svc.ListModelHealth(context.Background(), ModelHealthWindowDays)
	if err != nil {
		t.Fatalf("ListModelHealth: %v", err)
	}
	if entries[0].Status != MonitorStatusOperational {
		t.Fatalf("an expired window must not mask the real status, got %q", entries[0].Status)
	}
}

func TestChannelMonitorInMaintenance(t *testing.T) {
	future := time.Now().UTC().Add(time.Hour)
	past := time.Now().UTC().Add(-time.Hour)

	if (&ChannelMonitor{}).InMaintenance(time.Now()) {
		t.Fatal("a monitor without a window is not in maintenance")
	}
	if (&ChannelMonitor{MaintenanceUntil: &past}).InMaintenance(time.Now()) {
		t.Fatal("an expired window is not in maintenance")
	}
	if !(&ChannelMonitor{MaintenanceUntil: &future}).InMaintenance(time.Now()) {
		t.Fatal("a future window is in maintenance")
	}
	var nilMonitor *ChannelMonitor
	if nilMonitor.InMaintenance(time.Now()) {
		t.Fatal("nil monitor must be nil-safe")
	}
}

func TestSetMaintenanceReusesUpdateAndClearsOnNil(t *testing.T) {
	repo := &stubChannelMonitorRepo{monitors: []*ChannelMonitor{{ID: 11, Name: "n", PrimaryModel: "m"}}}
	svc := NewChannelMonitorService(repo, nil)

	until := time.Now().UTC().Add(3 * time.Hour).Truncate(time.Second)
	if _, err := svc.SetMaintenance(context.Background(), 11, &until, "  upstream upgrade  "); err != nil {
		t.Fatalf("SetMaintenance: %v", err)
	}
	if repo.updated == nil || repo.updated.MaintenanceUntil == nil {
		t.Fatal("entering maintenance must persist the window")
	}
	if !repo.updated.MaintenanceUntil.Equal(until) {
		t.Fatalf("window mismatch: %v != %v", repo.updated.MaintenanceUntil, until)
	}
	if repo.updated.MaintenanceNote != "upstream upgrade" {
		t.Fatalf("note must be trimmed, got %q", repo.updated.MaintenanceNote)
	}

	if _, err := svc.SetMaintenance(context.Background(), 11, nil, ""); err != nil {
		t.Fatalf("SetMaintenance(clear): %v", err)
	}
	if repo.updated.MaintenanceUntil != nil {
		t.Fatalf("nil until must clear the window, got %v", repo.updated.MaintenanceUntil)
	}
}

func TestApplyMonitorMaintenanceUpdateClearBeatsUntil(t *testing.T) {
	existing := &ChannelMonitor{MaintenanceUntil: &time.Time{}}
	until := time.Now().UTC().Add(time.Hour)
	if err := applyMonitorMaintenanceUpdate(existing, ChannelMonitorUpdateParams{
		ClearMaintenance: true,
		MaintenanceUntil: &until,
	}); err != nil {
		t.Fatalf("applyMonitorMaintenanceUpdate: %v", err)
	}
	if existing.MaintenanceUntil != nil {
		t.Fatal("ClearMaintenance must win over MaintenanceUntil")
	}

	untouched := &ChannelMonitor{}
	if err := applyMonitorMaintenanceUpdate(untouched, ChannelMonitorUpdateParams{}); err != nil {
		t.Fatalf("applyMonitorMaintenanceUpdate: %v", err)
	}
	if untouched.MaintenanceUntil != nil || untouched.MaintenanceNote != "" {
		t.Fatal("absent params must not modify the window")
	}
}

func TestClampMaintenanceNote(t *testing.T) {
	if got := clampMaintenanceNote("  short  "); got != "short" {
		t.Fatalf("expected trimmed note, got %q", got)
	}
	long := strings.Repeat("a", 500)
	if got := clampMaintenanceNote(long); len(got) != modelHealthNoteMaxLen {
		t.Fatalf("expected clamp to %d, got %d", modelHealthNoteMaxLen, len(got))
	}
	// Multi-byte runes are clamped by rune count (the column is VARCHAR(200)),
	// so no rune may be split.
	cyrillic := strings.Repeat("д", 300)
	got := clampMaintenanceNote(cyrillic)
	if runes := []rune(got); len(runes) != modelHealthNoteMaxLen {
		t.Fatalf("expected %d runes, got %d", modelHealthNoteMaxLen, len(runes))
	}
	if strings.ContainsRune(got, utf8.RuneError) || !strings.HasPrefix(cyrillic, got) {
		t.Fatalf("clamped note broke a multi-byte rune: %q", got[len(got)-4:])
	}
}

func TestMonitorModelsDeduplicatesAndSkipsBlanks(t *testing.T) {
	models := monitorModels(&ChannelMonitor{
		PrimaryModel: " a ",
		ExtraModels:  []string{"a", "", "b"},
	})
	if len(models) != 2 || models[0] != "a" || models[1] != "b" {
		t.Fatalf("unexpected model list: %v", models)
	}
	if monitorModels(nil) != nil {
		t.Fatal("nil monitor must yield no models")
	}
}

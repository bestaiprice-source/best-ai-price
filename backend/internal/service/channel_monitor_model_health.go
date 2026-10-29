package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ModelHealthWindowDays 是模型健康视图默认的可用率窗口（天）。
const ModelHealthWindowDays = 7

// modelHealthNoteMaxLen 与 channel_monitors.maintenance_note 的 schema 上限一致。
const modelHealthNoteMaxLen = 200

// ListModelHealth 返回「模型 → 健康」的扁平 read model。
//
// 它是纯读投影：不新增表、不新增写入，只把已有聚合（批量 latest + 批量
// 可用率）按模型展开。用户端仪表盘健康块与模型目录卡片共用这一份数据，
// 因此两处的状态/可用率不可能各说各话。
//
// 处于计划维护窗口内的监控，其模型状态为 maintenance、可用率为 nil：
// 计划内停机不应被读成一次可用率下跌。
func (s *ChannelMonitorService) ListModelHealth(ctx context.Context, windowDays int) ([]ModelHealthEntry, error) {
	if windowDays <= 0 {
		windowDays = ModelHealthWindowDays
	}

	monitors, err := s.repo.ListEnabled(ctx)
	if err != nil {
		return nil, fmt.Errorf("list enabled monitors: %w", err)
	}
	if len(monitors) == 0 {
		return []ModelHealthEntry{}, nil
	}

	ids := make([]int64, 0, len(monitors))
	for _, m := range monitors {
		ids = append(ids, m.ID)
	}

	availByMonitor, err := s.repo.ComputeAvailabilityForMonitors(ctx, ids, windowDays)
	if err != nil {
		// 与其它用户视图一致：聚合失败只降级（无可用率），不阻断整个响应。
		availByMonitor = map[int64][]*ChannelMonitorAvailability{}
	}
	latestByMonitor, err := s.repo.ListLatestForMonitorIDs(ctx, ids)
	if err != nil {
		latestByMonitor = map[int64][]*ChannelMonitorLatest{}
	}

	now := time.Now().UTC()
	out := make([]ModelHealthEntry, 0, len(monitors)*2)
	for _, m := range monitors {
		latest := indexLatestByModel(latestByMonitor[m.ID])
		avail := indexAvailabilityByModel(availByMonitor[m.ID])
		inMaintenance := m.InMaintenance(now)

		for _, model := range monitorModels(m) {
			entry := ModelHealthEntry{
				Model:            model,
				Candidates:       ModelNameCandidates(model, nil),
				Provider:         m.Provider,
				MonitorID:        m.ID,
				MonitorName:      m.Name,
				MaintenanceUntil: m.MaintenanceUntil,
				MaintenanceNote:  m.MaintenanceNote,
			}
			if row := latest[model]; row != nil {
				entry.Status = row.Status
			}
			if row := avail[model]; row != nil {
				pct := row.AvailabilityPct
				entry.AvailabilityPct = &pct
				entry.AvgLatencyMs = row.AvgLatencyMs
			}
			if inMaintenance {
				entry.Status = MonitorStatusMaintenance
				entry.AvailabilityPct = nil
			}
			out = append(out, entry)
		}
	}

	// 稳定排序：先按监控 id，再按模型名，保证同一份数据每次顺序一致。
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].MonitorID != out[j].MonitorID {
			return out[i].MonitorID < out[j].MonitorID
		}
		return out[i].Model < out[j].Model
	})
	return out, nil
}

// monitorModels 返回监控覆盖的全部模型（主模型在前，去重、去空白）。
func monitorModels(m *ChannelMonitor) []string {
	if m == nil {
		return nil
	}
	models := make([]string, 0, 1+len(m.ExtraModels))
	seen := make(map[string]bool, 1+len(m.ExtraModels))
	for _, raw := range append([]string{m.PrimaryModel}, m.ExtraModels...) {
		model := strings.TrimSpace(raw)
		if model == "" || seen[model] {
			continue
		}
		seen[model] = true
		models = append(models, model)
	}
	return models
}

// clampMaintenanceNote 裁剪维护说明，避免超出 schema 上限导致写库失败。
//
// 按 RUNE 截断：channel_monitors.maintenance_note 是 VARCHAR(200)，
// PostgreSQL 按字符计数，按字节截断会在多字节字符中间劈开一个 rune。
func clampMaintenanceNote(note string) string {
	note = strings.TrimSpace(note)
	runes := []rune(note)
	if len(runes) <= modelHealthNoteMaxLen {
		return note
	}
	return string(runes[:modelHealthNoteMaxLen])
}

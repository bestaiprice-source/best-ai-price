package observability

import "testing"

func TestMetricsSnapshotUsesRequiredNamesAndAveragesLatency(t *testing.T) {
	var metrics Metrics
	metrics.ObserveRequest(10)
	metrics.ObserveRequest(30)
	metrics.RequestFailed()
	metrics.Upstream429()
	metrics.Rebinding()
	metrics.RebindingFailed()
	metrics.AddUsage(42)
	metrics.SummaryJob()
	metrics.Snapshot()

	got := metrics.Current()
	if got.RequestsTotal != 2 || got.RequestsFailedTotal != 1 || got.LatencyMs != 20 {
		t.Fatalf("unexpected request metrics: %+v", got)
	}
	if got.Upstream429Total != 1 || got.RebindingTotal != 1 || got.RebindingFailedTotal != 1 {
		t.Fatalf("unexpected recovery metrics: %+v", got)
	}
	if got.UsageUnitsTotal != 42 || got.SummaryJobsTotal != 1 || got.SnapshotTotal != 1 {
		t.Fatalf("unexpected work metrics: %+v", got)
	}
}

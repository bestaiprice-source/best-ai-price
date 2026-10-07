package yunwu

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
)

func TestContextWithModelRouting(t *testing.T) {
	ctx := context.Background()
	if got := ModelRoutingFromContext(ctx); got != nil {
		t.Fatalf("expected nil routing for empty context, got %v", got)
	}

	if got := ModelRoutingFromContext(ContextWithModelRouting(ctx, nil)); got != nil {
		t.Fatalf("expected nil routing for empty list, got %v", got)
	}
	if got := ModelRoutingFromContext(ContextWithModelRouting(ctx, []int64{0, -3})); got != nil {
		t.Fatalf("expected nil routing for non-positive ids, got %v", got)
	}

	got := ModelRoutingFromContext(ContextWithModelRouting(ctx, []int64{5, 5, 9, 0}))
	if len(got) != 2 || got[0] != 5 || got[1] != 9 {
		t.Fatalf("expected deduped ordered routing [5 9], got %v", got)
	}
}

func TestUpstreamStatusCode(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{nil, 0},
		{errors.New("connection reset by peer"), 0},
		{fmt.Errorf("upstream error status 429: slow down"), 429},
		{fmt.Errorf("upstream error status 500: boom"), 500},
		{fmt.Errorf("upstream error status abc: bad"), 0},
	}
	for _, tc := range cases {
		if got := upstreamStatusCode(tc.err); got != tc.want {
			t.Errorf("upstreamStatusCode(%v) = %d, want %d", tc.err, got, tc.want)
		}
	}
}

func TestIsClientRequestError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"network timeout", errors.New("context deadline exceeded"), false},
		{"client reset", errors.New("read: connection reset by peer"), false},
		{"429 upstream", fmt.Errorf("upstream error status 429: rate limited"), false},
		{"500 upstream", fmt.Errorf("upstream error status 500: internal error"), false},
		{"400 invalid request", fmt.Errorf(`upstream error status 400: {"error":{"type":"invalid_request_error","message":"bad param"}}`), true},
		{"422 validation", fmt.Errorf(`upstream error status 422: validation error: max_tokens too large`), true},
		{"400 model not found is failover-eligible", fmt.Errorf(`upstream error status 400: {"error":"model_not_found"}`), false},
		{"400 no available channel is failover-eligible", fmt.Errorf("upstream error status 400: no available channel for model"), false},
		{"400 quota is failover-eligible", fmt.Errorf("upstream error status 400: insufficient quota"), false},
		{"400 overload is failover-eligible", fmt.Errorf("upstream error status 400: upstream overload"), false},
		{"404 not client error", fmt.Errorf("upstream error status 404: not found"), false},
		{"401 key invalid not client error", fmt.Errorf("upstream error status 401: unauthorized"), false},
		{"400 unspecified body", fmt.Errorf("upstream error status 400: some unknown problem"), false},
	}
	for _, tc := range cases {
		if got := isClientRequestError(tc.err); got != tc.want {
			t.Errorf("%s: isClientRequestError = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestDescribeRouting(t *testing.T) {
	if got := describeRouting(nil); got != "pool" {
		t.Errorf("describeRouting(nil) = %q, want pool", got)
	}
	if got := describeRouting([]int64{1, 2}); got != "explicit([1 2])" {
		t.Errorf("describeRouting = %q", got)
	}
}

func TestSortNodesByPrice(t *testing.T) {
	nodes := []health.ExecutionNode{
		{NodeID: "expensive"},
		{NodeID: "cheap"},
		{NodeID: "mid"},
		{NodeID: "unpriced"},
	}
	prices := map[string]float64{
		"expensive": 0.180,
		"cheap":     0.018,
		"mid":       0.060,
	}
	sortNodesByPrice(nodes, prices)

	got := []string{nodes[0].NodeID, nodes[1].NodeID, nodes[2].NodeID, nodes[3].NodeID}
	want := []string{"cheap", "mid", "expensive", "unpriced"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sortNodesByPrice order = %v, want %v", got, want)
		}
	}
}

func TestPricePrimaryBackup(t *testing.T) {
	prices := map[string]float64{"a": 0.018, "b": 0.060, "c": 0.180}

	// Distinct price tiers → primary cheapest, backup next tier.
	nodes := []health.ExecutionNode{{NodeID: "a"}, {NodeID: "b"}, {NodeID: "c"}}
	primary, backup := pricePrimaryBackup(nodes, prices)
	if primary != "a" || backup != "b" {
		t.Fatalf("expected primary=a backup=b, got primary=%s backup=%s", primary, backup)
	}

	// Single price tier (ties) → no distinct backup.
	tie := []health.ExecutionNode{{NodeID: "a"}, {NodeID: "x"}}
	primary, backup = pricePrimaryBackup(tie, map[string]float64{"a": 0.05, "x": 0.05})
	if primary != "a" || backup != "" {
		t.Fatalf("expected primary=a backup=\"\", got primary=%s backup=%s", primary, backup)
	}

	// No prices → empty.
	primary, backup = pricePrimaryBackup(nodes, map[string]float64{})
	if primary != "" || backup != "" {
		t.Fatalf("expected empty result, got primary=%s backup=%s", primary, backup)
	}
}

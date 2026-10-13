package yunwu

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/config"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func TestClassifyProbeStatus(t *testing.T) {
	cases := []struct {
		name   string
		status int
		err    error
		want   probeOutcome
	}{
		{"200 healthy", http.StatusOK, nil, probeHealthy},
		{"204 healthy", http.StatusNoContent, nil, probeHealthy},
		{"404 reachable", http.StatusNotFound, nil, probeHealthy},
		{"405 reachable", http.StatusMethodNotAllowed, nil, probeHealthy},
		{"400 reachable", http.StatusBadRequest, nil, probeHealthy},
		{"401 invalid key", http.StatusUnauthorized, nil, probeAuthInvalid},
		{"403 invalid key", http.StatusForbidden, nil, probeAuthInvalid},
		{"429 rate limited", http.StatusTooManyRequests, nil, probeRateLimited},
		{"500 server error", http.StatusInternalServerError, nil, probeServerError},
		{"503 server error", http.StatusServiceUnavailable, nil, probeServerError},
		{"network error", 0, errors.New("connection refused"), probeUnreachable},
	}
	for _, tc := range cases {
		if got := classifyProbeStatus(tc.status, tc.err); got != tc.want {
			t.Errorf("%s: classifyProbeStatus(%d,%v) = %d, want %d", tc.name, tc.status, tc.err, got, tc.want)
		}
	}
}

func TestYunwuHealthProber_ProbeEndpoint(t *testing.T) {
	ctx := context.Background()

	// 1. Healthy 200
	srv200 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/models", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data": []}`))
	}))
	defer srv200.Close()

	prober := NewYunwuHealthProber(nil, nil, nil, nil)
	outcome, lat := prober.probeEndpoint(ctx, srv200.URL, "test-key")
	assert.Equal(t, probeHealthy, outcome)
	assert.GreaterOrEqual(t, lat, int64(0))

	// 2. Auth invalid 401
	srv401 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error": "invalid api key"}`))
	}))
	defer srv401.Close()

	outcome, _ = prober.probeEndpoint(ctx, srv401.URL, "bad-key")
	assert.Equal(t, probeAuthInvalid, outcome)

	// 3. Rate limited 429
	srv429 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv429.Close()

	outcome, _ = prober.probeEndpoint(ctx, srv429.URL, "key")
	assert.Equal(t, probeRateLimited, outcome)

	// 4. Server error 500
	srv500 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv500.Close()

	outcome, _ = prober.probeEndpoint(ctx, srv500.URL, "key")
	assert.Equal(t, probeServerError, outcome)

	// 5. Unreachable (closed port)
	outcome, _ = prober.probeEndpoint(ctx, "http://127.0.0.1:54321", "key")
	assert.Equal(t, probeUnreachable, outcome)
}

func TestYunwuHealthProber_MarkUnavailableAndRecover(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := storage.NewStorage(nil, rdb)
	ctx := context.Background()

	cfg := &config.HypervisorConfig{
		TelegramAlertsEnabled: false,
	}
	notifier := NewTelegramNotifier(store, cfg)
	prober := NewYunwuHealthProber(store, cfg, nil, notifier)

	nodeID := "node-probe-100"
	node := &health.ExecutionNode{
		NodeID:      nodeID,
		AccountName: "Acc-100",
		Provider:    "openlux",
		State:       "healthy",
	}
	_ = prober.nm.SaveNode(ctx, node)

	state := &health.NodeRuntimeState{
		NodeID: nodeID,
	}

	// 1. Mark unavailable
	prober.markUnavailable(ctx, nodeID, state, "Acc-100", "openlux")
	savedNode, err := prober.nm.GetNode(ctx, nodeID)
	if err != nil || savedNode == nil {
		t.Fatalf("expected node: %v", err)
	}
	assert.Equal(t, "unavailable", savedNode.State)

	// 2. Recover node when Redis flag not set -> does nothing
	prober.recoverNode(ctx, nodeID, state)
	savedNode, _ = prober.nm.GetNode(ctx, nodeID)
	assert.Equal(t, "unavailable", savedNode.State)

	// 3. Set Redis flag, then recover node
	_ = store.Redis().Set(ctx, probeUnavailableKey(nodeID), "1", time.Hour).Err()
	prober.recoverNode(ctx, nodeID, state)
	savedNode, _ = prober.nm.GetNode(ctx, nodeID)
	assert.Equal(t, "healthy", savedNode.State)
}

func TestYunwuHealthProber_ProbeAccount(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := storage.NewStorage(nil, rdb)
	ctx := context.Background()

	prober := NewYunwuHealthProber(store, nil, nil, nil)

	// Empty creds
	prober.probeAccount(ctx, 101, "Acc-101", "openlux", nil)
	prober.probeAccount(ctx, 102, "Acc-102", "openlux", map[string]any{"api_key": ""})

	// Healthy server
	srvHealthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srvHealthy.Close()

	prober.probeAccount(ctx, 103, "Acc-103", "openlux", map[string]any{
		"api_key":  "sk-test",
		"base_url": srvHealthy.URL,
	})
	s, err := prober.nm.GetNodeRuntimeState(ctx, "103")
	assert.NoError(t, err)
	assert.NotNil(t, s)
	assert.GreaterOrEqual(t, s.ConsecutiveSuccesses, 1)

	// Auth invalid server
	srvAuthInvalid := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srvAuthInvalid.Close()

	prober.probeAccount(ctx, 104, "Acc-104", "openlux", map[string]any{
		"api_key":  "sk-test-bad",
		"base_url": srvAuthInvalid.URL,
	})
	s4, err := prober.nm.GetNodeRuntimeState(ctx, "104")
	assert.NoError(t, err)
	assert.NotNil(t, s4)
	assert.Equal(t, 0, s4.ConsecutiveSuccesses)

	// Start disabled
	prober.Start(ctx, 0)
	(*YunwuHealthProber)(nil).Start(ctx, time.Second)
}

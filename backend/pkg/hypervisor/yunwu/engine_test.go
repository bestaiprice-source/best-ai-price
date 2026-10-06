package yunwu

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/quota"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/runtime"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/session"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
)

type mockYunwuRuntime struct {
	lastNodeID string
}

func (m *mockYunwuRuntime) Execute(ctx context.Context, nodeID string, reqPayload []byte, stream io.Writer) (*runtime.ResponseMetadata, error) {
	m.lastNodeID = nodeID
	return &runtime.ResponseMetadata{
		StatusCode:   200,
		InputTokens:  150,
		OutputTokens: 250,
	}, nil
}

type mockYunwuLiveRuntime struct {
	mockYunwuRuntime
	nodeID string
	path   string
}

func (m *mockYunwuLiveRuntime) ProxyWebSocket(_ context.Context, nodeID, path string, _ http.Header, _ runtime.WebSocketPeer) error {
	m.nodeID = nodeID
	m.path = path
	return nil
}

func setupTestYunwuStore(t *testing.T) *storage.Storage {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	t.Cleanup(func() { _ = rdb.Close() })
	return storage.NewStorage(nil, rdb)
}

func setTestBalance(t *testing.T, store *storage.Storage, nodeID string, microUSD int64) {
	t.Helper()
	now := time.Now().UTC()
	b, err := json.Marshal(BalanceSnapshot{AvailableUSD: float64(microUSD) / 1_000_000, AvailableMicroUSD: microUSD, APIAvailableMicroUSD: microUSD, EstimatedAvailableMicroUSD: microUSD, ReserveMicroUSD: microUSD / 10, SynchronizedAt: now, StaleAt: now.Add(30 * time.Minute), Status: "fresh"})
	require.NoError(t, err)
	require.NoError(t, store.Redis().Set(context.Background(), balanceKey(nodeID), b, 0).Err())
}

func TestYunwuHypervisorEndToEnd(t *testing.T) {
	t.Setenv("ASH_YUNWU_TEST_MODE", "true")

	store := setupTestYunwuStore(t)
	rt := &mockYunwuRuntime{}
	yh := NewYunwuHypervisor(store, rt)

	// Add test node in Redis
	nm := health.NewNodeManager(store)
	err := nm.SaveNode(context.Background(), &health.ExecutionNode{
		NodeID:      "yunwu-101",
		AccountName: "Yunwu Channel Test 1",
		Provider:    "yunwu",
		State:       "healthy",
	})
	require.NoError(t, err)

	err = store.Redis().SAdd(context.Background(), "ash:yunwu:nodes", "yunwu-101").Err()
	require.NoError(t, err)

	// Set test credit balance and node runtime state
	setTestBalance(t, store, "yunwu-101", 50_000_000)

	err = nm.SaveNodeRuntimeState(context.Background(), &health.NodeRuntimeState{
		NodeID:      "yunwu-101",
		SuccessRate: 1.0,
	})
	require.NoError(t, err)

	// Save test user access profile
	err = yh.qm.SaveUserAccess(context.Background(), &quota.UserAccess{
		OwnerID:      "test-owner-1",
		PlanID:       "pool-basic",
		MonthlyLimit: 1000000,
		UsedUnits:    100,
		Priority:     1,
		IsActive:     true,
	}, "yunwu")
	require.NoError(t, err)

	var buf bytes.Buffer
	meta, err := yh.HandleRequest(context.Background(), "test-owner-1", "conv-debate-1", []byte(`{"model":"claude-3-5-sonnet-20241022"}`), &buf, "anthropic")
	require.NoError(t, err)
	assert.NotNil(t, meta)
	assert.Equal(t, 200, meta.StatusCode)
	assert.Equal(t, int64(150), meta.InputTokens)
	assert.Equal(t, int64(250), meta.OutputTokens)

	// Verify Health Status API
	status := yh.GetHealthStatus(context.Background())
	assert.Equal(t, "ok", status["status"])
	assert.Equal(t, "yunwu", status["platform"])
	assert.Equal(t, 1, status["total_nodes"])
	assert.Equal(t, 1, status["healthy_nodes"])
}

func TestYunwuLiveUsesIsolatedFreshBalanceAndReconcilesReservation(t *testing.T) {
	t.Setenv("ASH_YUNWU_TEST_MODE", "true")
	store := setupTestYunwuStore(t)
	rt := &mockYunwuLiveRuntime{}
	yh := NewYunwuHypervisor(store, rt)
	nm := health.NewNodeManager(store)
	require.NoError(t, nm.SaveNode(context.Background(), &health.ExecutionNode{NodeID: "live-node", AccountName: "Live", Provider: "yunwu", State: "healthy"}))
	require.NoError(t, store.Redis().SAdd(context.Background(), "ash:yunwu:nodes", "live-node").Err())
	setTestBalance(t, store, "live-node", 50_000_000)
	require.NoError(t, yh.qm.SaveUserAccess(context.Background(), &quota.UserAccess{OwnerID: "live-owner", MonthlyLimitMicroUSD: 50_000_000, Priority: 1, IsActive: true}, "yunwu"))

	err := yh.HandleLive(context.Background(), "live-owner", "session", "/ws/google.ai.generativelanguage.v1beta.GenerativeService.BidiGenerateContent", nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "live-node", rt.nodeID)
	assert.Contains(t, rt.path, "BidiGenerateContent")
	reserved, err := store.Redis().Get(context.Background(), reservedKey("live-node")).Int64()
	if err != nil {
		assert.ErrorIs(t, err, redis.Nil)
	} else {
		assert.Zero(t, reserved)
	}
}

func TestYunwuEphemeralLiveUsesOpaqueSingleUseTokenAndIssuingNode(t *testing.T) {
	t.Setenv("ASH_YUNWU_TEST_MODE", "true")
	store := setupTestYunwuStore(t)
	rt := &mockYunwuLiveRuntime{}
	yh := NewYunwuHypervisor(store, rt)
	nm := health.NewNodeManager(store)
	for _, nodeID := range []string{"issuer-node", "other-node"} {
		require.NoError(t, nm.SaveNode(context.Background(), &health.ExecutionNode{NodeID: nodeID, AccountName: nodeID, Provider: "yunwu", State: "healthy"}))
		require.NoError(t, store.Redis().SAdd(context.Background(), "ash:yunwu:nodes", nodeID).Err())
		setTestBalance(t, store, nodeID, 50_000_000)
	}
	require.NoError(t, yh.qm.SaveUserAccess(context.Background(), &quota.UserAccess{OwnerID: "ephemeral-owner", MonthlyLimitMicroUSD: 50_000_000, Priority: 1, IsActive: true}, "yunwu"))
	upstream := "auth_tokens/vendor-secret"
	require.NoError(t, store.Redis().Set(context.Background(), yunwuUpstreamTokenNodeKey(upstream), "issuer-node", time.Hour).Err())
	gatewayToken, err := yh.ExchangeEphemeralToken(context.Background(), "ephemeral-owner", upstream, time.Minute, 1)
	require.NoError(t, err)
	assert.NotContains(t, gatewayToken, "vendor-secret")

	err = yh.HandleEphemeralLive(context.Background(), gatewayToken, "session", "/ws/google.ai.generativelanguage.v1beta.GenerativeService.BidiGenerateContentConstrained", nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "issuer-node", rt.nodeID)
	assert.Contains(t, rt.path, "access_token=auth_tokens%2Fvendor-secret")

	err = yh.HandleEphemeralLive(context.Background(), gatewayToken, "session-2", "/ws/google.ai.generativelanguage.v1beta.GenerativeService.BidiGenerateContentConstrained", nil, nil)
	require.ErrorContains(t, err, "invalid or expired ephemeral token")
}

func TestYunwuRebindingOnFailure(t *testing.T) {
	store := setupTestYunwuStore(t)
	nm := health.NewNodeManager(store)

	node1 := health.ExecutionNode{NodeID: "yw-1", AccountName: "Yunwu 1", State: "healthy"}
	node2 := health.ExecutionNode{NodeID: "yw-2", AccountName: "Yunwu 2", State: "healthy"}

	_ = nm.SaveNode(context.Background(), &node1)
	_ = nm.SaveNode(context.Background(), &node2)

	rm := NewYunwuRebindingManager(store)
	reboundNode, err := rm.HandleYunwuFailure(context.Background(), "session-yw-test", "yw-1", assert.AnError, []health.ExecutionNode{node1, node2}, 1, map[string]float64{"yw-2": 10.0}, "NORMAL")

	require.NoError(t, err)
	assert.NotNil(t, reboundNode)
	assert.Equal(t, "yw-2", reboundNode.NodeID)

	// Verify yw-1 is now in cooldown
	failedState, err := nm.GetNodeRuntimeState(context.Background(), "yw-1")
	require.NoError(t, err)
	assert.NotNil(t, failedState.CooldownUntil)
}

func TestYunwuSchedulerScoringAndSelection(t *testing.T) {
	sch := NewYunwuScheduler()
	ctx := context.Background()

	nodes := []health.ExecutionNode{
		{NodeID: "yw-1", AccountName: "Yunwu 1", State: "healthy"},
		{NodeID: "yw-2", AccountName: "Yunwu 2", State: "healthy"},
		{NodeID: "yw-3", AccountName: "Yunwu 3", State: "unavailable"},
	}

	states := map[string]*health.NodeRuntimeState{
		"yw-1": {NodeID: "yw-1", SuccessRate: 0.9},
		"yw-2": {NodeID: "yw-2", SuccessRate: 0.99},
	}

	credits := map[string]float64{
		"yw-1": 10.0,
		"yw-2": 100.0,
	}

	// yw-2 has higher credit and success rate -> should win
	winning, err := sch.SelectNode(ctx, nodes, states, credits, 1, "", "NORMAL")
	require.NoError(t, err)
	require.NotNil(t, winning)
	assert.Equal(t, "yw-2", winning.NodeID)

	// Test no nodes available
	_, err = sch.SelectNode(ctx, nil, nil, nil, 1, "", "NORMAL")
	require.ErrorIs(t, err, ErrNoYunwuNodesAvailable)
}

func TestYunwuSchedulerAllowsMissingBalanceInOptionalMode(t *testing.T) {
	sch := NewYunwuScheduler(false)
	winning, err := sch.SelectNode(context.Background(), []health.ExecutionNode{{NodeID: "yw-optional", State: "healthy"}}, nil, nil, 1, "", "NORMAL")
	require.NoError(t, err)
	require.NotNil(t, winning)
	assert.Equal(t, "yw-optional", winning.NodeID)
}

func TestClassifyMediaRequest(t *testing.T) {
	tests := []struct {
		name, method, path, model, body, kind string
		units                                 int64
	}{
		{"image batch", http.MethodPost, "/v1/images/generations", "imagen-3", `{"n":3}`, "images", 3},
		{"native gemini image", http.MethodPost, "/v1beta/models/gemini-2.5-flash-image:generateContent", "gemini-2.5-flash-image", `{}`, "images", 1},
		{"video seconds", http.MethodPost, "/v1/videos", "veo-3", `{"duration":8}`, "video", 8},
		{"speech", http.MethodPost, "/v1/audio/speech", "gemini-tts", `{}`, "audio", 1},
		{"operation poll", http.MethodGet, "/v1beta/operations/abc", "", `{}`, "media_control", 0},
		{"file upload", http.MethodPost, "/upload/v1beta/files", "", `{}`, "media_control", 0},
		{"count tokens", http.MethodPost, "/v1beta/models/gemini-2.5-flash:countTokens", "gemini-2.5-flash", `{}`, "media_control", 0},
		{"native text", http.MethodPost, "/v1beta/models/gemini-2.5-flash:generateContent", "gemini-2.5-flash", `{}`, "", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := runtime.WithUpstreamRequest(context.Background(), tc.method, tc.path, "application/json")
			got := classifyMediaRequest(ctx, tc.model, []byte(tc.body))
			assert.Equal(t, tc.kind, got.Kind)
			assert.Equal(t, tc.units, got.Units)
		})
	}
}

func TestNumericMicroUSDUsesConservativeCeil(t *testing.T) {
	got, ok := numericMicroUSD("1250.01")
	require.True(t, ok)
	assert.Equal(t, int64(1251), got)
}

func TestYunwuOperationAffinityNames(t *testing.T) {
	ctx := runtime.WithUpstreamRequest(context.Background(), http.MethodGet, "/v1beta/operations/veo-job-1?alt=json", "application/json")
	assert.Equal(t, "operations/veo-job-1", operationNameFromRequest(ctx))
	assert.Equal(t, "operations/veo-job-1", operationNameFromResponse([]byte(`{"name":"operations/veo-job-1","done":false}`)))
	assert.Empty(t, operationNameFromResponse([]byte(`{"name":"files/abc"}`)))
}

func TestModelNameFromNativeGeminiRequest(t *testing.T) {
	ctx := runtime.WithUpstreamRequest(context.Background(), http.MethodPost, "/v1beta/models/veo-3.0-generate-preview:predictLongRunning", "application/json")
	assert.Equal(t, "veo-3.0-generate-preview", modelNameFromRequest(ctx))
}

func TestYunwuInteractionAffinityNames(t *testing.T) {
	ctx := runtime.WithUpstreamRequest(context.Background(), http.MethodGet, "/v1beta/interactions/int-123?stream=true", "application/json")
	assert.Equal(t, "int-123", interactionIDFromRequest(ctx))
	createCtx := runtime.WithUpstreamRequest(context.Background(), http.MethodPost, "/v1beta/interactions", "application/json")
	assert.Equal(t, "int-123", interactionIDFromResponse(createCtx, []byte(`{"id":"int-123","status":"in_progress"}`)))
	assert.Empty(t, interactionIDFromResponse(context.Background(), []byte(`{"id":"int-123"}`)))
}

func TestYunwuHealthControllerAndStatsWorker(t *testing.T) {
	store := setupTestYunwuStore(t)
	ctx, cancel := context.WithCancel(context.Background())

	hc := NewYunwuHealthController(store)
	assert.Equal(t, "NORMAL", hc.GetSystemState(ctx))

	hc.Start(ctx, 10*time.Millisecond, func(ctx context.Context) []health.ExecutionNode {
		return []health.ExecutionNode{
			{NodeID: "yw-1", State: "healthy"},
		}
	})
	time.Sleep(25 * time.Millisecond)
	cancel()

	sw := NewStatsWorker(store)
	assert.NotNil(t, sw)
}

func TestStatsWorkerSyncNodeStats(t *testing.T) {
	store := setupTestYunwuStore(t)
	ctx := context.Background()

	// Mock server for primary credit_grants API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"object":"credit_grants","total_granted":100,"total_used":20,"total_available":80}`))
	}))
	defer server.Close()

	sw := NewStatsWorker(store)
	node := &health.ExecutionNode{NodeID: "yw-sync-1", AccountName: "Yunwu Sync Node"}

	grants, err := sw.SyncNodeStats(ctx, node, "sk-test-key", server.URL)
	require.NoError(t, err)
	require.NotNil(t, grants)
	assert.Equal(t, 80.0, grants.TotalAvailable)

	// Verify the authoritative balance snapshot was saved (legacy credit keys are not written).
	snapshot, err := sw.Snapshot(ctx, "yw-sync-1", time.Hour)
	require.NoError(t, err)
	assert.Equal(t, int64(80_000_000), snapshot.APIAvailableMicroUSD)
	assert.Equal(t, int64(8_000_000), snapshot.ReserveMicroUSD)
}

func TestStatsWorkerFallbackAndSyncLoop(t *testing.T) {
	store := setupTestYunwuStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Mock server returning 404 for credit_grants and 200 for /api/user/self
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/user/self" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"success":true,"data":{"quota":200,"used_quota":50,"balance":150}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	yh := NewYunwuHypervisor(store, &mockYunwuRuntime{})
	grants, err := yh.SyncNodeBalance(ctx, "yw-fallback-1", "sk-fallback-key", server.URL)
	require.NoError(t, err)
	require.NotNil(t, grants)
	assert.Equal(t, 150.0, grants.TotalAvailable)

	// Test StartSyncLoop
	yh.sw.StartSyncLoop(ctx, 10*time.Millisecond, func(ctx context.Context) ([]health.ExecutionNode, map[string]string, map[string]string) {
		return []health.ExecutionNode{{NodeID: "yw-fallback-1"}}, map[string]string{"yw-fallback-1": "key"}, map[string]string{"yw-fallback-1": server.URL}
	})
	time.Sleep(25 * time.Millisecond)

	// Test GetRebinding24hCount
	cnt, err := yh.rm.GetRebinding24hCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), cnt)
}

func TestYunwuRebindingExhausted(t *testing.T) {
	store := setupTestYunwuStore(t)
	ctx := context.Background()
	rm := NewYunwuRebindingManager(store)

	node1 := health.ExecutionNode{NodeID: "yw-e1", State: "healthy"}
	node2 := health.ExecutionNode{NodeID: "yw-e2", State: "unavailable"}

	_, err := rm.HandleYunwuFailure(ctx, "session-exhausted", "yw-e1", assert.AnError, []health.ExecutionNode{node1, node2}, 1, nil, "NORMAL")
	require.ErrorIs(t, err, ErrYunwuRebindingExhausted)
}

type failingYunwuRuntime struct{}

func (f *failingYunwuRuntime) Execute(ctx context.Context, nodeID string, reqPayload []byte, stream io.Writer) (*runtime.ResponseMetadata, error) {
	if nodeID == "yw-fail-1" {
		return nil, assert.AnError
	}
	return &runtime.ResponseMetadata{StatusCode: 200, InputTokens: 50, OutputTokens: 50}, nil
}

func TestYunwuReconcileHealthAndErrorRebinding(t *testing.T) {
	t.Setenv("ASH_YUNWU_TEST_MODE", "true")

	store := setupTestYunwuStore(t)
	ctx := context.Background()
	nm := health.NewNodeManager(store)

	hc := NewYunwuHealthController(store)

	nodes := []health.ExecutionNode{
		{NodeID: "yw-h1", State: "healthy"},
		{NodeID: "yw-h2", State: "cooldown"},
		{NodeID: "yw-h3", State: "limited"},
		{NodeID: "yw-h4", State: "unavailable"},
	}

	for _, n := range nodes {
		_ = nm.SaveNode(ctx, &n)
	}

	state := hc.ReconcileHealth(ctx, nodes)
	assert.NotEmpty(t, state)

	// Test HandleRequest error rebinding
	yh := NewYunwuHypervisor(store, &failingYunwuRuntime{})
	node1 := &health.ExecutionNode{NodeID: "yw-fail-1", AccountName: "Node 1", Provider: "yunwu", State: "healthy"}
	node2 := &health.ExecutionNode{NodeID: "yw-ok-2", AccountName: "Node 2", Provider: "yunwu", State: "healthy"}

	_ = nm.SaveNode(ctx, node1)
	_ = nm.SaveNode(ctx, node2)
	setTestBalance(t, store, node1.NodeID, 10_000_000)
	setTestBalance(t, store, node2.NodeID, 10_000_000)
	_ = store.Redis().SAdd(ctx, "ash:yunwu:nodes", "yw-fail-1", "yw-ok-2").Err()

	// Quota
	_ = yh.qm.SaveUserAccess(ctx, &quota.UserAccess{
		OwnerID:      "u-rebind",
		IsActive:     true,
		MonthlyLimit: 100000,
	}, "yunwu")

	var buf bytes.Buffer
	meta, err := yh.HandleRequest(ctx, "u-rebind", "conv-rebind", []byte(`{"model":"claude"}`), &buf, "yunwu")
	require.NoError(t, err)
	assert.NotNil(t, meta)
}

func TestYunwuBypassesHypervisorQuota(t *testing.T) {
	t.Setenv("ASH_YUNWU_TEST_MODE", "true")
	store := setupTestYunwuStore(t)
	ctx := context.Background()
	nm := health.NewNodeManager(store)
	node := &health.ExecutionNode{NodeID: "yw-quota-node", AccountName: "Quota Node", Provider: "yunwu", State: "healthy"}
	_ = nm.SaveNode(ctx, node)
	setTestBalance(t, store, node.NodeID, 10_000_000)
	_ = store.Redis().SAdd(ctx, "ash:yunwu:nodes", "yw-quota-node").Err()

	yh := NewYunwuHypervisor(store, &mockYunwuRuntime{})

	// User exceeding quota in QuotaManager should NOT be blocked in Yunwu
	_ = yh.qm.SaveUserAccess(ctx, &quota.UserAccess{OwnerID: "u-exceeded", IsActive: true, MonthlyLimitMicroUSD: 100, UsedMicroUSD: 150}, "yunwu")

	var buf bytes.Buffer
	meta, err := yh.HandleRequest(ctx, "u-exceeded", "conv-1", []byte(`{"model":"claude"}`), &buf, "yunwu")
	require.NoError(t, err)
	assert.NotNil(t, meta)

	// Inactive user in QuotaManager should also NOT be blocked in Yunwu (billing/auth handled at gateway layer)
	_ = yh.qm.SaveUserAccess(ctx, &quota.UserAccess{OwnerID: "u-inactive", IsActive: false}, "yunwu")
	meta, err = yh.HandleRequest(ctx, "u-inactive", "conv-2", []byte(`{"model":"claude"}`), &buf, "yunwu")
	require.NoError(t, err)
	assert.NotNil(t, meta)
}

func TestYunwuGetHealthStatusWithCooldown(t *testing.T) {
	t.Setenv("ASH_YUNWU_TEST_MODE", "true")

	store := setupTestYunwuStore(t)
	ctx := context.Background()
	yh := NewYunwuHypervisor(store, &mockYunwuRuntime{})
	nm := health.NewNodeManager(store)

	_ = nm.SaveNode(ctx, &health.ExecutionNode{NodeID: "yw-cd-1", State: "cooldown"})
	_ = store.Redis().SAdd(ctx, "ash:yunwu:nodes", "yw-cd-1").Err()

	status := yh.GetHealthStatus(ctx)
	assert.Equal(t, 1, status["cooldown_nodes"])
}

func TestYunwuContextExtractionAndCompilation(t *testing.T) {
	store := setupTestYunwuStore(t)
	yh := NewYunwuHypervisor(store, &mockYunwuRuntime{})

	sess := &session.VirtualSession{
		ID:             "session-yunwu-ctx-test",
		OwnerID:        "owner-ctx",
		ConversationID: "conv-ctx",
		State:          "active",
		Version:        1,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// Test 1: extract context from Anthropic-style payload with system + messages
	payload := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"system": "You are a helpful assistant.",
		"messages": [
			{"role": "user", "content": "Hello, how are you?"},
			{"role": "assistant", "content": "I am doing well, thanks!"},
			{"role": "user", "content": "What is 2+2?"}
		]
	}`)
	yh.extractAndStoreContext(sess, payload)

	require.NotNil(t, sess.Context)
	assert.Equal(t, "You are a helpful assistant.", sess.Context.SystemPrompt)
	assert.Equal(t, 3, len(sess.Context.RecentMessages))
	assert.Equal(t, "user", sess.Context.RecentMessages[0].Role)
	assert.Equal(t, "Hello, how are you?", sess.Context.RecentMessages[0].Content)

	// Test 2: compile and inject context into retry payload
	retryPayload := yh.compileAndInjectContext(sess, payload)
	require.NotNil(t, retryPayload)
	var raw rawRequestPayload
	require.NoError(t, json.Unmarshal(retryPayload, &raw))
	assert.True(t, len(raw.Messages) >= 4, "expected at least 4 messages (1 context system + 3 original)")
	assert.Equal(t, "system", raw.Messages[0].Role)
	assert.Contains(t, raw.Messages[0].Content.(string), "System Prompt:")
	assert.Contains(t, raw.Messages[0].Content.(string), "You are a helpful assistant.")

	// Test 3: content block array extraction
	sess2 := &session.VirtualSession{
		ID:             "session-yunwu-ctx-blocks",
		OwnerID:        "owner-blocks",
		ConversationID: "conv-blocks",
	}
	payloadBlocks := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": "Part one"}, {"type": "text", "text": "Part two"}]},
			{"role": "assistant", "content": "Response"}
		]
	}`)
	yh.extractAndStoreContext(sess2, payloadBlocks)
	require.NotNil(t, sess2.Context)
	assert.Equal(t, 2, len(sess2.Context.RecentMessages))
	assert.Equal(t, "Part one\nPart two", sess2.Context.RecentMessages[0].Content)
}

func TestYunwuContextCompilationEmptyContext(t *testing.T) {
	store := setupTestYunwuStore(t)
	yh := NewYunwuHypervisor(store, &mockYunwuRuntime{})

	sess := &session.VirtualSession{
		ID:             "session-yunwu-empty-ctx",
		OwnerID:        "owner-empty",
		ConversationID: "conv-empty",
	}

	// Nil context should return original payload unchanged
	payload := []byte(`{"model":"claude","messages":[{"role":"user","content":"hi"}]}`)
	result := yh.compileAndInjectContext(sess, payload)
	assert.Equal(t, string(payload), string(result))

	// Empty context should also return original (compiler produces empty string)
	sess.Context = &session.ExecutionContext{}
	result = yh.compileAndInjectContext(sess, payload)
	assert.Equal(t, string(payload), string(result))
}

func TestPrepareNodePayload(t *testing.T) {
	store := setupTestYunwuStore(t)
	yh := NewYunwuHypervisor(store, &mockYunwuRuntime{})

	node := &health.ExecutionNode{
		NodeID:   "node-test-1",
		Provider: "openlux",
	}

	// Canonical anthropic model -> stripped
	payload := []byte(`{"model":"anthropic/claude-3-7-sonnet","messages":[{"role":"user","content":"hi"}]}`)
	prepared := yh.prepareNodePayload(context.Background(), node, payload)
	var bodyMap map[string]any
	require.NoError(t, json.Unmarshal(prepared, &bodyMap))
	assert.Equal(t, "claude-3-7-sonnet", bodyMap["model"])

	// Canonical deepseek model -> deepseek-v3
	payloadDS := []byte(`{"model":"deepseek/deepseek-chat","messages":[{"role":"user","content":"hi"}]}`)
	preparedDS := yh.prepareNodePayload(context.Background(), node, payloadDS)
	var bodyMapDS map[string]any
	require.NoError(t, json.Unmarshal(preparedDS, &bodyMapDS))
	assert.Equal(t, "deepseek-v3", bodyMapDS["model"])

	// Nil node -> unchanged
	unchanged := yh.prepareNodePayload(context.Background(), nil, payload)
	assert.Equal(t, string(payload), string(unchanged))
}


func TestCalculateBilledMicroUSDRoundsUp(t *testing.T) {
	assert.Equal(t, int64(3), CalculateBilledMicroUSD(1, 1, 0.0011, 0.0011))
	assert.Equal(t, int64(10000), CalculateBilledMicroUSD(1000, 2000, 0.002, 0.004))
}

func TestExtractContentString(t *testing.T) {
	assert.Equal(t, "hello", extractContentString("hello"))

	blocks := []any{
		map[string]any{"type": "text", "text": "first"},
		map[string]any{"type": "text", "text": "second"},
	}
	assert.Equal(t, "first\nsecond", extractContentString(blocks))

	blocksInput := []any{
		map[string]any{"type": "input_text", "text": "user query"},
	}
	assert.Equal(t, "user query", extractContentString(blocksInput))

	assert.Equal(t, "", extractContentString([]any{map[string]any{"type": "image", "url": "http://..."}}))
	assert.Equal(t, "", extractContentString(nil))
}

func TestToolCallsToString(t *testing.T) {
	assert.Equal(t, "", toolCallsToString(nil))
	assert.Equal(t, "", toolCallsToString([]session.ToolCall{}))

	tc := []session.ToolCall{
		{ID: "call_1", Type: "function", Function: "get_weather"},
	}
	assert.Equal(t, "[function] call_1: get_weather", toolCallsToString(tc))

	tc2 := []session.ToolCall{
		{ID: "call_1", Type: "function", Function: "get_weather"},
		{ID: "call_2", Type: "function", Function: "search_web"},
	}
	result := toolCallsToString(tc2)
	assert.Contains(t, result, "call_1")
	assert.Contains(t, result, "call_2")
}

func TestYunwuModelFamilyRouting(t *testing.T) {
	t.Setenv("ASH_YUNWU_TEST_MODE", "true")

	store := setupTestYunwuStore(t)
	rt := &mockYunwuRuntime{}
	yh := NewYunwuHypervisor(store, rt)
	nm := health.NewNodeManager(store)
	ctx := context.Background()

	// Node 1: Claude only
	err := nm.SaveNode(ctx, &health.ExecutionNode{
		NodeID:       "node-claude",
		AccountName:  "Claude Node",
		Provider:     "openlux",
		State:        "healthy",
		Capabilities: []string{FamilyClaude},
	})
	require.NoError(t, err)
	err = store.Redis().SAdd(ctx, "ash:yunwu:nodes", "node-claude").Err()
	require.NoError(t, err)
	setTestBalance(t, store, "node-claude", 10_000_000)

	// Node 2: GPT only
	err = nm.SaveNode(ctx, &health.ExecutionNode{
		NodeID:       "node-gpt",
		AccountName:  "GPT Node",
		Provider:     "openlux",
		State:        "healthy",
		Capabilities: []string{FamilyGPT},
	})
	require.NoError(t, err)
	err = store.Redis().SAdd(ctx, "ash:yunwu:nodes", "node-gpt").Err()
	require.NoError(t, err)
	setTestBalance(t, store, "node-gpt", 10_000_000)

	// Register user access
	err = yh.qm.SaveUserAccess(ctx, &quota.UserAccess{
		OwnerID:      "test-user",
		PlanID:       "basic",
		MonthlyLimit: 1_000_000,
		UsedUnits:    0,
		Priority:     1,
		IsActive:     true,
	}, "yunwu")
	require.NoError(t, err)

	// 1. Request a Claude model -> must route to node-claude
	claudePayload := []byte(`{"model": "claude-3-5-sonnet-20241022", "messages": [{"role": "user", "content": "hi"}]}`)
	resp, err := yh.HandleRequest(ctx, "test-user", "conv-claude", claudePayload, io.Discard, "yunwu")
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "node-claude", rt.lastNodeID)

	// 2. Request a GPT model -> must route to node-gpt
	gptPayload := []byte(`{"model": "gpt-4o", "messages": [{"role": "user", "content": "hi"}]}`)
	resp, err = yh.HandleRequest(ctx, "test-user", "conv-gpt", gptPayload, io.Discard, "yunwu")
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "node-gpt", rt.lastNodeID)

	// 3. Request a Gemini model -> no compatible node, returns ErrNoCompatibleNodes
	geminiPayload := []byte(`{"model": "gemini-2.5-flash", "messages": [{"role": "user", "content": "hi"}]}`)
	_, err = yh.HandleRequest(ctx, "test-user", "conv-gemini", geminiPayload, io.Discard, "yunwu")
	require.ErrorIs(t, err, ErrNoCompatibleNodes)
}

func TestYunwuPriceFilteringAndFailover(t *testing.T) {
	t.Setenv("ASH_YUNWU_TEST_MODE", "true")

	store := setupTestYunwuStore(t)
	rt := &mockYunwuRuntime{}
	yh := NewYunwuHypervisor(store, rt)
	nm := health.NewNodeManager(store)
	ctx := context.Background()

	// Node 1: Supports Claude, but has NO price for claude-3-7-sonnet
	err := nm.SaveNode(ctx, &health.ExecutionNode{
		NodeID:       "node-no-price",
		AccountName:  "Claude Node Without Pricing",
		Provider:     "openlux",
		State:        "healthy",
		Capabilities: []string{FamilyClaude},
	})
	require.NoError(t, err)
	err = store.Redis().SAdd(ctx, "ash:yunwu:nodes", "node-no-price").Err()
	require.NoError(t, err)
	setTestBalance(t, store, "node-no-price", 10_000_000)

	// Node 2: Supports Claude, AND has valid pricing for claude-3-7-sonnet
	err = nm.SaveNode(ctx, &health.ExecutionNode{
		NodeID:       "node-has-price",
		AccountName:  "Claude Node With Pricing",
		Provider:     "portdan",
		State:        "healthy",
		Capabilities: []string{FamilyClaude},
	})
	require.NoError(t, err)
	err = store.Redis().SAdd(ctx, "ash:yunwu:nodes", "node-has-price").Err()
	require.NoError(t, err)
	setTestBalance(t, store, "node-has-price", 10_000_000)

	// Configure price manager:
	// "node-no-price" only has pricing for "other-model"
	// "node-has-price" has pricing for "claude-3-7-sonnet"
	ppm := yh.PriceManager()
	require.NotNil(t, ppm)
	ppm.SetProviderPrices("node-no-price", map[string]ProviderModelPrice{
		"other-model": {InputPricePer1k: 0.001, OutputPricePer1k: 0.002, Available: true},
	})
	ppm.SetProviderPrices("node-has-price", map[string]ProviderModelPrice{
		"claude-3-7-sonnet": {InputPricePer1k: 0.015, OutputPricePer1k: 0.075, Available: true},
	})

	// Register user access
	err = yh.qm.SaveUserAccess(ctx, &quota.UserAccess{
		OwnerID:      "user-price-test",
		PlanID:       "basic",
		MonthlyLimit: 1_000_000,
		UsedUnits:    0,
		Priority:     1,
		IsActive:     true,
	}, "yunwu")
	require.NoError(t, err)

	// 1. Request claude-3-7-sonnet:
	// Node 1 (node-no-price) has NO pricing for claude-3-7-sonnet -> must be skipped.
	// Node 2 (node-has-price) HAS pricing -> must be chosen.
	claudePayload := []byte(`{"model": "claude-3-7-sonnet", "messages": [{"role": "user", "content": "hello"}]}`)
	resp, err := yh.HandleRequest(ctx, "user-price-test", "conv-pricing", claudePayload, io.Discard, "yunwu")
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "node-has-price", rt.lastNodeID)

	// 2. Request an unpriced model:
	// claude-unlisted has NO pricing on ANY compatible node in the pool.
	// Request must fail immediately with ErrNoPricingAvailable without executing upstream or deducting units.
	initialAccess, err := yh.qm.GetUserAccess(ctx, "user-price-test", "yunwu")
	require.NoError(t, err)
	initialUsed := initialAccess.UsedUnits

	unpricedPayload := []byte(`{"model": "claude-unlisted", "messages": [{"role": "user", "content": "hello"}]}`)
	_, err = yh.HandleRequest(ctx, "user-price-test", "conv-unpriced", unpricedPayload, io.Discard, "yunwu")
	require.ErrorIs(t, err, ErrNoPricingAvailable)

	// Verify no money / units were deducted
	afterAccess, err := yh.qm.GetUserAccess(ctx, "user-price-test", "yunwu")
	require.NoError(t, err)
	assert.Equal(t, initialUsed, afterAccess.UsedUnits)
}

func TestYunwuRequireFreshBalanceAndExhaustion(t *testing.T) {
	t.Setenv("ASH_YUNWU_TEST_MODE", "true")
	t.Setenv("ASH_YUNWU_REQUIRE_FRESH_BALANCE", "true")

	store := setupTestYunwuStore(t)
	rt := &mockYunwuRuntime{}
	yh := NewYunwuHypervisor(store, rt)
	nm := health.NewNodeManager(store)
	ctx := context.Background()

	err := nm.SaveNode(ctx, &health.ExecutionNode{
		NodeID:      "fresh-node-1",
		AccountName: "Fresh Node",
		Provider:    "yunwu",
		State:       "healthy",
	})
	require.NoError(t, err)
	err = store.Redis().SAdd(ctx, "ash:yunwu:nodes", "fresh-node-1").Err()
	require.NoError(t, err)

	// Set test balance
	setTestBalance(t, store, "fresh-node-1", 10_000_000)

	err = yh.qm.SaveUserAccess(ctx, &quota.UserAccess{
		OwnerID:      "user-fresh",
		PlanID:       "basic",
		MonthlyLimit: 1_000_000,
		UsedUnits:    0,
		Priority:     1,
		IsActive:     true,
	}, "yunwu")
	require.NoError(t, err)

	// Request with balance available
	payload := []byte(`{"model": "claude", "messages": [{"role": "user", "content": "hi"}]}`)
	resp, err := yh.HandleRequest(ctx, "user-fresh", "conv-fresh-1", payload, io.Discard, "yunwu")
	require.NoError(t, err)
	assert.NotNil(t, resp)

	// Deplete balance to 0 so Reserve fails
	setTestBalance(t, store, "fresh-node-1", 0)
	_, err = yh.HandleRequest(ctx, "user-fresh", "conv-fresh-2", payload, io.Discard, "yunwu")
	require.ErrorIs(t, err, ErrNodeBalanceExhausted)
}

func TestYunwuAffinityAndMediaHandling(t *testing.T) {
	t.Setenv("ASH_YUNWU_TEST_MODE", "true")

	store := setupTestYunwuStore(t)
	rt := &mockYunwuRuntime{}
	yh := NewYunwuHypervisor(store, rt)
	nm := health.NewNodeManager(store)
	ctx := context.Background()

	// 1. Add node
	err := nm.SaveNode(ctx, &health.ExecutionNode{
		NodeID:      "aff-node-1",
		AccountName: "Affinity Node",
		Provider:    "yunwu",
		State:       "healthy",
	})
	require.NoError(t, err)
	err = store.Redis().SAdd(ctx, "ash:yunwu:nodes", "aff-node-1").Err()
	require.NoError(t, err)
	setTestBalance(t, store, "aff-node-1", 10_000_000)

	err = yh.qm.SaveUserAccess(ctx, &quota.UserAccess{
		OwnerID:      "user-aff",
		PlanID:       "basic",
		MonthlyLimit: 1_000_000,
		UsedUnits:    0,
		Priority:     1,
		IsActive:     true,
	}, "yunwu")
	require.NoError(t, err)

	// Set operation affinity in Redis
	err = store.Redis().Set(ctx, yunwuOperationNodeKey("operations/op-aff-123"), "aff-node-1", 0).Err()
	require.NoError(t, err)

	opCtx := runtime.WithUpstreamRequest(ctx, http.MethodGet, "/v1beta/operations/op-aff-123", "application/json")
	payload := []byte(`{"model": "gemini-2.5-flash-image"}`)
	resp, err := yh.HandleRequest(opCtx, "user-aff", "conv-aff-1", payload, io.Discard, "yunwu")
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "aff-node-1", rt.lastNodeID)

	// Test interaction affinity
	err = store.Redis().Set(ctx, yunwuInteractionNodeKey("int-aff-456"), "aff-node-1", 0).Err()
	require.NoError(t, err)
	intCtx := runtime.WithUpstreamRequest(ctx, http.MethodGet, "/v1beta/interactions/int-aff-456", "application/json")
	resp, err = yh.HandleRequest(intCtx, "user-aff", "conv-aff-2", payload, io.Discard, "yunwu")
	require.NoError(t, err)
	assert.NotNil(t, resp)

	// Test stateful resource affinity
	err = store.Redis().Set(ctx, yunwuResourceNodeKey("batches/batch-aff-789"), "aff-node-1", 0).Err()
	require.NoError(t, err)
	resCtx := runtime.WithUpstreamRequest(ctx, http.MethodGet, "/v1beta/batches/batch-aff-789", "application/json")
	resp, err = yh.HandleRequest(resCtx, "user-aff", "conv-aff-3", payload, io.Discard, "yunwu")
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

type mockFailoverLossRuntime struct {
	attempts int
}

func (m *mockFailoverLossRuntime) Execute(ctx context.Context, nodeID string, reqPayload []byte, stream io.Writer) (*runtime.ResponseMetadata, error) {
	m.attempts++
	if m.attempts == 1 {
		// First attempt fails, but upstream returned partial token consumption (e.g. stream broken)
		return &runtime.ResponseMetadata{
			StatusCode:   500,
			InputTokens:  120,
			OutputTokens: 30,
		}, errors.New("upstream connection dropped")
	}
	// Second attempt succeeds cleanly
	return &runtime.ResponseMetadata{
		StatusCode:   200,
		InputTokens:  120,
		OutputTokens: 250,
	}, nil
}

func TestUpstreamFailoverLossAccounting(t *testing.T) {
	t.Setenv("ASH_YUNWU_TEST_MODE", "true")
	ctx := context.Background()
	store := setupTestYunwuStore(t)
	rt := &mockFailoverLossRuntime{}
	yh := NewYunwuHypervisor(store, rt)

	nm := health.NewNodeManager(store)
	// Add primary node
	require.NoError(t, nm.SaveNode(ctx, &health.ExecutionNode{
		NodeID:       "node-loss-1",
		AccountName:  "Primary Loss Node",
		Provider:     "yunwu",
		Capabilities: []string{"claude"},
		State:        "healthy",
	}))
	require.NoError(t, store.Redis().SAdd(ctx, "ash:yunwu:nodes", "node-loss-1").Err())
	setTestBalance(t, store, "node-loss-1", 50_000_000)

	// Add backup node
	require.NoError(t, nm.SaveNode(ctx, &health.ExecutionNode{
		NodeID:       "node-loss-2",
		AccountName:  "Backup Loss Node",
		Provider:     "yunwu",
		Capabilities: []string{"claude"},
		State:        "healthy",
	}))
	require.NoError(t, store.Redis().SAdd(ctx, "ash:yunwu:nodes", "node-loss-2").Err())
	setTestBalance(t, store, "node-loss-2", 50_000_000)

	// Set pricing
	ppm := yh.PriceManager()
	require.NotNil(t, ppm)
	ppm.SetProviderPrices("node-loss-1", map[string]ProviderModelPrice{
		"claude-3-7-sonnet": {InputPricePer1k: 0.015, OutputPricePer1k: 0.075, Available: true},
	})
	ppm.SetProviderPrices("node-loss-2", map[string]ProviderModelPrice{
		"claude-3-7-sonnet": {InputPricePer1k: 0.015, OutputPricePer1k: 0.075, Available: true},
	})

	// User quota
	require.NoError(t, yh.qm.SaveUserAccess(ctx, &quota.UserAccess{
		OwnerID:      "user-loss-test",
		PlanID:       "pro",
		MonthlyLimit: 1_000_000,
		Priority:     1,
		IsActive:     true,
	}, "yunwu"))

	payload := []byte(`{"model": "claude-3-7-sonnet", "messages": [{"role": "user", "content": "hi"}]}`)
	resp, err := yh.HandleRequest(ctx, "user-loss-test", "conv-failover-loss", payload, io.Discard, "yunwu")
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, 2, rt.attempts)
	assert.Equal(t, int64(120), resp.InputTokens)
	assert.Equal(t, int64(250), resp.OutputTokens)

	// Verify failover loss was handled without failing request
	// And direct call to recordUpstreamFailoverLoss works safely even without postgres DB
	yh.recordUpstreamFailoverLoss(ctx, "user-loss-test", "sess-test", health.ExecutionNode{
		NodeID:   "node-loss-1",
		Provider: "yunwu",
	}, "claude-3-7-sonnet", &runtime.ResponseMetadata{
		InputTokens:  100,
		OutputTokens: 50,
	}, 1, false)
}

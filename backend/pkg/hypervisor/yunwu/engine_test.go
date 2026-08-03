package yunwu

import (
	"bytes"
	"context"
	"encoding/json"
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

type mockYunwuRuntime struct{}

func (m *mockYunwuRuntime) Execute(ctx context.Context, nodeID string, reqPayload []byte, stream io.Writer) (*runtime.ResponseMetadata, error) {
	return &runtime.ResponseMetadata{
		StatusCode:   200,
		InputTokens:  150,
		OutputTokens: 250,
	}, nil
}

func setupTestYunwuStore(t *testing.T) *storage.Storage {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	return storage.NewStorage(nil, rdb)
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
	err = store.Redis().Set(context.Background(), "ash:yunwu:credit:yunwu-101", 50.0, 24*time.Hour).Err()
	require.NoError(t, err)

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

func TestYunwuRebindingOnFailure(t *testing.T) {
	store := setupTestYunwuStore(t)
	nm := health.NewNodeManager(store)

	node1 := health.ExecutionNode{NodeID: "yw-1", AccountName: "Yunwu 1", State: "healthy"}
	node2 := health.ExecutionNode{NodeID: "yw-2", AccountName: "Yunwu 2", State: "healthy"}

	_ = nm.SaveNode(context.Background(), &node1)
	_ = nm.SaveNode(context.Background(), &node2)

	rm := NewYunwuRebindingManager(store)
	reboundNode, err := rm.HandleYunwuFailure(context.Background(), "yw-1", assert.AnError, []health.ExecutionNode{node1, node2})

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

	// Verify saved to Redis
	val, err := store.Redis().Get(ctx, "ash:yunwu:credit:yw-sync-1").Result()
	require.NoError(t, err)
	assert.Equal(t, "80", val)
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

	_, err := rm.HandleYunwuFailure(ctx, "yw-e1", assert.AnError, []health.ExecutionNode{node1, node2})
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

func TestYunwuQuotaExceeded(t *testing.T) {
	store := setupTestYunwuStore(t)
	ctx := context.Background()
	yh := NewYunwuHypervisor(store, &mockYunwuRuntime{})

	// User exceeding quota
	_ = yh.qm.SaveUserAccess(ctx, &quota.UserAccess{OwnerID: "u-exceeded", IsActive: true, MonthlyLimit: 100, UsedUnits: 150}, "yunwu")

	var buf bytes.Buffer
	_, err := yh.HandleRequest(ctx, "u-exceeded", "conv-1", []byte(`{"model":"claude"}`), &buf, "yunwu")
	require.ErrorIs(t, err, quota.ErrQuotaExceeded)

	// Inactive user
	_ = yh.qm.SaveUserAccess(ctx, &quota.UserAccess{OwnerID: "u-inactive", IsActive: false}, "yunwu")
	_, err = yh.HandleRequest(ctx, "u-inactive", "conv-1", []byte(`{"model":"claude"}`), &buf, "yunwu")
	require.ErrorIs(t, err, quota.ErrUserInactive)
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









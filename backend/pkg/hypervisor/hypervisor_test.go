package hypervisor_test

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/events"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/lease"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/quota"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/runtime"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/scheduler"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/session"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func newTestStorage(t *testing.T) *storage.Storage {
	t.Helper()

	// Setup SQLite in-memory DB
	name := strings.ReplaceAll(t.Name(), "/", "_")
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", name)
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	// Setup miniredis
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	t.Cleanup(func() { _ = rdb.Close() })

	return storage.NewStorage(client, rdb)
}

func TestHypervisorDay2(t *testing.T) {
	store := newTestStorage(t)
	ctx := context.Background()

	// 1. Test SessionManager
	sm := session.NewSessionManager(store)
	sessID := "session-123"
	sess := &session.VirtualSession{
		ID:             sessID,
		OwnerID:        "owner-456",
		ConversationID: "conv-789",
		State:          "new",
		Version:        1,
		CreatedAt:      time.Now().Round(time.Second),
		UpdatedAt:      time.Now().Round(time.Second),
		Context: &session.ExecutionContext{
			SystemPrompt: "test system prompt",
			Summary:      "test summary",
			PinnedFacts:  []string{"fact1", "fact2"},
			RecentMessages: []session.Message{
				{Role: "user", Content: "hello"},
				{Role: "assistant", Content: "hi there"},
			},
			TerminalState: "done",
			ToolState: []session.ToolCall{
				{ID: "call-1", Type: "function", Function: "get_weather"},
			},
		},
	}

	// Verify Save and Get
	err := sm.SaveSession(ctx, sess)
	require.NoError(t, err)

	retrievedSess, err := sm.GetSession(ctx, sessID)
	require.NoError(t, err)
	require.Equal(t, sess.ID, retrievedSess.ID)
	require.Equal(t, sess.OwnerID, retrievedSess.OwnerID)
	require.Equal(t, sess.ConversationID, retrievedSess.ConversationID)
	require.Equal(t, sess.State, retrievedSess.State)
	require.Equal(t, sess.Version, retrievedSess.Version)
	require.NotNil(t, retrievedSess.Context)
	require.Equal(t, sess.Context.SystemPrompt, retrievedSess.Context.SystemPrompt)
	require.Equal(t, sess.Context.Summary, retrievedSess.Context.Summary)
	require.Equal(t, sess.Context.PinnedFacts, retrievedSess.Context.PinnedFacts)
	require.Equal(t, len(sess.Context.RecentMessages), len(retrievedSess.Context.RecentMessages))
	require.Equal(t, sess.Context.RecentMessages[0].Content, retrievedSess.Context.RecentMessages[0].Content)
	require.Equal(t, sess.Context.ToolState[0].Function, retrievedSess.Context.ToolState[0].Function)

	// Verify Delete
	err = sm.DeleteSession(ctx, sessID)
	require.NoError(t, err)
	_, err = sm.GetSession(ctx, sessID)
	require.Error(t, err) // Should not exist in Redis

	// 2. Test LeaseManager
	lm := lease.NewLeaseManager(store)
	leaseSessID := "lease-session-999"
	expiresAt := time.Now().Add(5 * time.Second).Round(time.Second)
	l := &lease.Lease{
		LeaseID:   "lease-uuid-abc",
		SessionID: leaseSessID,
		NodeID:    "node-xyz",
		GrantedAt: time.Now().Round(time.Second),
		ExpiresAt: &expiresAt,
	}

	err = lm.SaveLease(ctx, l)
	require.NoError(t, err)

	retrievedLease, err := lm.GetLease(ctx, leaseSessID)
	require.NoError(t, err)
	require.Equal(t, l.LeaseID, retrievedLease.LeaseID)
	require.Equal(t, l.SessionID, retrievedLease.SessionID)
	require.Equal(t, l.NodeID, retrievedLease.NodeID)
	require.WithinDuration(t, l.GrantedAt, retrievedLease.GrantedAt, time.Second)
	require.WithinDuration(t, *l.ExpiresAt, *retrievedLease.ExpiresAt, time.Second)

	err = lm.DeleteLease(ctx, leaseSessID)
	require.NoError(t, err)
	_, err = lm.GetLease(ctx, leaseSessID)
	require.Error(t, err)

	// 3. Test NodeManager
	nm := health.NewNodeManager(store)
	nodeID := "node-claude-max-1"
	node := &health.ExecutionNode{
		NodeID:       nodeID,
		AccountName:  "Claude Max 1",
		Provider:     "anthropic",
		State:        "healthy",
		Capabilities: []string{"claude-3-opus", "claude-3-5-sonnet"},
		Priority:     10,
		UpdatedAt:    time.Now().Round(time.Second),
	}

	err = nm.SaveNode(ctx, node)
	require.NoError(t, err)

	retrievedNode, err := nm.GetNode(ctx, nodeID)
	require.NoError(t, err)
	require.Equal(t, node.NodeID, retrievedNode.NodeID)
	require.Equal(t, node.AccountName, retrievedNode.AccountName)
	require.Equal(t, node.Provider, retrievedNode.Provider)
	require.Equal(t, node.State, retrievedNode.State)
	require.Equal(t, node.Capabilities, retrievedNode.Capabilities)
	require.Equal(t, node.Priority, retrievedNode.Priority)

	err = nm.DeleteNode(ctx, nodeID)
	require.NoError(t, err)
	_, err = nm.GetNode(ctx, nodeID)
	require.Error(t, err)

	// Test NodeRuntimeState
	cooldownUntil := time.Now().Add(10 * time.Minute).Round(time.Second)
	lastErrorAt := time.Now().Round(time.Second)
	runtimeState := &health.NodeRuntimeState{
		NodeID:         nodeID,
		SuccessRate:    0.95,
		AvgLatencyMs:   450,
		QueueDepth:     2,
		Recent429:      1,
		RecentTimeouts: 0,
		LastErrorAt:    &lastErrorAt,
		CooldownUntil:  &cooldownUntil,
	}

	err = nm.SaveNodeRuntimeState(ctx, runtimeState)
	require.NoError(t, err)

	retrievedState, err := nm.GetNodeRuntimeState(ctx, nodeID)
	require.NoError(t, err)
	require.Equal(t, runtimeState.NodeID, retrievedState.NodeID)
	require.Equal(t, runtimeState.SuccessRate, retrievedState.SuccessRate)
	require.Equal(t, runtimeState.AvgLatencyMs, retrievedState.AvgLatencyMs)
	require.Equal(t, runtimeState.QueueDepth, retrievedState.QueueDepth)
	require.Equal(t, runtimeState.Recent429, retrievedState.Recent429)
	require.Equal(t, runtimeState.RecentTimeouts, retrievedState.RecentTimeouts)
	require.WithinDuration(t, *runtimeState.LastErrorAt, *retrievedState.LastErrorAt, time.Second)
	require.WithinDuration(t, *runtimeState.CooldownUntil, *retrievedState.CooldownUntil, time.Second)

	err = nm.DeleteNodeRuntimeState(ctx, nodeID)
	require.NoError(t, err)
	_, err = nm.GetNodeRuntimeState(ctx, nodeID)
	require.Error(t, err)

	// 4. Test EventWriter
	ew := events.NewEventWriter(store)
	eventSessID := "session-xyz"
	eventNodeID := "node-abc"
	payload := map[string]any{"reason": "health check failure"}

	event, err := ew.LogEvent(ctx, "owner-777", &eventSessID, &eventNodeID, "NodeHealthDegraded", payload)
	require.NoError(t, err)
	require.NotNil(t, event)
	require.Equal(t, "owner-777", event.OwnerID)
	require.Equal(t, eventSessID, *event.SessionID)
	require.Equal(t, eventNodeID, *event.NodeID)
	require.Equal(t, "NodeHealthDegraded", event.EventType)
	require.Equal(t, "health check failure", event.Payload["reason"])
}

func TestHypervisorDay3(t *testing.T) {
	store := newTestStorage(t)
	ctx := context.Background()

	// 1. Test QuotaManager
	qm := quota.NewQuotaManager(store)
	ownerID := "user-owner-123"

	// Register some active users and nodes to test dynamic limits
	err := qm.RegisterActiveUser(ctx, "user-owner-123")
	require.NoError(t, err)
	err = qm.RegisterActiveUser(ctx, "user-owner-456")
	require.NoError(t, err)
	err = qm.RegisterActiveUser(ctx, "user-owner-789")
	require.NoError(t, err)
	err = qm.RegisterActiveUser(ctx, "user-owner-abc")
	require.NoError(t, err) // 4 active users

	err = qm.RegisterNode(ctx, "node-1")
	require.NoError(t, err) // 1 node registered

	// 1 node capacity (1,000,000) / 4 active users = 250,000 limit
	dynLimit, err := qm.GetDynamicLimit(ctx, ownerID)
	require.NoError(t, err)
	require.Equal(t, int64(quota.NodeCapacity/4), dynLimit)

	// Save UserAccess state
	ua := &quota.UserAccess{
		OwnerID:      ownerID,
		PlanID:       "plan-pro",
		MonthlyLimit: 0, // 0 triggers dynamic limit calculation
		DailyBurst:   0,
		UsedUnits:    0,
		Priority:     1,
		IsActive:     true,
	}

	err = qm.SaveUserAccess(ctx, ua)
	require.NoError(t, err)

	// Check quota (should pass)
	allowed, err := qm.CheckQuota(ctx, ownerID)
	require.NoError(t, err)
	require.True(t, allowed)

	// Consume quota up to limit
	err = qm.ConsumeQuota(ctx, ownerID, 250000)
	require.NoError(t, err)

	// Check quota again (should be blocked due to limit exceed)
	allowed, err = qm.CheckQuota(ctx, ownerID)
	require.ErrorIs(t, err, quota.ErrQuotaExceeded)
	require.False(t, allowed)

	// Let's add a second node to the pool: 2 nodes / 4 users = 500,000 limit per user
	err = qm.RegisterNode(ctx, "node-2")
	require.NoError(t, err)

	// Now check quota again (should pass since limit increased to 500,000 and used is 250,000)
	allowed, err = qm.CheckQuota(ctx, ownerID)
	require.NoError(t, err)
	require.True(t, allowed)

	// 2. Test Scheduler
	sch := scheduler.NewScheduler()

	nodes := []health.ExecutionNode{
		{
			NodeID:   "node-unavail",
			State:    "unavailable",
			Priority: 1,
		},
		{
			NodeID:   "node-cooldown",
			State:    "cooldown",
			Priority: 1,
		},
		{
			NodeID:   "node-limited",
			State:    "limited",
			Priority: 1,
		},
		{
			NodeID:   "node-healthy-1",
			State:    "healthy",
			Priority: 1,
		},
		{
			NodeID:   "node-healthy-2",
			State:    "healthy",
			Priority: 1,
		},
	}

	states := map[string]health.NodeRuntimeState{
		"node-healthy-1": {
			NodeID:         "node-healthy-1",
			SuccessRate:    0.9,
			AvgLatencyMs:   500,
			QueueDepth:     1,
			Recent429:      0,
			RecentTimeouts: 0,
		},
		"node-healthy-2": {
			NodeID:         "node-healthy-2",
			SuccessRate:    0.95, // higher success rate
			AvgLatencyMs:   200,  // lower latency
			QueueDepth:     0,    // no queue
			Recent429:      0,
			RecentTimeouts: 0,
		},
	}

	// Select best node (should pick node-healthy-2 because it has better success rate and no latency/queue penalty)
	selectedNode, err := sch.SelectNode(ctx, nodes, states, 1, "", "NORMAL")
	require.NoError(t, err)
	require.Equal(t, "node-healthy-2", selectedNode.NodeID)

	// Now test affinity bonus (if we have currentLeaseNodeID pointing to node-healthy-1)
	// node-healthy-1 will get a bonus of 15.0 which will make its score higher than node-healthy-2
	selectedNode, err = sch.SelectNode(ctx, nodes, states, 1, "node-healthy-1", "NORMAL")
	require.NoError(t, err)
	require.Equal(t, "node-healthy-1", selectedNode.NodeID)

	// Test limited state node selected only when healthy/busy nodes are absent
	onlyLimitedNodes := []health.ExecutionNode{
		{
			NodeID:   "node-limited",
			State:    "limited",
			Priority: 1,
		},
	}
	selectedNode, err = sch.SelectNode(ctx, onlyLimitedNodes, nil, 1, "", "NORMAL")
	require.NoError(t, err)
	require.Equal(t, "node-limited", selectedNode.NodeID)
}

func TestHealthControllerAndDegradation(t *testing.T) {
	ctx := context.Background()
	store := newTestStorage(t)
	hc := health.NewHealthController(store)

	// Register some mock nodes
	_ = store.Redis().SAdd(ctx, "ash:nodes", "node-1", "node-2").Err()

	// Initial reconcile should compute NORMAL state
	err := hc.Reconcile(ctx)
	require.NoError(t, err)
	require.Equal(t, "NORMAL", hc.GetSystemState(ctx, "anthropic"))

	// Now check warning penalties in scheduler
	sch := scheduler.NewScheduler()
	nodes := []health.ExecutionNode{
		{NodeID: "node-1", State: "healthy"},
	}
	states := map[string]health.NodeRuntimeState{
		"node-1": {
			NodeID:       "node-1",
			SuccessRate:  1.0,
			AvgLatencyMs: 100,
			Recent429:    1,
		},
	}

	// Under NORMAL state
	node, err := sch.SelectNode(ctx, nodes, states, 1, "", "NORMAL")
	require.NoError(t, err)
	require.NotNil(t, node)

	// Under WARNING state, penalties are double, so check it runs without panic
	nodeWarn, err := sch.SelectNode(ctx, nodes, states, 1, "", "WARNING")
	require.NoError(t, err)
	require.NotNil(t, nodeWarn)
}

func TestOpenAIMockAndStatsEndpoints(t *testing.T) {
	ctx := context.Background()
	store := newTestStorage(t)

	// Create an account with platform = "openai"
	acc, err := store.DB().Account.Create().
		SetName("openai-mock-node").
		SetPlatform("openai").
		SetType("api_key").
		SetStatus("active").
		SetCredentials(map[string]any{"api_key": "sk-mock-key"}).
		Save(ctx)
	require.NoError(t, err)

	rt := runtime.NewMockRuntime(store)
	var buf bytes.Buffer
	res, err := rt.Execute(ctx, fmt.Sprintf("%d", acc.ID), []byte(`{"model":"gpt-4o"}`), &buf)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, 200, res.StatusCode)

	// Verify the stream contains OpenAI chunk data
	streamStr := buf.String()
	require.Contains(t, streamStr, "chat.completion.chunk")
	require.Contains(t, streamStr, "[DONE]")
}

func TestYunwuHypervisorDispatch(t *testing.T) {
	ctx := context.Background()
	store := newTestStorage(t)

	// Create a Yunwu account in DB
	acc, err := store.DB().Account.Create().
		SetName("yunwu-node-1").
		SetPlatform("yunwu").
		SetType("api_key").
		SetStatus("active").
		SetCredentials(map[string]any{"api_key": "sk-yunwu-test-key", "base_url": "https://yunwu.ai/v1"}).
		Save(ctx)
	require.NoError(t, err)
	_ = acc

	rt := runtime.NewMockRuntime(store)
	hyp := hypervisor.NewHypervisor(store, rt)

	require.NotNil(t, hyp.YunwuEngine())

	var buf bytes.Buffer
	res, err := hyp.HandleRequest(ctx, "user-123", "conv-yunwu-1", []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`), &buf, "yunwu")
	require.NoError(t, err)
	require.NotNil(t, res)
}



package hypervisor_test

import (
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

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/events"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/lease"
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

package compiler

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/session"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
	_ "modernc.org/sqlite"
)

func newTestStorage(t *testing.T) *storage.Storage {
	t.Helper()

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

	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return storage.NewStorage(client, rdb)
}

func TestSummaryWorkerShouldSummarize(t *testing.T) {
	sw := NewSummaryWorker(nil)

	// Under threshold
	execCtxSmall := ExecutionContext{
		RecentMessages: []Message{
			{Role: "user", Content: "Hello"},
		},
	}
	assert.False(t, sw.ShouldSummarize(execCtxSmall))

	// Over message count threshold (> 20)
	var messages []Message
	for i := 0; i < 25; i++ {
		messages = append(messages, Message{Role: "user", Content: "test message"})
	}
	execCtxLarge := ExecutionContext{
		RecentMessages: messages,
	}
	assert.True(t, sw.ShouldSummarize(execCtxLarge))
}

func TestSummaryWorkerGenerateAndTriggerAsync(t *testing.T) {
	store := newTestStorage(t)
	sw := NewSummaryWorker(store)

	var messages []Message
	for i := 0; i < 22; i++ {
		messages = append(messages, Message{Role: "user", Content: fmt.Sprintf("Message payload %d", i)})
	}
	execCtx := ExecutionContext{
		Summary:        "Initial overview",
		RecentMessages: messages,
	}
	sm := session.NewSessionManager(store)
	require.NoError(t, sm.SaveSession(context.Background(), &session.VirtualSession{
		ID: "sess-summary-1", OwnerID: "user-1", ConversationID: "summary", State: "active",
		Context: &session.ExecutionContext{}, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}))

	summary := sw.GenerateSummary(execCtx)
	assert.Contains(t, summary, "Initial overview")
	assert.Contains(t, summary, "22 total messages")

	sw.TriggerAsyncSummary(context.Background(), "user-1", "sess-summary-1", execCtx)
	require.Eventually(t, func() bool {
		sess, err := sm.GetSession(context.Background(), "sess-summary-1")
		return err == nil && sess != nil && sess.Context != nil && strings.Contains(sess.Context.Summary, "22 total messages")
	}, time.Second, 10*time.Millisecond)
	events, err := store.DB().AshEvent.Query().All(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, events)
	assert.Equal(t, "SummaryUpdated", events[len(events)-1].EventType)
}

package stats_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/stats"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestStatsCollector(t *testing.T) {
	store := newTestStorage(t)
	sc := stats.NewStatsCollector(store)
	ctx := context.Background()

	gStats, err := sc.GetGlobalStats(ctx)
	require.NoError(t, err)
	assert.NotNil(t, gStats)
	_, err = store.RecordUsage(ctx, "user-1", nil, nil, 12, 8, 20)
	require.NoError(t, err)

	uStat, err := sc.GetUserStats(ctx, "user-1", "standard")
	require.NoError(t, err)
	assert.Equal(t, "user-1", uStat.OwnerID)
	assert.Equal(t, int64(12), uStat.InputTokens)
	assert.Equal(t, int64(8), uStat.OutputTokens)

	nStats, err := sc.GetNodeStats(ctx)
	require.NoError(t, err)
	assert.Empty(t, nStats)

	analytics, err := sc.GetAnalytics(ctx)
	require.NoError(t, err)
	assert.Equal(t, "ok", analytics.Status)
}

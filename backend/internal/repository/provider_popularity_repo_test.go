package repository

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func newProviderPopularityRepoSQLite(t *testing.T) (service.ProviderPopularityRepository, *dbent.Client) {
	t.Helper()

	dbName := fmt.Sprintf("file:provider_popularity_test_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano())
	db, err := sql.Open("sqlite", dbName)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	repo := NewProviderPopularityRepository(client)
	return repo, client
}

func TestProviderPopularityRepository_IncrementAndGet(t *testing.T) {
	ctx := context.Background()
	repo, _ := newProviderPopularityRepoSQLite(t)

	// 1. Initial get should return 0 clicks
	pop, err := repo.GetByProvider(ctx, "openai")
	require.NoError(t, err)
	assert.Equal(t, "openai", pop.Provider)
	assert.Equal(t, int64(0), pop.ClickCount)

	// 2. Increment click for openai
	inc1, err := repo.IncrementClick(ctx, "openai")
	require.NoError(t, err)
	assert.Equal(t, "openai", inc1.Provider)
	assert.Equal(t, int64(1), inc1.ClickCount)
	assert.NotNil(t, inc1.LastClickedAt)

	// 3. Increment again
	inc2, err := repo.IncrementClick(ctx, "openai")
	require.NoError(t, err)
	assert.Equal(t, int64(2), inc2.ClickCount)

	// 4. Increment for another provider
	incClaude, err := repo.IncrementClick(ctx, "claude")
	require.NoError(t, err)
	assert.Equal(t, "claude", incClaude.Provider)
	assert.Equal(t, int64(1), incClaude.ClickCount)

	// 5. List popularity
	list, err := repo.ListPopularity(ctx)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "openai", list[0].Provider)
	assert.Equal(t, int64(2), list[0].ClickCount)
	assert.Equal(t, 1, list[0].Rank)

	assert.Equal(t, "claude", list[1].Provider)
	assert.Equal(t, int64(1), list[1].ClickCount)
	assert.Equal(t, 2, list[1].Rank)
}

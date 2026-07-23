package quota

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
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

func TestQuotaManagerGetUserAccessAndCheckQuota(t *testing.T) {
	store := newTestStorage(t)
	qm := NewQuotaManager(store)

	ctx := context.Background()
	ownerID := "user-100"

	// Initial check quota should succeed with default mock pool limit
	ok, err := qm.CheckQuota(ctx, ownerID)
	require.NoError(t, err)
	assert.True(t, ok)

	// Consume some quota
	err = qm.ConsumeQuota(ctx, ownerID, 500)
	require.NoError(t, err)

	ua, err := qm.GetUserAccess(ctx, ownerID)
	require.NoError(t, err)
	assert.Equal(t, int64(500), ua.UsedUnits)

	// Register active user and nodes for dynamic limit test
	err = qm.RegisterActiveUser(ctx, ownerID)
	require.NoError(t, err)
	err = qm.RegisterNode(ctx, "node-1")
	require.NoError(t, err)

	dynLimit, err := qm.GetDynamicLimit(ctx, ownerID)
	require.NoError(t, err)
	assert.Greater(t, dynLimit, int64(0))
}

func TestQuotaManagerExceeded(t *testing.T) {
	store := newTestStorage(t)
	qm := NewQuotaManager(store)

	ctx := context.Background()
	ownerID := "user-exceeded"

	ua := &UserAccess{
		OwnerID:      ownerID,
		PlanID:       "pool-basic",
		MonthlyLimit: 100,
		UsedUnits:    150,
		Priority:     1,
		IsActive:     true,
	}
	err := qm.SaveUserAccess(ctx, ua)
	require.NoError(t, err)

	ok, err := qm.CheckQuota(ctx, ownerID)
	assert.ErrorIs(t, err, ErrQuotaExceeded)
	assert.False(t, ok)
}

func TestQuotaManagerInactive(t *testing.T) {
	store := newTestStorage(t)
	qm := NewQuotaManager(store)

	ctx := context.Background()
	ownerID := "user-inactive"

	ua := &UserAccess{
		OwnerID:      ownerID,
		PlanID:       "pool-basic",
		MonthlyLimit: 1000,
		UsedUnits:    0,
		Priority:     1,
		IsActive:     false,
	}
	err := qm.SaveUserAccess(ctx, ua)
	require.NoError(t, err)

	ok, err := qm.CheckQuota(ctx, ownerID)
	assert.ErrorIs(t, err, ErrUserInactive)
	assert.False(t, ok)
}

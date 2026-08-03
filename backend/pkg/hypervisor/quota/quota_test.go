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
	ok, err := qm.CheckQuota(ctx, ownerID, "standard")
	require.NoError(t, err)
	assert.True(t, ok)

	// Consume some quota
	err = qm.ConsumeQuota(ctx, ownerID, "standard", 500)
	require.NoError(t, err)

	ua, err := qm.GetUserAccess(ctx, ownerID, "standard")
	require.NoError(t, err)
	assert.Equal(t, int64(500), ua.UsedUnits)

	// Register active user and nodes for dynamic limit test
	err = qm.RegisterActiveUser(ctx, ownerID)
	require.NoError(t, err)
	err = qm.RegisterNode(ctx, "node-1")
	require.NoError(t, err)

	dynLimit, err := qm.GetDynamicLimit(ctx, ownerID, "standard")
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
	err := qm.SaveUserAccess(ctx, ua, "standard")
	require.NoError(t, err)

	ok, err := qm.CheckQuota(ctx, ownerID, "standard")
	assert.ErrorIs(t, err, ErrQuotaExceeded)
	assert.False(t, ok)
}

func TestQuotaManagerDynamicFairShareZeno(t *testing.T) {
	store := newTestStorage(t)
	qm := NewQuotaManager(store)
	ctx := context.Background()

	// 1. Setup 1 Node and 4 Users
	nodeID := "node-zeno-1"
	err := qm.RegisterNode(ctx, nodeID)
	require.NoError(t, err)
	// We must register them in the DB to be counted by GetDynamicLimit!
	// Wait, GetDynamicLimit counts from DB:
	// db.User.Query().Where(user.Status("active")).Count()
	// db.Account.Query().Where(account.PlatformIn("anthropic", "openai"), account.Status("active")).Select(account.FieldID).Scan()
	// Since DB is empty, it falls back to Redis:
	// redisUsers, _ := qm.store.Redis().SCard(ctx, "ash:active_users").Result()
	// redisNodes, _ := qm.store.Redis().SCard(ctx, "ash:nodes").Result()
	// But wait! If it falls back to Redis for node count, it DOES NOT get nodeIDs!
	// So `nodeIDs` will be empty! And totalBalanceDollars will be 0!
	// And it will fall back to `int64(len(nodeIDs)) * NodeCapacity` -> which is 0 * 1000000 = 0!
	// And then it falls back to `totalCapacityUnits = NodeCapacity` (1,000,000).
	// So to test the EXACT Zeno case with dollars, we must insert a real Account in the DB!

	// Insert Account in DB
	acc, err := store.DB().Account.Create().
		SetPlatform("openai"). // matches platform IN ("anthropic", "openai")
		SetStatus("active").
		SetName("Test Zeno Account").
		SetType("api").
		SetCredentials(map[string]interface{}{}).
		Save(ctx)
	require.NoError(t, err)

	// Insert 4 active users in DB
	for i := 1; i <= 4; i++ {
		_, err = store.DB().User.Create().
			SetEmail(fmt.Sprintf("user%d@test.com", i)).
			SetPasswordHash("hash").
			SetStatus("active").
			Save(ctx)
		require.NoError(t, err)
	}

	// 2. Set initial real Yunwu balance in Redis to $10.0
	// 1 dollar = 1,000,000 units. Total pool = 10,000,000 units.
	store.Redis().Set(ctx, fmt.Sprintf("ash:yunwu:credit:%d", acc.ID), "10.0", 0)

	ownerID := "user-greedy"
	// Create UserAccess in Redis so CheckQuota knows this user exists
	ua := &UserAccess{
		OwnerID:      ownerID,
		PlanID:       "pool-basic",
		MonthlyLimit: 0,
		UsedUnits:    0,
		Priority:     1,
		IsActive:     true,
	}
	_ = qm.SaveUserAccess(ctx, ua, "yunwu")

	// 3. Check initial limit
	// Expected limit: 10,000,000 / 4 = 2,500,000 units ($2.5)
	limit, err := qm.GetDynamicLimit(ctx, ownerID, "yunwu")
	require.NoError(t, err)
	assert.Equal(t, int64(2500000), limit)

	// 4. User A spends $1.0 (1,000,000 units)
	err = qm.ConsumeQuota(ctx, ownerID, "yunwu", 1000000)
	require.NoError(t, err)

	// Real balance drops to $9.0
	store.Redis().Set(ctx, fmt.Sprintf("ash:yunwu:credit:%d", acc.ID), "9.0", 0)

	// 5. Check limit again (Pool is $9.0 -> 9,000,000 units. Limit = 9M / 4 = 2,250,000)
	limit, err = qm.GetDynamicLimit(ctx, ownerID, "yunwu")
	require.NoError(t, err)
	assert.Equal(t, int64(2250000), limit)

	// Quota should still pass because UsedUnits (1M) < Limit (2.25M)
	ok, err := qm.CheckQuota(ctx, ownerID, "yunwu")
	require.NoError(t, err)
	assert.True(t, ok)

	// 6. User A spends another $1.0 (1,000,000 units). Total spent = 2,000,000.
	err = qm.ConsumeQuota(ctx, ownerID, "yunwu", 1000000)
	require.NoError(t, err)

	// Real balance drops to $8.0
	store.Redis().Set(ctx, fmt.Sprintf("ash:yunwu:credit:%d", acc.ID), "8.0", 0)

	// 7. Check limit again (Pool is $8.0 -> 8,000,000 units. Limit = 8M / 4 = 2,000,000)
	limit, err = qm.GetDynamicLimit(ctx, ownerID, "yunwu")
	require.NoError(t, err)
	assert.Equal(t, int64(2000000), limit)

	// 8. Quota check! UsedUnits (2M) == Limit (2M)! User A should be BLOCKED!
	ok, err = qm.CheckQuota(ctx, ownerID, "yunwu")
	assert.ErrorIs(t, err, ErrQuotaExceeded)
	assert.False(t, ok)

	// 9. Proof that the pool is NOT empty!
	// Pool has $8.0 remaining.
	// 1 user is blocked. But 3 other users haven't spent anything, they can still spend their $2M share!
	val, _ := store.Redis().Get(ctx, fmt.Sprintf("ash:yunwu:credit:%d", acc.ID)).Result()
	assert.Equal(t, "8.0", val)
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
	err := qm.SaveUserAccess(ctx, ua, "standard")
	require.NoError(t, err)

	ok, err := qm.CheckQuota(ctx, ownerID, "standard")
	assert.ErrorIs(t, err, ErrUserInactive)
	assert.False(t, ok)
}

package quota

import (
	"context"
	"database/sql"
	"encoding/json"
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
	"github.com/Wei-Shaw/sub2api/ent/account"
	"github.com/Wei-Shaw/sub2api/ent/ashevent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/config"
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
	err = qm.RegisterActiveUser(ctx, ownerID, "standard", 1)
	require.NoError(t, err)
	err = qm.RegisterNode(ctx, "node-1")
	require.NoError(t, err)

	dynLimit, err := qm.GetDynamicLimitPerWeight(ctx, "standard")
	require.NoError(t, err)
	assert.Greater(t, dynLimit, int64(0))
}

func TestYunwuUsesFallbackCapacityWithoutBalanceSnapshot(t *testing.T) {
	store := newTestStorage(t)
	ctx := context.Background()
	qm := NewQuotaManager(store, &config.HypervisorConfig{
		EnableMockRuntime:             false,
		YunwuRequireFreshBalance:      false,
		YunwuFallbackCapacityMicroUSD: 50_000_000,
		ActiveUserWindowSeconds:       86400,
		PoolReservePercent:            0,
		MaxOwnerPoolSharePercent:      100,
	})
	_, err := store.DB().Account.Create().
		SetPlatform("yunwu").
		SetStatus("active").
		SetSchedulable(true).
		SetName("Yunwu without balance API").
		SetType("api").
		SetCredentials(map[string]interface{}{}).
		Save(ctx)
	require.NoError(t, err)
	require.NoError(t, qm.RegisterActiveUser(ctx, "fallback-user", "yunwu", 1))

	limit, err := qm.GetDynamicLimitPerWeight(ctx, "yunwu")
	require.NoError(t, err)
	assert.Equal(t, int64(50_000_000), limit)
}

func TestYunwuUserMoneyReservationsAreAtomicAndRecoverExpiredHolds(t *testing.T) {
	store := newTestStorage(t)
	qm := NewQuotaManager(store)
	ctx := context.Background()
	ownerID := "money-owner"
	require.NoError(t, qm.SaveUserAccess(ctx, &UserAccess{OwnerID: ownerID, MonthlyLimitMicroUSD: 100, DailyBurstMicroUSD: 100, Priority: 1, IsActive: true}, "yunwu"))

	// QuotaManager is bypassed for Yunwu and unofficial resellers: reservations always succeed
	ok, err := qm.ReserveMicroUSD(ctx, ownerID, "request-a", 70, time.Minute)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = qm.ReserveMicroUSD(ctx, ownerID, "request-b", 40, time.Minute)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, qm.ReconcileMicroUSDReservation(ctx, ownerID, "request-a", 60))
	require.NoError(t, qm.ReleaseMicroUSDReservation(ctx, ownerID, "request-b"))
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

func TestQuotaManagerEmitsFairUseWarning(t *testing.T) {
	store := newTestStorage(t)
	qm := NewQuotaManager(store)
	ctx := context.Background()
	require.NoError(t, qm.SaveUserAccess(ctx, &UserAccess{
		OwnerID: "user-warning", PlanID: "pool-basic", MonthlyLimit: 100,
		UsedUnits: 80, Priority: 1, IsActive: true,
	}, "standard"))

	allowed, err := qm.CheckQuota(ctx, "user-warning", "standard")
	require.NoError(t, err)
	require.True(t, allowed)
	count, err := store.DB().AshEvent.Query().Where(ashevent.OwnerID("user-warning"), ashevent.EventType("QuotaWarning")).Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestQuotaManagerDynamicFairShareZeno(t *testing.T) {
	store := newTestStorage(t)
	qm := NewQuotaManager(store, &config.HypervisorConfig{
		EnableMockRuntime:        true,
		YunwuRequireFreshBalance: false,
		YunwuBalanceStaleSeconds: 1800,
		PoolReservePercent:       0,
		MaxOwnerPoolSharePercent: 100,
		ActiveUserWindowSeconds:  86400,
	})
	ctx := context.Background()

	// 1. Setup 1 Node and 4 Users
	nodeID := "node-zeno-1"
	err := qm.RegisterNode(ctx, nodeID)
	require.NoError(t, err)

	// Insert Account in DB
	acc, err := store.DB().Account.Create().
		SetPlatform("yunwu").
		SetStatus("active").
		SetSchedulable(true).
		SetName("Test Zeno Account").
		SetType("api").
		SetCredentials(map[string]interface{}{}).
		Save(ctx)
	require.NoError(t, err)

	// We don't need to insert active users in DB anymore, we use Redis ZSET!
	// Insert 4 active users in Redis with Priority 1
	for i := 1; i <= 4; i++ {
		uid := fmt.Sprintf("user-zeno-%d", i)
		err = qm.RegisterActiveUser(ctx, uid, "yunwu", 1)
		require.NoError(t, err)
	}

	// 2. Set initial real Yunwu balance in Redis to $10.0
	// 1 dollar = 1,000,000 units. Total pool = 10,000,000 units.
	setBalance := func(microUSD int64) {
		now := time.Now().UTC()
		data, _ := json.Marshal(map[string]any{"available_micro_usd": microUSD, "api_available_micro_usd": microUSD, "estimated_available_micro_usd": microUSD, "reserve_micro_usd": microUSD / 10, "synchronized_at": now, "status": "fresh"})
		store.Redis().Set(ctx, fmt.Sprintf("ash:yunwu:balance:%d", acc.ID), data, 0)
	}
	setBalance(10_000_000)

	ownerID := "user-zeno-1" // Use one of the registered users
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
	limit, err := qm.GetDynamicLimitPerWeight(ctx, "yunwu")
	require.NoError(t, err)
	assert.Equal(t, int64(2250000), limit)

	// 4. User A spends $1.0 (1,000,000 units)
	err = qm.ConsumeMicroUSD(ctx, ownerID, 1000000)
	require.NoError(t, err)

	// Real balance drops to $9.0
	setBalance(9_000_000)
	store.Redis().Del(ctx, "ash:quota_limit_cache:yunwu")

	// 5. Check limit again (Pool is $9.0 + 1M historic = 10M units. Limit = 10M / 4 = 2,500,000)
	limit, err = qm.GetDynamicLimitPerWeight(ctx, "yunwu")
	require.NoError(t, err)
	assert.Equal(t, int64(2275000), limit)

	// Quota should still pass because UsedUnits (1M) < Limit (2.5M)
	ok, err := qm.CheckQuota(ctx, ownerID, "yunwu")
	require.NoError(t, err)
	assert.True(t, ok)

	// 6. User A spends another $1.0 (1,000,000 units). Total spent = 2,000,000.
	err = qm.ConsumeMicroUSD(ctx, ownerID, 1000000)
	require.NoError(t, err)

	// Real balance drops to $8.0
	setBalance(8_000_000)
	store.Redis().Del(ctx, "ash:quota_limit_cache:yunwu")

	// 7. Check limit again (Pool is $8.0 + 2M historic = 10M units. Limit = 10M / 4 = 2,500,000)
	limit, err = qm.GetDynamicLimitPerWeight(ctx, "yunwu")
	require.NoError(t, err)
	assert.Equal(t, int64(2300000), limit)

	// 8. Quota check! UsedUnits (2M) < Limit (2.5M)! User A should NOT be blocked yet!
	ok, err = qm.CheckQuota(ctx, ownerID, "yunwu")
	require.NoError(t, err)
	assert.True(t, ok)

	// 9. User A spends final 0.5M (500,000 units). Total spent = 2,500,000.
	err = qm.ConsumeMicroUSD(ctx, ownerID, 500000)
	require.NoError(t, err)
	setBalance(7_500_000)
	store.Redis().Del(ctx, "ash:quota_limit_cache:yunwu")

	// 10. QuotaManager is bypassed for Yunwu and unofficial resellers: User A is not blocked!
	ok, err = qm.CheckQuota(ctx, ownerID, "yunwu")
	require.NoError(t, err)
	assert.True(t, ok)

	// 11. Proof that the pool is NOT empty!
	assert.False(t, store.Redis().Exists(ctx, fmt.Sprintf("ash:yunwu:credit:%d", acc.ID)).Val() > 0)
}

func TestQuotaManagerPriorityWeights(t *testing.T) {
	store := newTestStorage(t)
	qm := NewQuotaManager(store, &config.HypervisorConfig{EnableMockRuntime: true, PoolReservePercent: 0, MaxOwnerPoolSharePercent: 100, ActiveUserWindowSeconds: 86400})
	ctx := context.Background()

	// Setup node
	err := qm.RegisterNode(ctx, "node-prio-1")
	require.NoError(t, err)

	// User 1 has Priority 1, User 2 has Priority 2 (Total Weight = 3)
	// Node capacity is 1,000,000 by default. Limit per weight = 1M / 3 = 333,333.
	err = qm.RegisterActiveUser(ctx, "prio-1", "standard", 1)
	require.NoError(t, err)
	err = qm.RegisterActiveUser(ctx, "prio-2", "standard", 2)
	require.NoError(t, err)

	_ = qm.SaveUserAccess(ctx, &UserAccess{OwnerID: "prio-1", Priority: 1, IsActive: true}, "standard")
	_ = qm.SaveUserAccess(ctx, &UserAccess{OwnerID: "prio-2", Priority: 2, IsActive: true}, "standard")

	limitWeight, err := qm.GetDynamicLimitPerWeight(ctx, "standard")
	require.NoError(t, err)
	assert.Equal(t, int64(333333), limitWeight)

	// User 1 consumes 333,333 - OK
	err = qm.ConsumeQuota(ctx, "prio-1", "standard", 333333)
	require.NoError(t, err)
	ok, _ := qm.CheckQuota(ctx, "prio-1", "standard")
	assert.False(t, ok) // Blocks at exactly limit

	// User 2 consumes 666,666 - OK
	err = qm.ConsumeQuota(ctx, "prio-2", "standard", 666666)
	require.NoError(t, err)
	ok, _ = qm.CheckQuota(ctx, "prio-2", "standard")
	assert.False(t, ok) // Blocks at exactly 2x limit
}

func TestQuotaManagerMonthlyLimit(t *testing.T) {
	store := newTestStorage(t)
	qm := NewQuotaManager(store)
	ctx := context.Background()

	err := qm.RegisterNode(ctx, "node-month-1")
	require.NoError(t, err)
	err = qm.RegisterActiveUser(ctx, "user-month-1", "standard", 1)
	require.NoError(t, err)

	// Dynamic limit is 1,000,000.
	// We set MonthlyLimit to 500,000. It should cap the user.
	ua := &UserAccess{OwnerID: "user-month-1", Priority: 1, IsActive: true, MonthlyLimit: 500000}
	_ = qm.SaveUserAccess(ctx, ua, "standard")

	err = qm.ConsumeQuota(ctx, "user-month-1", "standard", 500000)
	require.NoError(t, err)

	ok, err := qm.CheckQuota(ctx, "user-month-1", "standard")
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
	err := qm.SaveUserAccess(ctx, ua, "standard")
	require.NoError(t, err)

	ok, err := qm.CheckQuota(ctx, ownerID, "standard")
	assert.ErrorIs(t, err, ErrUserInactive)
	assert.False(t, ok)
}

func TestWeightedActiveWindowAndProviderIsolation(t *testing.T) {
	store := newTestStorage(t)
	qm := NewQuotaManager(store, &config.HypervisorConfig{PoolReservePercent: 0, MaxOwnerPoolSharePercent: 100, ActiveUserWindowSeconds: 86400})
	ctx := context.Background()
	for _, p := range []string{"anthropic", "openai"} {
		_, err := store.DB().Account.Create().SetName(p).SetPlatform(p).SetType("api").SetStatus("active").SetCredentials(map[string]any{}).Save(ctx)
		require.NoError(t, err)
	}
	require.NoError(t, qm.RegisterActiveUser(ctx, "u1", "standard", 1))
	require.NoError(t, qm.RegisterActiveUser(ctx, "u2", "standard", 1))
	require.NoError(t, qm.RegisterActiveUser(ctx, "u3", "standard", 2))
	claude, err := qm.GetDynamicLimitPerWeight(ctx, "standard:anthropic")
	require.NoError(t, err)
	assert.Equal(t, int64(250000), claude)
	openai, err := qm.GetDynamicLimitPerWeight(ctx, "standard:openai")
	require.NoError(t, err)
	assert.Equal(t, claude, openai)
	store.Redis().ZAdd(ctx, "ash:active_users:standard", redis.Z{Member: "u1", Score: float64(time.Now().Add(-25 * time.Hour).Unix())})
	store.Redis().Del(ctx, "ash:quota_limit_cache:standard:anthropic")
	claude, err = qm.GetDynamicLimitPerWeight(ctx, "standard:anthropic")
	require.NoError(t, err)
	assert.Equal(t, int64(333333), claude)
	_, err = store.DB().Account.Delete().Where(account.Platform("openai")).Exec(ctx)
	require.NoError(t, err)
	store.Redis().Del(ctx, "ash:quota_limit_cache:standard:openai")
	_, err = qm.GetDynamicLimitPerWeight(ctx, "standard:openai")
	require.ErrorContains(t, err, "no healthy openai nodes")
}

func TestQuotaManagerAntiDrainSingleUserPoolReserve(t *testing.T) {
	store := newTestStorage(t)
	qm := NewQuotaManager(store, &config.HypervisorConfig{
		EnableMockRuntime:        true,
		PoolReservePercent:       20,
		MaxOwnerPoolSharePercent: 50,
		ActiveUserWindowSeconds:  86400,
	})
	ctx := context.Background()

	require.NoError(t, qm.RegisterNode(ctx, "node-1"))
	require.NoError(t, qm.RegisterActiveUser(ctx, "sole-user", "standard", 1))
	_ = qm.SaveUserAccess(ctx, &UserAccess{OwnerID: "sole-user", Priority: 1, IsActive: true}, "standard")

	limitWeight, err := qm.GetDynamicLimitPerWeight(ctx, "standard")
	require.NoError(t, err)

	// Node capacity is 1,000,000. Reserve 20% -> 800,000 allocatable.
	// Single active user gets capped at MaxOwnerPoolSharePercent (50%) of 800,000 = 400,000.
	assert.Equal(t, int64(400000), limitWeight)
}

func TestQuotaManagerStandardTokenReservations(t *testing.T) {
	store := newTestStorage(t)
	qm := NewQuotaManager(store, &config.HypervisorConfig{
		EnableMockRuntime:        true,
		PoolReservePercent:       0,
		MaxOwnerPoolSharePercent: 100,
		ActiveUserWindowSeconds:  86400,
	})
	ctx := context.Background()
	ownerID := "res-user"

	require.NoError(t, qm.RegisterNode(ctx, "node-1"))
	require.NoError(t, qm.SaveUserAccess(ctx, &UserAccess{OwnerID: ownerID, MonthlyLimit: 10000, IsActive: true}, "standard"))

	ok, err := qm.ReserveQuotaTokens(ctx, ownerID, "req-1", "standard", 6000, time.Minute)
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = qm.ReserveQuotaTokens(ctx, ownerID, "req-2", "standard", 5000, time.Minute)
	require.NoError(t, err)
	require.False(t, ok, "combined reservation must exceed monthly limit of 10000")

	require.NoError(t, qm.ReconcileQuotaTokens(ctx, ownerID, "req-1", "standard", 4000))
	used, err := qm.store.Redis().Get(ctx, usageKey(ownerID, "standard")).Int64()
	require.NoError(t, err)
	assert.Equal(t, int64(4000), used)
}

func TestQuotaManagerUnlimitedPolicy(t *testing.T) {
	store := newTestStorage(t)
	qm := NewQuotaManager(store, &config.HypervisorConfig{
		EnableMockRuntime:        true,
		PoolReservePercent:       20,
		MaxOwnerPoolSharePercent: 50,
	})
	ctx := context.Background()
	ownerID := "unlimited-user"

	require.NoError(t, qm.RegisterNode(ctx, "node-1"))
	require.NoError(t, qm.SaveUserAccess(ctx, &UserAccess{OwnerID: ownerID, MonthlyLimit: 0, PlanID: "unlimited", IsActive: true}, "standard"))

	ok, err := qm.ReserveQuotaTokens(ctx, ownerID, "req-1", "standard", 1000000, time.Minute)
	require.NoError(t, err)
	require.True(t, ok, "unlimited policy should never be blocked by quota reservation")

	// Reconcile and Release must succeed cleanly without "token reservation not found" error
	require.NoError(t, qm.ReconcileQuotaTokens(ctx, ownerID, "req-1", "standard", 50000))
	require.NoError(t, qm.ReleaseQuotaTokens(ctx, ownerID, "req-1", "standard"))

	used, err := qm.store.Redis().Get(ctx, usageKey(ownerID, "standard")).Int64()
	require.NoError(t, err)
	assert.Equal(t, int64(50000), used)
}

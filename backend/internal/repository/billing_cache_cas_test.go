package repository

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBillingCache_CASVersioning(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	cache := NewBillingCache(client)
	ctx := context.Background()
	userID := int64(9901)

	// 1. Initial SetUserBalance with version 1000
	err := cache.SetUserBalance(ctx, userID, 10.00, 1000)
	require.NoError(t, err)

	bal, err := cache.GetUserBalance(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, 10.00, bal)

	// 2. Newer version 2000 overwrites version 1000
	err = cache.SetUserBalance(ctx, userID, 15.00, 2000)
	require.NoError(t, err)

	bal, err = cache.GetUserBalance(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, 15.00, bal)

	// 3. Stale read with older version (1500 < 2000) is rejected by CAS
	err = cache.SetUserBalance(ctx, userID, 8.00, 1500)
	require.NoError(t, err)

	// Balance must remain 15.00, NOT overwritten by stale 8.00!
	bal, err = cache.GetUserBalance(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, 15.00, bal)

	// 4. Deduction updates balance and bumps version, preventing stale overwrite
	deductTime := time.Now().UnixNano()
	err = cache.DeductUserBalance(ctx, userID, 3.00)
	require.NoError(t, err)

	bal, err = cache.GetUserBalance(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, 12.00, bal)

	// A delayed DB load from before the deduction (with version <= deductTime) is rejected
	err = cache.SetUserBalance(ctx, userID, 15.00, deductTime-1000)
	require.NoError(t, err)

	bal, err = cache.GetUserBalance(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, 12.00, bal)

	// 5. Invalidate deletes the key
	err = cache.InvalidateUserBalance(ctx, userID)
	require.NoError(t, err)

	_, err = cache.GetUserBalance(ctx, userID)
	require.ErrorIs(t, err, redis.Nil)
}

func TestBillingCache_DeductOnNonExistent_IsNoOp(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	cache := NewBillingCache(client)
	ctx := context.Background()
	userID := int64(9902)

	err := cache.DeductUserBalance(ctx, userID, 5.00)
	require.NoError(t, err)

	// Must remain cache miss
	_, err = cache.GetUserBalance(ctx, userID)
	require.ErrorIs(t, err, redis.Nil)
}

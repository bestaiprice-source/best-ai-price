package hypervisor_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/quota"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/runtime"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/yunwu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConcurrentHandleRequestTokenReservations(t *testing.T) {
	store := newTestStorage(t)
	ctx := context.Background()

	// Create 1 standard node (NodeCapacity = 1,000,000 tokens)
	_, err := store.DB().Account.Create().
		SetName("standard-node-conc").
		SetPlatform("anthropic").
		SetType("api").
		SetStatus("active").
		SetSchedulable(true).
		SetCredentials(map[string]any{"api_key": "sk-ant-test"}).
		Save(ctx)
	require.NoError(t, err)

	rt := runtime.NewMockRuntime(store)
	rt.CustomInputTokens = 50000
	rt.CustomOutputTokens = 50000
	hv := hypervisor.NewHypervisor(store, rt)

	// User priority = 3 (high priority)
	// Even with priority 3, MaxOwnerPoolSharePercent (50%) caps user share at 400,000 tokens (800,000 * 50%).
	_ = store.DB().AshUserPolicy.Create().
		SetOwnerID("high-prio-user").
		SetPoolType("standard").
		SetPlanID("pro").
		SetPriority(3).
		SetMonthlyLimit(1000000).
		SetIsActive(true).
		Exec(ctx)

	// Payload specifies max_tokens = 100,000
	payload := []byte(`{"model":"claude-3-5-sonnet-20241022","max_tokens":100000,"messages":[{"role":"user","content":"hi"}]}`)

	var wg sync.WaitGroup
	var successCount int64
	var quotaExceededCount int64
	var concurrentLimitCount int64

	// Launch 4 concurrent worker requests
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(reqIdx int) {
			defer wg.Done()
			var buf bytes.Buffer
			convID := fmt.Sprintf("conv-%d", reqIdx)
			meta, reqErr := hv.HandleRequest(ctx, "high-prio-user", convID, payload, &buf, "anthropic")
			if reqErr == nil && meta != nil && meta.StatusCode == 200 {
				atomic.AddInt64(&successCount, 1)
			} else if reqErr == quota.ErrQuotaExceeded {
				atomic.AddInt64(&quotaExceededCount, 1)
			} else if reqErr == quota.ErrConcurrentLimitExceeded {
				atomic.AddInt64(&concurrentLimitCount, 1)
			}
		}(i)
	}
	wg.Wait()

	// Verify atomic reservations prevented overshooting the owner share cap
	assert.Greater(t, successCount, int64(0), "At least some requests should succeed")
	assert.Greater(t, quotaExceededCount, int64(0), "Requests exceeding the 400,000 token owner cap must be rejected with ErrQuotaExceeded")

	// Verify total recorded usage does not exceed allocatable owner share cap (400,000 tokens)
	usages, err := store.DB().AshUsage.Query().All(ctx)
	require.NoError(t, err)
	var totalCost int64
	for _, u := range usages {
		totalCost += u.CostUnits
	}
	assert.LessOrEqual(t, totalCost, int64(400000), "Total consumed cost must respect owner pool share cap of 400,000 tokens")
}

func TestConcurrentHandleRequestYunwuReservations(t *testing.T) {
	store := newTestStorage(t)
	ctx := context.Background()

	node, err := store.DB().Account.Create().
		SetName("yunwu-node-conc").
		SetPlatform("yunwu").
		SetType("api").
		SetStatus("active").
		SetSchedulable(true).
		SetCredentials(map[string]any{"api_key": "sk-yunwu-test"}).
		Save(ctx)
	require.NoError(t, err)

	rt := runtime.NewMockRuntime(store)
	rt.CustomInputTokens = 10000
	rt.CustomOutputTokens = 10000
	hv := hypervisor.NewHypervisor(store, rt)

	// Save fresh Yunwu balance in Redis: $1.00 (1,000,000 microUSD)
	now := time.Now().UTC()
	snapshot := yunwu.BalanceSnapshot{
		GrantedUSD:                 1,
		AvailableUSD:               1,
		AvailableMicroUSD:          1_000_000,
		APIAvailableMicroUSD:       1_000_000,
		EstimatedAvailableMicroUSD: 1_000_000,
		ReserveMicroUSD:            0,
		StaleAt:                    now.Add(30 * time.Minute),
		Source:                     "test",
		SynchronizedAt:             now,
		Status:                     "fresh",
	}
	jsonBytes, err := json.Marshal(snapshot)
	require.NoError(t, err)
	_ = store.Redis().Set(ctx, fmt.Sprintf("ash:yunwu:balance:%d", node.ID), jsonBytes, 10*time.Minute).Err()

	// User has explicit monetary limit of 500,000 microUSD ($0.50)
	_ = store.DB().AshUserPolicy.Create().
		SetOwnerID("yunwu-conc-user").
		SetPoolType("yunwu").
		SetPlanID("pro").
		SetMonthlyLimitMicroUsd(500000).
		SetDailyBurstMicroUsd(500000).
		SetPriority(1).
		SetIsActive(true).
		Exec(ctx)

	payload := []byte(`{"model":"gpt-4o","max_tokens":150000,"messages":[{"role":"user","content":"hi"}]}`)

	var wg sync.WaitGroup
	var successCount int64
	var quotaExceededCount int64

	// Launch 4 concurrent worker requests
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(reqIdx int) {
			defer wg.Done()
			var buf bytes.Buffer
			convID := fmt.Sprintf("conv-yw-%d", reqIdx)
			meta, reqErr := hv.HandleRequest(ctx, "yunwu-conc-user", convID, payload, &buf, "yunwu")
			if reqErr == nil && meta != nil && meta.StatusCode == 200 {
				atomic.AddInt64(&successCount, 1)
			} else if reqErr == quota.ErrQuotaExceeded {
				atomic.AddInt64(&quotaExceededCount, 1)
			}
		}(i)
	}
	wg.Wait()

	assert.Greater(t, successCount, int64(0), "At least some Yunwu requests must succeed")
	assert.Greater(t, quotaExceededCount, int64(0), "Parallel requests exceeding monetary quota must be rejected with ErrQuotaExceeded")

	// Verify total recorded microUSD usage does not exceed 500,000 microUSD
	used, _ := store.Redis().Get(ctx, fmt.Sprintf("ash:quota_used_micro_usd:yunwu-conc-user:yunwu:%s", now.Format("2006-01"))).Int64()
	assert.LessOrEqual(t, used, int64(500000), "Total billed Yunwu spend must respect 500,000 microUSD limit")
}

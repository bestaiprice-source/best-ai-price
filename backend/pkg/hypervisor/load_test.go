package hypervisor_test

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/quota"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/runtime"
	"github.com/stretchr/testify/require"
)

func TestHighConcurrencyLoad(t *testing.T) {
	store := newTestStorage(t)
	ctx := context.Background()

	// 1. Setup 10 mock execution nodes in DB
	for i := 1; i <= 10; i++ {
		_, err := store.DB().Account.Create().
			SetName(fmt.Sprintf("LoadNode-%d", i)).
			SetPlatform("anthropic").
			SetType("oauth").
			SetCredentials(map[string]any{"access_token": fmt.Sprintf("sk-test-%d", i)}).
			SetStatus("active").
			Save(ctx)
		require.NoError(t, err)

		nodeID := fmt.Sprintf("%d", i)
		_ = store.Redis().SAdd(ctx, "ash:nodes", nodeID).Err()
		_ = store.Redis().HSet(ctx, "ash:node:"+nodeID, map[string]any{
			"NodeID":      nodeID,
			"AccountName": fmt.Sprintf("LoadNode-%d", i),
			"Provider":    "anthropic",
			"State":       "healthy",
		}).Err()
	}

	// 2. Instantiate Hypervisor with MockRuntime
	mockRt := runtime.NewMockRuntime(store)
	hyp := hypervisor.NewHypervisor(store, mockRt)

	qm := quota.NewQuotaManager(store)
	for i := 0; i < 100; i++ {
		uid := fmt.Sprintf("user-%d", i)
		_ = qm.RegisterActiveUser(ctx, uid, "standard", 1)
		_ = qm.SaveUserAccess(ctx, &quota.UserAccess{
			OwnerID:      uid,
			IsActive:     true,
			Priority:     1,
			MonthlyLimit: 0,
		}, "standard")
	}

	// 3. Launch 100 concurrent workers
	concurrency := 100
	var wg sync.WaitGroup
	wg.Add(concurrency)

	start := time.Now()

	for i := 0; i < concurrency; i++ {
		workerID := i
		go func() {
			defer wg.Done()
			ownerID := fmt.Sprintf("user-%d", workerID%100)
			convID := fmt.Sprintf("conv-%d", workerID)
			payload := []byte(`{"model":"claude-3-5-sonnet-20241022","messages":[{"role":"user","content":"hi"}]}`)

			var writer bytes.Buffer
			res, err := hyp.HandleRequest(context.Background(), ownerID, convID, payload, &writer, "anthropic")
			require.NoError(t, err)
			require.NotNil(t, res)
		}()
	}

	wg.Wait()
	elapsed := time.Since(start)

	t.Logf("Completed %d concurrent requests in %v (avg %.2f ms/req)", concurrency, elapsed, float64(elapsed.Milliseconds())/float64(concurrency))

	// Verify SLA: Hypervisor overhead under high concurrency should complete rapidly
	require.Less(t, elapsed.Milliseconds(), int64(10000))

	// Verify events were logged
	eventsCount, err := store.DB().AshEvent.Query().Count(ctx)
	require.NoError(t, err)
	require.Greater(t, eventsCount, 0)
}

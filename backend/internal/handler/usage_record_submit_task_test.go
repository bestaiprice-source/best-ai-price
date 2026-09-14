package handler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func newUsageRecordTestPool(t *testing.T) *service.UsageRecordWorkerPool {
	t.Helper()
	pool := service.NewUsageRecordWorkerPoolWithOptions(service.UsageRecordWorkerPoolOptions{
		WorkerCount:           1,
		QueueSize:             8,
		TaskTimeout:           time.Second,
		OverflowPolicy:        "drop",
		OverflowSamplePercent: 0,
		AutoScaleEnabled:      false,
	})
	t.Cleanup(pool.Stop)
	return pool
}

func TestGatewayHandlerSubmitUsageRecordTask_WithPool(t *testing.T) {
	pool := newUsageRecordTestPool(t)
	h := &GatewayHandler{usageRecordWorkerPool: pool}

	done := make(chan struct{})
	h.submitUsageRecordTask(func(ctx context.Context) {
		close(done)
	})

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("task not executed")
	}
}

func TestGatewayHandlerSubmitUsageRecordTask_WithoutPoolSyncFallback(t *testing.T) {
	h := &GatewayHandler{}
	var called atomic.Bool

	h.submitUsageRecordTask(func(ctx context.Context) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("expected deadline in fallback context")
		}
		called.Store(true)
	})

	require.True(t, called.Load())
}

func TestGatewayHandlerSubmitUsageRecordTask_NilTask(t *testing.T) {
	h := &GatewayHandler{}
	require.NotPanics(t, func() {
		h.submitUsageRecordTask(nil)
	})
}

func TestGatewayHandlerSubmitUsageRecordTask_WithoutPool_TaskPanicRecovered(t *testing.T) {
	h := &GatewayHandler{}
	var called atomic.Bool

	require.NotPanics(t, func() {
		h.submitUsageRecordTask(func(ctx context.Context) {
			panic("usage task panic")
		})
	})

	h.submitUsageRecordTask(func(ctx context.Context) {
		called.Store(true)
	})
	require.True(t, called.Load(), "panic 后后续任务应仍可执行")
}

func TestOpenAIGatewayHandlerSubmitUsageRecordTask_WithPool(t *testing.T) {
	pool := newUsageRecordTestPool(t)
	h := &OpenAIGatewayHandler{usageRecordWorkerPool: pool}

	done := make(chan struct{})
	h.submitUsageRecordTask(func(ctx context.Context) {
		close(done)
	})

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("task not executed")
	}
}

func TestOpenAIGatewayHandlerSubmitUsageRecordTask_WithoutPoolSyncFallback(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	var called atomic.Bool

	h.submitUsageRecordTask(func(ctx context.Context) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("expected deadline in fallback context")
		}
		called.Store(true)
	})

	require.True(t, called.Load())
}

func TestOpenAIGatewayHandlerSubmitUsageRecordTask_NilTask(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	require.NotPanics(t, func() {
		h.submitUsageRecordTask(nil)
	})
}

func TestOpenAIGatewayHandlerSubmitUsageRecordTask_WithoutPool_TaskPanicRecovered(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	var called atomic.Bool

	require.NotPanics(t, func() {
		h.submitUsageRecordTask(func(ctx context.Context) {
			panic("usage task panic")
		})
	})

	h.submitUsageRecordTask(func(ctx context.Context) {
		called.Store(true)
	})
	require.True(t, called.Load(), "panic 后后续任务应仍可执行")
}

func TestSoraGatewayHandlerSubmitUsageRecordTask_WithPool(t *testing.T) {
	pool := newUsageRecordTestPool(t)
	h := &SoraGatewayHandler{usageRecordWorkerPool: pool}

	done := make(chan struct{})
	h.submitUsageRecordTask(func(ctx context.Context) {
		close(done)
	})

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("task not executed")
	}
}

func TestSoraGatewayHandlerSubmitUsageRecordTask_WithoutPoolSyncFallback(t *testing.T) {
	h := &SoraGatewayHandler{}
	var called atomic.Bool

	h.submitUsageRecordTask(func(ctx context.Context) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("expected deadline in fallback context")
		}
		called.Store(true)
	})

	require.True(t, called.Load())
}

func TestSoraGatewayHandlerSubmitUsageRecordTask_NilTask(t *testing.T) {
	h := &SoraGatewayHandler{}
	require.NotPanics(t, func() {
		h.submitUsageRecordTask(nil)
	})
}

func TestSoraGatewayHandlerSubmitUsageRecordTask_WithoutPool_TaskPanicRecovered(t *testing.T) {
	h := &SoraGatewayHandler{}
	var called atomic.Bool

	require.NotPanics(t, func() {
		h.submitUsageRecordTask(func(ctx context.Context) {
			panic("usage task panic")
		})
	})

	h.submitUsageRecordTask(func(ctx context.Context) {
		called.Store(true)
	})
	require.True(t, called.Load(), "panic 后后续任务应仍可执行")
}

func TestGatewayHandlerSubmitUsageRecordTask_PoolFullGuaranteesExecution(t *testing.T) {
	pool := service.NewUsageRecordWorkerPoolWithOptions(service.UsageRecordWorkerPoolOptions{
		WorkerCount:      1,
		QueueSize:        1,
		TaskTimeout:      time.Second,
		OverflowPolicy:   "drop",
		AutoScaleEnabled: false,
	})
	defer pool.Stop()

	// Block worker
	workerBlocked := make(chan struct{})
	releaseWorker := make(chan struct{})
	require.Equal(t, service.UsageRecordSubmitModeEnqueued, pool.Submit(func(ctx context.Context) {
		close(workerBlocked)
		<-releaseWorker
	}))
	<-workerBlocked

	// Fill queue
	require.Equal(t, service.UsageRecordSubmitModeEnqueued, pool.Submit(func(ctx context.Context) {}))

	// Handler submit with full pool must execute synchronously, never drop
	h := &GatewayHandler{usageRecordWorkerPool: pool}
	var executed atomic.Bool
	h.submitUsageRecordTask(func(ctx context.Context) {
		executed.Store(true)
	})
	require.True(t, executed.Load(), "GatewayHandler must execute task even when pool queue is full")

	close(releaseWorker)
}

func TestOpenAIGatewayHandlerSubmitUsageRecordTask_PoolFullGuaranteesExecution(t *testing.T) {
	pool := service.NewUsageRecordWorkerPoolWithOptions(service.UsageRecordWorkerPoolOptions{
		WorkerCount:      1,
		QueueSize:        1,
		TaskTimeout:      time.Second,
		OverflowPolicy:   "drop",
		AutoScaleEnabled: false,
	})
	defer pool.Stop()

	workerBlocked := make(chan struct{})
	releaseWorker := make(chan struct{})
	require.Equal(t, service.UsageRecordSubmitModeEnqueued, pool.Submit(func(ctx context.Context) {
		close(workerBlocked)
		<-releaseWorker
	}))
	<-workerBlocked

	require.Equal(t, service.UsageRecordSubmitModeEnqueued, pool.Submit(func(ctx context.Context) {}))

	h := &OpenAIGatewayHandler{usageRecordWorkerPool: pool}
	var executed atomic.Bool
	h.submitUsageRecordTask(func(ctx context.Context) {
		executed.Store(true)
	})
	require.True(t, executed.Load(), "OpenAIGatewayHandler must execute task even when pool queue is full")

	close(releaseWorker)
}

func TestSoraGatewayHandlerSubmitUsageRecordTask_PoolFullGuaranteesExecution(t *testing.T) {
	pool := service.NewUsageRecordWorkerPoolWithOptions(service.UsageRecordWorkerPoolOptions{
		WorkerCount:      1,
		QueueSize:        1,
		TaskTimeout:      time.Second,
		OverflowPolicy:   "drop",
		AutoScaleEnabled: false,
	})
	defer pool.Stop()

	workerBlocked := make(chan struct{})
	releaseWorker := make(chan struct{})
	require.Equal(t, service.UsageRecordSubmitModeEnqueued, pool.Submit(func(ctx context.Context) {
		close(workerBlocked)
		<-releaseWorker
	}))
	<-workerBlocked

	require.Equal(t, service.UsageRecordSubmitModeEnqueued, pool.Submit(func(ctx context.Context) {}))

	h := &SoraGatewayHandler{usageRecordWorkerPool: pool}
	var executed atomic.Bool
	h.submitUsageRecordTask(func(ctx context.Context) {
		executed.Store(true)
	})
	require.True(t, executed.Load(), "SoraGatewayHandler must execute task even when pool queue is full")

	close(releaseWorker)
}


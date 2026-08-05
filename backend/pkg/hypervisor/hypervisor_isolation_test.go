package hypervisor_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStandardEngineAccountIsolation(t *testing.T) {
	store := newTestStorage(t)
	ctx := context.Background()

	// Create a standard account (platform = anthropic)
	stdAcc, err := store.DB().Account.Create().
		SetName("standard-node-1").
		SetPlatform("anthropic").
		SetType("api").
		SetStatus("active").
		SetSchedulable(true).
		SetCredentials(map[string]any{"api_key": "sk-ant-test"}).
		Save(ctx)
	require.NoError(t, err)

	// Create a Yunwu account (platform = yunwu)
	_, err = store.DB().Account.Create().
		SetName("yunwu-node-1").
		SetPlatform("yunwu").
		SetType("api").
		SetStatus("active").
		SetSchedulable(true).
		SetCredentials(map[string]any{"api_key": "sk-yunwu-test"}).
		Save(ctx)
	require.NoError(t, err)

	rt := &runtime.MockRuntime{}
	hv := hypervisor.NewHypervisor(store, rt)

	payload := []byte(`{"model":"claude-3-5-sonnet-20241022","messages":[{"role":"user","content":"hi"}]}`)
	var buf bytes.Buffer
	meta, err := hv.HandleRequest(ctx, "std-user", "conv-1", payload, &buf, "anthropic")
	require.NoError(t, err)
	assert.Equal(t, 200, meta.StatusCode)

	// Verify standard usage was recorded for standard account (stdAcc.Name)
	usages, err := store.DB().AshUsage.Query().All(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, usages)
	require.NotNil(t, usages[0].NodeID)
	assert.Equal(t, fmt.Sprint(stdAcc.ID), *usages[0].NodeID)
}

func TestYunwuEngineAccountIsolation(t *testing.T) {
	os.Setenv("ASH_YUNWU_REQUIRE_FRESH_BALANCE", "false")
	defer os.Unsetenv("ASH_YUNWU_REQUIRE_FRESH_BALANCE")

	store := newTestStorage(t)
	ctx := context.Background()

	// Create a standard account (platform = anthropic)
	_, err := store.DB().Account.Create().
		SetName("standard-node-2").
		SetPlatform("anthropic").
		SetType("api").
		SetStatus("active").
		SetSchedulable(true).
		SetCredentials(map[string]any{"api_key": "sk-ant-test"}).
		Save(ctx)
	require.NoError(t, err)

	// Create a Yunwu account (platform = yunwu)
	ywAcc, err := store.DB().Account.Create().
		SetName("yunwu-node-2").
		SetPlatform("yunwu").
		SetType("api").
		SetStatus("active").
		SetSchedulable(true).
		SetCredentials(map[string]any{"api_key": "sk-yunwu-test"}).
		Save(ctx)
	require.NoError(t, err)

	rt := &runtime.MockRuntime{}
	hv := hypervisor.NewHypervisor(store, rt)

	payload := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`)
	var buf bytes.Buffer
	meta, err := hv.HandleRequest(ctx, "yw-user", "conv-2", payload, &buf, "yunwu")
	require.NoError(t, err)
	assert.Equal(t, 200, meta.StatusCode)

	// Verify Yunwu usage was recorded specifically for Yunwu account
	usages, err := store.DB().AshUsage.Query().All(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, usages)
	require.NotNil(t, usages[0].NodeID)
	assert.Equal(t, fmt.Sprint(ywAcc.ID), *usages[0].NodeID)
}

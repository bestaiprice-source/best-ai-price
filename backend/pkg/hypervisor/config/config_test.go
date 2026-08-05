package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigLoadFromEnvDefaults(t *testing.T) {
	os.Unsetenv("ASH_ENABLED")
	os.Unsetenv("ASH_MAX_RETRY")
	os.Unsetenv("ASH_NODE_COOLDOWN_SECONDS")

	cfg := LoadFromEnv()
	assert.True(t, cfg.Enabled)
	assert.True(t, cfg.EnableRebinding)
	assert.True(t, cfg.EnableAffinity)
	assert.Equal(t, 1, cfg.MaxRetry)
	assert.Equal(t, 900, cfg.NodeCooldownSeconds)
	assert.True(t, cfg.YunwuRequireFreshBalance)
	assert.Equal(t, int64(1_000_000_000), cfg.YunwuFallbackCapacityMicroUSD)
	assert.Equal(t, 20, cfg.PoolReservePercent)
	assert.Equal(t, 50, cfg.MaxOwnerPoolSharePercent)
}

func TestConfigLoadFromEnvOverrides(t *testing.T) {
	os.Setenv("ASH_ENABLED", "false")
	os.Setenv("ASH_MAX_RETRY", "3")
	os.Setenv("ASH_NODE_COOLDOWN_SECONDS", "300")
	defer func() {
		os.Unsetenv("ASH_ENABLED")
		os.Unsetenv("ASH_MAX_RETRY")
		os.Unsetenv("ASH_NODE_COOLDOWN_SECONDS")
	}()

	cfg := LoadFromEnv()
	assert.False(t, cfg.Enabled)
	assert.Equal(t, 1, cfg.MaxRetry)
	assert.Equal(t, 300, cfg.NodeCooldownSeconds)
}

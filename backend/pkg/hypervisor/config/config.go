package config

import (
	"os"
	"strconv"
)

type HypervisorConfig struct {
	Enabled                bool `mapstructure:"enabled"`
	EnableRebinding        bool `mapstructure:"enable_rebinding"`
	EnableAffinity         bool `mapstructure:"enable_affinity"`
	EnableSummary          bool `mapstructure:"enable_summary"`
	EnableHealthController bool `mapstructure:"enable_health_controller"`
	EnableQuotaManager     bool `mapstructure:"enable_quota_manager"`
	EnableMockRuntime      bool `mapstructure:"enable_mock_runtime"`
	MaxRetry               int  `mapstructure:"max_retry"`
	NodeCooldownSeconds    int  `mapstructure:"node_cooldown_seconds"`
}

// LoadFromEnv loads Hypervisor config from environment variables as overrides/fallbacks.
func LoadFromEnv() *HypervisorConfig {
	cfg := &HypervisorConfig{
		Enabled:                getEnvBool("ASH_ENABLED", true),
		EnableRebinding:        getEnvBool("ASH_ENABLE_REBINDING", true),
		EnableAffinity:         getEnvBool("ASH_ENABLE_AFFINITY", true),
		EnableSummary:          getEnvBool("ASH_ENABLE_SUMMARY", true),
		EnableHealthController: getEnvBool("ASH_ENABLE_HEALTH_CONTROLLER", true),
		EnableQuotaManager:     getEnvBool("ASH_ENABLE_QUOTA_MANAGER", true),
		EnableMockRuntime:      getEnvBool("ASH_ENABLE_MOCK_RUNTIME", true),
		MaxRetry:               getEnvInt("ASH_MAX_RETRY", 1),
		NodeCooldownSeconds:    getEnvInt("ASH_NODE_COOLDOWN_SECONDS", 900),
	}
	return cfg
}

func getEnvBool(key string, defaultVal bool) bool {
	val, exists := os.LookupEnv(key)
	if !exists {
		return defaultVal
	}
	boolVal, err := strconv.ParseBool(val)
	if err != nil {
		return defaultVal
	}
	return boolVal
}

func getEnvInt(key string, defaultVal int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	intVal, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return intVal
}

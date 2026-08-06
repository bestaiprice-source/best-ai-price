package config

import (
	"os"
	"strconv"
)

type HypervisorConfig struct {
	Enabled                       bool `mapstructure:"enabled"`
	EnableRebinding               bool `mapstructure:"enable_rebinding"`
	EnableAffinity                bool `mapstructure:"enable_affinity"`
	EnableSummary                 bool `mapstructure:"enable_summary"`
	EnableHealthController        bool `mapstructure:"enable_health_controller"`
	EnableQuotaManager            bool `mapstructure:"enable_quota_manager"`
	EnableUsageRecorder           bool `mapstructure:"enable_usage_recorder"`
	EnableBackup                  bool `mapstructure:"enable_backup"`
	EnableMockRuntime             bool `mapstructure:"enable_mock_runtime"`
	DisableSurvivalMode           bool `mapstructure:"disable_survival_mode"`
	MaxRetry                      int  `mapstructure:"max_retry"`
	NodeCooldownSeconds           int  `mapstructure:"node_cooldown_seconds"`
	SessionLockTTLSeconds         int  `mapstructure:"session_lock_ttl_seconds"`
	YunwuDefaultInputUSDPer1K     float64
	YunwuDefaultOutputUSDPer1K    float64
	YunwuBalanceSyncSeconds       int
	YunwuBalanceStaleSeconds      int
	YunwuBalanceReservePercent    int
	YunwuRequireFreshBalance      bool
	YunwuFallbackCapacityMicroUSD int64
	ActiveUserWindowSeconds       int
	PoolReservePercent            int
	MaxOwnerPoolSharePercent      int
}

// LoadFromEnv loads Hypervisor config from environment variables as overrides/fallbacks.
func LoadFromEnv() *HypervisorConfig {
	cfg := &HypervisorConfig{
		Enabled:                       getEnvBool("ASH_ENABLED", true),
		EnableRebinding:               getEnvBool("ASH_ENABLE_REBINDING", true),
		EnableAffinity:                getEnvBool("ASH_ENABLE_AFFINITY", true),
		EnableSummary:                 getEnvBool("ASH_ENABLE_SUMMARY", true),
		EnableHealthController:        getEnvBool("ASH_ENABLE_HEALTH_CONTROLLER", true),
		EnableQuotaManager:            getEnvBool("ASH_ENABLE_QUOTA_MANAGER", true),
		EnableUsageRecorder:           getEnvBool("ASH_ENABLE_USAGE_RECORDER", true),
		EnableBackup:                  getEnvBool("ASH_ENABLE_BACKUP", true),
		EnableMockRuntime:             getEnvBool("ASH_ENABLE_MOCK_RUNTIME", true),
		DisableSurvivalMode:           getEnvBool("ASH_DISABLE_SURVIVAL_MODE", false),
		MaxRetry:                      getEnvInt("ASH_MAX_RETRY", 1),
		NodeCooldownSeconds:           getEnvInt("ASH_NODE_COOLDOWN_SECONDS", 900),
		SessionLockTTLSeconds:         getEnvInt("ASH_SESSION_LOCK_TTL_SECONDS", 120),
		YunwuDefaultInputUSDPer1K:     getEnvFloat("ASH_YUNWU_DEFAULT_INPUT_USD_PER_1K", 0.002),
		YunwuDefaultOutputUSDPer1K:    getEnvFloat("ASH_YUNWU_DEFAULT_OUTPUT_USD_PER_1K", 0.002),
		YunwuBalanceSyncSeconds:       getEnvInt("ASH_YUNWU_BALANCE_SYNC_SECONDS", 300),
		YunwuBalanceStaleSeconds:      getEnvInt("ASH_YUNWU_BALANCE_STALE_SECONDS", 1800),
		YunwuBalanceReservePercent:    getEnvInt("ASH_YUNWU_BALANCE_RESERVE_PERCENT", 10),
		YunwuRequireFreshBalance:      getEnvBool("ASH_YUNWU_REQUIRE_FRESH_BALANCE", false),
		YunwuFallbackCapacityMicroUSD: getEnvInt64("ASH_YUNWU_FALLBACK_CAPACITY_MICRO_USD", 1_000_000_000),
		ActiveUserWindowSeconds:       getEnvInt("ASH_ACTIVE_USER_WINDOW_SECONDS", 86400),
		PoolReservePercent:            getEnvInt("ASH_POOL_RESERVE_PERCENT", 20),
		MaxOwnerPoolSharePercent:      getEnvInt("ASH_MAX_OWNER_POOL_SHARE_PERCENT", 50),
	}
	// Retrying more than once is unsafe for non-idempotent upstream requests.
	if cfg.MaxRetry < 0 {
		cfg.MaxRetry = 0
	} else if cfg.MaxRetry > 1 {
		cfg.MaxRetry = 1
	}
	if cfg.NodeCooldownSeconds <= 0 {
		cfg.NodeCooldownSeconds = 900
	}
	if cfg.SessionLockTTLSeconds <= 0 {
		cfg.SessionLockTTLSeconds = 120
	}
	if cfg.YunwuBalanceSyncSeconds <= 0 {
		cfg.YunwuBalanceSyncSeconds = 300
	}
	if cfg.YunwuBalanceStaleSeconds <= 0 {
		cfg.YunwuBalanceStaleSeconds = 1800
	}
	if cfg.YunwuBalanceReservePercent < 0 || cfg.YunwuBalanceReservePercent >= 100 {
		cfg.YunwuBalanceReservePercent = 10
	}
	if cfg.YunwuFallbackCapacityMicroUSD <= 0 {
		cfg.YunwuFallbackCapacityMicroUSD = 1_000_000_000
	}
	if cfg.ActiveUserWindowSeconds <= 0 {
		cfg.ActiveUserWindowSeconds = 86400
	}
	if cfg.PoolReservePercent < 0 || cfg.PoolReservePercent >= 100 {
		cfg.PoolReservePercent = 20
	}
	if cfg.MaxOwnerPoolSharePercent <= 0 || cfg.MaxOwnerPoolSharePercent > 100 {
		cfg.MaxOwnerPoolSharePercent = 50
	}
	return cfg
}

func getEnvFloat(key string, defaultVal float64) float64 {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	parsed, err := strconv.ParseFloat(val, 64)
	if err != nil || parsed < 0 {
		return defaultVal
	}
	return parsed
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

func getEnvInt64(key string, defaultVal int64) int64 {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	parsed, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return defaultVal
	}
	return parsed
}

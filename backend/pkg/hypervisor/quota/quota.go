package quota

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/account"
	"github.com/Wei-Shaw/sub2api/ent/ashusage"
	"github.com/Wei-Shaw/sub2api/ent/ashuserpolicy"
	hyperconfig "github.com/Wei-Shaw/sub2api/pkg/hypervisor/config"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
	"github.com/redis/go-redis/v9"
)

var (
	ErrQuotaExceeded      = errors.New("user quota exceeded")
	ErrUserInactive       = errors.New("user is inactive")
	ErrDailyBurstExceeded = errors.New("daily burst limit exceeded")
)

const (
	NodeCapacity    int64 = 1000000 // default capacity in units/tokens per node
	ZenoDenominator int64 = 4       // Shrink factor for used pool
)

type UserAccess struct {
	OwnerID              string `json:"owner_id"`
	PlanID               string `json:"plan_id"`
	MonthlyLimit         int64  `json:"monthly_limit"`
	DailyBurst           int64  `json:"daily_burst"`
	UsedUnits            int64  `json:"used_units"`
	MonthlyLimitMicroUSD int64  `json:"monthly_limit_micro_usd"`
	DailyBurstMicroUSD   int64  `json:"daily_burst_micro_usd"`
	UsedMicroUSD         int64  `json:"used_micro_usd"`
	Priority             int    `json:"priority"`
	DepositState         string `json:"deposit_state"`
	IsActive             bool   `json:"is_active"`
}

type QuotaManager struct {
	store *storage.Storage
	cfg   *hyperconfig.HypervisorConfig
}

func NewQuotaManager(store *storage.Storage, configs ...*hyperconfig.HypervisorConfig) *QuotaManager {
	cfg := hyperconfig.LoadFromEnv()
	if len(configs) > 0 && configs[0] != nil {
		cfg = configs[0]
	}
	return &QuotaManager{store: store, cfg: cfg}
}

func UserAccessKey(ownerID string, poolType string) string {
	return fmt.Sprintf("ash:quota:%s:%s", ownerID, poolType)
}

func monthKey() string { return time.Now().UTC().Format("2006-01") }
func usageKey(ownerID, poolType string) string {
	return fmt.Sprintf("ash:quota_used:%s:%s:%s", ownerID, poolType, monthKey())
}
func moneyUsageKey(ownerID string) string {
	return fmt.Sprintf("ash:quota_used_micro_usd:%s:yunwu:%s", ownerID, monthKey())
}

func moneyReservedKey(ownerID string) string {
	return fmt.Sprintf("ash:quota_reserved_micro_usd:%s:yunwu:%s", ownerID, monthKey())
}

func moneyReservationsKey(ownerID string) string {
	return fmt.Sprintf("ash:quota_reservations_micro_usd:%s", ownerID)
}

func moneyReservationExpiryKey(ownerID string) string {
	return fmt.Sprintf("ash:quota_reservation_expiry_micro_usd:%s", ownerID)
}

// ReserveMicroUSD atomically checks monthly and daily monetary limits while
// accounting for all in-flight requests from the same user.
func (qm *QuotaManager) ReserveMicroUSD(ctx context.Context, ownerID, requestID string, amount int64, ttl time.Duration) (bool, error) {
	if amount <= 0 {
		amount = 1
	}
	access, err := qm.GetUserAccess(ctx, ownerID, "yunwu")
	if err != nil {
		return false, err
	}
	today := time.Now().UTC().Format("2006-01-02")
	dailyUsedKey := fmt.Sprintf("ash:daily_usage_micro_usd:%s:%s", ownerID, today)
	dailyReservedKey := fmt.Sprintf("ash:daily_reserved_micro_usd:%s:%s", ownerID, today)
	const script = `
local expired = redis.call('ZRANGEBYSCORE', KEYS[6], '-inf', ARGV[5])
for _, request_id in ipairs(expired) do
  local expired_amount = tonumber(redis.call('HGET', KEYS[5], request_id) or '0')
  local expired_date = redis.call('HGET', KEYS[5], request_id .. ':date')
  local expired_month = redis.call('HGET', KEYS[5], request_id .. ':month')
  if expired_amount > 0 then
    if expired_month then
      local monthly_key = ARGV[9] .. expired_month
      local monthly = redis.call('DECRBY', monthly_key, expired_amount)
      if monthly < 0 then redis.call('SET', monthly_key, 0) end
    end
    if expired_date then
      local daily = redis.call('DECRBY', ARGV[8] .. expired_date, expired_amount)
      if daily < 0 then redis.call('SET', ARGV[8] .. expired_date, 0) end
    end
  end
  redis.call('HDEL', KEYS[5], request_id, request_id .. ':date', request_id .. ':month')
  redis.call('ZREM', KEYS[6], request_id)
end
if redis.call('HEXISTS', KEYS[5], ARGV[6]) == 1 then return 0 end
local used = tonumber(redis.call('GET', KEYS[1]) or '0')
local reserved = tonumber(redis.call('GET', KEYS[2]) or '0')
local daily_used = tonumber(redis.call('GET', KEYS[3]) or '0')
local daily_reserved = tonumber(redis.call('GET', KEYS[4]) or '0')
local amount = tonumber(ARGV[1])
local monthly_limit = tonumber(ARGV[2])
local daily_limit = tonumber(ARGV[3])
if monthly_limit > 0 and used + reserved + amount > monthly_limit then return 0 end
if daily_limit > 0 and daily_used + daily_reserved + amount > daily_limit then return 0 end
redis.call('HSET', KEYS[5], ARGV[6], amount, ARGV[6] .. ':date', ARGV[7], ARGV[6] .. ':month', ARGV[10])
redis.call('ZADD', KEYS[6], ARGV[4], ARGV[6])
redis.call('INCRBY', KEYS[2], amount)
redis.call('INCRBY', KEYS[4], amount)
redis.call('EXPIRE', KEYS[2], 2678400)
redis.call('EXPIRE', KEYS[4], 172800)
return 1`
	expiresAt := time.Now().Add(ttl).UnixMilli()
	result, err := qm.store.Redis().Eval(ctx, script, []string{moneyUsageKey(ownerID), moneyReservedKey(ownerID), dailyUsedKey, dailyReservedKey, moneyReservationsKey(ownerID), moneyReservationExpiryKey(ownerID)}, amount, access.MonthlyLimitMicroUSD, access.DailyBurstMicroUSD, expiresAt, time.Now().UnixMilli(), requestID, today, fmt.Sprintf("ash:daily_reserved_micro_usd:%s:", ownerID), fmt.Sprintf("ash:quota_reserved_micro_usd:%s:yunwu:", ownerID), monthKey()).Int()
	return result == 1, err
}

func (qm *QuotaManager) ReleaseMicroUSDReservation(ctx context.Context, ownerID, requestID string) error {
	const script = `
local amount = tonumber(redis.call('HGET', KEYS[1], ARGV[1]) or '0')
if amount <= 0 then return 0 end
local date = redis.call('HGET', KEYS[1], ARGV[1] .. ':date')
local month = redis.call('HGET', KEYS[1], ARGV[1] .. ':month')
redis.call('HDEL', KEYS[1], ARGV[1], ARGV[1] .. ':date', ARGV[1] .. ':month')
redis.call('ZREM', KEYS[2], ARGV[1])
local monthly_key = ARGV[3] .. month
local monthly = redis.call('DECRBY', monthly_key, amount)
local daily_key = ARGV[2] .. date
local daily = redis.call('DECRBY', daily_key, amount)
if monthly < 0 then redis.call('SET', monthly_key, 0) end
if daily < 0 then redis.call('SET', daily_key, 0) end
return amount`
	return qm.store.Redis().Eval(ctx, script, []string{moneyReservationsKey(ownerID), moneyReservationExpiryKey(ownerID)}, requestID, fmt.Sprintf("ash:daily_reserved_micro_usd:%s:", ownerID), fmt.Sprintf("ash:quota_reserved_micro_usd:%s:yunwu:", ownerID)).Err()
}

// ReconcileMicroUSDReservation atomically converts an in-flight hold into
// durable month/day usage so another concurrent request cannot steal the gap.
func (qm *QuotaManager) ReconcileMicroUSDReservation(ctx context.Context, ownerID, requestID string, billed int64) error {
	if billed < 0 {
		billed = 0
	}
	const script = `
local estimate = tonumber(redis.call('HGET', KEYS[1], ARGV[2]) or '0')
if estimate <= 0 then return -1 end
local date = redis.call('HGET', KEYS[1], ARGV[2] .. ':date')
local month = redis.call('HGET', KEYS[1], ARGV[2] .. ':month')
redis.call('HDEL', KEYS[1], ARGV[2], ARGV[2] .. ':date', ARGV[2] .. ':month')
redis.call('ZREM', KEYS[2], ARGV[2])
local monthly_reserved_key = ARGV[5] .. month
local monthly_reserved = redis.call('DECRBY', monthly_reserved_key, estimate)
local daily_reserved_key = ARGV[3] .. date
local daily_reserved = redis.call('DECRBY', daily_reserved_key, estimate)
if monthly_reserved < 0 then redis.call('SET', monthly_reserved_key, 0) end
if daily_reserved < 0 then redis.call('SET', daily_reserved_key, 0) end
local billed = tonumber(ARGV[1])
redis.call('INCRBY', KEYS[4], billed)
redis.call('INCRBY', ARGV[4] .. date, billed)
redis.call('INCRBY', KEYS[6], billed)
redis.call('EXPIRE', ARGV[4] .. date, 4233600)
return billed`
	result, err := qm.store.Redis().Eval(ctx, script, []string{
		moneyReservationsKey(ownerID), moneyReservationExpiryKey(ownerID), "unused", moneyUsageKey(ownerID), "unused", fmt.Sprintf("ash:yunwu:pool_used_micro_usd:%s", monthKey()),
	}, billed, requestID, fmt.Sprintf("ash:daily_reserved_micro_usd:%s:", ownerID), fmt.Sprintf("ash:daily_usage_micro_usd:%s:", ownerID), fmt.Sprintf("ash:quota_reserved_micro_usd:%s:yunwu:", ownerID)).Int64()
	if err != nil {
		return err
	}
	if result < 0 {
		return fmt.Errorf("micro-USD reservation not found")
	}
	return nil
}

func (qm *QuotaManager) GetUserAccess(ctx context.Context, ownerID string, poolType string) (*UserAccess, error) {
	usedKey := usageKey(ownerID, poolType)
	var usedUnits int64
	uVal, uErr := qm.store.Redis().Get(ctx, usedKey).Int64()
	if uErr == nil {
		usedUnits = uVal
	}

	key := UserAccessKey(ownerID, poolType)
	data, err := qm.store.Redis().Get(ctx, key).Bytes()
	if err == nil {
		var ua UserAccess
		if json.Unmarshal(data, &ua) == nil {
			if uErr == nil {
				ua.UsedUnits = usedUnits
			}
			if poolType == "yunwu" {
				if monetary, moneyErr := qm.store.Redis().Get(ctx, moneyUsageKey(ownerID)).Int64(); moneyErr == nil {
					ua.UsedMicroUSD = monetary
				}
			}
			return &ua, nil
		}
	}

	// PostgreSQL is the durable policy source; Redis only caches it.
	var policy *ent.AshUserPolicy
	if qm.store.DB() != nil {
		policy, _ = qm.store.DB().AshUserPolicy.Query().
			Where(ashuserpolicy.OwnerID(ownerID), ashuserpolicy.PoolType(poolType)).
			Only(ctx)
	}

	// Fallback to the owning user only when no explicit policy exists.
	var id int64
	isActive := true
	if _, parseErr := fmt.Sscanf(ownerID, "%d", &id); parseErr == nil {
		u, dbErr := qm.store.DB().User.Get(ctx, id)
		if dbErr == nil && u != nil {
			isActive = (u.Status == "active")
		}
	}

	if uErr != nil {
		var sum []struct {
			Cost int64 `json:"cost" xml:"cost"`
		}
		now := time.Now()
		startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

		query := qm.store.DB().AshUsage.Query().
			Where(
				ashusage.OwnerID(ownerID),
				ashusage.PoolType(poolType),
				ashusage.CreatedAtGTE(startOfMonth),
			)
		_ = query.
			Aggregate(
				ent.As(ent.Sum(ashusage.FieldCostUnits), "cost"),
			).
			Scan(ctx, &sum)

		if len(sum) > 0 {
			usedUnits = sum[0].Cost
			_ = qm.store.Redis().Set(ctx, usedKey, usedUnits, 31*24*time.Hour).Err()
		}
	}

	ua := &UserAccess{
		OwnerID:      ownerID,
		PlanID:       "pool-basic",
		MonthlyLimit: 0, // dynamic pool capacity
		UsedUnits:    usedUnits,
		Priority:     1,
		IsActive:     isActive,
	}
	if policy != nil {
		ua.PlanID = policy.PlanID
		ua.MonthlyLimit = policy.MonthlyLimit
		ua.DailyBurst = policy.DailyBurst
		ua.MonthlyLimitMicroUSD = policy.MonthlyLimitMicroUsd
		ua.DailyBurstMicroUSD = policy.DailyBurstMicroUsd
		ua.Priority = policy.Priority
		ua.DepositState = policy.DepositState
		ua.IsActive = policy.IsActive
	}
	if poolType == "yunwu" && qm.store.DB() != nil {
		var money []struct {
			Cost int64 `json:"cost"`
		}
		_ = qm.store.DB().AshUsage.Query().Where(ashusage.OwnerID(ownerID), ashusage.PoolType("yunwu"), ashusage.CreatedAtGTE(time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.Local))).Aggregate(ent.As(ent.Sum(ashusage.FieldBilledMicroUsd), "cost")).Scan(ctx, &money)
		if len(money) > 0 {
			ua.UsedMicroUSD = money[0].Cost
		}
		_ = qm.store.Redis().Set(ctx, moneyUsageKey(ownerID), ua.UsedMicroUSD, 35*24*time.Hour).Err()
	}
	// Cache in Redis for subsequent checks
	_ = qm.SaveUserAccess(ctx, ua, poolType)
	return ua, nil
}

func (qm *QuotaManager) SaveUserAccess(ctx context.Context, ua *UserAccess, poolType string) error {
	if qm.store.DB() != nil {
		if ua.Priority <= 0 {
			ua.Priority = 1
		}
		err := qm.store.DB().AshUserPolicy.Create().
			SetOwnerID(ua.OwnerID).SetPoolType(poolType).SetPlanID(ua.PlanID).
			SetMonthlyLimit(ua.MonthlyLimit).SetDailyBurst(ua.DailyBurst).
			SetMonthlyLimitMicroUsd(ua.MonthlyLimitMicroUSD).SetDailyBurstMicroUsd(ua.DailyBurstMicroUSD).
			SetPriority(ua.Priority).SetDepositState(ua.DepositState).SetIsActive(ua.IsActive).
			OnConflictColumns(ashuserpolicy.FieldOwnerID, ashuserpolicy.FieldPoolType).
			UpdateNewValues().Exec(ctx)
		if err != nil {
			return fmt.Errorf("persist user quota policy: %w", err)
		}
	}
	key := UserAccessKey(ua.OwnerID, poolType)
	data, err := json.Marshal(ua)
	if err != nil {
		return err
	}
	usedKey := usageKey(ua.OwnerID, poolType)
	_ = qm.store.Redis().Set(ctx, usedKey, ua.UsedUnits, 31*24*time.Hour).Err()
	if poolType == "yunwu" {
		_ = qm.store.Redis().Set(ctx, moneyUsageKey(ua.OwnerID), ua.UsedMicroUSD, 35*24*time.Hour).Err()
	}
	if err := qm.store.Redis().Set(ctx, key, data, 1*time.Hour).Err(); err != nil {
		return err
	}
	_ = qm.store.Redis().Del(ctx, "ash:quota_limit_cache:"+poolType, "ash:quota_limit_cache:standard:anthropic", "ash:quota_limit_cache:standard:openai").Err()
	createdMarker := fmt.Sprintf("ash:event:user_created:%s:%s", poolType, ua.OwnerID)
	if created, _ := qm.store.Redis().SetNX(ctx, createdMarker, "1", 0).Result(); created {
		_, _ = qm.store.LogEvent(ctx, ua.OwnerID, nil, nil, "UserCreated", map[string]any{
			"pool_type": poolType,
			"plan_id":   ua.PlanID,
		})
	}
	return nil
}

func (qm *QuotaManager) GetDailyUsage(ctx context.Context, ownerID string, poolType string) (int64, error) {
	today := time.Now().Format("2006-01-02")
	dailyKey := fmt.Sprintf("ash:daily_usage:%s:%s:%s", ownerID, poolType, today)
	val, err := qm.store.Redis().Get(ctx, dailyKey).Int64()
	if err == redis.Nil {
		if qm.store.DB() == nil {
			return 0, nil
		}
		start := time.Now().Truncate(24 * time.Hour)
		var sum []struct {
			Cost int64 `json:"cost"`
		}
		q := qm.store.DB().AshUsage.Query().Where(ashusage.OwnerID(ownerID), ashusage.PoolType(poolType), ashusage.CreatedAtGTE(start))
		if e := q.Aggregate(ent.As(ent.Sum(ashusage.FieldCostUnits), "cost")).Scan(ctx, &sum); e != nil {
			return 0, e
		}
		if len(sum) > 0 {
			val = sum[0].Cost
		}
		_ = qm.store.Redis().Set(ctx, dailyKey, val, 25*time.Hour).Err()
		return val, nil
	}
	return val, err
}

func (qm *QuotaManager) CheckQuota(ctx context.Context, ownerID string, poolType string) (bool, error) {
	return qm.CheckQuotaScope(ctx, ownerID, poolType)
}

// CheckQuotaScope accepts standard:anthropic, standard:openai, or yunwu.
// Standard usage remains shared while provider capacity is isolated.
func (qm *QuotaManager) CheckQuotaScope(ctx context.Context, ownerID string, scope string) (bool, error) {
	poolType := scope
	if len(scope) > len("standard:") && scope[:len("standard:")] == "standard:" {
		poolType = "standard"
	}
	ua, err := qm.GetUserAccess(ctx, ownerID, poolType)
	if err != nil {
		slog.Error("[QuotaManager] Failed to get user access", "owner_id", ownerID, "error", err)
		return false, err
	}

	if !ua.IsActive {
		slog.Warn("[QuotaManager] User is inactive", "owner_id", ownerID)
		return false, ErrUserInactive
	}

	if ua.DailyBurst > 0 {
		dailyUsed, _ := qm.GetDailyUsage(ctx, ownerID, poolType)
		if dailyUsed >= ua.DailyBurst {
			slog.Warn("[QuotaManager] Daily burst limit exceeded",
				"owner_id", ownerID,
				"daily_used", dailyUsed,
				"daily_burst_limit", ua.DailyBurst,
			)
			return false, ErrDailyBurstExceeded
		}
	}
	if poolType == "yunwu" && ua.DailyBurstMicroUSD > 0 {
		key := fmt.Sprintf("ash:daily_usage_micro_usd:%s:%s", ownerID, time.Now().UTC().Format("2006-01-02"))
		used, _ := qm.store.Redis().Get(ctx, key).Int64()
		if used >= ua.DailyBurstMicroUSD {
			return false, ErrDailyBurstExceeded
		}
	}
	if poolType == "standard" && ua.MonthlyLimit > 0 && ua.UsedUnits >= ua.MonthlyLimit {
		return false, ErrQuotaExceeded
	}
	if poolType == "yunwu" && ua.MonthlyLimitMicroUSD > 0 && ua.UsedMicroUSD >= ua.MonthlyLimitMicroUSD {
		return false, ErrQuotaExceeded
	}

	dynLimitPerWeight, err := qm.GetDynamicLimitPerWeight(ctx, scope)
	if err != nil {
		slog.Error("[QuotaManager] Failed to compute dynamic pool limit", "error", err)
		return false, err
	}

	priority := ua.Priority
	if priority <= 0 {
		priority = 1
	}

	dynLimit := dynLimitPerWeight * int64(priority)
	if dynLimit <= 0 {
		dynLimit = NodeCapacity * int64(priority)
	}

	limit, used := dynLimit, ua.UsedUnits
	if scope != poolType {
		var usageErr error
		used, usageErr = qm.store.Redis().Get(ctx, usageKey(ownerID, scope)).Int64()
		if usageErr == redis.Nil && qm.store.DB() != nil {
			provider := scope[len("standard:"):]
			var sum []struct {
				Cost int64 `json:"cost"`
			}
			start := time.Date(time.Now().UTC().Year(), time.Now().UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
			_ = qm.store.DB().AshUsage.Query().Where(ashusage.OwnerID(ownerID), ashusage.PoolType("standard"), ashusage.Provider(provider), ashusage.CreatedAtGTE(start)).Aggregate(ent.As(ent.Sum(ashusage.FieldCostUnits), "cost")).Scan(ctx, &sum)
			if len(sum) > 0 {
				used = sum[0].Cost
			}
			_ = qm.store.Redis().Set(ctx, usageKey(ownerID, scope), used, 35*24*time.Hour).Err()
		}
	}
	if poolType == "yunwu" {
		used = ua.UsedMicroUSD
	}

	warningLimit := limit
	if poolType == "standard" && ua.MonthlyLimit > 0 && ua.MonthlyLimit < warningLimit {
		warningLimit = ua.MonthlyLimit
	}
	if float64(used) >= float64(warningLimit)*0.8 && float64(used) < float64(warningLimit) {
		slog.Warn("[QuotaManager] Quota warning: user approaching quota limit",
			"owner_id", ownerID,
			"used_units", used,
			"fair_use_limit", warningLimit,
		)
		_, _ = qm.store.LogEvent(ctx, ownerID, nil, nil, "QuotaWarning", map[string]any{
			"used_units":     used,
			"fair_use_limit": warningLimit,
		})
	}

	if used >= limit {
		slog.Warn("[QuotaManager] User quota exceeded",
			"owner_id", ownerID,
			"used_units", ua.UsedUnits,
			"fair_use_limit", limit,
		)
		return false, ErrQuotaExceeded
	}

	slog.Info("[QuotaManager] Quota check passed",
		"owner_id", ownerID,
		"used_units", ua.UsedUnits,
		"fair_use_limit", limit,
		"remaining_units", limit-ua.UsedUnits,
	)
	return true, nil
}

// AcquireConcurrentSlot ensures a user does not exceed the maximum allowed concurrent requests.
// This prevents bypassing the quota system via high-concurrency bursts (TOCTOU attacks).
func (qm *QuotaManager) AcquireConcurrentSlot(ctx context.Context, ownerID string, poolType string) error {
	key := fmt.Sprintf("ash:concurrent_reqs:%s:%s", poolType, ownerID)

	// Increment active requests
	count, err := qm.store.Redis().Incr(ctx, key).Result()
	if err != nil {
		return err
	}

	// Ensure cleanup if process crashes (5 mins is longer than max request time)
	qm.store.Redis().Expire(ctx, key, 5*time.Minute)

	// Hard limit of 5 concurrent requests for Fair Use stop-loss
	if count > 5 {
		qm.store.Redis().Decr(ctx, key)
		slog.Warn("[QuotaManager] Concurrent request limit exceeded", "owner_id", ownerID, "active", count)
		return errors.New("concurrent request limit exceeded (max 5)")
	}

	return nil
}

func (qm *QuotaManager) ReleaseConcurrentSlot(ctx context.Context, ownerID string, poolType string) {
	key := fmt.Sprintf("ash:concurrent_reqs:%s:%s", poolType, ownerID)
	qm.store.Redis().Decr(ctx, key)
}

func (qm *QuotaManager) ConsumeQuota(ctx context.Context, ownerID string, poolType string, units int64) error {
	if units <= 0 {
		return nil
	}

	usedKey := usageKey(ownerID, poolType)
	newUsed, err := qm.store.Redis().IncrBy(ctx, usedKey, units).Result()
	if err != nil {
		slog.Error("[QuotaManager] Failed to increment used units", "owner_id", ownerID, "error", err)
		return err
	}

	// Increment daily usage for DailyBurst enforcement
	today := time.Now().Format("2006-01-02")
	dailyKey := fmt.Sprintf("ash:daily_usage:%s:%s:%s", ownerID, poolType, today)
	_ = qm.store.Redis().IncrBy(ctx, dailyKey, units).Err()
	qm.store.Redis().Expire(ctx, dailyKey, 25*time.Hour)

	// Update cached UserAccess struct if present
	if !(len(poolType) > len("standard:") && poolType[:len("standard:")] == "standard:") {
		ua, getErr := qm.GetUserAccess(ctx, ownerID, poolType)
		if getErr == nil && ua != nil {
			ua.UsedUnits = newUsed
			_ = qm.SaveUserAccess(ctx, ua, poolType)
		}
	}
	slog.Info("[QuotaManager] Consumed quota",
		"owner_id", ownerID,
		"pool_type", poolType,
		"deducted_tokens", units,
		"new_used_balance", newUsed,
	)
	return nil
}

func (qm *QuotaManager) ConsumeMicroUSD(ctx context.Context, ownerID string, microUSD int64) error {
	if microUSD <= 0 {
		return nil
	}
	newUsed, err := qm.store.Redis().IncrBy(ctx, moneyUsageKey(ownerID), microUSD).Result()
	if err != nil {
		return err
	}
	_ = qm.store.Redis().IncrBy(ctx, fmt.Sprintf("ash:yunwu:pool_used_micro_usd:%s", monthKey()), microUSD).Err()
	today := time.Now().UTC().Format("2006-01-02")
	key := fmt.Sprintf("ash:daily_usage_micro_usd:%s:%s", ownerID, today)
	_ = qm.store.Redis().IncrBy(ctx, key, microUSD).Err()
	_ = qm.store.Redis().Expire(ctx, key, 49*time.Hour).Err()
	if ua, getErr := qm.GetUserAccess(ctx, ownerID, "yunwu"); getErr == nil {
		ua.UsedMicroUSD = newUsed
		_ = qm.SaveUserAccess(ctx, ua, "yunwu")
	}
	return nil
}

func (qm *QuotaManager) RegisterActiveUser(ctx context.Context, ownerID string, poolType string, priority int) error {
	if priority <= 0 {
		priority = 1
	}
	key := fmt.Sprintf("ash:active_users:%s", poolType)
	pipe := qm.store.Redis().TxPipeline()
	pipe.ZAdd(ctx, key, redis.Z{
		Score:  float64(time.Now().Unix()),
		Member: ownerID,
	})
	pipe.HSet(ctx, fmt.Sprintf("ash:active_user_priority:%s", poolType), ownerID, priority)
	pipe.Del(ctx, fmt.Sprintf("ash:quota_limit_cache:%s", poolType), "ash:quota_limit_cache:standard:anthropic", "ash:quota_limit_cache:standard:openai")
	_, err := pipe.Exec(ctx)
	return err
}

func (qm *QuotaManager) RegisterNode(ctx context.Context, nodeID string) error {
	return qm.store.Redis().SAdd(ctx, "ash:nodes", nodeID).Err()
}

func (qm *QuotaManager) GetDynamicLimitPerWeight(ctx context.Context, poolType string) (int64, error) {
	cacheKey := fmt.Sprintf("ash:quota_limit_cache:%s", poolType)
	cachedVal, err := qm.store.Redis().Get(ctx, cacheKey).Result()
	if err == nil && cachedVal != "" {
		if limit, parseErr := strconv.ParseInt(cachedVal, 10, 64); parseErr == nil {
			return limit, nil
		}
	}

	activePool := poolType
	if len(poolType) > len("standard:") && poolType[:len("standard:")] == "standard:" {
		activePool = "standard"
	}
	activeUsersKey := fmt.Sprintf("ash:active_users:%s", activePool)
	window := int64(qm.cfg.ActiveUserWindowSeconds)
	cutoff := strconv.FormatInt(time.Now().Unix()-window, 10)
	expired, _ := qm.store.Redis().ZRangeByScore(ctx, activeUsersKey, &redis.ZRangeBy{Min: "-inf", Max: cutoff}).Result()
	pipe := qm.store.Redis().TxPipeline()
	pipe.ZRemRangeByScore(ctx, activeUsersKey, "-inf", cutoff)
	if len(expired) > 0 {
		pipe.HDel(ctx, fmt.Sprintf("ash:active_user_priority:%s", activePool), expired...)
	}
	_, _ = pipe.Exec(ctx)
	members, err := qm.store.Redis().ZRangeWithScores(ctx, activeUsersKey, 0, -1).Result()
	if err != nil {
		return 0, err
	}

	var totalWeights float64 = 0
	for _, m := range members {
		priority, pErr := qm.store.Redis().HGet(ctx, fmt.Sprintf("ash:active_user_priority:%s", activePool), fmt.Sprint(m.Member)).Int()
		if pErr != nil || priority <= 0 {
			priority = 1
		}
		totalWeights += float64(priority)
	}

	if totalWeights == 0 {
		totalWeights = 1 // Prevent division by zero
	}

	// Fetch all active nodes
	var nodeIDs []int64
	if qm.store.DB() != nil {
		providers := []string{"anthropic", "openai"}
		if len(poolType) > len("standard:") && poolType[:len("standard:")] == "standard:" {
			providers = []string{poolType[len("standard:"):]}
		}
		if poolType == "yunwu" {
			providers = []string{"yunwu"}
		}
		_ = qm.store.DB().Account.Query().
			Where(
				account.PlatformIn(providers...),
				account.Status("active"),
				account.Schedulable(true),
			).
			Select(account.FieldID).
			Scan(ctx, &nodeIDs)
	}

	if len(nodeIDs) == 0 && qm.cfg.EnableMockRuntime {
		strIDs, _ := qm.store.Redis().SMembers(ctx, "ash:nodes").Result()
		for _, sid := range strIDs {
			var id int64
			if _, err := fmt.Sscanf(sid, "node-%d", &id); err == nil {
				nodeIDs = append(nodeIDs, id)
			} else {
				nodeIDs = append(nodeIDs, 1)
			}
		}
	}

	var totalCapacityUnits int64
	if poolType == "yunwu" {
		keys, _ := qm.store.Redis().Keys(ctx, "ash:yunwu:balance:*").Result()
		var remaining int64
		trackedHealthyNodes := make(map[string]struct{})
		for _, key := range keys {
			var snap struct {
				AvailableMicroUSD          int64     `json:"available_micro_usd"`
				EstimatedAvailableMicroUSD int64     `json:"estimated_available_micro_usd"`
				ReserveMicroUSD            int64     `json:"reserve_micro_usd"`
				SynchronizedAt             time.Time `json:"synchronized_at"`
				Status                     string    `json:"status"`
			}
			if data, e := qm.store.Redis().Get(ctx, key).Bytes(); e == nil && json.Unmarshal(data, &snap) == nil && snap.Status == "fresh" && time.Since(snap.SynchronizedAt) <= time.Duration(qm.cfg.YunwuBalanceStaleSeconds)*time.Second {
				available := snap.EstimatedAvailableMicroUSD
				if available == 0 {
					available = snap.AvailableMicroUSD
				}
				if snap.ReserveMicroUSD > 0 {
					available -= snap.ReserveMicroUSD
				}
				if available > 0 {
					remaining += available
					trackedHealthyNodes[strings.TrimPrefix(key, "ash:yunwu:balance:")] = struct{}{}
				}
			}
		}
		localUsed, _ := qm.store.Redis().Get(ctx, fmt.Sprintf("ash:yunwu:pool_used_micro_usd:%s", monthKey())).Int64()
		totalCapacityUnits = remaining + localUsed
		if !qm.cfg.YunwuRequireFreshBalance {
			for _, id := range nodeIDs {
				if _, tracked := trackedHealthyNodes[strconv.FormatInt(id, 10)]; !tracked {
					totalCapacityUnits += qm.cfg.YunwuFallbackCapacityMicroUSD
				}
			}
		}
	}

	for _, id := range nodeIDs {
		if poolType != "yunwu" {
			totalCapacityUnits += NodeCapacity
		}
		_ = id
	}
	if totalCapacityUnits == 0 {
		if poolType == "standard" && qm.cfg.EnableMockRuntime {
			totalCapacityUnits = NodeCapacity
		} else {
			return 0, fmt.Errorf("no healthy %s nodes", providerName(poolType))
		}
	}

	limitPerWeight := totalCapacityUnits / int64(totalWeights)

	// Cache the result
	_ = qm.store.Redis().SetEx(ctx, cacheKey, limitPerWeight, 60*time.Second).Err()

	return limitPerWeight, nil
}

func providerName(scope string) string {
	if len(scope) > len("standard:") && scope[:len("standard:")] == "standard:" {
		return scope[len("standard:"):]
	}
	return scope
}

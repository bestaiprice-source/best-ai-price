package quota

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/account"
	"github.com/Wei-Shaw/sub2api/ent/ashusage"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
	"github.com/redis/go-redis/v9"
)

var (
	ErrQuotaExceeded = errors.New("user quota exceeded")
	ErrUserInactive  = errors.New("user is inactive")
)

const (
	NodeCapacity    int64 = 1000000 // default capacity in units/tokens per node
	ZenoDenominator int64 = 4       // Shrink factor for used pool
)

type UserAccess struct {
	OwnerID      string `json:"owner_id"`
	PlanID       string `json:"plan_id"`
	MonthlyLimit int64  `json:"monthly_limit"`
	DailyBurst   int64  `json:"daily_burst"`
	UsedUnits    int64  `json:"used_units"`
	Priority     int    `json:"priority"`
	DepositState string `json:"deposit_state"`
	IsActive     bool   `json:"is_active"`
}

type QuotaManager struct {
	store *storage.Storage
}

func NewQuotaManager(store *storage.Storage) *QuotaManager {
	return &QuotaManager{store: store}
}

func UserAccessKey(ownerID string, poolType string) string {
	return fmt.Sprintf("ash:quota:%s:%s", ownerID, poolType)
}

func (qm *QuotaManager) GetUserAccess(ctx context.Context, ownerID string, poolType string) (*UserAccess, error) {
	key := UserAccessKey(ownerID, poolType)
	data, err := qm.store.Redis().Get(ctx, key).Bytes()
	if err == nil {
		var ua UserAccess
		if json.Unmarshal(data, &ua) == nil {
			return &ua, nil
		}
	}

	// Fallback to database query if not found in Redis
	var id int64
	isActive := true
	if _, parseErr := fmt.Sscanf(ownerID, "%d", &id); parseErr == nil {
		u, dbErr := qm.store.DB().User.Get(ctx, id)
		if dbErr == nil && u != nil {
			isActive = (u.Status == "active")
		}
	}

	// For legacy compatibility, we don't differentiate usage history right now if it's the first time loaded.
	// But it will differentiate going forward because UsedUnits is saved to Redis per pool.
	var sum []struct {
		Cost int64 `json:"cost" xml:"cost"`
	}
	now := time.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	_ = qm.store.DB().AshUsage.Query().
		Where(
			ashusage.OwnerID(ownerID),
			ashusage.CreatedAtGTE(startOfMonth),
		).
		Aggregate(
			ent.As(ent.Sum(ashusage.FieldCostUnits), "cost"),
		).
		Scan(ctx, &sum)

	var usedUnits int64
	if len(sum) > 0 {
		// Ideally we would filter by poolType in the DB, but since usage records don't have it yet, we just start fresh for new pools
		if poolType == "yunwu" {
			usedUnits = 0 // start Yunwu from 0 for cleaner separation
		} else {
			usedUnits = sum[0].Cost
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
	// Cache in Redis for subsequent checks
	_ = qm.SaveUserAccess(ctx, ua, poolType)
	return ua, nil
}

func (qm *QuotaManager) SaveUserAccess(ctx context.Context, ua *UserAccess, poolType string) error {
	key := UserAccessKey(ua.OwnerID, poolType)
	data, err := json.Marshal(ua)
	if err != nil {
		return err
	}
	return qm.store.Redis().Set(ctx, key, data, 1*time.Hour).Err()
}

func (qm *QuotaManager) CheckQuota(ctx context.Context, ownerID string, poolType string) (bool, error) {
	ua, err := qm.GetUserAccess(ctx, ownerID, poolType)
	if err != nil {
		slog.Error("[QuotaManager] Failed to get user access", "owner_id", ownerID, "error", err)
		return false, err
	}

	if !ua.IsActive {
		slog.Warn("[QuotaManager] User is inactive", "owner_id", ownerID)
		return false, ErrUserInactive
	}

	dynLimitPerWeight, err := qm.GetDynamicLimitPerWeight(ctx, poolType)
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

	limit := dynLimit
	if ua.MonthlyLimit > 0 {
		if dynLimit > ua.MonthlyLimit {
			limit = ua.MonthlyLimit
		}
	}

	if ua.UsedUnits >= limit {
		slog.Warn("[QuotaManager] User quota exceeded",
			"owner_id", ownerID,
			"used_units", ua.UsedUnits,
			"limit", limit,
		)
		return false, ErrQuotaExceeded
	}

	slog.Debug("[QuotaManager] Quota check passed",
		"owner_id", ownerID,
		"used_units", ua.UsedUnits,
		"limit", limit,
	)
	return true, nil
}

func (qm *QuotaManager) ConsumeQuota(ctx context.Context, ownerID string, poolType string, units int64) error {
	ua, err := qm.GetUserAccess(ctx, ownerID, poolType)
	if err != nil {
		return err
	}

	ua.UsedUnits += units
	
	// Track historical global usage for Yunwu to prevent the shrinking pool double-penalty
	if poolType == "yunwu" {
		qm.store.Redis().IncrBy(ctx, "ash:yunwu:total_used", units)
	}
	
	return qm.SaveUserAccess(ctx, ua, poolType)
}

func (qm *QuotaManager) RegisterActiveUser(ctx context.Context, ownerID string, poolType string, priority int) error {
	if priority <= 0 {
		priority = 1
	}
	key := fmt.Sprintf("ash:active_users:%s", poolType)
	return qm.store.Redis().ZAdd(ctx, key, redis.Z{
		Score:  float64(priority),
		Member: ownerID,
	}).Err()
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

	// Calculate total weights of active users in this pool
	activeUsersKey := fmt.Sprintf("ash:active_users:%s", poolType)
	members, err := qm.store.Redis().ZRangeWithScores(ctx, activeUsersKey, 0, -1).Result()
	
	var totalWeights float64 = 0
	for _, m := range members {
		totalWeights += m.Score
	}

	if totalWeights == 0 {
		totalWeights = 1 // Prevent division by zero
	}

	// Fetch all active nodes
	var nodeIDs []int64
	if qm.store.DB() != nil {
		_ = qm.store.DB().Account.Query().
			Where(
				account.PlatformIn("anthropic", "openai"),
				account.Status("active"),
			).
			Select(account.FieldID).
			Scan(ctx, &nodeIDs)
	}

	if len(nodeIDs) == 0 {
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

	var totalCapacityUnits int64 = 0

	for _, id := range nodeIDs {
		// Try to fetch credit from redis (for Yunwu accounts)
		valStr, err := qm.store.Redis().Get(ctx, fmt.Sprintf("ash:yunwu:credit:%d", id)).Result()
		isYunwu := err == nil && valStr != ""

		if poolType == "yunwu" {
			if isYunwu {
				var val float64
				if _, scanErr := fmt.Sscanf(valStr, "%f", &val); scanErr == nil {
					totalCapacityUnits += int64(val * 1_000_000)
				}
			}
		} else if poolType == "standard" {
			if !isYunwu {
				totalCapacityUnits += NodeCapacity
			}
		}
	}

	// For Yunwu, add historical usage to prevent double-penalty shrinking pool
	if poolType == "yunwu" {
		totalUsed, _ := qm.store.Redis().Get(ctx, "ash:yunwu:total_used").Int64()
		totalCapacityUnits += totalUsed
	}

	if totalCapacityUnits == 0 {
		totalCapacityUnits = NodeCapacity
	}

	limitPerWeight := totalCapacityUnits / int64(totalWeights)

	// Cache the result
	_ = qm.store.Redis().SetEx(ctx, cacheKey, limitPerWeight, 60*time.Second).Err()

	return limitPerWeight, nil
}

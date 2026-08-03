package quota

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/account"
	"github.com/Wei-Shaw/sub2api/ent/ashusage"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
)

var (
	ErrQuotaExceeded = errors.New("user quota exceeded")
	ErrUserInactive  = errors.New("user is inactive")
)

const (
	NodeCapacity = 1000000 // default capacity in units/tokens per node
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

	limit := ua.MonthlyLimit
	if limit <= 0 {
		dynLimit, err := qm.GetDynamicLimit(ctx, ownerID, poolType)
		if err != nil {
			slog.Error("[QuotaManager] Failed to compute dynamic pool limit", "owner_id", ownerID, "error", err)
			return false, err
		}
		limit = dynLimit
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
	return qm.SaveUserAccess(ctx, ua, poolType)
}

func (qm *QuotaManager) RegisterActiveUser(ctx context.Context, ownerID string) error {
	return qm.store.Redis().SAdd(ctx, "ash:active_users", ownerID).Err()
}

func (qm *QuotaManager) RegisterNode(ctx context.Context, nodeID string) error {
	return qm.store.Redis().SAdd(ctx, "ash:nodes", nodeID).Err()
}

func (qm *QuotaManager) GetDynamicLimit(ctx context.Context, ownerID string, poolType string) (int64, error) {
	activeUsers, _ := qm.store.DB().User.Query().
		Where(user.Status("active")).
		Count(ctx)

	// Fetch all active nodes
	var nodeIDs []int64
	_ = qm.store.DB().Account.Query().
		Where(
			account.PlatformIn("anthropic", "openai"),
			account.Status("active"),
		).
		Select(account.FieldID).
		Scan(ctx, &nodeIDs)

	if activeUsers == 0 {
		activeUsers = 1
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

	if totalCapacityUnits == 0 {
		totalCapacityUnits = NodeCapacity
	}

	limit := totalCapacityUnits / int64(activeUsers)
	return limit, nil
}

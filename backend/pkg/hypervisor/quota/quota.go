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

func UserAccessKey(ownerID string) string {
	return fmt.Sprintf("ash:user_access:%s", ownerID)
}

func (qm *QuotaManager) GetUserAccess(ctx context.Context, ownerID string) (*UserAccess, error) {
	key := UserAccessKey(ownerID)
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

	// Sum user usage tokens for the current calendar month from ash_usage
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
		usedUnits = sum[0].Cost
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
	_ = qm.SaveUserAccess(ctx, ua)
	return ua, nil
}

func (qm *QuotaManager) SaveUserAccess(ctx context.Context, ua *UserAccess) error {
	key := UserAccessKey(ua.OwnerID)
	data, err := json.Marshal(ua)
	if err != nil {
		return err
	}
	return qm.store.Redis().Set(ctx, key, data, 1*time.Hour).Err()
}

func (qm *QuotaManager) CheckQuota(ctx context.Context, ownerID string) (bool, error) {
	ua, err := qm.GetUserAccess(ctx, ownerID)
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
		dynLimit, err := qm.GetDynamicLimit(ctx, ownerID)
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

func (qm *QuotaManager) ConsumeQuota(ctx context.Context, ownerID string, units int64) error {
	ua, err := qm.GetUserAccess(ctx, ownerID)
	if err != nil {
		slog.Error("[QuotaManager] Failed to fetch user access for consuming quota", "owner_id", ownerID, "error", err)
		return err
	}
	ua.UsedUnits += units
	slog.Debug("[QuotaManager] Consumed user quota", "owner_id", ownerID, "consumed_units", units, "total_used_units", ua.UsedUnits)
	return qm.SaveUserAccess(ctx, ua)
}

func (qm *QuotaManager) RegisterActiveUser(ctx context.Context, ownerID string) error {
	return qm.store.Redis().SAdd(ctx, "ash:active_users", ownerID).Err()
}

func (qm *QuotaManager) RegisterNode(ctx context.Context, nodeID string) error {
	return qm.store.Redis().SAdd(ctx, "ash:nodes", nodeID).Err()
}

func (qm *QuotaManager) GetDynamicLimit(ctx context.Context, ownerID string) (int64, error) {
	// Try loading counts from Postgres DB first
	activeUsers, _ := qm.store.DB().User.Query().
		Where(user.Status("active")).
		Count(ctx)
	nodes, _ := qm.store.DB().Account.Query().
		Where(
			account.PlatformIn("anthropic", "openai"),
			account.Status("active"),
		).
		Count(ctx)

	// If DB has no active users/accounts (e.g., in unit tests), fall back to Redis sets
	if activeUsers == 0 || nodes == 0 {
		redisUsers, _ := qm.store.Redis().SCard(ctx, "ash:active_users").Result()
		redisNodes, _ := qm.store.Redis().SCard(ctx, "ash:nodes").Result()
		
		if activeUsers == 0 && redisUsers > 0 {
			activeUsers = int(redisUsers)
		}
		if nodes == 0 && redisNodes > 0 {
			nodes = int(redisNodes)
		}
	}

	if activeUsers == 0 {
		activeUsers = 1
	}
	if nodes == 0 {
		nodes = 1
	}

	limit := (int64(nodes) * NodeCapacity) / int64(activeUsers)
	return limit, nil
}

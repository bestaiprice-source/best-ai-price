package quota

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

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
	if err != nil {
		return nil, err
	}
	var ua UserAccess
	if err := json.Unmarshal(data, &ua); err != nil {
		return nil, err
	}
	return &ua, nil
}

func (qm *QuotaManager) SaveUserAccess(ctx context.Context, ua *UserAccess) error {
	key := UserAccessKey(ua.OwnerID)
	data, err := json.Marshal(ua)
	if err != nil {
		return err
	}
	return qm.store.Redis().Set(ctx, key, data, 0).Err()
}

func (qm *QuotaManager) CheckQuota(ctx context.Context, ownerID string) (bool, error) {
	ua, err := qm.GetUserAccess(ctx, ownerID)
	if err != nil {
		return false, err
	}
	if !ua.IsActive {
		return false, ErrUserInactive
	}

	limit := ua.MonthlyLimit
	if limit <= 0 {
		// If no monthly limit is set explicitly, use the dynamic limit from the pool
		dynLimit, err := qm.GetDynamicLimit(ctx, ownerID)
		if err != nil {
			return false, err
		}
		limit = dynLimit
	}

	if ua.UsedUnits >= limit {
		return false, ErrQuotaExceeded
	}
	return true, nil
}

func (qm *QuotaManager) ConsumeQuota(ctx context.Context, ownerID string, units int64) error {
	ua, err := qm.GetUserAccess(ctx, ownerID)
	if err != nil {
		return err
	}
	ua.UsedUnits += units
	return qm.SaveUserAccess(ctx, ua)
}

func (qm *QuotaManager) RegisterActiveUser(ctx context.Context, ownerID string) error {
	return qm.store.Redis().SAdd(ctx, "ash:active_users", ownerID).Err()
}

func (qm *QuotaManager) RegisterNode(ctx context.Context, nodeID string) error {
	return qm.store.Redis().SAdd(ctx, "ash:nodes", nodeID).Err()
}

func (qm *QuotaManager) GetDynamicLimit(ctx context.Context, ownerID string) (int64, error) {
	activeUsers, err := qm.store.Redis().SCard(ctx, "ash:active_users").Result()
	if err != nil {
		return 0, err
	}
	if activeUsers == 0 {
		activeUsers = 1
	}
	nodes, err := qm.store.Redis().SCard(ctx, "ash:nodes").Result()
	if err != nil {
		return 0, err
	}
	if nodes == 0 {
		nodes = 1
	}

	limit := (nodes * NodeCapacity) / activeUsers
	return limit, nil
}

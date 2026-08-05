package lease

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
)

type Lease struct {
	LeaseID   string     `json:"lease_id"`
	SessionID string     `json:"session_id"`
	NodeID    string     `json:"node_id"`
	GrantedAt time.Time  `json:"granted_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type LeaseManager struct {
	store *storage.Storage
}

func NewLeaseManager(store *storage.Storage) *LeaseManager {
	return &LeaseManager{store: store}
}

func (lm *LeaseManager) GetLease(ctx context.Context, sessionID string) (*Lease, error) {
	key := storage.LeaseKey(sessionID)
	data, err := lm.store.Redis().Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}
	var l Lease
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, err
	}
	return &l, nil
}

func (lm *LeaseManager) SaveLease(ctx context.Context, l *Lease) error {
	key := storage.LeaseKey(l.SessionID)
	data, err := json.Marshal(l)
	if err != nil {
		return err
	}
	var ttl time.Duration
	if l.ExpiresAt != nil {
		ttl = time.Until(*l.ExpiresAt)
		if ttl <= 0 {
			return nil // Already expired, don't write
		}
	}
	err = lm.store.Redis().Set(ctx, key, data, ttl).Err()
	if err == nil {
		_, _ = lm.store.LogEvent(ctx, "system", &l.SessionID, &l.NodeID, "LeaseGranted", map[string]any{
			"lease_id": l.LeaseID,
		})
	}
	return err
}

func (lm *LeaseManager) DeleteLease(ctx context.Context, sessionID string) error {
	key := storage.LeaseKey(sessionID)
	err := lm.store.Redis().Del(ctx, key).Err()
	if err == nil {
		_, _ = lm.store.LogEvent(ctx, "system", &sessionID, nil, "LeaseReleased", nil)
	}
	return err
}

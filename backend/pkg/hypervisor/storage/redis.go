package storage

import (
	"context"
	"fmt"
	"time"
)

// Key generation helpers
func SessionKey(sessionID string) string {
	return fmt.Sprintf("ash:session:%s", sessionID)
}

func LeaseKey(sessionID string) string {
	return fmt.Sprintf("ash:lease:%s", sessionID)
}

func NodeKey(nodeID string) string {
	return fmt.Sprintf("ash:node:%s", nodeID)
}

func LockSessionKey(sessionID string) string {
	return fmt.Sprintf("ash:lock:session:%s", sessionID)
}

func AffinityKey(ownerID string) string {
	return fmt.Sprintf("ash:affinity:%s", ownerID)
}

func QueueKey(priority int) string {
	return fmt.Sprintf("ash:queue:%d", priority)
}

// AcquireLock attempts to acquire a Redis distributed lock for a session
func (s *Storage) AcquireLock(ctx context.Context, sessionID string, ttl time.Duration) (bool, error) {
	lockKey := LockSessionKey(sessionID)
	// Simple SET NX PX lock
	ok, err := s.redis.SetNX(ctx, lockKey, "locked", ttl).Result()
	return ok, err
}

// ReleaseLock releases the Redis lock for a session
func (s *Storage) ReleaseLock(ctx context.Context, sessionID string) error {
	lockKey := LockSessionKey(sessionID)
	return s.redis.Del(ctx, lockKey).Err()
}

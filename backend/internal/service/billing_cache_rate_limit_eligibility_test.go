//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

type rateLimitLoaderStub struct {
	data *APIKeyRateLimitData
	err  error
}

func (s *rateLimitLoaderStub) GetRateLimitData(_ context.Context, _ int64) (*APIKeyRateLimitData, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.data, nil
}

// The hypervisor (yunwu) dispatch paths call this exported check directly, so it
// must enforce the 5h/1d/7d windows and stay a no-op without them.
func TestCheckAPIKeyRateLimitEligibility(t *testing.T) {
	windowStart := time.Now()

	t.Run("nil service and key are no-ops", func(t *testing.T) {
		var nilSvc *BillingCacheService
		if err := nilSvc.CheckAPIKeyRateLimitEligibility(context.Background(), &APIKey{RateLimit5h: 1}); err != nil {
			t.Fatalf("nil service must not error, got %v", err)
		}
		if err := (&BillingCacheService{}).CheckAPIKeyRateLimitEligibility(context.Background(), nil); err != nil {
			t.Fatalf("nil key must not error, got %v", err)
		}
	})

	t.Run("key without windows is a no-op", func(t *testing.T) {
		svc := &BillingCacheService{apiKeyRateLimitLoader: &rateLimitLoaderStub{err: errors.New("must not be called")}}
		if err := svc.CheckAPIKeyRateLimitEligibility(context.Background(), &APIKey{ID: 7}); err != nil {
			t.Fatalf("unlimited key must not error, got %v", err)
		}
	})

	t.Run("exceeded 5h window is rejected", func(t *testing.T) {
		svc := &BillingCacheService{apiKeyRateLimitLoader: &rateLimitLoaderStub{data: &APIKeyRateLimitData{
			Usage5h:       0.0034,
			Window5hStart: &windowStart,
		}}}
		err := svc.CheckAPIKeyRateLimitEligibility(context.Background(), &APIKey{ID: 7, RateLimit5h: 0.001})
		if !errors.Is(err, ErrAPIKeyRateLimit5hExceeded) {
			t.Fatalf("expected ErrAPIKeyRateLimit5hExceeded, got %v", err)
		}
	})

	t.Run("usage below the window passes", func(t *testing.T) {
		svc := &BillingCacheService{apiKeyRateLimitLoader: &rateLimitLoaderStub{data: &APIKeyRateLimitData{
			Usage5h:       0.0001,
			Window5hStart: &windowStart,
		}}}
		if err := svc.CheckAPIKeyRateLimitEligibility(context.Background(), &APIKey{ID: 7, RateLimit5h: 0.001}); err != nil {
			t.Fatalf("usage below limit must pass, got %v", err)
		}
	})

	t.Run("loader failure fails open", func(t *testing.T) {
		svc := &BillingCacheService{apiKeyRateLimitLoader: &rateLimitLoaderStub{err: errors.New("db down")}}
		if err := svc.CheckAPIKeyRateLimitEligibility(context.Background(), &APIKey{ID: 7, RateLimit5h: 0.001}); err != nil {
			t.Fatalf("a DB read failure must not block traffic, got %v", err)
		}
	})
}

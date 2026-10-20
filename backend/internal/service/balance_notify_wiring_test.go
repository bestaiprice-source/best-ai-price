//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The process-wide notifier follows the SetDefault*/GetDefault* pattern and the
// post-billing hook must be a safe no-op when it is not installed.
func TestBalanceNotifyDefaultSetterAndGuards(t *testing.T) {
	SetDefaultBalanceNotifyService(nil)
	assert.Nil(t, GetDefaultBalanceNotifyService())

	// No service installed -> no panic, no work.
	notifyAfterUsageBilling(context.Background(), &User{ID: 1, Balance: 10}, 10, &CostBreakdown{ActualCost: 1}, &Account{ID: 1})

	// Installed but with nil dependencies -> still a no-op (canNotifyBalance fails).
	SetDefaultBalanceNotifyService(NewBalanceNotifyService(nil, nil, nil))
	t.Cleanup(func() { SetDefaultBalanceNotifyService(nil) })
	notifyAfterUsageBilling(context.Background(), &User{ID: 1, Balance: 10}, 10, &CostBreakdown{ActualCost: 1}, &Account{ID: 1})

	// Zero/negative cost must short-circuit before touching the service.
	notifyAfterUsageBilling(context.Background(), &User{ID: 1, Balance: 10}, 10, &CostBreakdown{ActualCost: 0}, &Account{ID: 1})
	notifyAfterUsageBilling(context.Background(), &User{ID: 1, Balance: 10}, 10, nil, nil)

	// Nil user/account must not panic.
	notifyAfterUsageBilling(context.Background(), nil, 0, &CostBreakdown{ActualCost: 0.5}, nil)
}

// The crossing detector must fire only on the downward transition.
func TestCrossedDownward(t *testing.T) {
	assert.True(t, crossedDownward(10, 9, 9.5))
	assert.False(t, crossedDownward(10, 10, 9.5), "no crossing when still at/above threshold")
	assert.False(t, crossedDownward(9, 8, 9.5), "already below the threshold -> no repeat alert")
	assert.False(t, crossedDownward(10, 9.5, 9.5), "exactly at the threshold is not below it")
}

// Threshold resolution: fixed ($) vs percentage of total recharged.
func TestResolveBalanceThreshold(t *testing.T) {
	assert.InDelta(t, 5.0, resolveBalanceThreshold(5, thresholdTypeFixed, 0), 1e-9)
	// 20% of $100 recharged = $20
	assert.InDelta(t, 20.0, resolveBalanceThreshold(20, thresholdTypePercentage, 100), 1e-9)
	// percentage with nothing recharged falls back to the absolute value
	assert.InDelta(t, 20.0, resolveBalanceThreshold(20, thresholdTypePercentage, 0), 1e-9)
}

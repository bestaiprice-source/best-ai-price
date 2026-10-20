//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// balanceNotifySettingStub returns exactly the settings it is given (missing keys
// stay absent, mirroring the real repository).
type balanceNotifySettingStub struct {
	values map[string]string
	err    error
}

func (s *balanceNotifySettingStub) Get(context.Context, string) (*Setting, error) {
	panic("unexpected Get call")
}
func (s *balanceNotifySettingStub) GetValue(context.Context, string) (string, error) {
	panic("unexpected GetValue call")
}
func (s *balanceNotifySettingStub) Set(context.Context, string, string) error {
	panic("unexpected Set call")
}
func (s *balanceNotifySettingStub) GetMultiple(context.Context, []string) (map[string]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.values, nil
}
func (s *balanceNotifySettingStub) SetMultiple(context.Context, map[string]string) error {
	panic("unexpected SetMultiple call")
}
func (s *balanceNotifySettingStub) GetAll(context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}
func (s *balanceNotifySettingStub) Delete(context.Context, string) error {
	panic("unexpected Delete call")
}

// With nothing configured the reminder is ON and the threshold is the built-in $3.
func TestBalanceLowNotifyDefaultsToThreeUSD(t *testing.T) {
	svc := NewBalanceNotifyService(nil, &balanceNotifySettingStub{values: map[string]string{}}, nil)

	enabled, threshold, _ := svc.getBalanceNotifyConfig(context.Background())
	assert.True(t, enabled, "the balance reminder is opt-out, not opt-in")
	assert.InDelta(t, DefaultBalanceLowNotifyThresholdUSD, threshold, 1e-9)
	assert.InDelta(t, 3.0, threshold, 1e-9, "the default reminder threshold is $3")

	// The effective per-user threshold must also resolve to $3 (a threshold of 0
	// would silently disable the alert).
	eff, _, ok := svc.resolveUserEffectiveThreshold(context.Background(), &User{ID: 1})
	require.True(t, ok)
	assert.InDelta(t, 3.0, eff, 1e-9)
}

// Explicit operator settings win over the defaults.
func TestBalanceLowNotifyRespectsExplicitSettings(t *testing.T) {
	svc := NewBalanceNotifyService(nil, &balanceNotifySettingStub{values: map[string]string{
		SettingKeyBalanceLowNotifyEnabled:   "false",
		SettingKeyBalanceLowNotifyThreshold: "10.5",
	}}, nil)

	enabled, threshold, _ := svc.getBalanceNotifyConfig(context.Background())
	assert.False(t, enabled, "an explicit false disables the reminder")
	assert.InDelta(t, 10.5, threshold, 1e-9)

	// A configured threshold of a different size must be honoured.
	svc2 := NewBalanceNotifyService(nil, &balanceNotifySettingStub{values: map[string]string{
		SettingKeyBalanceLowNotifyThreshold: "7",
	}}, nil)
	enabled2, threshold2, _ := svc2.getBalanceNotifyConfig(context.Background())
	assert.True(t, enabled2)
	assert.InDelta(t, 7.0, threshold2, 1e-9)

	// Garbage / non-positive thresholds fall back to the default rather than
	// disabling the alert.
	svc3 := NewBalanceNotifyService(nil, &balanceNotifySettingStub{values: map[string]string{
		SettingKeyBalanceLowNotifyThreshold: "not-a-number",
	}}, nil)
	_, threshold3, _ := svc3.getBalanceNotifyConfig(context.Background())
	assert.InDelta(t, 3.0, threshold3, 1e-9)

	svc4 := NewBalanceNotifyService(nil, &balanceNotifySettingStub{values: map[string]string{
		SettingKeyBalanceLowNotifyThreshold: "0",
	}}, nil)
	_, threshold4, _ := svc4.getBalanceNotifyConfig(context.Background())
	assert.InDelta(t, 3.0, threshold4, 1e-9)
}

// A settings read failure must keep the reminder alive (fail safe).
func TestBalanceLowNotifyFailsSafe(t *testing.T) {
	svc := NewBalanceNotifyService(nil, &balanceNotifySettingStub{err: errors.New("db down")}, nil)

	enabled, threshold, _ := svc.getBalanceNotifyConfig(context.Background())
	assert.True(t, enabled)
	assert.InDelta(t, DefaultBalanceLowNotifyThresholdUSD, threshold, 1e-9)
}

// The reminder fires exactly on the downward crossing of the threshold.
func TestBalanceLowNotifyCrossingBelowThreeDollars(t *testing.T) {
	const threshold = DefaultBalanceLowNotifyThresholdUSD
	assert.True(t, crossedDownward(3.42, 2.87, threshold), "crossing $3.42 -> $2.87 must alert")
	assert.False(t, crossedDownward(3.42, 3.01, threshold), "staying above $3 must not alert")
	assert.False(t, crossedDownward(2.87, 1.10, threshold), "already below the threshold: no repeat alert")
	assert.True(t, crossedDownward(3.00, 2.99, threshold), "exactly at the threshold counts as crossing down")
}

// A low-balance reminder must reach the account owner: their own address is the
// primary recipient, plus verified extra addresses (deduplicated, case-insensitive).
func TestBalanceLowNotifyRecipientsIncludeOwner(t *testing.T) {
	svc := NewBalanceNotifyService(nil, &balanceNotifySettingStub{values: map[string]string{}}, nil)

	user := &User{
		ID:    7,
		Email: "Owner@Example.com",
		BalanceNotifyExtraEmails: []NotifyEmailEntry{
			{Email: "owner@example.com", Verified: true},   // duplicate of the owner
			{Email: "finance@example.com", Verified: true}, // verified extra
			{Email: "notverified@example.com"},             // must be skipped
			{Email: "off@example.com", Verified: true, Disabled: true},
		},
	}

	recipients := svc.collectBalanceNotifyRecipients(user)
	assert.Equal(t, []string{"Owner@Example.com", "finance@example.com"}, recipients)

	// Without an account email only the verified extras are used...
	assert.Equal(t, []string{"finance@example.com"}, svc.collectBalanceNotifyRecipients(&User{
		BalanceNotifyExtraEmails: []NotifyEmailEntry{{Email: "finance@example.com", Verified: true}},
	}))
	// ...and a user with no usable address has no recipients at all.
	assert.Empty(t, svc.collectBalanceNotifyRecipients(&User{}))
	assert.Nil(t, svc.collectBalanceNotifyRecipients(nil))
}

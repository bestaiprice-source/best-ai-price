//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// Password reset is gated by its OWN toggle. Requiring the signup-time email
// verification switch as well made the exported setting unreachable for
// deployments whose registration flow does not send a verify code (the SSO
// portal registers with an empty verify_code), which silently disabled the
// whole forgot-password API in production.
func TestPasswordResetEnabled_DoesNotRequireEmailVerification(t *testing.T) {
	ctx := context.Background()
	repo := newMockSettingRepo()
	repo.data[SettingKeyEmailVerifyEnabled] = "false"
	repo.data[SettingKeyPasswordResetEnabled] = "true"

	svc := NewSettingService(repo, &config.Config{})

	require.True(t, svc.IsPasswordResetEnabled(ctx),
		"password_reset_enabled=true must be enough on its own")

	public, err := svc.GetPublicSettings(ctx)
	require.NoError(t, err)
	require.True(t, public.PasswordResetEnabled,
		"the public setting consumed by the reset screens must report the same state")
	require.False(t, public.EmailVerifyEnabled)
}

func TestPasswordResetEnabled_OffUnlessExplicitlyEnabled(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name             string
		emailVerify      string
		passwordResetRaw string
	}{
		{name: "both unset", emailVerify: "false", passwordResetRaw: ""},
		{name: "explicitly disabled", emailVerify: "true", passwordResetRaw: "false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockSettingRepo()
			repo.data[SettingKeyEmailVerifyEnabled] = tc.emailVerify
			if tc.passwordResetRaw != "" {
				repo.data[SettingKeyPasswordResetEnabled] = tc.passwordResetRaw
			}

			svc := NewSettingService(repo, &config.Config{})
			require.False(t, svc.IsPasswordResetEnabled(ctx))

			public, err := svc.GetPublicSettings(ctx)
			require.NoError(t, err)
			require.False(t, public.PasswordResetEnabled)
		})
	}
}

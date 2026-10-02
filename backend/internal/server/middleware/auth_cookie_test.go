//go:build unit

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/gatewayctx"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAuthCookie_SetAndClear(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	ctx := gatewayctx.NewNative(req, rec, nil, "127.0.0.1:1234")

	SetAuthCookiesContext(ctx, "test-access", "test-refresh", 3600)

	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 2)

	var hasAccess, hasRefresh bool
	for _, c := range cookies {
		if c.Name == CookieAccessTokenName {
			hasAccess = true
			require.Equal(t, "test-access", c.Value)
			require.Equal(t, 3600, c.MaxAge)
			require.True(t, c.HttpOnly)
			require.True(t, c.Secure)
			require.Equal(t, "/", c.Path)
		}
		if c.Name == CookieRefreshTokenName {
			hasRefresh = true
			require.Equal(t, "test-refresh", c.Value)
			require.Equal(t, 86400*30, c.MaxAge)
			require.True(t, c.HttpOnly)
			require.True(t, c.Secure)
			require.Equal(t, "/", c.Path)
		}
	}
	require.True(t, hasAccess)
	require.True(t, hasRefresh)

	// Test ClearAuthCookiesContext
	rec2 := httptest.NewRecorder()
	ctx2 := gatewayctx.NewNative(req, rec2, nil, "127.0.0.1:1234")
	ClearAuthCookiesContext(ctx2)

	clearCookies := rec2.Result().Cookies()
	require.Len(t, clearCookies, 2)
	for _, c := range clearCookies {
		require.Equal(t, -1, c.MaxAge)
		require.Equal(t, "", c.Value)
	}

	// Test nil context safety
	SetAuthCookiesContext(nil, "a", "b", 10)
	ClearAuthCookiesContext(nil)
}

func TestApplyAdminAuthContext_CookieAuth(t *testing.T) {
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "test-secret", ExpireHour: 1}}
	authService := service.NewAuthService(nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, nil)

	admin := &service.User{
		ID:           1,
		Email:        "admin@example.com",
		Role:         service.RoleAdmin,
		Status:       service.StatusActive,
		TokenVersion: 1,
		Concurrency:  1,
	}

	userRepo := &stubUserRepo{
		getByID: func(ctx context.Context, id int64) (*service.User, error) {
			if id != admin.ID {
				return nil, service.ErrUserNotFound
			}
			clone := *admin
			return &clone, nil
		},
	}
	userService := service.NewUserService(userRepo, nil, nil)

	token, err := authService.GenerateToken(admin)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.AddCookie(&http.Cookie{
		Name:  CookieAccessTokenName,
		Value: token,
	})
	rec := httptest.NewRecorder()
	ctx := gatewayctx.NewNative(req, rec, nil, "127.0.0.1:1234")

	ok := ApplyAdminAuthContext(authService, userService, nil, ctx)
	require.True(t, ok)

	val, exists := ctx.Value(string(ContextKeyUserRole))
	require.True(t, exists)
	require.Equal(t, service.RoleAdmin, val)
}

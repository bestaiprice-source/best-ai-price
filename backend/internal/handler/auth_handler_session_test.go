//go:build unit

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/gatewayctx"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAuthHandler_GetSession_WithCookie(t *testing.T) {
	harness := newAuthHandlerBackendHarness(t)
	admin := harness.users["admin"]

	token, err := harness.handler.authService.GenerateToken(admin)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	req.AddCookie(&http.Cookie{
		Name:  middleware2.CookieAccessTokenName,
		Value: token,
	})
	rec := httptest.NewRecorder()
	ctx := gatewayctx.NewNative(req, rec, nil, "127.0.0.1:1234")

	harness.handler.GetSessionGateway(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Code int             `json:"code"`
		Data SessionResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, token, resp.Data.AccessToken)
	require.Equal(t, admin.ID, resp.Data.User.ID)
	require.Equal(t, admin.Email, resp.Data.User.Email)
}

func TestAuthHandler_GetSession_WithAuthHeader(t *testing.T) {
	harness := newAuthHandlerBackendHarness(t)
	admin := harness.users["admin"]

	token, err := harness.handler.authService.GenerateToken(admin)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	ctx := gatewayctx.NewNative(req, rec, nil, "127.0.0.1:1234")

	harness.handler.GetSessionGateway(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Code int             `json:"code"`
		Data SessionResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, token, resp.Data.AccessToken)
	require.Equal(t, admin.ID, resp.Data.User.ID)
}

func TestAuthHandler_GetSession_GinContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	harness := newAuthHandlerBackendHarness(t)
	admin := harness.users["admin"]

	token, err := harness.handler.authService.GenerateToken(admin)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	c.Request = req

	harness.handler.GetSession(c)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestAuthHandler_GetSession_SilentRefresh(t *testing.T) {
	harness := newAuthHandlerBackendHarness(t)
	admin := harness.users["admin"]

	// Generate a valid token pair to get a real refresh token
	tokenPair, err := harness.handler.authService.GenerateTokenPair(context.Background(), admin, "")
	if err == nil {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
		req.AddCookie(&http.Cookie{
			Name:  middleware2.CookieRefreshTokenName,
			Value: tokenPair.RefreshToken,
		})
		rec := httptest.NewRecorder()
		ctx := gatewayctx.NewNative(req, rec, nil, "127.0.0.1:1234")

		harness.handler.GetSessionGateway(ctx)
		require.Equal(t, http.StatusOK, rec.Code)
	}
}

func TestAuthHandler_GetSession_Unauthorized(t *testing.T) {
	harness := newAuthHandlerBackendHarness(t)

	// No token/cookie
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	rec := httptest.NewRecorder()
	ctx := gatewayctx.NewNative(req, rec, nil, "127.0.0.1:1234")

	harness.handler.GetSessionGateway(ctx)

	require.Equal(t, http.StatusUnauthorized, rec.Code)

	// Invalid token
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	req2.Header.Set("Authorization", "Bearer invalid.token.value")
	rec2 := httptest.NewRecorder()
	ctx2 := gatewayctx.NewNative(req2, rec2, nil, "127.0.0.1:1234")

	harness.handler.GetSessionGateway(ctx2)

	require.Equal(t, http.StatusUnauthorized, rec2.Code)
}

func TestAuthHandler_Logout_ClearsCookies(t *testing.T) {
	harness := newAuthHandlerBackendHarness(t)

	body, _ := json.Marshal(LogoutRequest{RefreshToken: "test-refresh-token"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	ctx := gatewayctx.NewNative(req, rec, nil, "127.0.0.1:1234")

	harness.handler.LogoutGateway(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	cookies := rec.Result().Cookies()
	require.NotEmpty(t, cookies)
	var foundAccess, foundRefresh bool
	for _, c := range cookies {
		if c.Name == middleware2.CookieAccessTokenName {
			foundAccess = true
			require.Equal(t, -1, c.MaxAge)
		}
		if c.Name == middleware2.CookieRefreshTokenName {
			foundRefresh = true
			require.Equal(t, -1, c.MaxAge)
		}
	}
	require.True(t, foundAccess, "access token cookie should be cleared")
	require.True(t, foundRefresh, "refresh token cookie should be cleared")
}

package middleware

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/server/gatewayctx"
)

const (
	// CookieAccessTokenName matches api-center session cookie
	CookieAccessTokenName = "apicenter-access-token"
	// CookieRefreshTokenName matches api-center refresh cookie
	CookieRefreshTokenName = "apicenter-refresh-token"
)

// ExtractJWTTokenFromGatewayContext extracts JWT token from Authorization header (Bearer)
// or falls back to session cookies (apicenter-access-token, access_token).
func ExtractJWTTokenFromGatewayContext(c gatewayctx.GatewayContext) string {
	if c == nil {
		return ""
	}
	// 1. Authorization header: Bearer <token>
	authHeader := c.HeaderValue("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			token := strings.TrimSpace(parts[1])
			if token != "" {
				return token
			}
		}
	}
	// 2. Cookie: apicenter-access-token (or fallback access_token)
	if cookieVal, err := c.CookieValue(CookieAccessTokenName); err == nil && strings.TrimSpace(cookieVal) != "" {
		return strings.TrimSpace(cookieVal)
	}
	if cookieVal, err := c.CookieValue("access_token"); err == nil && strings.TrimSpace(cookieVal) != "" {
		return strings.TrimSpace(cookieVal)
	}
	return ""
}

// SetAuthCookiesContext sets session cookies for single sign-on across apicenter and sub2api.
func SetAuthCookiesContext(c gatewayctx.GatewayContext, accessToken, refreshToken string, expiresInSeconds int) {
	if c == nil {
		return
	}
	isSecure := false
	if req := c.Request(); req != nil {
		if req.TLS != nil || strings.EqualFold(req.Header.Get("X-Forwarded-Proto"), "https") {
			isSecure = true
		}
	}

	if accessToken != "" {
		maxAge := expiresInSeconds
		if maxAge <= 0 {
			maxAge = 86400 * 7
		}
		c.SetCookie(&http.Cookie{
			Name:     CookieAccessTokenName,
			Value:    accessToken,
			Path:     "/",
			MaxAge:   maxAge,
			HttpOnly: true,
			Secure:   isSecure,
			SameSite: http.SameSiteLaxMode,
		})
	}

	if refreshToken != "" {
		maxAge := 86400 * 30 // 30 days
		c.SetCookie(&http.Cookie{
			Name:     CookieRefreshTokenName,
			Value:    refreshToken,
			Path:     "/",
			MaxAge:   maxAge,
			HttpOnly: true,
			Secure:   isSecure,
			SameSite: http.SameSiteLaxMode,
		})
	}
}

// ClearAuthCookiesContext clears authentication cookies on logout.
func ClearAuthCookiesContext(c gatewayctx.GatewayContext) {
	if c == nil {
		return
	}
	isSecure := false
	if req := c.Request(); req != nil {
		if req.TLS != nil || strings.EqualFold(req.Header.Get("X-Forwarded-Proto"), "https") {
			isSecure = true
		}
	}
	c.SetCookie(&http.Cookie{
		Name:     CookieAccessTokenName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
	})
	c.SetCookie(&http.Cookie{
		Name:     CookieRefreshTokenName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

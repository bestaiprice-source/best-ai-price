//go:build unit

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/gatewayctx"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// The model catalog is metadata, not a billed request: a client with a valid key
// and an empty balance must still be able to list models. Enforcing the balance
// gate here made OpenAI-/Gemini-compatible clients report "invalid key" on the
// very first request they send.
func TestBillingExemptPaths(t *testing.T) {
	free := []string{
		"/v1/usage",
		"/v1/models",
		"/v1beta/models",
		"/v1beta/models/:model",
		"/antigravity/models",
		"/antigravity/v1beta/models",
		"/antigravity/v1beta/models/:model",
	}
	for _, path := range free {
		require.True(t, skipBillingForPath(path), "expected %s to be auth-only", path)
	}

	paid := []string{
		"/v1/messages",
		"/v1/chat/completions",
		"/responses",
		"/v1beta/models/*modelAction",
		"/antigravity/v1beta/models/*modelAction",
		"/v1/images/generations",
	}
	for _, path := range paid {
		require.False(t, skipBillingForPath(path), "expected %s to stay billed", path)
	}
}

func zeroBalanceKeyService(apiKey *service.APIKey, cfg *config.Config) *service.APIKeyService {
	repo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			clone := *apiKey
			return &clone, nil
		},
		updateLastUsed: func(ctx context.Context, id int64, usedAt time.Time) error { return nil },
	}
	return service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, cfg)
}

func TestZeroBalanceKeyCanListModelsButNotChat(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &service.User{
		ID:          7,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Balance:     0, // operationally empty: below service.MinOperationalBalance
		Concurrency: 3,
	}
	apiKey := &service.APIKey{
		ID:     100,
		UserID: user.ID,
		Key:    "test-key",
		Status: service.StatusActive,
		User:   user,
	}

	cfg := &config.Config{RunMode: config.RunModeStandard}
	apiKeyService := zeroBalanceKeyService(apiKey, cfg)

	router := gin.New()
	router.GET("/v1/models", func(c *gin.Context) {
		if !ApplyAPIKeyAuthWithSubscriptionContext(apiKeyService, nil, cfg, gatewayctx.FromGin(c)) {
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		if !ApplyAPIKeyAuthWithSubscriptionContext(apiKeyService, nil, cfg, gatewayctx.FromGin(c)) {
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	do := func(method, path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer "+apiKey.Key)
		router.ServeHTTP(w, req)
		return w
	}

	listResp := do(http.MethodGet, "/v1/models")
	require.Equal(t, http.StatusOK, listResp.Code, "model list must not require a funded balance: %s", listResp.Body.String())

	chatResp := do(http.MethodPost, "/v1/chat/completions")
	require.Equal(t, http.StatusForbidden, chatResp.Code)
	require.Contains(t, chatResp.Body.String(), "INSUFFICIENT_BALANCE", "paid routes must keep the balance gate")
}

func TestZeroBalanceKeyCanListGeminiModelsButNotGenerate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &service.User{
		ID:          8,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Balance:     0,
		Concurrency: 3,
	}
	apiKey := &service.APIKey{
		ID:     101,
		UserID: user.ID,
		Key:    "google-key",
		Status: service.StatusActive,
		User:   user,
	}

	cfg := &config.Config{RunMode: config.RunModeStandard}
	apiKeyService := zeroBalanceKeyService(apiKey, cfg)

	router := gin.New()
	router.GET("/v1beta/models", func(c *gin.Context) {
		if !ApplyAPIKeyAuthWithSubscriptionGoogleContext(apiKeyService, nil, cfg, gatewayctx.FromGin(c)) {
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	router.POST("/v1beta/models/*modelAction", func(c *gin.Context) {
		if !ApplyAPIKeyAuthWithSubscriptionGoogleContext(apiKeyService, nil, cfg, gatewayctx.FromGin(c)) {
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	do := func(method, path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("x-goog-api-key", apiKey.Key)
		router.ServeHTTP(w, req)
		return w
	}

	listResp := do(http.MethodGet, "/v1beta/models")
	require.Equal(t, http.StatusOK, listResp.Code, "gemini model list must stay free: %s", listResp.Body.String())

	genResp := do(http.MethodPost, "/v1beta/models/gemini-2.5-flash:generateContent")
	require.Equal(t, http.StatusForbidden, genResp.Code, "generateContent stays billed")
}

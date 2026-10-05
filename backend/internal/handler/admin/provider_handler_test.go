package admin

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	_ "modernc.org/sqlite"
	"github.com/stretchr/testify/require"
)

type dummyEncryptor struct{}

func (d *dummyEncryptor) Encrypt(plaintext string) (string, error) {
	return "enc:v1:" + plaintext, nil
}

func (d *dummyEncryptor) Decrypt(ciphertext string) (string, error) {
	if len(ciphertext) >= len("enc:v1:") && ciphertext[:len("enc:v1:")] == "enc:v1:" {
		return ciphertext[len("enc:v1:"):], nil
	}
	return ciphertext, nil
}

func setupTestEntClient(t *testing.T) *ent.Client {
	db, err := sql.Open("sqlite", fmt.Sprintf("file:provider_test_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	client := enttest.NewClient(t, enttest.WithOptions(ent.Driver(entsql.OpenDB(dialect.SQLite, db))))
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func setupTestRedis(t *testing.T) *redis.Client {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return rdb
}

func TestMaskAPIKey(t *testing.T) {
	require.Equal(t, "", MaskAPIKey(""))
	require.Equal(t, "sk-***", MaskAPIKey("short"))
	require.Equal(t, "sk-***", MaskAPIKey("12345678"))
	require.Equal(t, "12-***90", MaskAPIKey("1234567890"))
	require.Equal(t, "sk-a-***9xyz", MaskAPIKey("sk-ant-api03-123456789xyz"))
}

func TestParseProviderDomain(t *testing.T) {
	require.Equal(t, "custom.domain.com", parseProviderDomain("custom.domain.com", "https://api.other.com"))
	require.Equal(t, "api.openlux.ai", parseProviderDomain("", "https://api.openlux.ai/v1"))
	require.Equal(t, "", parseProviderDomain("", "invalid-url"))
}

func TestDetermineProviderState(t *testing.T) {
	// 1. Disabled
	require.Equal(t, "Disabled", determineProviderState("disabled", false, 100, 20, nil, "healthy"))
	require.Equal(t, "Disabled", determineProviderState("inactive", false, 100, 20, nil, "healthy"))

	// 2. Invalid key
	require.Equal(t, "Invalid key", determineProviderState("error", false, 100, 20, nil, "healthy"))
	require.Equal(t, "Invalid key", determineProviderState("active", true, 100, 20, nil, "healthy"))

	// 3. Low balance
	require.Equal(t, "Low balance", determineProviderState("active", false, 15.0, 20.0, nil, "healthy"))
	require.Equal(t, "Low balance", determineProviderState("active", false, 0.0, 20.0, nil, "healthy"))

	// 4. Down
	rtDown := &health.NodeRuntimeState{TotalRequests: 10, SuccessRate: 0.1}
	require.Equal(t, "Down", determineProviderState("active", false, 100, 20, rtDown, "healthy"))
	require.Equal(t, "Down", determineProviderState("active", false, 100, 20, nil, "unavailable"))

	// 5. Degraded
	rtDegradedRate := &health.NodeRuntimeState{TotalRequests: 10, SuccessRate: 0.75}
	require.Equal(t, "Degraded", determineProviderState("active", false, 100, 20, rtDegradedRate, "healthy"))
	rtDegradedLatency := &health.NodeRuntimeState{TotalRequests: 10, SuccessRate: 0.95, AvgLatencyMs: 6000}
	require.Equal(t, "Degraded", determineProviderState("active", false, 100, 20, rtDegradedLatency, "healthy"))
	require.Equal(t, "Degraded", determineProviderState("active", false, 100, 20, nil, "cooldown"))

	// 6. Healthy
	rtHealthy := &health.NodeRuntimeState{TotalRequests: 10, SuccessRate: 0.99, AvgLatencyMs: 350}
	require.Equal(t, "Healthy", determineProviderState("active", false, 100, 20, rtHealthy, "healthy"))
}

func TestProviderHandler_CRUD(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := setupTestEntClient(t)
	stubAdmin := newStubAdminService()
	stubAdmin.accounts = nil
	encryptor := &dummyEncryptor{}

	h := NewProviderHandler(stubAdmin, client, nil, encryptor, nil)

	r := gin.New()
	r.GET("/api/v1/admin/providers", h.List)
	r.POST("/api/v1/admin/providers", h.Create)
	r.GET("/api/v1/admin/providers/:id", h.GetByID)
	r.PUT("/api/v1/admin/providers/:id", h.Update)
	r.DELETE("/api/v1/admin/providers/:id", h.Delete)
	r.POST("/api/v1/admin/providers/:id/test", h.TestConnection)
	r.POST("/api/v1/admin/providers/test", h.TestDraftConnection)

	// 1. Empty list
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/providers", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var resp struct {
			Code int `json:"code"`
			Data ProviderListResponse `json:"data"`
		}
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.Equal(t, 0, resp.Data.Summary.TotalProviders)
		require.Empty(t, resp.Data.Items)
	}

	// 2. Create Provider with validation errors
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers", bytes.NewBufferString(`{}`))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	}

	// 3. Create Valid Provider
	var createdID int64
	{
		createReq := CreateProviderRequest{
			ProviderName: "OpenLux Primary",
			Provider:     "openlux",
			Domain:       "api.openlux.ai",
			BaseURL:      "https://api.openlux.ai/v1",
			Protocol:     "openai",
			APIKey:       "sk-openlux-secret-key-123456",
			SupportedModelFamilies: []string{"claude", "gpt"},
			ModelMapping: map[string]string{
				"claude-3-7-sonnet": "claude-3-7-sonnet-20250219",
			},
			Status: "active",
		}
		raw, _ := json.Marshal(createReq)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers", bytes.NewBuffer(raw))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var resp struct {
			Code int `json:"code"`
			Data ProviderResponse `json:"data"`
		}
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.Equal(t, "OpenLux Primary", resp.Data.ProviderName)
		require.Equal(t, "api.openlux.ai", resp.Data.Domain)
		require.Equal(t, "sk-o-***3456", resp.Data.APIKeyMasked)
		require.Equal(t, 20.0, resp.Data.MinBalanceUSD)
		createdID = resp.Data.ID
		require.True(t, createdID > 0)
	}

	// 4. List now returns 1 item
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/providers", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var resp struct {
			Code int `json:"code"`
			Data ProviderListResponse `json:"data"`
		}
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.Equal(t, 1, resp.Data.Summary.TotalProviders)
		require.Equal(t, 1, len(resp.Data.Items))
		require.Equal(t, createdID, resp.Data.Items[0].ID)
	}

	// 5. GetByID
	{
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/admin/providers/%d", createdID), nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var resp struct {
			Code int `json:"code"`
			Data ProviderResponse `json:"data"`
		}
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.Equal(t, "OpenLux Primary", resp.Data.ProviderName)
	}

	// 6. Update Provider
	{
		newMin := 35.0
		updateReq := UpdateProviderRequest{
			ProviderName: "OpenLux Upgraded",
			MinBalanceUSD: &newMin,
			Status: "disabled",
			APIKey: "sk-o-***3456", // masked key should NOT overwrite secret
		}
		raw, _ := json.Marshal(updateReq)
		req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/admin/providers/%d", createdID), bytes.NewBuffer(raw))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var resp struct {
			Code int `json:"code"`
			Data ProviderResponse `json:"data"`
		}
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.Equal(t, "OpenLux Upgraded", resp.Data.ProviderName)
		require.Equal(t, 35.0, resp.Data.MinBalanceUSD)
		require.Equal(t, "disabled", resp.Data.Status)
		require.Equal(t, "Disabled", resp.Data.State)
	}

	// 7. Delete Provider
	{
		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/admin/providers/%d", createdID), nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	}

	// 8. GetByID after delete returns 404
	{
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/admin/providers/%d", createdID), nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusNotFound, rec.Code)
	}
}

func TestProviderHandler_TestConnection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := setupTestEntClient(t)
	stubAdmin := newStubAdminService()
	encryptor := &dummyEncryptor{}

	// Mock upstream server returning models list
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			auth := r.Header.Get("Authorization")
			if auth == "Bearer valid-test-key" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"gpt-4o"},{"id":"claude-3-7-sonnet"}]}`))
				return
			}
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	h := NewProviderHandler(stubAdmin, client, nil, encryptor, nil)

	r := gin.New()
	r.POST("/api/v1/admin/providers/test", h.TestDraftConnection)
	r.POST("/api/v1/admin/providers/:id/test", h.TestConnection)

	// 1. Successful connection
	{
		reqBody := TestConnectionRequest{
			BaseURL: server.URL + "/v1",
			APIKey:  "valid-test-key",
		}
		raw, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers/test", bytes.NewBuffer(raw))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var resp struct {
			Code int `json:"code"`
			Data TestConnectionResponse `json:"data"`
		}
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.True(t, resp.Data.Success)
		require.Equal(t, 2, resp.Data.ModelCount)
		require.Equal(t, http.StatusOK, resp.Data.StatusCode)
		require.True(t, resp.Data.LatencyMs >= 0)
	}

	// 2. Authentication failure
	{
		reqBody := TestConnectionRequest{
			BaseURL: server.URL + "/v1",
			APIKey:  "invalid-key",
		}
		raw, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers/test", bytes.NewBuffer(raw))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var resp struct {
			Code int `json:"code"`
			Data TestConnectionResponse `json:"data"`
		}
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.False(t, resp.Data.Success)
		require.Equal(t, http.StatusUnauthorized, resp.Data.StatusCode)
	}

	// 3. Draft test with missing base_url
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers/test", bytes.NewBufferString(`{"api_key":"test"}`))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	}

	// 4. Draft test with missing api_key
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers/test", bytes.NewBufferString(`{"base_url":"https://api.test.com"}`))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	}

	// 5. Draft test with invalid JSON
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers/test", bytes.NewBufferString(`{invalid`))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	}

	// 6. Test existing provider by ID
	{
		created, err := stubAdmin.CreateAccount(context.Background(), &service.CreateAccountInput{
			Name:     "Existing Test Provider",
			Platform: domain.PlatformYunwu,
			Credentials: map[string]any{
				"api_key": "enc:v1:valid-test-key",
			},
			Extra: map[string]any{
				"base_url": server.URL + "/v1",
			},
		})
		require.NoError(t, err)

		// Non-existent ID
		{
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers/88888/test", nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			t.Logf("Got code %d, body: %s", rec.Code, rec.Body.String())
			require.Equal(t, http.StatusNotFound, rec.Code)
		}

		// Invalid ID
		{
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers/invalid-id/test", nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			require.Equal(t, http.StatusBadRequest, rec.Code)
		}

		// Valid provider test
		{
			req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/admin/providers/%d/test", created.ID), nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			require.Equal(t, http.StatusOK, rec.Code)

			var resp struct {
				Code int `json:"code"`
				Data TestConnectionResponse `json:"data"`
			}
			err := json.Unmarshal(rec.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.True(t, resp.Data.Success)
			require.Equal(t, 2, resp.Data.ModelCount)
		}
	}
}

func TestProviderHandler_SyncBalance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := setupTestEntClient(t)
	stubAdmin := newStubAdminService()
	encryptor := &dummyEncryptor{}

	h := NewProviderHandler(stubAdmin, client, nil, encryptor, nil)

	r := gin.New()
	r.POST("/api/v1/admin/providers/:id/sync-balance", h.SyncBalance)
	r.POST("/api/v1/admin/providers/sync-all", h.SyncAllBalances)

	// Invalid ID
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers/abc/sync-balance", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	}

	// Provider not found
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers/999/sync-balance", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusNotFound, rec.Code)
	}

	// Provider exists but stats worker unavailable
	{
		h.SetStatsWorker(nil)
		created, err := stubAdmin.CreateAccount(context.Background(), &service.CreateAccountInput{
			Name:     "Yunwu No-Sync Provider",
			Platform: domain.PlatformYunwu,
			Credentials: map[string]any{
				"api_key": "enc:v1:test-key",
			},
		})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/admin/providers/%d/sync-balance", created.ID), nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusInternalServerError, rec.Code)

		// Sync all with stats worker unavailable
		reqAll := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers/sync-all", nil)
		recAll := httptest.NewRecorder()
		r.ServeHTTP(recAll, reqAll)
		require.Equal(t, http.StatusInternalServerError, recAll.Code)
	}

	// Successful balance sync with active statsWorker and mock server
	{
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"object":          "sub2api_usage",
				"total_available": 42.5,
				"total_granted":   100.0,
				"total_used":      57.5,
			})
		}))
		defer mockServer.Close()

		rdb := setupTestRedis(t)
		hSync := NewProviderHandler(stubAdmin, client, rdb, encryptor, nil)
		rSync := gin.New()
		rSync.POST("/api/v1/admin/providers/:id/sync-balance", hSync.SyncBalance)
		rSync.POST("/api/v1/admin/providers/sync-all", hSync.SyncAllBalances)

		created, err := stubAdmin.CreateAccount(context.Background(), &service.CreateAccountInput{
			Name:     "Synced Provider",
			Platform: domain.PlatformYunwu,
			Credentials: map[string]any{
				"api_key": "enc:v1:test-key-sync",
			},
			Extra: map[string]any{
				"provider": "custom",
				"base_url": mockServer.URL,
				"min_balance_usd": 20.0,
			},
		})
		require.NoError(t, err)

		// Sync single
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/admin/providers/%d/sync-balance", created.ID), nil)
		rec := httptest.NewRecorder()
		rSync.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		// Sync all
		reqAll := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers/sync-all", nil)
		recAll := httptest.NewRecorder()
		rSync.ServeHTTP(recAll, reqAll)
		require.Equal(t, http.StatusOK, recAll.Code)
	}
}

func TestProviderHandler_EdgeCases(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := setupTestEntClient(t)
	stubAdmin := newStubAdminService()
	encryptor := &dummyEncryptor{}

	h := NewProviderHandler(stubAdmin, client, nil, encryptor, nil)

	r := gin.New()
	r.GET("/api/v1/admin/providers", h.List)
	r.POST("/api/v1/admin/providers", h.Create)
	r.PUT("/api/v1/admin/providers/:id", h.Update)
	r.DELETE("/api/v1/admin/providers/:id", h.Delete)
	r.GET("/api/v1/admin/providers/:id", h.GetByID)

	// Create with empty name
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers", bytes.NewBufferString(`{"provider_name":" ","api_key":"key"}`))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	}

	// Create with empty api_key
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers", bytes.NewBufferString(`{"provider_name":"Name","api_key":" "}`))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	}

	// Update with invalid ID
	{
		req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/providers/0", bytes.NewBufferString(`{}`))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	}

	// Update with invalid JSON
	{
		req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/providers/1", bytes.NewBufferString(`{bad`))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	}

	// Update non-existent provider
	{
		req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/providers/9999", bytes.NewBufferString(`{"provider_name":"Updated"}`))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusNotFound, rec.Code)
	}

	// Delete with invalid ID
	{
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/providers/-1", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	}

	// GetByID with invalid ID
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/providers/xyz", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	}
}

func TestProviderHandler_AllExecutionNodesAsProviders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := setupTestEntClient(t)
	stubAdmin := newStubAdminService()
	now := time.Now().UTC()
	stubAdmin.accounts = []service.Account{
		{
			ID:       1,
			Name:     "Anthropic Node",
			Platform: domain.PlatformAnthropic,
			Type:     service.AccountTypeAPIKey,
			Status:   service.StatusActive,
			Extra: map[string]any{
				"alert_threshold": 15.0,
				"balance":         80.0,
			},
			Credentials: map[string]any{
				"api_key": "sk-ant-test-key-12345",
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:       2,
			Name:     "OpenAI Node",
			Platform: domain.PlatformOpenAI,
			Type:     service.AccountTypeAPIKey,
			Status:   service.StatusActive,
			Extra: map[string]any{
				"cost_threshold": 25.0,
				"balance_usd":    50.0,
			},
			Credentials: map[string]any{
				"api_key": "sk-proj-test-key-67890",
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:       3,
			Name:     "Gemini Node",
			Platform: domain.PlatformGemini,
			Type:     service.AccountTypeAPIKey,
			Status:   service.StatusActive,
			Extra: map[string]any{
				"threshold":       10.0,
				"total_available": 100.0,
			},
			Credentials: map[string]any{
				"token": "AIzaSy-test-gemini-key",
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:       4,
			Name:     "Antigravity Node",
			Platform: domain.PlatformAntigravity,
			Type:     service.AccountTypeAPIKey,
			Status:   service.StatusActive,
			Extra: map[string]any{
				"limit":       45.0,
				"balance_usd": 120.0,
			},
			Credentials: map[string]any{
				"access_token": "antigravity-token-xyz",
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
	encryptor := &dummyEncryptor{}

	h := NewProviderHandler(stubAdmin, client, nil, encryptor, nil)
	r := gin.New()
	r.GET("/api/v1/admin/providers", h.List)
	r.GET("/api/v1/admin/providers/:id", h.GetByID)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/providers", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Code int                  `json:"code"`
		Data ProviderListResponse `json:"data"`
	}
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Equal(t, 4, resp.Data.Summary.TotalProviders)
	require.Equal(t, 4, len(resp.Data.Items))

	// Verify each provider has correct parameters derived from the node
	foundAnthropic := false
	foundOpenAI := false
	foundGemini := false
	foundAntigravity := false

	for _, item := range resp.Data.Items {
		switch item.ID {
		case 1:
			foundAnthropic = true
			require.Equal(t, "Anthropic Node", item.ProviderName)
			require.Equal(t, "anthropic", item.Protocol)
			require.Equal(t, 15.0, item.MinBalanceUSD)
			require.Equal(t, 80.0, item.BalanceUSD)
			require.Equal(t, "https://api.anthropic.com", item.BaseURL)
			require.Contains(t, item.SupportedModelFamilies, "claude")
		case 2:
			foundOpenAI = true
			require.Equal(t, "OpenAI Node", item.ProviderName)
			require.Equal(t, "openai", item.Protocol)
			require.Equal(t, 25.0, item.MinBalanceUSD)
			require.Equal(t, 50.0, item.BalanceUSD)
			require.Equal(t, "https://api.openai.com/v1", item.BaseURL)
			require.Contains(t, item.SupportedModelFamilies, "gpt")
		case 3:
			foundGemini = true
			require.Equal(t, "Gemini Node", item.ProviderName)
			require.Equal(t, "gemini", item.Protocol)
			require.Equal(t, 10.0, item.MinBalanceUSD)
			require.Equal(t, 100.0, item.BalanceUSD)
			require.Equal(t, "https://generativelanguage.googleapis.com", item.BaseURL)
			require.Contains(t, item.SupportedModelFamilies, "gemini")
		case 4:
			foundAntigravity = true
			require.Equal(t, "Antigravity Node", item.ProviderName)
			require.Equal(t, 45.0, item.MinBalanceUSD)
			require.Equal(t, 120.0, item.BalanceUSD)
			require.Equal(t, "https://api.antigravity.ai", item.BaseURL)
		}
	}

	require.True(t, foundAnthropic)
	require.True(t, foundOpenAI)
	require.True(t, foundGemini)
	require.True(t, foundAntigravity)
}


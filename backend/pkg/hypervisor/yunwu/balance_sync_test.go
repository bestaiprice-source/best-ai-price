package yunwu

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
)

func newTestYunwuStorageWithDB(t *testing.T) (*storage.Storage, *dbent.Client) {
	t.Helper()

	name := strings.ReplaceAll(t.Name(), "/", "_")
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", name)
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB("sqlite3", db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return storage.NewStorage(client, rdb), client
}

func TestSyncSub2APIUsage(t *testing.T) {
	ctx := context.Background()

	t.Run("unrestricted mode with remaining balance", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/usage", r.URL.Path)
			assert.Equal(t, "Bearer test-sub2api-key", r.Header.Get("Authorization"))
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"mode":      "unrestricted",
				"balance":   50.0,
				"remaining": 45.5,
			})
		}))
		defer server.Close()

		sw := NewStatsWorker(nil)
		sw.httpClient = server.Client()

		grants, err := sw.syncSub2APIUsage(ctx, server.URL, "/v1", "test-sub2api-key")
		require.NoError(t, err)
		assert.NotNil(t, grants)
		assert.Equal(t, "sub2api_usage", grants.Object)
		assert.Equal(t, 45.5, grants.TotalAvailable)
		assert.Equal(t, 50.0, grants.TotalGranted)
		assert.Equal(t, 4.5, grants.TotalUsed)
	})

	t.Run("quota_limited mode with quota object", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"mode": "quota_limited",
				"quota": map[string]any{
					"limit":     100.0,
					"used":      30.0,
					"remaining": 70.0,
				},
			})
		}))
		defer server.Close()

		sw := NewStatsWorker(nil)
		sw.httpClient = server.Client()

		grants, err := sw.syncSub2APIUsage(ctx, server.URL, "/v1", "test-sub2api-key")
		require.NoError(t, err)
		assert.NotNil(t, grants)
		assert.Equal(t, 70.0, grants.TotalAvailable)
		assert.Equal(t, 100.0, grants.TotalGranted)
		assert.Equal(t, 30.0, grants.TotalUsed)
	})

	t.Run("zero balance remaining", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"mode":      "unrestricted",
				"balance":   20.0,
				"remaining": 0.0,
			})
		}))
		defer server.Close()

		sw := NewStatsWorker(nil)
		sw.httpClient = server.Client()

		grants, err := sw.syncSub2APIUsage(ctx, server.URL, "", "test-sub2api-key")
		require.NoError(t, err)
		assert.NotNil(t, grants)
		assert.Equal(t, 0.0, grants.TotalAvailable)
	})

	t.Run("HTML response rejected gracefully", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("<!DOCTYPE html><html><body>Login required</body></html>"))
		}))
		defer server.Close()

		sw := NewStatsWorker(nil)
		sw.httpClient = server.Client()

		_, err := sw.syncSub2APIUsage(ctx, server.URL, "", "test-sub2api-key")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "non-JSON content-type")
	})

	t.Run("HTML body with json header rejected gracefully", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("<!DOCTYPE html><html><body>Login required</body></html>"))
		}))
		defer server.Close()

		sw := NewStatsWorker(nil)
		sw.httpClient = server.Client()

		_, err := sw.syncSub2APIUsage(ctx, server.URL, "", "test-sub2api-key")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "HTML response")
	})

	t.Run("malformed response or 404", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		}))
		defer server.Close()

		sw := NewStatsWorker(nil)
		sw.httpClient = server.Client()

		_, err := sw.syncSub2APIUsage(ctx, server.URL, "", "test-sub2api-key")
		assert.Error(t, err)
	})
}

func TestSyncSubscriptionUsageAndGrantsHTMLRejection(t *testing.T) {
	ctx := context.Background()

	t.Run("subscription returns valid zero balance", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"hard_limit_usd":  0.0,
				"total_available": 0.0,
			})
		}))
		defer server.Close()

		sw := NewStatsWorker(nil)
		sw.httpClient = server.Client()

		grants, err := sw.syncSubscriptionUsage(ctx, server.URL, "", "test-key")
		require.NoError(t, err)
		assert.NotNil(t, grants)
		assert.Equal(t, 0.0, grants.TotalAvailable)
	})

	t.Run("subscription rejects HTML response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<!DOCTYPE html><html>SPA</html>"))
		}))
		defer server.Close()

		sw := NewStatsWorker(nil)
		sw.httpClient = server.Client()

		_, err := sw.syncSubscriptionUsage(ctx, server.URL, "", "test-key")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "non-JSON content-type")
	})

	t.Run("subscription rejects HTML body with json content type", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("<!DOCTYPE html><html>SPA</html>"))
		}))
		defer server.Close()

		sw := NewStatsWorker(nil)
		sw.httpClient = server.Client()

		_, err := sw.syncSubscriptionUsage(ctx, server.URL, "", "test-key")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "HTML response")
	})

	t.Run("credit_grants rejects HTML response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<!DOCTYPE html><html>SPA</html>"))
		}))
		defer server.Close()

		sw := NewStatsWorker(nil)
		sw.httpClient = server.Client()

		_, err := sw.syncCreditGrants(ctx, server.URL, "", "test-key")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "non-JSON content-type")
	})

	t.Run("credit_grants rejects HTML body with json content type", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("<!DOCTYPE html><html>SPA</html>"))
		}))
		defer server.Close()

		sw := NewStatsWorker(nil)
		sw.httpClient = server.Client()

		_, err := sw.syncCreditGrants(ctx, server.URL, "", "test-key")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "HTML response")
	})
}

func TestSyncNodeStatsPortdanPriority(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/usage" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"mode":      "unrestricted",
				"balance":   100.0,
				"remaining": 88.0,
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	store := setupTestYunwuStore(t)
	sw := NewStatsWorker(store)
	sw.httpClient = server.Client()

	node := &health.ExecutionNode{
		NodeID:   "portdan-node",
		Provider: ProviderPortdan,
	}

	grants, err := sw.SyncNodeStats(ctx, node, "test-key", server.URL)
	require.NoError(t, err)
	assert.NotNil(t, grants)
	assert.Equal(t, 88.0, grants.TotalAvailable)
	assert.Equal(t, "sub2api_usage", grants.Object)
}

func TestYunwuHypervisorGroupAndAllBalanceSync(t *testing.T) {
	store, client := newTestYunwuStorageWithDB(t)
	ctx := context.Background()

	// Setup mock upstream server that answers OpenLux (subscription) and Portdan (/v1/usage)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/dashboard/billing/subscription", "/dashboard/billing/subscription":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"hard_limit_usd":  50.0,
				"total_available": 35.0,
			})
		case "/v1/usage":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"mode":      "unrestricted",
				"balance":   20.0,
				"remaining": 15.0,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	// Create Group 10 and Group 20
	grp10, err := client.Group.Create().
		SetName("Yunwu Group 10").
		Save(ctx)
	require.NoError(t, err)

	grp20, err := client.Group.Create().
		SetName("Yunwu Group 20").
		Save(ctx)
	require.NoError(t, err)

	// Create accounts in Group 10
	acc1, err := client.Account.Create().
		SetName("OpenLux Node").
		SetPlatform("yunwu").
		SetType("api").
		SetStatus("active").
		SetSchedulable(true).
		SetPriority(10).
		SetCredentials(map[string]any{
			"api_key":  "key-openlux",
			"base_url": server.URL,
		}).
		SetExtra(map[string]any{
			"provider": "openlux",
		}).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.AccountGroup.Create().
		SetAccountID(acc1.ID).
		SetGroupID(grp10.ID).
		Save(ctx)
	require.NoError(t, err)

	acc2, err := client.Account.Create().
		SetName("Portdan Node").
		SetPlatform("yunwu").
		SetType("api").
		SetStatus("active").
		SetSchedulable(true).
		SetPriority(5).
		SetCredentials(map[string]any{
			"api_key":  "key-portdan",
			"base_url": server.URL,
		}).
		SetExtra(map[string]any{
			"provider": "portdan",
		}).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.AccountGroup.Create().
		SetAccountID(acc2.ID).
		SetGroupID(grp10.ID).
		Save(ctx)
	require.NoError(t, err)

	// Create account in Group 20 with missing API key to test error handling
	acc3, err := client.Account.Create().
		SetName("Broken Node").
		SetPlatform("yunwu").
		SetType("api").
		SetStatus("active").
		SetSchedulable(true).
		SetCredentials(map[string]any{}).
		SetExtra(map[string]any{
			"provider": "custom",
		}).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.AccountGroup.Create().
		SetAccountID(acc3.ID).
		SetGroupID(grp20.ID).
		Save(ctx)
	require.NoError(t, err)

	rt := &mockYunwuRuntime{}
	yh := NewYunwuHypervisor(store, rt)
	yh.sw.httpClient = server.Client()

	// 1. Sync Group 10
	summary10, err := yh.SyncGroupBalances(ctx, grp10.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(grp10.ID), summary10.GroupID)
	assert.Equal(t, 2, summary10.TotalNodes)
	assert.Equal(t, 2, summary10.SyncedNodes)
	assert.Equal(t, 0, summary10.FailedNodes)
	assert.InDelta(t, 50.0, summary10.TotalAvailableUSD, 0.01) // 35.0 + 15.0

	// Verify extra fields updated in DB
	refreshedAcc1, err := client.Account.Get(ctx, acc1.ID)
	require.NoError(t, err)
	assert.Equal(t, 35.0, refreshedAcc1.Extra["balance_usd"])
	assert.NotEmpty(t, refreshedAcc1.Extra["balance_updated_at"])

	// 2. Sync All Groups
	summaryAll, err := yh.SyncAllBalances(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, summaryAll.TotalNodes)
	assert.Equal(t, 2, summaryAll.SyncedNodes)
	assert.Equal(t, 1, summaryAll.FailedNodes) // acc3 has empty api key

	// 3. Sync Non-existent group
	summaryEmpty, err := yh.SyncAllBalances(ctx, 99999)
	require.NoError(t, err)
	assert.Equal(t, 0, summaryEmpty.TotalNodes)
	assert.Equal(t, 0, summaryEmpty.SyncedNodes)

	// 4. Test getAllYunwuNodes with DB accounts
	nodes, err := yh.getAllYunwuNodes(ctx)
	require.NoError(t, err)
	assert.Len(t, nodes, 3)

	// 5. Database unavailable error case
	yhNilStore := NewYunwuHypervisor(nil, rt)
	_, err = yhNilStore.SyncAllBalances(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database unavailable")
}

func TestSyncNewAPITokenUsage(t *testing.T) {
	ctx := context.Background()

	t.Run("new-api token usage endpoint with raw quota ($3 remaining)", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/usage/token", r.URL.Path)
			assert.Equal(t, "Bearer sk-test-key", r.Header.Get("Authorization"))
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    true,
				"message": "ok",
				"data": map[string]any{
					"object":          "token_usage",
					"name":            "api top",
					"total_granted":   100000000.0, // 100M quota = $200.00
					"total_used":      98500000.0,  // 98.5M quota = $197.00
					"total_available": 1500000.0,   // 1.5M quota = $3.00
					"unlimited_quota": false,
				},
			})
		}))
		defer server.Close()

		sw := NewStatsWorker(nil)
		sw.httpClient = server.Client()

		grants, err := sw.syncNewAPITokenUsage(ctx, server.URL, "", "sk-test-key")
		require.NoError(t, err)
		assert.NotNil(t, grants)
		assert.Equal(t, "token_usage", grants.Object)
		assert.InDelta(t, 3.0, grants.TotalAvailable, 0.001)
		assert.InDelta(t, 200.0, grants.TotalGranted, 0.001)
		assert.InDelta(t, 197.0, grants.TotalUsed, 0.001)
	})

	t.Run("unlimited quota token falls through when total_available is 0", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"total_used":      500000.0,
					"total_available": 0.0,
					"unlimited_quota": true,
				},
			})
		}))
		defer server.Close()

		sw := NewStatsWorker(nil)
		sw.httpClient = server.Client()

		_, err := sw.syncNewAPITokenUsage(ctx, server.URL, "", "sk-test-key")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unlimited_quota")
	})

	t.Run("unlimited quota token with explicit remaining quota", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"total_used":      500000.0,
					"total_available": 1500000.0, // 1.5M quota = $3.00
					"unlimited_quota": true,
				},
			})
		}))
		defer server.Close()

		sw := NewStatsWorker(nil)
		sw.httpClient = server.Client()

		grants, err := sw.syncNewAPITokenUsage(ctx, server.URL, "", "sk-test-key")
		require.NoError(t, err)
		assert.NotNil(t, grants)
		assert.Equal(t, "token_usage", grants.Object)
		assert.InDelta(t, 3.0, grants.TotalAvailable, 0.001)
	})

	t.Run("HTML response rejection", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html>Error</html>"))
		}))
		defer server.Close()

		sw := NewStatsWorker(nil)
		sw.httpClient = server.Client()

		_, err := sw.syncNewAPITokenUsage(ctx, server.URL, "", "sk-test-key")
		assert.Error(t, err)
	})
}

func TestSyncUserSelf(t *testing.T) {
	ctx := context.Background()

	t.Run("returns quota in raw units normalized to USD ($3.00)", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/user/self", r.URL.Path)
			assert.Equal(t, "Bearer sk-test-key", r.Header.Get("Authorization"))
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"id":         1,
					"username":   "apitop_user",
					"quota":      1500000.0, // 1.5M quota units = $3.00 USD
					"used_quota": 500000.0,  // 500k quota units = $1.00 USD
				},
			})
		}))
		defer server.Close()

		sw := NewStatsWorker(nil)
		sw.httpClient = server.Client()

		grants, err := sw.syncUserSelf(ctx, server.URL, "", "sk-test-key")
		require.NoError(t, err)
		assert.NotNil(t, grants)
		assert.Equal(t, "user_self", grants.Object)
		assert.InDelta(t, 3.0, grants.TotalAvailable, 0.001)
		assert.InDelta(t, 1.0, grants.TotalUsed, 0.001)
		assert.InDelta(t, 4.0, grants.TotalGranted, 0.001)
	})

	t.Run("returns direct balance in USD ($3.00)", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"balance": 3.0,
				},
			})
		}))
		defer server.Close()

		sw := NewStatsWorker(nil)
		sw.httpClient = server.Client()

		grants, err := sw.syncUserSelf(ctx, server.URL, "", "sk-test-key")
		require.NoError(t, err)
		assert.NotNil(t, grants)
		assert.Equal(t, "user_self", grants.Object)
		assert.InDelta(t, 3.0, grants.TotalAvailable, 0.001)
	})
}

func TestSyncSubscriptionUsageSubtraction(t *testing.T) {
	ctx := context.Background()

	t.Run("hard_limit_usd $200 with total_usage 19700 cents ($197) yields $3 remaining", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if strings.Contains(r.URL.Path, "subscription") {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"object":         "billing_subscription",
					"hard_limit_usd": 200.0,
				})
				return
			}
			if strings.Contains(r.URL.Path, "usage") {
				assert.Contains(t, r.URL.RawQuery, "start_date=2020-01-01")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"total_usage": 19700.0, // 19700 cents = $197.00
				})
				return
			}
			http.NotFound(w, r)
		}))
		defer server.Close()

		sw := NewStatsWorker(nil)
		sw.httpClient = server.Client()

		grants, err := sw.syncSubscriptionUsage(ctx, server.URL, "/v1", "sk-test-key")
		require.NoError(t, err)
		assert.NotNil(t, grants)
		assert.Equal(t, "subscription", grants.Object)
		assert.InDelta(t, 3.0, grants.TotalAvailable, 0.001)
		assert.InDelta(t, 200.0, grants.TotalGranted, 0.001)
		assert.InDelta(t, 197.0, grants.TotalUsed, 0.001)
	})
}

func TestSyncNodeStatsApiTopEndToEnd(t *testing.T) {
	ctx := context.Background()
	store := setupTestYunwuStore(t)

	t.Run("api top with unlimited token falls through to /api/user/self to get exact $3.00 instead of 200 or 999999", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			// 1. /api/usage/token reports unlimited quota token with no quota set
			if r.URL.Path == "/api/usage/token" {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"code":    true,
					"message": "ok",
					"data": map[string]any{
						"object":          "token_usage",
						"name":            "api top",
						"total_granted":   0.0,
						"total_used":      0.0,
						"total_available": 0.0,
						"unlimited_quota": true,
					},
				})
				return
			}
			// 2. /api/user/self reports the actual user wallet quota: 1.5M units = $3.00!
			if r.URL.Path == "/api/user/self" {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"success": true,
					"data": map[string]any{
						"id":         42,
						"username":   "cucum",
						"quota":      1500000.0, // $3.00
						"used_quota": 0.0,
					},
				})
				return
			}
			// 3. Dummy subscription returning 200.0 must NOT be selected!
			if strings.Contains(r.URL.Path, "subscription") {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"hard_limit_usd": 200.0,
				})
				return
			}
			http.NotFound(w, r)
		}))
		defer server.Close()

		sw := NewStatsWorker(store)
		sw.httpClient = server.Client()

		node := &health.ExecutionNode{
			NodeID:      "api-top-node",
			AccountName: "api top",
			Provider:    "api-top",
		}

		grants, err := sw.SyncNodeStats(ctx, node, "sk-test", server.URL)
		require.NoError(t, err)
		assert.NotNil(t, grants)
		assert.Equal(t, "user_self", grants.Object)
		assert.InDelta(t, 3.0, grants.TotalAvailable, 0.001)
	})

	t.Run("api top with finite token quota returns token usage ($3.00)", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path == "/api/usage/token" {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"code":    true,
					"message": "ok",
					"data": map[string]any{
						"object":          "token_usage",
						"name":            "api top",
						"total_granted":   100000000.0, // 100M quota = $200.00
						"total_used":      98500000.0,  // 98.5M quota = $197.00
						"total_available": 1500000.0,   // 1.5M quota = $3.00
						"unlimited_quota": false,
					},
				})
				return
			}
			http.NotFound(w, r)
		}))
		defer server.Close()

		sw := NewStatsWorker(store)
		sw.httpClient = server.Client()

		node := &health.ExecutionNode{
			NodeID:      "api-top-finite-node",
			AccountName: "api top",
			Provider:    "api-top",
		}

		grants, err := sw.SyncNodeStats(ctx, node, "sk-test", server.URL)
		require.NoError(t, err)
		assert.NotNil(t, grants)
		assert.Equal(t, "token_usage", grants.Object)
		assert.InDelta(t, 3.0, grants.TotalAvailable, 0.001)
		assert.InDelta(t, 200.0, grants.TotalGranted, 0.001)
		assert.InDelta(t, 197.0, grants.TotalUsed, 0.001)
	})

	t.Run("api top with limited quota 0 correctly returns $0.00 without falling into subscription mock", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path == "/api/usage/token" {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"code":    true,
					"message": "ok",
					"data": map[string]any{
						"object":          "token_usage",
						"name":            "api top",
						"total_granted":   0.0,
						"total_used":      0.0,
						"total_available": 0.0,
						"remain_quota":    0.0,
						"unlimited_quota": false,
					},
				})
				return
			}
			// Mock subscription returning 200.0 should NOT be reached
			if strings.Contains(r.URL.Path, "subscription") {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"hard_limit_usd": 200.0,
				})
				return
			}
			http.NotFound(w, r)
		}))
		defer server.Close()

		sw := NewStatsWorker(store)
		sw.httpClient = server.Client()

		node := &health.ExecutionNode{
			NodeID:      "api-top-zero-quota-node",
			AccountName: "api top",
			Provider:    "api-top",
		}

		grants, err := sw.SyncNodeStats(ctx, node, "sk-test", server.URL)
		require.NoError(t, err)
		assert.NotNil(t, grants)
		assert.Equal(t, "token_usage", grants.Object)
		assert.InDelta(t, 0.0, grants.TotalAvailable, 0.001)
		assert.InDelta(t, 0.0, grants.TotalGranted, 0.001)
	})

	t.Run("api top fallback to subscription+usage subtraction correctly calculates $3.00", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if strings.Contains(r.URL.Path, "usage/token") {
				http.NotFound(w, r)
				return
			}
			if strings.Contains(r.URL.Path, "user/self") {
				http.NotFound(w, r)
				return
			}
			if strings.Contains(r.URL.Path, "credit_grants") {
				http.NotFound(w, r)
				return
			}
			if strings.Contains(r.URL.Path, "subscription") {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"hard_limit_usd": 200.0,
				})
				return
			}
			if strings.Contains(r.URL.Path, "usage") {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"total_usage": 19700.0, // 19700 cents = $197.00
				})
				return
			}
			http.NotFound(w, r)
		}))
		defer server.Close()

		sw := NewStatsWorker(store)
		sw.httpClient = server.Client()

		node := &health.ExecutionNode{
			NodeID:      "api-top-fallback-node",
			AccountName: "api top",
			Provider:    "openlux",
		}

		grants, err := sw.SyncNodeStats(ctx, node, "sk-test", server.URL+"/v1")
		require.NoError(t, err)
		assert.NotNil(t, grants)
		assert.Equal(t, "subscription", grants.Object)
		assert.InDelta(t, 3.0, grants.TotalAvailable, 0.001)
		assert.InDelta(t, 200.0, grants.TotalGranted, 0.001)
		assert.InDelta(t, 197.0, grants.TotalUsed, 0.001)
	})
}

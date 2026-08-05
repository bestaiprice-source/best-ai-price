package routes

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/runtime"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/session"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
	_ "modernc.org/sqlite"
)

func newTestStorage(t *testing.T) *storage.Storage {
	t.Helper()

	name := strings.ReplaceAll(t.Name(), "/", "_")
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", name)
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return storage.NewStorage(client, rdb)
}

func TestAshHealthAndStatsEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := newTestStorage(t)
	mockRt := runtime.NewMockRuntime(store)
	engine := hypervisor.NewHypervisor(store, mockRt)

	gatewayHandler := &handler.GatewayHandler{}
	gatewayHandler.SetHypervisorEngine(engine)

	h := &handler.Handlers{
		Gateway: gatewayHandler,
	}

	r := gin.New()

	dummyMiddleware := func(c *gin.Context) {
		c.Next()
	}

	RegisterAshRoutes(r, h, dummyMiddleware, dummyMiddleware)

	// Test GET /ash/health
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/ash/health", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test GET /api/v1/ash/health
	wV1 := httptest.NewRecorder()
	reqV1, _ := http.NewRequest("GET", "/api/v1/ash/health", nil)
	r.ServeHTTP(wV1, reqV1)
	assert.Equal(t, http.StatusOK, wV1.Code)

	// Test GET /ash/stats
	wStats := httptest.NewRecorder()
	reqStats, _ := http.NewRequest("GET", "/ash/stats", nil)
	r.ServeHTTP(wStats, reqStats)
	assert.Equal(t, http.StatusOK, wStats.Code)
	var statsBody map[string]any
	require.NoError(t, json.Unmarshal(wStats.Body.Bytes(), &statsBody))
	metricsBody, ok := statsBody["metrics"].(map[string]any)
	require.True(t, ok)
	for _, name := range []string{
		"requests_total", "requests_failed_total", "upstream_429_total",
		"rebinding_total", "rebinding_failed_total", "node_state_count",
		"queue_depth", "latency_ms", "usage_units_total", "active_sessions",
		"pool_utilization", "summary_jobs_total", "snapshot_total",
	} {
		assert.Contains(t, metricsBody, name)
	}

	wUserStats := httptest.NewRecorder()
	reqUserStats, _ := http.NewRequest("GET", "/ash/stats/users/42", nil)
	r.ServeHTTP(wUserStats, reqUserStats)
	assert.Equal(t, http.StatusOK, wUserStats.Code)
	for _, field := range []string{"remaining_fair_use", "active_sessions", "rebinding_count", "last_errors", "deposit_state"} {
		assert.Contains(t, wUserStats.Body.String(), field)
	}

	// Test GET /ash/stats/nodes
	wNodes := httptest.NewRecorder()
	reqNodes, _ := http.NewRequest("GET", "/ash/stats/nodes", nil)
	r.ServeHTTP(wNodes, reqNodes)
	assert.Equal(t, http.StatusOK, wNodes.Code)

	// Test GET /ash/stats/analytics
	wAnalytics := httptest.NewRecorder()
	reqAnalytics, _ := http.NewRequest("GET", "/ash/stats/analytics", nil)
	r.ServeHTTP(wAnalytics, reqAnalytics)
	assert.Equal(t, http.StatusOK, wAnalytics.Code)
	for _, field := range []string{"top_users", "heavy_users", "unused_capacity_units", "upstream_429_rate", "revenue_estimate_cents"} {
		assert.Contains(t, wAnalytics.Body.String(), field)
	}

	// Test POST /ash/admin/deposits
	wDepPost := httptest.NewRecorder()
	reqDepPost, _ := http.NewRequest("POST", "/ash/admin/deposits", strings.NewReader(`{"owner_id":"user-dep-1","amount_cents":2500,"currency":"USD","notice_days":10}`))
	reqDepPost.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(wDepPost, reqDepPost)
	assert.Equal(t, http.StatusOK, wDepPost.Code)

	// Test GET /ash/admin/deposits
	wDepGet := httptest.NewRecorder()
	reqDepGet, _ := http.NewRequest("GET", "/ash/admin/deposits?owner_id=user-dep-1", nil)
	r.ServeHTTP(wDepGet, reqDepGet)
	assert.Equal(t, http.StatusOK, wDepGet.Code)
}

func TestAshAnalyticsRequiresAdminAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := newTestStorage(t)
	engine := hypervisor.NewHypervisor(store, runtime.NewMockRuntime(store))
	gatewayHandler := &handler.GatewayHandler{}
	gatewayHandler.SetHypervisorEngine(engine)
	h := &handler.Handlers{Gateway: gatewayHandler}

	jwtCalls := 0
	adminCalls := 0
	jwtAuth := func(c *gin.Context) {
		jwtCalls++
		c.Next()
	}
	adminAuth := func(c *gin.Context) {
		adminCalls++
		c.AbortWithStatus(http.StatusUnauthorized)
	}

	r := gin.New()
	RegisterAshRoutes(r, h, jwtAuth, adminAuth)

	for _, path := range []string{"/ash/stats/analytics", "/api/v1/ash/stats/analytics"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code, path)
	}
	assert.Equal(t, 0, jwtCalls)
	assert.Equal(t, 2, adminCalls)

	// Health remains intentionally public.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/ash/health", nil))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 2, adminCalls)
}

func TestAshSessionSnapshotAndRestoreAcceptance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestStorage(t)
	engine := hypervisor.NewHypervisor(store, runtime.NewMockRuntime(store))
	gatewayHandler := &handler.GatewayHandler{}
	gatewayHandler.SetHypervisorEngine(engine)

	sess := &session.VirtualSession{
		ID: "session-42-recovery", OwnerID: "42", ConversationID: "recovery",
		State: "active", Version: 9,
		Context: &session.ExecutionContext{
			Summary:        "durable summary",
			RecentMessages: []session.Message{{Role: "user", Content: "durable message"}},
			PinnedFacts:    []string{"durable fact"},
		},
	}
	require.NoError(t, session.NewSessionManager(store).SaveSession(context.Background(), sess))

	jwtAuth := func(c *gin.Context) {
		c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 42})
		c.Next()
	}
	pass := func(c *gin.Context) { c.Next() }
	r := gin.New()
	RegisterAshRoutes(r, &handler.Handlers{Gateway: gatewayHandler}, jwtAuth, pass)

	snapshotBody := `{"workspace_id":"ws-42","git_branch":"main","git_commit":"abc","snapshot_ref":"snap-42"}`
	wSnapshot := httptest.NewRecorder()
	reqSnapshot := httptest.NewRequest(http.MethodPost, "/api/v1/ash/me/sessions/session-42-recovery/snapshot", strings.NewReader(snapshotBody))
	reqSnapshot.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(wSnapshot, reqSnapshot)
	require.Equal(t, http.StatusCreated, wSnapshot.Code, wSnapshot.Body.String())
	assert.Contains(t, wSnapshot.Body.String(), "durable summary")

	// Remove both primary copies to prove recovery comes from the snapshot.
	require.NoError(t, store.Redis().Del(context.Background(), storage.SessionKey(sess.ID)).Err())
	require.NoError(t, store.DB().AshSession.DeleteOneID(sess.ID).Exec(context.Background()))

	wRestore := httptest.NewRecorder()
	r.ServeHTTP(wRestore, httptest.NewRequest(http.MethodPost, "/api/v1/ash/me/sessions/session-42-recovery/restore", nil))
	require.Equal(t, http.StatusOK, wRestore.Code, wRestore.Body.String())
	assert.Contains(t, wRestore.Body.String(), "durable summary")
	assert.Contains(t, wRestore.Body.String(), "durable message")

	restored, err := session.NewSessionManager(store).GetSession(context.Background(), sess.ID)
	require.NoError(t, err)
	require.NotNil(t, restored.Context)
	assert.Equal(t, "durable fact", restored.Context.PinnedFacts[0])

	events, err := store.DB().AshEvent.Query().All(context.Background())
	require.NoError(t, err)
	var created, restoredEvent bool
	for _, event := range events {
		created = created || event.EventType == "SnapshotCreated"
		restoredEvent = restoredEvent || event.EventType == "SnapshotRestored"
	}
	assert.True(t, created)
	assert.True(t, restoredEvent)
}

package routes

import (
	"database/sql"
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
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/runtime"
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

	// Test GET /ash/stats/nodes
	wNodes := httptest.NewRecorder()
	reqNodes, _ := http.NewRequest("GET", "/ash/stats/nodes", nil)
	r.ServeHTTP(wNodes, reqNodes)
	assert.Equal(t, http.StatusOK, wNodes.Code)
}

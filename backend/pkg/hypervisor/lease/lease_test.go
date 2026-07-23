package lease

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
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

func TestLeaseManagerSaveGetDelete(t *testing.T) {
	store := newTestStorage(t)
	lm := NewLeaseManager(store)

	expiresAt := time.Now().Add(10 * time.Minute)
	l := &Lease{
		LeaseID:   "lease-1",
		SessionID: "sess-1",
		NodeID:    "node-1",
		GrantedAt: time.Now(),
		ExpiresAt: &expiresAt,
	}

	err := lm.SaveLease(context.Background(), l)
	require.NoError(t, err)

	fetched, err := lm.GetLease(context.Background(), "sess-1")
	require.NoError(t, err)
	assert.Equal(t, "lease-1", fetched.LeaseID)
	assert.Equal(t, "node-1", fetched.NodeID)

	err = lm.DeleteLease(context.Background(), "sess-1")
	require.NoError(t, err)

	_, err = lm.GetLease(context.Background(), "sess-1")
	assert.Error(t, err)
}

func TestLeaseManagerExpiredLease(t *testing.T) {
	store := newTestStorage(t)
	lm := NewLeaseManager(store)

	expiredTime := time.Now().Add(-1 * time.Minute)
	l := &Lease{
		LeaseID:   "lease-expired",
		SessionID: "sess-2",
		NodeID:    "node-2",
		GrantedAt: time.Now().Add(-10 * time.Minute),
		ExpiresAt: &expiredTime,
	}

	err := lm.SaveLease(context.Background(), l)
	require.NoError(t, err)

	_, err = lm.GetLease(context.Background(), "sess-2")
	assert.Error(t, err)
}

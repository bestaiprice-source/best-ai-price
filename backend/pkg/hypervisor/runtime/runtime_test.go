package runtime

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

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

func TestMockRuntimeSuccessAndFailures(t *testing.T) {
	store := newTestStorage(t)
	mockRt := NewMockRuntime(store)

	var buf bytes.Buffer
	res, err := mockRt.Execute(context.Background(), "node-1", []byte(`{"prompt":"hi"}`), &buf)
	require.NoError(t, err)
	assert.NotNil(t, res)
	assert.Contains(t, buf.String(), "message_start")

	// Force 429 scenario
	mockRt.Force429 = true
	_, err = mockRt.Execute(context.Background(), "node-1", []byte(`{}`), &buf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "429")

	// Force disconnect scenario
	mockRt.Force429 = false
	mockRt.ForceDisconnect = true
	_, err = mockRt.Execute(context.Background(), "node-1", []byte(`{}`), &buf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "closed abruptly")
}

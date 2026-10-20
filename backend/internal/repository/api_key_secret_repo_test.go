//go:build unit

package repository

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func newAPIKeyRepoWithSQLMock(t *testing.T) (*apiKeyRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return newAPIKeyRepositoryWithSQL(nil, db), mock
}

func TestAPIKeyRepo_SetEncryptedKey(t *testing.T) {
	repo, mock := newAPIKeyRepoWithSQLMock(t)

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE api_keys SET key_encrypted = $1, updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL`)).
		WithArgs("cipher-text", int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, repo.SetEncryptedKey(context.Background(), 5, "cipher-text"))
	require.NoError(t, mock.ExpectationsWereMet())
}

// Legacy rows (NULL / empty) must report not-found rather than an error.
func TestAPIKeyRepo_GetEncryptedKey(t *testing.T) {
	repo, mock := newAPIKeyRepoWithSQLMock(t)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT key_encrypted FROM api_keys WHERE id = $1 AND deleted_at IS NULL`)).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"key_encrypted"}).AddRow("cipher-text"))

	got, found, err := repo.GetEncryptedKey(context.Background(), 5)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "cipher-text", got)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT key_encrypted FROM api_keys WHERE id = $1 AND deleted_at IS NULL`)).
		WithArgs(int64(6)).
		WillReturnRows(sqlmock.NewRows([]string{"key_encrypted"}).AddRow(nil))

	got, found, err = repo.GetEncryptedKey(context.Background(), 6)
	require.NoError(t, err)
	assert.False(t, found, "a NULL column means a legacy hash-only key")
	assert.Empty(t, got)

	require.NoError(t, mock.ExpectationsWereMet())
}

// Only keys with a stored ciphertext are flagged as revealable.
func TestAPIKeyRepo_MarkKeysWithSecret(t *testing.T) {
	repo, mock := newAPIKeyRepoWithSQLMock(t)

	keys := []*service.APIKey{{ID: 1}, {ID: 2}, {ID: 3}}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM api_keys WHERE deleted_at IS NULL AND key_encrypted IS NOT NULL AND key_encrypted <> '' AND id IN ($1, $2, $3)`)).
		WithArgs(int64(1), int64(2), int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(2)))

	require.NoError(t, repo.MarkKeysWithSecret(context.Background(), keys))
	assert.False(t, keys[0].HasFullKey)
	assert.True(t, keys[1].HasFullKey)
	assert.False(t, keys[2].HasFullKey)

	// No keys -> no query at all.
	require.NoError(t, repo.MarkKeysWithSecret(context.Background(), nil))
	require.NoError(t, mock.ExpectationsWereMet())
}

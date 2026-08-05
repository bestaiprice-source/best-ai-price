package repository

import (
	"context"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/ent/apikey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyRepositoryHashesKeyAtRestAndAuthenticatesPlaintext(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "hash-at-rest@test.local")
	plaintext := "sk-secret-value-that-must-not-be-stored"
	key := &service.APIKey{
		UserID: user.ID,
		Key:    plaintext,
		Name:   "hashed",
		Status: service.StatusActive,
	}

	require.NoError(t, repo.Create(ctx, key))
	require.Equal(t, plaintext, key.Key, "creator must receive the one-time cleartext")

	stored, err := client.APIKey.Query().Where(apikey.IDEQ(key.ID)).Only(ctx)
	require.NoError(t, err)
	require.NotEqual(t, plaintext, stored.Key)
	require.True(t, strings.HasPrefix(stored.Key, "sha256:sk-secre:"), stored.Key)
	require.NotContains(t, stored.Key, "value-that-must-not-be-stored")

	authenticated, err := repo.GetByKeyForAuth(ctx, plaintext)
	require.NoError(t, err)
	require.Equal(t, key.ID, authenticated.ID)

	exists, err := repo.ExistsByKey(ctx, plaintext)
	require.NoError(t, err)
	require.True(t, exists)

	_, err = repo.GetByKeyForAuth(ctx, plaintext+"-wrong")
	require.ErrorIs(t, err, service.ErrAPIKeyNotFound)
}

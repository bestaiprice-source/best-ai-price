package repository

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/credentials"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func TestAccountRepositoryEncryptsUpstreamCredentialsAtRest(t *testing.T) {
	db, err := sql.Open("sqlite", "file:account_credentials_at_rest?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(entsql.OpenDB(dialect.SQLite, db))))
	t.Cleanup(func() { _ = client.Close() })

	encryptor := &AESEncryptor{key: []byte("0123456789abcdef0123456789abcdef")}
	repo := newAccountRepositoryWithSQL(client, db, nil, encryptor)
	account := &service.Account{
		Name:        "encrypted-openai",
		Platform:    "openai",
		Type:        "oauth",
		Status:      "active",
		Concurrency: 1,
		Priority:    1,
		Credentials: map[string]any{
			"access_token":       "access-plaintext",
			"client_secret":      "client-plaintext",
			"base_url":           "https://example.test",
			"chatgpt_account_id": "account-id",
		},
	}

	require.NoError(t, repo.Create(context.Background(), account))
	require.Equal(t, "access-plaintext", account.Credentials["access_token"], "service model must retain plaintext for immediate use")

	stored, err := client.Account.Get(context.Background(), account.ID)
	require.NoError(t, err)
	for _, key := range []string{"access_token", "client_secret"} {
		value, ok := stored.Credentials[key].(string)
		require.True(t, ok)
		require.True(t, strings.HasPrefix(value, credentials.EnvelopePrefix), value)
		require.NotContains(t, value, "plaintext")
	}
	require.Equal(t, "https://example.test", stored.Credentials["base_url"])
	require.Equal(t, "account-id", stored.Credentials["chatgpt_account_id"])

	imported := &service.Account{Name: "imported-yunwu", Platform: "yunwu", Type: "api_key", Status: "active", Concurrency: 1, Priority: 1, Credentials: map[string]any{"api_key": "import-plaintext", "base_url": "https://yunwu.test"}}
	require.NoError(t, repo.CreateImportPlaceholders(context.Background(), []*service.Account{imported}))
	storedImported, err := client.Account.Get(context.Background(), imported.ID)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(storedImported.Credentials["api_key"].(string), credentials.EnvelopePrefix))
	require.NotContains(t, storedImported.Credentials["api_key"], "import-plaintext")
}

func TestAccountRepositoryMigratesLegacyPlaintextCredentials(t *testing.T) {
	db, err := sql.Open("sqlite", "file:account_credentials_migration?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(entsql.OpenDB(dialect.SQLite, db))))
	t.Cleanup(func() { _ = client.Close() })

	legacy, err := client.Account.Create().
		SetName("legacy-claude").SetPlatform("anthropic").SetType("oauth").
		SetCredentials(map[string]any{"access_token": "legacy-plaintext", "base_url": "https://example.test"}).
		Save(context.Background())
	require.NoError(t, err)

	repo := newAccountRepositoryWithSQL(client, db, nil, &AESEncryptor{key: []byte("0123456789abcdef0123456789abcdef")})
	require.NoError(t, repo.encryptLegacyCredentials(context.Background()))
	stored, err := client.Account.Get(context.Background(), legacy.ID)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(stored.Credentials["access_token"].(string), credentials.EnvelopePrefix))
	require.NotContains(t, stored.Credentials["access_token"], "legacy-plaintext")
	require.Equal(t, "https://example.test", stored.Credentials["base_url"])
}

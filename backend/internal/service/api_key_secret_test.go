//go:build unit

package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// reversibleEncryptor is a tiny SecretEncryptor stand-in for tests.
type reversibleEncryptor struct{}

func (reversibleEncryptor) Encrypt(p string) (string, error) { return "enc:" + p, nil }
func (reversibleEncryptor) Decrypt(c string) (string, error) {
	if !strings.HasPrefix(c, "enc:") {
		return "", errors.New("bad ciphertext")
	}
	return strings.TrimPrefix(c, "enc:"), nil
}

// revealRepoFake satisfies APIKeyRepository by embedding the interface (unused
// methods panic if unexpectedly called) and implements the optional secret
// store capability.
type revealRepoFake struct {
	APIKeyRepository
	key       *APIKey
	cipher    string
	hasSecret bool
	setCalls  int
	marked    int
}

func (f *revealRepoFake) GetByID(_ context.Context, _ int64) (*APIKey, error) {
	return f.key, nil
}

func (f *revealRepoFake) SetEncryptedKey(_ context.Context, _ int64, encrypted string) error {
	f.setCalls++
	f.cipher = encrypted
	f.hasSecret = true
	return nil
}

func (f *revealRepoFake) GetEncryptedKey(_ context.Context, _ int64) (string, bool, error) {
	return f.cipher, f.hasSecret, nil
}

func (f *revealRepoFake) MarkKeysWithSecret(_ context.Context, keys []*APIKey) error {
	for _, k := range keys {
		if k != nil && f.hasSecret {
			k.HasFullKey = true
			f.marked++
		}
	}
	return nil
}

func TestSecretEncryptorDefaultSetter(t *testing.T) {
	SetDefaultSecretEncryptor(nil)
	assert.Nil(t, GetDefaultSecretEncryptor())
	SetDefaultSecretEncryptor(reversibleEncryptor{})
	t.Cleanup(func() { SetDefaultSecretEncryptor(nil) })
	assert.NotNil(t, GetDefaultSecretEncryptor())
}

// A newly created key gets an encrypted (recoverable) copy and is revealable.
func TestPersistAndRevealEncryptedKey(t *testing.T) {
	SetDefaultSecretEncryptor(reversibleEncryptor{})
	t.Cleanup(func() { SetDefaultSecretEncryptor(nil) })

	repo := &revealRepoFake{key: &APIKey{ID: 7, UserID: 42, Key: "sha256:sk-abc:deadbeef"}}
	svc := &APIKeyService{apiKeyRepo: repo}

	svc.persistEncryptedKey(context.Background(), &APIKey{ID: 7, Key: "sk-plain-value"})

	require.Equal(t, 1, repo.setCalls)
	assert.Equal(t, "enc:sk-plain-value", repo.cipher, "must store the encrypted plaintext")

	repo.key = &APIKey{ID: 7, UserID: 42, HasFullKey: true}
	plain, err := svc.RevealKey(context.Background(), 7, 42, false)
	require.NoError(t, err)
	assert.Equal(t, "sk-plain-value", plain)
}

// Hash-only (legacy) keys must report not-recoverable, never an error that
// leaks stored material.
func TestRevealLegacyHashOnlyKey_NotRecoverable(t *testing.T) {
	SetDefaultSecretEncryptor(reversibleEncryptor{})
	t.Cleanup(func() { SetDefaultSecretEncryptor(nil) })

	repo := &revealRepoFake{key: &APIKey{ID: 9, UserID: 42}}
	svc := &APIKeyService{apiKeyRepo: repo}

	_, err := svc.RevealKey(context.Background(), 9, 42, false)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAPIKeyNotRecoverable)
}

// Another user's key must not be revealable (and must not disclose existence).
func TestRevealForeignKeyNotFound(t *testing.T) {
	SetDefaultSecretEncryptor(reversibleEncryptor{})
	t.Cleanup(func() { SetDefaultSecretEncryptor(nil) })

	repo := &revealRepoFake{key: &APIKey{ID: 3, UserID: 1}, cipher: "enc:secret", hasSecret: true}
	svc := &APIKeyService{apiKeyRepo: repo}

	_, err := svc.RevealKey(context.Background(), 3, 999, false)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAPIKeyNotFound)

	// An admin may reveal any key.
	plain, err := svc.RevealKey(context.Background(), 3, 999, true)
	require.NoError(t, err)
	assert.Equal(t, "secret", plain)
}

// persistEncryptedKey must skip hash-only values (never encrypt a hash) and be
// a no-op when no encryptor is installed.
func TestPersistEncryptedKeySkipsNonPlaintext(t *testing.T) {
	SetDefaultSecretEncryptor(reversibleEncryptor{})
	t.Cleanup(func() { SetDefaultSecretEncryptor(nil) })

	repo := &revealRepoFake{}
	svc := &APIKeyService{apiKeyRepo: repo}

	svc.persistEncryptedKey(context.Background(), &APIKey{ID: 1, Key: "sha256:sk-x:abc"})
	assert.Zero(t, repo.setCalls, "a hash must never be stored as the recoverable value")

	repo.setCalls = 0
	svc.persistEncryptedKey(context.Background(), nil)
	svc.persistEncryptedKey(context.Background(), &APIKey{ID: 0, Key: "sk-x"})
	assert.Zero(t, repo.setCalls)

	SetDefaultSecretEncryptor(nil)
	svc.persistEncryptedKey(context.Background(), &APIKey{ID: 5, Key: "sk-plain"})
	assert.Zero(t, repo.setCalls, "without an encryptor keys stay hash-only")
}

// List/GetByID annotate keys with the recoverable flag.
func TestMarkHasFullKey(t *testing.T) {
	repo := &revealRepoFake{hasSecret: true}
	svc := &APIKeyService{apiKeyRepo: repo}

	keys := []*APIKey{{ID: 1}, {ID: 2}}
	svc.markHasFullKey(context.Background(), keys)
	for _, k := range keys {
		assert.True(t, k.HasFullKey)
	}

	// Repositories without the capability must not panic.
	svcNoCap := &APIKeyService{apiKeyRepo: nil}
	svcNoCap.markHasFullKey(context.Background(), keys)
	_ = svcNoCap
}

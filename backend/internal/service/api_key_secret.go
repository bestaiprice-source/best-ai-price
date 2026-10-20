package service

import (
	"context"
	"log/slog"
	"strings"
	"sync"
)

// apiKeySecretStore is an optional capability of an APIKeyRepository: it stores
// and reads a recoverable (AES-encrypted) copy of a key. Repositories that do
// not implement it keep working — keys simply stay hash-only (not revealable).
//
// Authentication never uses this data: it always resolves keys through the
// deterministic hash in api_keys.key, so enabling recoverable storage cannot
// break keys that were created earlier.
type apiKeySecretStore interface {
	SetEncryptedKey(ctx context.Context, id int64, encrypted string) error
	GetEncryptedKey(ctx context.Context, id int64) (string, bool, error)
	MarkKeysWithSecret(ctx context.Context, keys []*APIKey) error
}

var (
	defaultSecretEncryptorMu sync.RWMutex
	defaultSecretEncryptor   SecretEncryptor
)

// SetDefaultSecretEncryptor installs the process-wide secret encryptor used to
// keep the recoverable copy of API keys (mirrors SetDefaultAdminAuditWriter).
// The key material is derived from configuration, so stored keys stay
// revealable across restarts and deploys.
func SetDefaultSecretEncryptor(e SecretEncryptor) {
	defaultSecretEncryptorMu.Lock()
	defaultSecretEncryptor = e
	defaultSecretEncryptorMu.Unlock()
}

// GetDefaultSecretEncryptor returns the process-wide encryptor (may be nil).
func GetDefaultSecretEncryptor() SecretEncryptor {
	defaultSecretEncryptorMu.RLock()
	defer defaultSecretEncryptorMu.RUnlock()
	return defaultSecretEncryptor
}

// persistEncryptedKey stores the recoverable copy of a freshly created key.
// Best-effort: on failure the key stays hash-only (irrecoverable) but valid.
func (s *APIKeyService) persistEncryptedKey(ctx context.Context, key *APIKey) {
	if key == nil || key.ID == 0 || key.Key == "" || strings.HasPrefix(key.Key, "sha256:") {
		return
	}
	store, ok := s.apiKeyRepo.(apiKeySecretStore)
	if !ok {
		return
	}
	enc := GetDefaultSecretEncryptor()
	if enc == nil {
		return
	}
	ciphertext, err := enc.Encrypt(key.Key)
	if err != nil {
		slog.Warn("[APIKey] failed to encrypt key for recoverable storage", "key_id", key.ID, "error", err)
		return
	}
	if err := store.SetEncryptedKey(ctx, key.ID, ciphertext); err != nil {
		slog.Warn("[APIKey] failed to persist encrypted key", "key_id", key.ID, "error", err)
		return
	}
	key.HasFullKey = true
}

// markHasFullKey annotates keys with whether a recoverable copy exists.
// Best-effort: on error keys simply report false (not revealable).
func (s *APIKeyService) markHasFullKey(ctx context.Context, keys []*APIKey) {
	store, ok := s.apiKeyRepo.(apiKeySecretStore)
	if !ok || len(keys) == 0 {
		return
	}
	if err := store.MarkKeysWithSecret(ctx, keys); err != nil {
		slog.Warn("[APIKey] failed to resolve recoverable-key flags", "error", err)
	}
}

// RevealKey returns the plaintext value of a key to its owner (or an admin).
// Legacy hash-only keys return ErrAPIKeyNotRecoverable.
func (s *APIKeyService) RevealKey(ctx context.Context, id, actorUserID int64, isAdmin bool) (string, error) {
	key, err := s.apiKeyRepo.GetByID(ctx, id)
	if err != nil {
		return "", err
	}
	if key == nil {
		return "", ErrAPIKeyNotFound
	}
	if !isAdmin && key.UserID != actorUserID {
		// Never disclose the existence of another user's key.
		return "", ErrAPIKeyNotFound
	}
	store, ok := s.apiKeyRepo.(apiKeySecretStore)
	if !ok {
		return "", ErrAPIKeyNotRecoverable
	}
	ciphertext, found, err := store.GetEncryptedKey(ctx, id)
	if err != nil {
		return "", err
	}
	if !found || ciphertext == "" {
		return "", ErrAPIKeyNotRecoverable
	}
	enc := GetDefaultSecretEncryptor()
	if enc == nil {
		return "", ErrAPIKeyNotRecoverable
	}
	plaintext, err := enc.Decrypt(ciphertext)
	if err != nil {
		slog.Warn("[APIKey] failed to decrypt stored key", "key_id", id, "error", err)
		return "", ErrAPIKeyNotRecoverable
	}
	return plaintext, nil
}

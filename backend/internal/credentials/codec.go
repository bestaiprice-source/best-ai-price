package credentials

import (
	"fmt"
	"strings"
)

// EnvelopePrefix identifies account credential values encrypted by the application.
const EnvelopePrefix = "enc:v1:"

// Cipher is implemented by the application's AES-256-GCM encryptor.
type Cipher interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

var sensitiveKeys = map[string]struct{}{
	"access_token":  {},
	"api_key":       {},
	"cookie":        {},
	"password":      {},
	"refresh_token": {},
	"secret":        {},
	"session_key":   {},
	"setup_token":   {},
	"token":         {},
	"key":           {},
}

// EncryptMap returns a deep copy in which sensitive string values are encrypted.
func EncryptMap(values map[string]any, cipher Cipher) (map[string]any, error) {
	if values == nil {
		return nil, nil
	}
	if cipher == nil {
		return nil, fmt.Errorf("credential encryptor is required")
	}
	value, err := transformMap(values, cipher, true)
	if err != nil {
		return nil, err
	}
	return value, nil
}

// DecryptMap returns a deep copy and decrypts every recognized envelope. Legacy
// plaintext values remain readable during the rolling data migration.
func DecryptMap(values map[string]any, cipher Cipher) (map[string]any, error) {
	if values == nil {
		return nil, nil
	}
	return transformMap(values, cipher, false)
}

// NeedsEncryption reports whether a map still contains a legacy plaintext
// sensitive string. It is used for the online migration of existing rows.
func NeedsEncryption(values map[string]any) bool {
	for key, value := range values {
		if needsEncryptionValue(key, value) {
			return true
		}
	}
	return false
}

func needsEncryptionValue(key string, value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		return NeedsEncryption(typed)
	case []any:
		for _, item := range typed {
			if needsEncryptionValue(key, item) {
				return true
			}
		}
	case string:
		return isSensitiveKey(key) && typed != "" && !strings.HasPrefix(typed, EnvelopePrefix)
	}
	return false
}

func transformMap(values map[string]any, cipher Cipher, encrypt bool) (map[string]any, error) {
	out := make(map[string]any, len(values))
	for key, value := range values {
		transformed, err := transformValue(key, value, cipher, encrypt)
		if err != nil {
			return nil, fmt.Errorf("credential %q: %w", key, err)
		}
		out[key] = transformed
	}
	return out, nil
}

func transformValue(key string, value any, cipher Cipher, encrypt bool) (any, error) {
	switch typed := value.(type) {
	case map[string]any:
		return transformMap(typed, cipher, encrypt)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			transformed, err := transformValue(key, item, cipher, encrypt)
			if err != nil {
				return nil, err
			}
			out[i] = transformed
		}
		return out, nil
	case string:
		if encrypt {
			if !isSensitiveKey(key) || typed == "" || strings.HasPrefix(typed, EnvelopePrefix) {
				return typed, nil
			}
			encrypted, err := cipher.Encrypt(typed)
			if err != nil {
				return nil, err
			}
			return EnvelopePrefix + encrypted, nil
		}
		if !strings.HasPrefix(typed, EnvelopePrefix) {
			return typed, nil
		}
		if cipher == nil {
			return nil, fmt.Errorf("credential decryptor is required")
		}
		return cipher.Decrypt(strings.TrimPrefix(typed, EnvelopePrefix))
	default:
		return value, nil
	}
}

func isSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	if _, ok := sensitiveKeys[normalized]; ok {
		return true
	}
	for _, suffix := range []string{"_token", "_key", "_secret", "_password", "_cookie"} {
		if strings.HasSuffix(normalized, suffix) {
			return true
		}
	}
	return false
}

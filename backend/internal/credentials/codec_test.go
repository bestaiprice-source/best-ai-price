package credentials

import (
	"errors"
	"strings"
	"testing"
)

type testCipher struct{}

func (testCipher) Encrypt(value string) (string, error) { return "cipher(" + value + ")", nil }
func (testCipher) Decrypt(value string) (string, error) {
	if !strings.HasPrefix(value, "cipher(") || !strings.HasSuffix(value, ")") {
		return "", errors.New("invalid ciphertext")
	}
	return strings.TrimSuffix(strings.TrimPrefix(value, "cipher("), ")"), nil
}

func TestCredentialCodecEncryptsSecretsOnlyAndRoundTrips(t *testing.T) {
	original := map[string]any{
		"api_key":            "sk-secret",
		"base_url":           "https://example.test",
		"chatgpt_account_id": "account-id",
		"nested": map[string]any{
			"refresh_token": "refresh-secret",
		},
	}

	encrypted, err := EncryptMap(original, testCipher{})
	if err != nil {
		t.Fatal(err)
	}
	if encrypted["api_key"] == original["api_key"] {
		t.Fatal("api_key remained plaintext")
	}
	if encrypted["base_url"] != original["base_url"] || encrypted["chatgpt_account_id"] != original["chatgpt_account_id"] {
		t.Fatal("routing metadata must remain plaintext")
	}
	if NeedsEncryption(encrypted) {
		t.Fatal("encrypted map was classified as legacy plaintext")
	}

	again, err := EncryptMap(encrypted, testCipher{})
	if err != nil {
		t.Fatal(err)
	}
	if again["api_key"] != encrypted["api_key"] {
		t.Fatal("envelope was encrypted twice")
	}

	decrypted, err := DecryptMap(encrypted, testCipher{})
	if err != nil {
		t.Fatal(err)
	}
	if decrypted["api_key"] != "sk-secret" {
		t.Fatalf("unexpected decrypted api_key: %v", decrypted["api_key"])
	}
	nested := decrypted["nested"].(map[string]any)
	if nested["refresh_token"] != "refresh-secret" {
		t.Fatalf("unexpected nested refresh token: %v", nested["refresh_token"])
	}
}

func TestCredentialCodecFailsClosedWithoutDecryptor(t *testing.T) {
	_, err := DecryptMap(map[string]any{"token": EnvelopePrefix + "cipher(secret)"}, nil)
	if err == nil {
		t.Fatal("encrypted credential was accepted without a decryptor")
	}
}

package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuthProvidersService_SaveAndRead(t *testing.T) {
	tempDir := t.TempDir()
	testPath1 := filepath.Join(tempDir, "p1", "auth_providers.json")
	testPath2 := filepath.Join(tempDir, "p2", "auth_providers.json")

	svc := NewAuthProvidersServiceWithPaths([]string{testPath1, testPath2})

	// Initial Read should return NotExist
	_, err := svc.Read()
	require.ErrorIs(t, err, os.ErrNotExist)

	// Save Invalid JSON should fail
	err = svc.Save([]byte("not valid json"))
	require.ErrorIs(t, err, ErrInvalidJSON)

	// Save Empty payload should fail
	err = svc.Save([]byte(""))
	require.ErrorIs(t, err, ErrInvalidJSON)

	// Save Valid JSON
	validJSON := []byte(`{"providers":[{"id":"openai","name":"OpenAI"}]}`)
	err = svc.Save(validJSON)
	require.NoError(t, err)

	// Read should return valid content
	content, err := svc.Read()
	require.NoError(t, err)
	require.JSONEq(t, string(validJSON), string(content))

	// Check files actually exist
	data1, err := os.ReadFile(testPath1)
	require.NoError(t, err)
	require.JSONEq(t, string(validJSON), string(data1))

	data2, err := os.ReadFile(testPath2)
	require.NoError(t, err)
	require.JSONEq(t, string(validJSON), string(data2))
}

func TestAuthProvidersService_DefaultService(t *testing.T) {
	svc := NewAuthProvidersService()
	require.NotNil(t, svc)
	require.Equal(t, DefaultAuthProvidersPaths, svc.getPaths())
}

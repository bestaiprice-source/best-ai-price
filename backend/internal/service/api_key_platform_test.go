package service

import (
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestGenerateKeyForPlatform(t *testing.T) {
	cfg := &config.Config{
		Default: config.DefaultConfig{
			APIKeyPrefix: "sk-",
		},
	}
	service := &APIKeyService{cfg: cfg}

	// Test Standard Key Generation
	stdKey, err := service.GenerateKeyForPlatform("claude")
	assert.NoError(t, err)
	assert.True(t, strings.HasPrefix(stdKey, "sk-"))

	// Test Yunwu Key Generation maintains standard visual format
	yunwuKey, err := service.GenerateKeyForPlatform("yunwu")
	assert.NoError(t, err)
	assert.True(t, strings.HasPrefix(yunwuKey, "sk-"))
}

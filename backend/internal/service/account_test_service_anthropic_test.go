//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/gatewayctx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type anthropicAccountTestUpstream struct {
	request *http.Request
}

func (u *anthropicAccountTestUpstream) Do(req *http.Request, proxyURL string, accountID int64, concurrency int) (*http.Response, error) {
	return u.DoWithTLS(req, proxyURL, accountID, concurrency, false)
}

func (u *anthropicAccountTestUpstream) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ bool) (*http.Response, error) {
	u.request = req.Clone(context.Background())
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("data: {\\\"type\\\":\\\"message_stop\\\"}\\n\\n")),
	}, nil
}

func TestAccountTestService_ClaudeOAuthTokenStoredAsAPIKeyUsesBearerOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := newSoraTestContext()
	upstream := &anthropicAccountTestUpstream{}
	svc := &AccountTestService{httpUpstream: upstream}
	account := &Account{
		ID:          41,
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-ant-oat-valid-token"},
	}

	err := svc.testClaudeAccountConnection(gatewayctx.FromGin(ctx), account, "claude-test")
	require.NoError(t, err)
	require.NotNil(t, upstream.request)
	require.Equal(t, "Bearer sk-ant-oat-valid-token", upstream.request.Header.Get("Authorization"))
	require.Empty(t, upstream.request.Header.Get("x-api-key"))
}

func TestAccountTestService_ClaudeAPIKeyUsesXAPIKeyOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := newSoraTestContext()
	upstream := &anthropicAccountTestUpstream{}
	svc := &AccountTestService{httpUpstream: upstream}
	account := &Account{
		ID:          42,
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-ant-api-valid-key"},
	}

	err := svc.testClaudeAccountConnection(gatewayctx.FromGin(ctx), account, "claude-test")
	require.NoError(t, err)
	require.NotNil(t, upstream.request)
	require.Equal(t, "sk-ant-api-valid-key", upstream.request.Header.Get("x-api-key"))
	require.Empty(t, upstream.request.Header.Get("Authorization"))
}

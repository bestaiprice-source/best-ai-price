package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanonicalizeReturnURL(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		srcHost    string
		srcURL     string
		expectErr  bool
		errCode    string
		expectedURL string
	}{
		{
			name:        "empty raw returns empty",
			raw:         "",
			srcHost:     "bestaiprice.com",
			expectErr:   false,
			expectedURL: "",
		},
		{
			name:        "canonical root return path",
			raw:         "https://bestaiprice.com/payment/result",
			srcHost:     "bestaiprice.com",
			expectErr:   false,
			expectedURL: "https://bestaiprice.com/payment/result",
		},
		{
			name:        "reverse proxy gateway prefix return path",
			raw:         "https://bestaiprice.com/gateway/payment/result",
			srcHost:     "bestaiprice.com",
			expectErr:   false,
			expectedURL: "https://bestaiprice.com/gateway/payment/result",
		},
		{
			name:        "reverse proxy with trailing slash",
			raw:         "https://bestaiprice.com/gateway/payment/result/",
			srcHost:     "bestaiprice.com",
			expectErr:   false,
			expectedURL: "https://bestaiprice.com/gateway/payment/result/",
		},
		{
			name:      "invalid relative URL",
			raw:       "/gateway/payment/result",
			srcHost:   "bestaiprice.com",
			expectErr: true,
			errCode:   "INVALID_RETURN_URL",
		},
		{
			name:      "unsupported scheme ftp",
			raw:       "ftp://bestaiprice.com/gateway/payment/result",
			srcHost:   "bestaiprice.com",
			expectErr: true,
			errCode:   "INVALID_RETURN_URL",
		},
		{
			name:      "wrong target path",
			raw:       "https://bestaiprice.com/gateway/dashboard",
			srcHost:   "bestaiprice.com",
			expectErr: true,
			errCode:   "INVALID_RETURN_URL",
		},
		{
			name:      "disallowed host",
			raw:       "https://evil.com/gateway/payment/result",
			srcHost:   "bestaiprice.com",
			expectErr: true,
			errCode:   "INVALID_RETURN_URL",
		},
		{
			name:        "allowed via referer host",
			raw:         "https://client.com/gateway/payment/result",
			srcHost:     "backend.internal:8080",
			srcURL:      "https://client.com/gateway/purchase",
			expectErr:   false,
			expectedURL: "https://client.com/gateway/payment/result",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := CanonicalizeReturnURL(tt.raw, tt.srcHost, tt.srcURL)
			if tt.expectErr {
				require.Error(t, err)
				if tt.errCode != "" {
					assert.Contains(t, err.Error(), tt.errCode)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedURL, result)
			}
		})
	}
}

func TestBuildPaymentReturnURL(t *testing.T) {
	t.Run("appends query params preserving reverse proxy prefix", func(t *testing.T) {
		base := "https://bestaiprice.com/gateway/payment/result"
		res, err := buildPaymentReturnURL(base, 4, "out_123", "token_abc")
		require.NoError(t, err)
		assert.Contains(t, res, "https://bestaiprice.com/gateway/payment/result?")
		assert.Contains(t, res, "order_id=4")
		assert.Contains(t, res, "out_trade_no=out_123")
		assert.Contains(t, res, "resume_token=token_abc")
		assert.Contains(t, res, "status=success")
	})
}

package provider

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func TestNowPaymentsStatusMapping(t *testing.T) {
	tests := []struct {
		inputStatus    string
		expectedStatus string
	}{
		{"finished", payment.ProviderStatusPaid},
		{"confirmed", payment.ProviderStatusPaid},
		{"FINISHED", payment.ProviderStatusPaid},
		{"CONFIRMED", payment.ProviderStatusPaid},
		{"sending", payment.ProviderStatusPending},
		{"SENDING", payment.ProviderStatusPending},
		{"confirming", payment.ProviderStatusPending},
		{"waiting", payment.ProviderStatusPending},
		{"partially_paid", payment.ProviderStatusPending},
		{"failed", payment.ProviderStatusFailed},
		{"expired", payment.ProviderStatusFailed},
		{"refunded", payment.ProviderStatusRefunded},
		{"unknown_status", payment.ProviderStatusPending},
	}

	for _, tt := range tests {
		t.Run(tt.inputStatus, func(t *testing.T) {
			got := mapNowPaymentsStatus(tt.inputStatus)
			require.Equal(t, tt.expectedStatus, got)
		})
	}
}

func TestNowPaymentsVerifyNotification_Statuses(t *testing.T) {
	ipnSecret := "test_ipn_secret_12345"
	p, err := NewNowPayments("nowpayments_test", map[string]string{
		"apiKey":    "test_api_key",
		"ipnSecret": ipnSecret,
	})
	require.NoError(t, err)

	tests := []struct {
		paymentStatus string
		wantSuccess   bool
		wantStatus    string
	}{
		{paymentStatus: "finished", wantSuccess: true, wantStatus: payment.NotificationStatusSuccess},
		{paymentStatus: "confirmed", wantSuccess: true, wantStatus: payment.NotificationStatusSuccess},
		{paymentStatus: "sending", wantSuccess: false, wantStatus: payment.ProviderStatusPending},
		{paymentStatus: "confirming", wantSuccess: false, wantStatus: payment.ProviderStatusPending},
		{paymentStatus: "waiting", wantSuccess: false, wantStatus: payment.ProviderStatusPending},
		{paymentStatus: "failed", wantSuccess: false, wantStatus: payment.ProviderStatusFailed},
		{paymentStatus: "expired", wantSuccess: false, wantStatus: payment.ProviderStatusFailed},
		{paymentStatus: "refunded", wantSuccess: false, wantStatus: payment.ProviderStatusRefunded},
	}

	for _, tt := range tests {
		t.Run(tt.paymentStatus, func(t *testing.T) {
			rawBody := `{"order_id":"sub2_order_123","payment_id":998877,"payment_status":"` + tt.paymentStatus + `","price_amount":10.5}`

			// Calculate HMAC
			mac := hmac.New(sha512.New, []byte(ipnSecret))
			mac.Write([]byte(rawBody))
			sig := hex.EncodeToString(mac.Sum(nil))

			headers := map[string]string{
				"x-nowpayments-sig": sig,
			}

			notif, err := p.VerifyNotification(context.Background(), rawBody, headers)
			require.NoError(t, err)
			require.NotNil(t, notif)
			require.Equal(t, tt.wantStatus, notif.Status)
			if tt.wantSuccess {
				require.Equal(t, payment.NotificationStatusSuccess, notif.Status)
			} else {
				require.NotEqual(t, payment.NotificationStatusSuccess, notif.Status)
			}
		})
	}
}

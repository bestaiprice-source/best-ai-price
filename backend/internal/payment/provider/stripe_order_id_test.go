package provider

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	stripe "github.com/stripe/stripe-go/v85"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractStripeOrderID(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]string
		expected string
	}{
		{
			name:     "orderId key",
			metadata: map[string]string{"orderId": "ORD_12345"},
			expected: "ORD_12345",
		},
		{
			name:     "order_id key",
			metadata: map[string]string{"order_id": "ORD_67890"},
			expected: "ORD_67890",
		},
		{
			name:     "out_trade_no key",
			metadata: map[string]string{"out_trade_no": "ORD_OUT_1"},
			expected: "ORD_OUT_1",
		},
		{
			name:     "outTradeNo key",
			metadata: map[string]string{"outTradeNo": "ORD_OUT_2"},
			expected: "ORD_OUT_2",
		},
		{
			name:     "client_reference_id key",
			metadata: map[string]string{"client_reference_id": "ORD_CLI_REF"},
			expected: "ORD_CLI_REF",
		},
		{
			name:     "empty metadata",
			metadata: map[string]string{},
			expected: "",
		},
		{
			name:     "nil metadata",
			metadata: nil,
			expected: "",
		},
		{
			name:     "priority check - orderId first",
			metadata: map[string]string{"orderId": "FIRST", "order_id": "SECOND"},
			expected: "FIRST",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pi := &stripe.PaymentIntent{Metadata: tt.metadata}
			actual := extractStripeOrderID(pi)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestParseStripePaymentIntent(t *testing.T) {
	pi := stripe.PaymentIntent{
		ID:       "pi_test_123",
		Amount:   2500, // $25.00
		Metadata: map[string]string{"order_id": "ORD_TEST_999"},
	}
	raw, err := json.Marshal(pi)
	require.NoError(t, err)

	event := &stripe.Event{
		Data: &stripe.EventData{Raw: raw},
	}

	notif, err := parseStripePaymentIntent(event, payment.ProviderStatusSuccess, string(raw))
	require.NoError(t, err)
	require.NotNil(t, notif)
	assert.Equal(t, "pi_test_123", notif.TradeNo)
	assert.Equal(t, "ORD_TEST_999", notif.OrderID)
	assert.Equal(t, 25.00, notif.Amount)
	assert.Equal(t, payment.ProviderStatusSuccess, notif.Status)
}

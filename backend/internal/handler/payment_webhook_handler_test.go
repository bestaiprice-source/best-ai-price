package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/server/gatewayctx"
	"github.com/stretchr/testify/require"
)

func TestIsValidOutTradeNo(t *testing.T) {
	validCases := []string{
		"sub2_20260914aB3kX9mQ",
		"order-12345",
		"order_67890",
		"123456",
		"ABCDEF",
		"a",
		strings.Repeat("a", 64),
	}

	for _, tc := range validCases {
		require.True(t, isValidOutTradeNo(tc), "expected %q to be valid", tc)
	}

	invalidCases := []string{
		"",                                 // empty
		strings.Repeat("a", 65),            // too long (>64)
		"sub2_2026' OR 1=1--",              // SQL injection attempt
		"order; DROP TABLE payment_orders", // command injection attempt
		"<script>alert(1)</script>",        // XSS attempt
		"order with spaces",                // whitespace
		"order#123",                        // invalid character '#'
		"order$456",                        // invalid character '$'
		"order/789",                        // path traversal / slash
		"order\x00null",                    // null byte
	}

	for _, tc := range invalidCases {
		require.False(t, isValidOutTradeNo(tc), "expected %q to be invalid", tc)
	}
}

func TestExtractOutTradeNo(t *testing.T) {
	t.Run("easypay url encoded", func(t *testing.T) {
		body := "pid=1000&out_trade_no=sub2_20260914test1&trade_no=20260914001&trade_status=TRADE_SUCCESS"
		extracted := extractOutTradeNo(body, payment.TypeEasyPay)
		require.Equal(t, "sub2_20260914test1", extracted)
	})

	t.Run("alipay url encoded", func(t *testing.T) {
		body := "out_trade_no=sub2_20260914alipay1&trade_no=20260914002"
		extracted := extractOutTradeNo(body, payment.TypeAlipay)
		require.Equal(t, "sub2_20260914alipay1", extracted)
	})

	t.Run("nowpayments json", func(t *testing.T) {
		body := `{"payment_status":"finished","order_id":"sub2_20260914crypto1"}`
		extracted := extractOutTradeNo(body, payment.TypeNowPayments)
		require.Equal(t, "sub2_20260914crypto1", extracted)
	})

	t.Run("stripe metadata orderId", func(t *testing.T) {
		body := `{"id":"evt_123","data":{"object":{"id":"pi_123","metadata":{"orderId":"sub2_stripe_order_001"}}}}`
		extracted := extractOutTradeNo(body, payment.TypeStripe)
		require.Equal(t, "sub2_stripe_order_001", extracted)
	})

	t.Run("stripe metadata out_trade_no", func(t *testing.T) {
		body := `{"id":"evt_123","data":{"object":{"id":"pi_123","metadata":{"out_trade_no":"sub2_stripe_order_002"}}}}`
		extracted := extractOutTradeNo(body, payment.TypeStripe)
		require.Equal(t, "sub2_stripe_order_002", extracted)
	})

	t.Run("stripe client_reference_id fallback", func(t *testing.T) {
		body := `{"id":"evt_123","data":{"object":{"id":"cs_123","client_reference_id":"sub2_stripe_order_003"}}}`
		extracted := extractOutTradeNo(body, payment.TypeStripe)
		require.Equal(t, "sub2_stripe_order_003", extracted)
	})

	t.Run("unknown provider", func(t *testing.T) {
		body := `{"id":"evt_123"}`
		extracted := extractOutTradeNo(body, "unknown_provider")
		require.Equal(t, "", extracted)
	})
}


func TestPaymentWebhookHandler_InvalidOutTradeNoRejected(t *testing.T) {
	handler := NewPaymentWebhookHandler(nil, nil)

	body := "pid=1000&out_trade_no=sub2_%27%20OR%201=1--&trade_status=TRADE_SUCCESS"
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payment/webhook/easypay", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	ctx := gatewayctx.NewNative(req, rec, nil, "127.0.0.1:1234")

	handler.handleNotifyGateway(ctx, payment.TypeEasyPay)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "invalid order id")
}

func TestPaymentWebhookHandler_WriteSuccessResponseGateway(t *testing.T) {
	tests := []struct {
		providerKey string
		wantBody    string
		wantJSON    bool
	}{
		{providerKey: payment.TypeWxpay, wantJSON: true},
		{providerKey: payment.TypeStripe, wantBody: ""},
		{providerKey: payment.TypeNowPayments, wantBody: "OK"},
		{providerKey: payment.TypeEasyPay, wantBody: "success"},
		{providerKey: payment.TypeAlipay, wantBody: "success"},
	}

	for _, tt := range tests {
		t.Run(tt.providerKey, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			ctx := gatewayctx.NewNative(req, rec, nil, "127.0.0.1:1234")

			writeSuccessResponseGateway(ctx, tt.providerKey)

			require.Equal(t, http.StatusOK, rec.Code)
			if tt.wantJSON {
				require.Contains(t, rec.Body.String(), "SUCCESS")
			} else {
				require.Equal(t, tt.wantBody, rec.Body.String())
			}
		})
	}
}

func TestPaymentWebhookHandler_ProviderEntrypoints(t *testing.T) {
	handler := NewPaymentWebhookHandler(nil, nil)

	t.Run("easypay get query with invalid out_trade_no", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/webhook/easypay?out_trade_no=invalid%20order", nil)
		rec := httptest.NewRecorder()
		ctx := gatewayctx.NewNative(req, rec, nil, "127.0.0.1:1234")
		handler.EasyPayNotifyGateway(ctx)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("alipay post with invalid out_trade_no", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/webhook/alipay", strings.NewReader("out_trade_no=bad%20order!"))
		rec := httptest.NewRecorder()
		ctx := gatewayctx.NewNative(req, rec, nil, "127.0.0.1:1234")
		handler.AlipayNotifyGateway(ctx)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("nowpayments post with invalid order_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/webhook/nowpayments", strings.NewReader(`{"order_id":"bad order#"}`))
		rec := httptest.NewRecorder()
		ctx := gatewayctx.NewNative(req, rec, nil, "127.0.0.1:1234")
		handler.NowPaymentsWebhookGateway(ctx)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("stripe post with invalid metadata order_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/webhook/stripe", strings.NewReader(`{"data":{"object":{"metadata":{"orderId":"bad order*"}}}}`))
		rec := httptest.NewRecorder()
		ctx := gatewayctx.NewNative(req, rec, nil, "127.0.0.1:1234")
		handler.StripeWebhookGateway(ctx)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})
}



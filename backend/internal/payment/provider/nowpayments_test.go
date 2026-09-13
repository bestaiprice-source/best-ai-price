package provider

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewNowPayments_Validation(t *testing.T) {
	// Missing apiKey
	_, err := NewNowPayments("np-1", map[string]string{
		"ipnSecret": "test-secret",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "apiKey")

	// Missing ipnSecret
	_, err = NewNowPayments("np-1", map[string]string{
		"apiKey": "test-key",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ipnSecret")

	// Valid production
	np, err := NewNowPayments("np-1", map[string]string{
		"apiKey":    "test-key",
		"ipnSecret": "test-secret",
	})
	require.NoError(t, err)
	assert.Equal(t, "NOWPayments", np.Name())
	assert.Equal(t, payment.TypeNowPayments, np.ProviderKey())
	assert.Equal(t, []payment.PaymentType{payment.TypeNowPayments}, np.SupportedTypes())
	assert.Equal(t, nowpaymentsProductionBaseURL, np.baseURL)
	assert.Equal(t, "usd", np.currency)

	// Valid sandbox and custom currency
	npSandbox, err := NewNowPayments("np-2", map[string]string{
		"apiKey":    "test-key",
		"ipnSecret": "test-secret",
		"sandbox":   "true",
		"currency":  "EUR",
	})
	require.NoError(t, err)
	assert.Equal(t, nowpaymentsSandboxBaseURL, npSandbox.baseURL)
	assert.Equal(t, "eur", npSandbox.currency)
}

func TestNowPayments_CreatePayment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/invoice", r.URL.Path)
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "test-api-key", r.Header.Get("x-api-key"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var req map[string]any
		err := json.NewDecoder(r.Body).Decode(&req)
		assert.NoError(t, err)
		assert.Equal(t, 25.5, req["price_amount"])
		assert.Equal(t, "usd", req["price_currency"])
		assert.Equal(t, "order-123", req["order_id"])
		assert.Equal(t, "Test Order", req["order_description"])
		assert.Equal(t, "https://example.com/notify", req["ipn_callback_url"])

		resp := map[string]any{
			"id":          "inv-98765",
			"order_id":    "order-123",
			"invoice_url": "https://nowpayments.io/payment/?iid=inv-98765",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	np, err := NewNowPayments("np-1", map[string]string{
		"apiKey":    "test-api-key",
		"ipnSecret": "test-secret",
	})
	require.NoError(t, err)
	np.baseURL = server.URL

	resp, err := np.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:   "order-123",
		Amount:    "25.50",
		Subject:   "Test Order",
		NotifyURL: "https://example.com/notify",
		ReturnURL: "https://example.com/return",
	})
	require.NoError(t, err)
	assert.Equal(t, "inv-98765", resp.TradeNo)
	assert.Equal(t, "https://nowpayments.io/payment/?iid=inv-98765", resp.PayURL)
	assert.Equal(t, payment.CreatePaymentResultOrderCreated, resp.ResultType)
}

func TestNowPayments_CreatePayment_OptionalURLs(t *testing.T) {
	t.Run("omits empty URLs so nowpayments API does not reject empty string", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req map[string]any
			err := json.NewDecoder(r.Body).Decode(&req)
			assert.NoError(t, err)
			assert.Nil(t, req["ipn_callback_url"])
			assert.Nil(t, req["success_url"])
			assert.Nil(t, req["cancel_url"])

			resp := map[string]any{
				"id":          "inv-111",
				"invoice_url": "https://nowpayments.io/payment/?iid=inv-111",
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		np, err := NewNowPayments("np-1", map[string]string{
			"apiKey":    "test-api-key",
			"ipnSecret": "test-secret",
		})
		require.NoError(t, err)
		np.baseURL = server.URL

		resp, err := np.CreatePayment(context.Background(), payment.CreatePaymentRequest{
			OrderID: "order-empty-urls",
			Amount:  "15.00",
		})
		require.NoError(t, err)
		assert.Equal(t, "inv-111", resp.TradeNo)
		assert.Equal(t, "https://nowpayments.io/payment/?iid=inv-111", resp.PayURL)
	})

	t.Run("derives ipn_callback_url from return URL origin when notify URL is not provided", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req map[string]any
			err := json.NewDecoder(r.Body).Decode(&req)
			assert.NoError(t, err)
			assert.Equal(t, "https://sub2api.example.com/api/v1/payment/webhook/nowpayments", req["ipn_callback_url"])
			assert.Equal(t, "https://sub2api.example.com/payment/result", req["success_url"])
			assert.Equal(t, "https://sub2api.example.com/payment/result", req["cancel_url"])

			resp := map[string]any{
				"id":          "inv-222",
				"invoice_url": "https://nowpayments.io/payment/?iid=inv-222",
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		np, err := NewNowPayments("np-1", map[string]string{
			"apiKey":    "test-api-key",
			"ipnSecret": "test-secret",
		})
		require.NoError(t, err)
		np.baseURL = server.URL

		resp, err := np.CreatePayment(context.Background(), payment.CreatePaymentRequest{
			OrderID:   "order-derived-webhook",
			Amount:    "20.00",
			ReturnURL: "https://sub2api.example.com/payment/result",
		})
		require.NoError(t, err)
		assert.Equal(t, "inv-222", resp.TradeNo)
	})

	t.Run("derives ipn_callback_url preserving reverse-proxy subpath prefix", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req map[string]any
			err := json.NewDecoder(r.Body).Decode(&req)
			assert.NoError(t, err)
			assert.Equal(t, "https://bestaiprice.com/gateway/api/v1/payment/webhook/nowpayments", req["ipn_callback_url"])
			assert.Equal(t, "https://bestaiprice.com/gateway/payment/result", req["success_url"])
			assert.Equal(t, "https://bestaiprice.com/gateway/payment/result", req["cancel_url"])

			resp := map[string]any{
				"id":          "inv-prefix-1",
				"invoice_url": "https://nowpayments.io/payment/?iid=inv-prefix-1",
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		np, err := NewNowPayments("np-1", map[string]string{
			"apiKey":    "test-api-key",
			"ipnSecret": "test-secret",
		})
		require.NoError(t, err)
		np.baseURL = server.URL

		resp, err := np.CreatePayment(context.Background(), payment.CreatePaymentRequest{
			OrderID:   "order-derived-prefix-webhook",
			Amount:    "25.00",
			ReturnURL: "https://bestaiprice.com/gateway/payment/result",
		})
		require.NoError(t, err)
		assert.Equal(t, "inv-prefix-1", resp.TradeNo)
	})

	t.Run("strips resume_token and keeps URL under 255 chars", func(t *testing.T) {
		hugeToken := strings.Repeat("abcde12345", 30) // 300 chars
		rawURL := fmt.Sprintf("https://sub2api.example.com/payment/result?order_id=999&out_trade_no=202609111855&resume_token=%s&status=success", hugeToken)
		assert.Greater(t, len(rawURL), 255)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req map[string]any
			err := json.NewDecoder(r.Body).Decode(&req)
			assert.NoError(t, err)

			successURL := req["success_url"].(string)
			assert.LessOrEqual(t, len(successURL), 255)
			assert.NotContains(t, successURL, "resume_token")
			assert.Contains(t, successURL, "order_id=999")

			resp := map[string]any{
				"id":          "inv-333",
				"invoice_url": "https://nowpayments.io/payment/?iid=inv-333",
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		np, err := NewNowPayments("np-1", map[string]string{
			"apiKey":    "test-api-key",
			"ipnSecret": "test-secret",
		})
		require.NoError(t, err)
		np.baseURL = server.URL

		resp, err := np.CreatePayment(context.Background(), payment.CreatePaymentRequest{
			OrderID:   "order-huge-url",
			Amount:    "30.00",
			ReturnURL: rawURL,
		})
		require.NoError(t, err)
		assert.Equal(t, "inv-333", resp.TradeNo)
	})
}

func TestNowPayments_CreatePayment_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"bad request"}`))
	}))
	defer server.Close()

	np, err := NewNowPayments("np-1", map[string]string{
		"apiKey":    "test-api-key",
		"ipnSecret": "test-secret",
	})
	require.NoError(t, err)
	np.baseURL = server.URL

	_, err = np.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID: "order-err",
		Amount:  "10.00",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 400")

	// Invalid amount format
	_, err = np.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID: "order-bad-amt",
		Amount:  "not-a-number",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid amount")

	// Missing invoice_url in response
	serverNoURL := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "inv-1"})
	}))
	defer serverNoURL.Close()

	npNoURL, _ := NewNowPayments("np-1", map[string]string{
		"apiKey":    "test-api-key",
		"ipnSecret": "test-secret",
	})
	npNoURL.baseURL = serverNoURL.URL
	_, err = npNoURL.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID: "order-no-url",
		Amount:  "10.00",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing invoice_url")
}

func TestNowPayments_QueryOrder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/payment/pay-123" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"payment_id":     "pay-123",
				"payment_status": "finished",
				"price_amount":   30.0,
				"pay_amount":     0.001,
				"pay_currency":   "btc",
				"updated_at":     "2026-09-09T16:00:00Z",
			})
			return
		}
		if r.URL.Path == "/payment/" && r.URL.Query().Get("invoice_id") == "inv-456" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{
						"payment_id":     "pay-789",
						"payment_status": "confirmed",
						"price_amount":   45.0,
						"pay_currency":   "usdttrc20",
						"updated_at":     "2026-09-09T16:05:00Z",
					},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	np, err := NewNowPayments("np-1", map[string]string{
		"apiKey":    "test-api-key",
		"ipnSecret": "test-secret",
	})
	require.NoError(t, err)
	np.baseURL = server.URL

	// Query by payment ID
	q1, err := np.QueryOrder(context.Background(), "pay-123")
	require.NoError(t, err)
	assert.Equal(t, "pay-123", q1.TradeNo)
	assert.Equal(t, payment.ProviderStatusPaid, q1.Status)
	assert.Equal(t, 30.0, q1.Amount)
	assert.Equal(t, "btc", q1.Metadata["pay_currency"])

	// Query by invoice ID (fallback)
	q2, err := np.QueryOrder(context.Background(), "inv-456")
	require.NoError(t, err)
	assert.Equal(t, "pay-789", q2.TradeNo)
	assert.Equal(t, payment.ProviderStatusPaid, q2.Status)
	assert.Equal(t, 45.0, q2.Amount)
	assert.Equal(t, "usdttrc20", q2.Metadata["pay_currency"])

	// Query not found
	_, err = np.QueryOrder(context.Background(), "not-found")
	assert.Error(t, err)
}

func TestNowPayments_VerifyNotification(t *testing.T) {
	ipnSecret := "my-super-secret-ipn-key"
	np, err := NewNowPayments("np-1", map[string]string{
		"apiKey":    "test-api-key",
		"ipnSecret": ipnSecret,
	})
	require.NoError(t, err)

	payloadMap := map[string]any{
		"payment_id":     5070216172,
		"invoice_id":     456789123,
		"payment_status": "finished",
		"pay_address":    "0xd9145CCE52D386f254917e481eB44e9943F39138",
		"price_amount":   100.0,
		"price_currency": "usd",
		"actual_amount":  100.0,
		"order_id":       "SUB2-ORD-9999",
		"pay_currency":   "usdttrc20",
	}

	sortedJSON, err := json.Marshal(payloadMap)
	require.NoError(t, err)

	mac := hmac.New(sha512.New, []byte(ipnSecret))
	mac.Write(sortedJSON)
	validSig := hex.EncodeToString(mac.Sum(nil))

	headers := map[string]string{
		"x-nowpayments-sig": validSig,
	}

	// Valid notification
	notif, err := np.VerifyNotification(context.Background(), string(sortedJSON), headers)
	require.NoError(t, err)
	assert.Equal(t, "5070216172", notif.TradeNo)
	assert.Equal(t, "SUB2-ORD-9999", notif.OrderID)
	assert.Equal(t, 100.0, notif.Amount)
	assert.Equal(t, payment.NotificationStatusSuccess, notif.Status)
	assert.Equal(t, "usdttrc20", notif.Metadata["pay_currency"])

	// Invalid signature
	badHeaders := map[string]string{
		"x-nowpayments-sig": "deadbeef",
	}
	_, err = np.VerifyNotification(context.Background(), string(sortedJSON), badHeaders)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid nowpayments ipn signature")

	// Missing header
	_, err = np.VerifyNotification(context.Background(), string(sortedJSON), map[string]string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing x-nowpayments-sig")
}

func TestNowPayments_Refund(t *testing.T) {
	np, err := NewNowPayments("np-1", map[string]string{
		"apiKey":    "test-api-key",
		"ipnSecret": "test-secret",
	})
	require.NoError(t, err)

	_, err = np.Refund(context.Background(), payment.RefundRequest{
		TradeNo: "pay-123",
		OrderID: "ord-123",
		Amount:  "10.00",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "automated refund is not supported")
}

func TestNowPayments_MapStatus(t *testing.T) {
	assert.Equal(t, payment.ProviderStatusPaid, mapNowPaymentsStatus("finished"))
	assert.Equal(t, payment.ProviderStatusPaid, mapNowPaymentsStatus("confirmed"))
	assert.Equal(t, payment.ProviderStatusPaid, mapNowPaymentsStatus("sending"))
	assert.Equal(t, payment.ProviderStatusFailed, mapNowPaymentsStatus("failed"))
	assert.Equal(t, payment.ProviderStatusFailed, mapNowPaymentsStatus("expired"))
	assert.Equal(t, payment.ProviderStatusRefunded, mapNowPaymentsStatus("refunded"))
	assert.Equal(t, payment.ProviderStatusPending, mapNowPaymentsStatus("waiting"))
	assert.Equal(t, payment.ProviderStatusPending, mapNowPaymentsStatus("confirming"))
	assert.Equal(t, payment.ProviderStatusPending, mapNowPaymentsStatus("unknown_status"))
}

func TestNowPayments_VerifyNotification_EdgeCases(t *testing.T) {
	ipnSecret := "test-secret"
	np, err := NewNowPayments("np-1", map[string]string{
		"apiKey":    "test-api-key",
		"ipnSecret": ipnSecret,
	})
	require.NoError(t, err)

	sign := func(payload map[string]any) (string, string) {
		sorted, _ := json.Marshal(payload)
		mac := hmac.New(sha512.New, []byte(ipnSecret))
		mac.Write(sorted)
		return string(sorted), hex.EncodeToString(mac.Sum(nil))
	}

	// Missing order_id
	body, sig := sign(map[string]any{"payment_id": "123"})
	_, err = np.VerifyNotification(context.Background(), body, map[string]string{"x-nowpayments-sig": sig})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing order_id")

	// Failed status
	body, sig = sign(map[string]any{
		"order_id":       "ord-failed",
		"payment_id":     "123",
		"payment_status": "failed",
		"price_amount":   "50.0",
	})
	notif, err := np.VerifyNotification(context.Background(), body, map[string]string{"x-nowpayments-sig": sig})
	require.NoError(t, err)
	assert.Equal(t, payment.ProviderStatusFailed, notif.Status)
	assert.Equal(t, 50.0, notif.Amount)

	// Invalid json
	_, err = np.VerifyNotification(context.Background(), "{invalid json", map[string]string{"x-nowpayments-sig": "any"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid nowpayments json")
}

func TestNowPayments_QueryOrder_EmptyData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/payment/inv-empty") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
	}))
	defer server.Close()

	np, err := NewNowPayments("np-1", map[string]string{
		"apiKey":    "test-api-key",
		"ipnSecret": "test-secret",
	})
	require.NoError(t, err)
	np.baseURL = server.URL

	q, err := np.QueryOrder(context.Background(), "inv-empty")
	require.NoError(t, err)
	assert.Equal(t, "inv-empty", q.TradeNo)
	assert.Equal(t, payment.ProviderStatusPending, q.Status)
}

func TestNowPayments_VerifyNotification_HTMLEscaping(t *testing.T) {
	ipnSecret := "test-secret"
	np, err := NewNowPayments("np-1", map[string]string{
		"apiKey":    "test-api-key",
		"ipnSecret": ipnSecret,
	})
	require.NoError(t, err)

	// Payload containing characters '&', '<', '>' which standard json.Marshal escapes
	rawBody := `{"order_description":"Pro & VIP <service>","order_id":"SUB2-HTML-1","payment_id":99999,"payment_status":"finished","price_amount":15.5}`

	mac := hmac.New(sha512.New, []byte(ipnSecret))
	mac.Write([]byte(rawBody))
	sig := hex.EncodeToString(mac.Sum(nil))

	notif, err := np.VerifyNotification(context.Background(), rawBody, map[string]string{"x-nowpayments-sig": sig})
	require.NoError(t, err)
	assert.Equal(t, "SUB2-HTML-1", notif.OrderID)
	assert.Equal(t, 15.5, notif.Amount)
	assert.Equal(t, payment.NotificationStatusSuccess, notif.Status)
}

func TestNowPayments_QueryOrder_MultiPaymentsPrioritizesPaid(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/payment/inv-multi") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{
					"payment_id":     101,
					"payment_status": "expired",
					"price_amount":   20.0,
					"pay_currency":   "btc",
					"updated_at":     "2026-09-01T10:00:00Z",
				},
				{
					"payment_id":     102,
					"payment_status": "confirmed",
					"price_amount":   20.0,
					"pay_currency":   "usdttrc20",
					"updated_at":     "2026-09-01T10:10:00Z",
				},
			},
		})
	}))
	defer server.Close()

	np, err := NewNowPayments("np-1", map[string]string{
		"apiKey":    "test-api-key",
		"ipnSecret": "test-secret",
	})
	require.NoError(t, err)
	np.baseURL = server.URL

	q, err := np.QueryOrder(context.Background(), "inv-multi")
	require.NoError(t, err)
	assert.Equal(t, "102", q.TradeNo)
	assert.Equal(t, payment.ProviderStatusPaid, q.Status)
	assert.Equal(t, "usdttrc20", q.Metadata["pay_currency"])
}


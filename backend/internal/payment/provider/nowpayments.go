// Package provider contains concrete payment provider implementations.
package provider

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

const (
	nowpaymentsProductionBaseURL = "https://api.nowpayments.io/v1"
	nowpaymentsSandboxBaseURL    = "https://api-sandbox.nowpayments.io/v1"
	nowpaymentsHTTPTimeout       = 15 * time.Second
	maxNowpaymentsResponseSize   = 1 << 20 // 1MB
)

// NowPayments implements payment.Provider for NOWPayments cryptocurrency gateway.
type NowPayments struct {
	instanceID string
	apiKey     string
	ipnSecret  string
	baseURL    string
	currency   string
	notifyURL  string
	httpClient *http.Client
}

// NewNowPayments creates a new NOWPayments provider instance.
// Supported config keys:
// - apiKey / apikey (required)
// - ipnSecret / ipnsecret (required)
// - sandbox: "true" for sandbox API, "false" for production (default false)
// - currency: base price currency, e.g. "usd" (default "usd")
// - notifyUrl / notify_url: default IPN callback URL (optional)
func NewNowPayments(instanceID string, config map[string]string) (*NowPayments, error) {
	apiKey := strings.TrimSpace(config["apiKey"])
	if apiKey == "" {
		apiKey = strings.TrimSpace(config["apikey"])
	}
	ipnSecret := strings.TrimSpace(config["ipnSecret"])
	if ipnSecret == "" {
		ipnSecret = strings.TrimSpace(config["ipnsecret"])
	}

	if apiKey == "" {
		return nil, fmt.Errorf("nowpayments config missing required key: apiKey")
	}
	if ipnSecret == "" {
		return nil, fmt.Errorf("nowpayments config missing required key: ipnSecret")
	}

	baseURL := nowpaymentsProductionBaseURL
	if strings.EqualFold(strings.TrimSpace(config["sandbox"]), "true") {
		baseURL = nowpaymentsSandboxBaseURL
	}

	currency := strings.ToLower(strings.TrimSpace(config["currency"]))
	if currency == "" {
		currency = "usd"
	}

	notifyURL := strings.TrimSpace(config["notifyUrl"])
	if notifyURL == "" {
		notifyURL = strings.TrimSpace(config["notify_url"])
	}

	return &NowPayments{
		instanceID: instanceID,
		apiKey:     apiKey,
		ipnSecret:  ipnSecret,
		baseURL:    baseURL,
		currency:   currency,
		notifyURL:  notifyURL,
		httpClient: &http.Client{Timeout: nowpaymentsHTTPTimeout},
	}, nil
}

func (n *NowPayments) Name() string        { return "NOWPayments" }
func (n *NowPayments) ProviderKey() string { return payment.TypeNowPayments }
func (n *NowPayments) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeNowPayments}
}

// CreatePayment initiates a payment invoice on NOWPayments.
func (n *NowPayments) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	amountFloat, err := strconv.ParseFloat(req.Amount, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid amount: %w", err)
	}

	orderID := strings.TrimSpace(req.OrderID)
	if len(orderID) > 255 {
		orderID = orderID[:255]
	}

	orderDesc := strings.TrimSpace(req.Subject)
	if orderDesc == "" {
		orderDesc = fmt.Sprintf("Order %s", orderID)
	}
	if len(orderDesc) > 255 {
		orderDesc = orderDesc[:255]
	}

	bodyMap := map[string]any{
		"price_amount":      amountFloat,
		"price_currency":    n.currency,
		"order_id":          orderID,
		"order_description": orderDesc,
	}

	notifyURL := sanitizeURLForNOWPayments(req.NotifyURL)
	if notifyURL == "" {
		notifyURL = sanitizeURLForNOWPayments(n.notifyURL)
	}
	if notifyURL == "" && req.ReturnURL != "" {
		if parsed, err := url.Parse(req.ReturnURL); err == nil && parsed.Scheme != "" && parsed.Host != "" {
			prefix := ""
			if idx := strings.Index(parsed.Path, "/payment/result"); idx > 0 {
				prefix = strings.TrimSuffix(parsed.Path[:idx], "/")
			}
			notifyURL = sanitizeURLForNOWPayments(fmt.Sprintf("%s://%s%s/api/v1/payment/webhook/nowpayments", parsed.Scheme, parsed.Host, prefix))
		}
	}
	if notifyURL != "" {
		bodyMap["ipn_callback_url"] = notifyURL
	}

	returnURL := sanitizeURLForNOWPayments(req.ReturnURL)
	if returnURL != "" {
		bodyMap["success_url"] = returnURL
		bodyMap["cancel_url"] = returnURL
	}

	reqBytes, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, fmt.Errorf("marshal nowpayments invoice request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, n.baseURL+"/invoice", bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("create nowpayments request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", n.apiKey)

	resp, err := n.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call nowpayments create invoice: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxNowpaymentsResponseSize))
	if err != nil {
		return nil, fmt.Errorf("read nowpayments response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("nowpayments invoice error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var invoiceResp struct {
		ID         any    `json:"id"`
		InvoiceURL string `json:"invoice_url"`
		Message    string `json:"message"`
	}
	if err := json.Unmarshal(respBody, &invoiceResp); err != nil {
		return nil, fmt.Errorf("unmarshal nowpayments invoice response: %w", err)
	}

	if invoiceResp.InvoiceURL == "" {
		return nil, fmt.Errorf("nowpayments response missing invoice_url: %s", string(respBody))
	}

	tradeNo := fmt.Sprintf("%v", invoiceResp.ID)
	if tradeNo == "" || tradeNo == "<nil>" {
		tradeNo = req.OrderID
	}

	return &payment.CreatePaymentResponse{
		TradeNo:    tradeNo,
		PayURL:     invoiceResp.InvoiceURL,
		ResultType: payment.CreatePaymentResultOrderCreated,
	}, nil
}

// sanitizeURLForNOWPayments ensures the URL fits within NOWPayments' 255-character DB column limit.
// NOWPayments stores invoice URLs in varchar(255) columns. Large URLs (e.g. containing JWT resume tokens)
// cause PostgreSQL "value too long for type character varying(255)" HTTP 500 errors.
func sanitizeURLForNOWPayments(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}

	parsed, err := url.Parse(rawURL)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		if len(rawURL) <= 255 {
			return rawURL
		}
		return ""
	}

	// Always strip large JWT resume tokens as NOWPayments does not use them
	q := parsed.Query()
	if q.Has("resume_token") {
		q.Del("resume_token")
		parsed.RawQuery = q.Encode()
	}

	if len(parsed.String()) <= 255 {
		return parsed.String()
	}

	// If still too long, keep only order_id & status
	cleanQuery := url.Values{}
	if orderID := q.Get("order_id"); orderID != "" {
		cleanQuery.Set("order_id", orderID)
	}
	if status := q.Get("status"); status != "" {
		cleanQuery.Set("status", status)
	}
	parsed.RawQuery = cleanQuery.Encode()
	if len(parsed.String()) <= 255 {
		return parsed.String()
	}

	// Strip query parameters completely
	parsed.RawQuery = ""
	if len(parsed.String()) <= 255 {
		return parsed.String()
	}

	// If base URL itself exceeds 255 chars, omit to avoid HTTP 500
	return ""
}

// QueryOrder queries payment status by payment ID or invoice ID.
func (n *NowPayments) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	// First attempt: query payment directly by ID
	paymentResp, err := n.queryPaymentByID(ctx, tradeNo)
	if err == nil && paymentResp != nil {
		return paymentResp, nil
	}

	// Second attempt: query payments filtered by invoice_id
	invoiceResp, err2 := n.queryPaymentByInvoiceID(ctx, tradeNo)
	if err2 == nil && invoiceResp != nil {
		return invoiceResp, nil
	}

	if err != nil {
		return nil, err
	}
	return nil, err2
}

func (n *NowPayments) queryPaymentByID(ctx context.Context, paymentID string) (*payment.QueryOrderResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/payment/%s", n.baseURL, paymentID), nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("x-api-key", n.apiKey)

	resp, err := n.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("payment not found (HTTP %d)", resp.StatusCode)
	}

	var p struct {
		PaymentID     any     `json:"payment_id"`
		PaymentStatus string  `json:"payment_status"`
		PriceAmount   float64 `json:"price_amount"`
		PayAmount     float64 `json:"pay_amount"`
		PayCurrency   string  `json:"pay_currency"`
		UpdatedAt     string  `json:"updated_at"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxNowpaymentsResponseSize)).Decode(&p); err != nil {
		return nil, err
	}

	status := mapNowPaymentsStatus(p.PaymentStatus)
	return &payment.QueryOrderResponse{
		TradeNo: fmt.Sprintf("%v", p.PaymentID),
		Status:  status,
		Amount:  p.PriceAmount,
		PaidAt:  p.UpdatedAt,
		Metadata: map[string]string{
			"payment_status": p.PaymentStatus,
			"pay_currency":   p.PayCurrency,
		},
	}, nil
}

func (n *NowPayments) queryPaymentByInvoiceID(ctx context.Context, invoiceID string) (*payment.QueryOrderResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/payment/?invoice_id=%s", n.baseURL, invoiceID), nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("x-api-key", n.apiKey)

	resp, err := n.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("invoice payments query failed (HTTP %d)", resp.StatusCode)
	}

	var listResp struct {
		Data []struct {
			PaymentID     any     `json:"payment_id"`
			PaymentStatus string  `json:"payment_status"`
			PriceAmount   float64 `json:"price_amount"`
			PayCurrency   string  `json:"pay_currency"`
			UpdatedAt     string  `json:"updated_at"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxNowpaymentsResponseSize)).Decode(&listResp); err != nil {
		return nil, err
	}

	if len(listResp.Data) == 0 {
		return &payment.QueryOrderResponse{
			TradeNo: invoiceID,
			Status:  payment.ProviderStatusPending,
		}, nil
	}

	selected := listResp.Data[0]
	for _, p := range listResp.Data {
		if mapNowPaymentsStatus(p.PaymentStatus) == payment.ProviderStatusPaid {
			selected = p
			break
		}
	}

	return &payment.QueryOrderResponse{
		TradeNo: fmt.Sprintf("%v", selected.PaymentID),
		Status:  mapNowPaymentsStatus(selected.PaymentStatus),
		Amount:  selected.PriceAmount,
		PaidAt:  selected.UpdatedAt,
		Metadata: map[string]string{
			"payment_status": selected.PaymentStatus,
			"pay_currency":   selected.PayCurrency,
		},
	}, nil
}

func mapNowPaymentsStatus(status string) string {
	switch strings.ToLower(status) {
	case "finished", "confirmed":
		return payment.ProviderStatusPaid
	case "sending", "confirming", "waiting":
		return payment.ProviderStatusPending
	case "failed", "expired":
		return payment.ProviderStatusFailed
	case "refunded":
		return payment.ProviderStatusRefunded
	default:
		return payment.ProviderStatusPending
	}
}

// VerifyNotification validates HMAC-SHA512 signature on x-nowpayments-sig header
// and extracts the payment notification payload.
func (n *NowPayments) VerifyNotification(_ context.Context, rawBody string, headers map[string]string) (*payment.PaymentNotification, error) {
	sig := ""
	for k, v := range headers {
		if strings.EqualFold(k, "x-nowpayments-sig") {
			sig = strings.TrimSpace(v)
			break
		}
	}
	if sig == "" {
		return nil, fmt.Errorf("missing x-nowpayments-sig header")
	}

	decoder := json.NewDecoder(strings.NewReader(rawBody))
	decoder.UseNumber()
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("invalid nowpayments json payload: %w", err)
	}

	// In Go stdlib json.Marshal, map keys are encoded in alphabetical order.
	// Disable HTML escaping (&, <, >) to match raw JSON hashing from NOWPayments.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(payload); err != nil {
		return nil, fmt.Errorf("marshal sorted payload: %w", err)
	}
	sortedJSON := bytes.TrimRight(buf.Bytes(), "\n")

	mac := hmac.New(sha512.New, []byte(n.ipnSecret))
	mac.Write(sortedJSON)
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(strings.ToLower(sig)), []byte(strings.ToLower(expectedSig))) {
		return nil, fmt.Errorf("invalid nowpayments ipn signature")
	}

	orderID, _ := payload["order_id"].(string)
	if orderID == "" {
		return nil, fmt.Errorf("nowpayments payload missing order_id")
	}

	paymentID := fmt.Sprintf("%v", payload["payment_id"])
	paymentStatus, _ := payload["payment_status"].(string)

	var amount float64
	switch v := payload["price_amount"].(type) {
	case json.Number:
		amount, _ = v.Float64()
	case float64:
		amount = v
	case string:
		amount, _ = strconv.ParseFloat(v, 64)
	}

	payCurrency, _ := payload["pay_currency"].(string)

	notificationStatus := payment.ProviderStatusPending
	switch strings.ToLower(paymentStatus) {
	case "finished", "confirmed":
		notificationStatus = payment.NotificationStatusSuccess
	case "failed", "expired":
		notificationStatus = payment.ProviderStatusFailed
	case "refunded":
		notificationStatus = payment.ProviderStatusRefunded
	default:
		notificationStatus = payment.ProviderStatusPending
	}

	return &payment.PaymentNotification{
		TradeNo:  paymentID,
		OrderID:  orderID,
		Amount:   amount,
		Status:   notificationStatus,
		RawData:  rawBody,
		Metadata: map[string]string{
			"payment_id":     paymentID,
			"payment_status": paymentStatus,
			"pay_currency":   payCurrency,
		},
	}, nil
}

// Refund returns an error as automated cryptocurrency refund is not supported.
func (n *NowPayments) Refund(_ context.Context, _ payment.RefundRequest) (*payment.RefundResponse, error) {
	return nil, fmt.Errorf("nowpayments automated refund is not supported; manual handling required")
}

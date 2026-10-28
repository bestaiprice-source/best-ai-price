package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

// The subject is shown verbatim on the provider's own checkout page ("More
// details"), so it has to name the currency that is actually charged.
func TestPaymentSubjectCurrency(t *testing.T) {
	cases := []struct {
		name string
		sel  *payment.InstanceSelection
		want string
	}{
		{name: "nil selection defaults to USD", sel: nil, want: "USD"},
		{name: "wxpay is CNY", sel: &payment.InstanceSelection{ProviderKey: payment.TypeWxpay}, want: "CNY"},
		{name: "alipay is CNY", sel: &payment.InstanceSelection{ProviderKey: payment.TypeAlipay}, want: "CNY"},
		{name: "easypay is CNY", sel: &payment.InstanceSelection{ProviderKey: payment.TypeEasyPay}, want: "CNY"},
		{name: "nowpayments defaults to USD", sel: &payment.InstanceSelection{ProviderKey: payment.TypeNowPayments}, want: "USD"},
		{
			name: "nowpayments honours the instance currency",
			sel:  &payment.InstanceSelection{ProviderKey: payment.TypeNowPayments, Config: map[string]string{"currency": "eur"}},
			want: "EUR",
		},
		{name: "stripe is USD", sel: &payment.InstanceSelection{ProviderKey: payment.TypeStripe}, want: "USD"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, paymentSubjectCurrency(tc.sel))
		})
	}
}

func TestBuildPaymentSubjectUsesTheChargedCurrency(t *testing.T) {
	svc := &PaymentService{}
	cfg := &PaymentConfig{}

	// The regression: a USD order used to be described as "100.00 CNY".
	require.Equal(t, "BestAIPrices 100.00 USD", svc.buildPaymentSubject(nil, 100, cfg, "USD"))
	require.Equal(t, "BestAIPrices 100.00 CNY", svc.buildPaymentSubject(nil, 100, cfg, "CNY"))

	// An empty currency must never produce a dangling amount.
	require.Equal(t, "BestAIPrices 100.00 USD", svc.buildPaymentSubject(nil, 100, cfg, ""))

	// A configured product-name prefix/suffix still wins outright.
	suffixed := &PaymentConfig{ProductNamePrefix: "BestAIPrice"}
	require.Equal(t, "BestAIPrice 25.00", svc.buildPaymentSubject(nil, 25, suffixed, "USD"))
}
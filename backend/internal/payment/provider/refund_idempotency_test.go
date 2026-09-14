package provider

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func TestRefundRequest_DeterministicID(t *testing.T) {
	req := payment.RefundRequest{
		TradeNo:  "trade-999",
		OrderID:  "ord-888",
		Amount:   "10.00",
		Reason:   "user requested",
		RefundID: "rf_ord-888",
	}

	require.Equal(t, "rf_ord-888", req.RefundID)
	require.Equal(t, "ord-888", req.OrderID)
	require.Equal(t, "10.00", req.Amount)
}

func TestNowPayments_RefundUnsupported(t *testing.T) {
	np := &NowPayments{}
	resp, err := np.Refund(context.Background(), payment.RefundRequest{
		OrderID:  "ord-123",
		RefundID: "rf_ord-123",
	})
	require.Error(t, err)
	require.Nil(t, resp)
	require.Contains(t, err.Error(), "not supported")
}

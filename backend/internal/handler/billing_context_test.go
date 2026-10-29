package handler

import (
	"context"
	"errors"
	"testing"
	"time"
)

type billingCtxKey struct{}

// Non-streaming yunwu completions used to lose their billing entirely: the
// response body is flushed while the handler is still running, the client closes
// the connection, and Go cancels the inbound request context — so the billing
// step (usage_logs row + balance deduction) ran on an already-cancelled context
// and silently skipped. detachedBillingContext must therefore drop the
// cancellation signal while preserving request-scoped values.
func TestDetachedBillingContextSurvivesRequestCancellation(t *testing.T) {
	requestCtx, cancelRequest := context.WithCancel(context.WithValue(context.Background(), billingCtxKey{}, "pricing-scope"))
	cancelRequest() // the client hung up right after reading the response body

	if !errors.Is(requestCtx.Err(), context.Canceled) {
		t.Fatalf("precondition: request context should be cancelled, got %v", requestCtx.Err())
	}

	billingCtx, cancelBilling := detachedBillingContext(requestCtx)
	defer cancelBilling()

	if err := billingCtx.Err(); err != nil {
		t.Fatalf("detached billing context must not inherit cancellation, got %v", err)
	}
	if got := billingCtx.Value(billingCtxKey{}); got != "pricing-scope" {
		t.Fatalf("detached billing context must preserve request values, got %v", got)
	}
	if _, ok := billingCtx.Deadline(); !ok {
		t.Fatal("detached billing context must carry a bounded deadline")
	}
	deadline, _ := billingCtx.Deadline()
	if remaining := time.Until(deadline); remaining <= 0 || remaining > yunwuBillingTimeout {
		t.Fatalf("unexpected billing deadline in %s, want (0, %s]", remaining, yunwuBillingTimeout)
	}
}

func TestDetachedBillingContextToleratesNilContext(t *testing.T) {
	//nolint:staticcheck // deliberately passing a nil context
	billingCtx, cancelBilling := detachedBillingContext(nil)
	defer cancelBilling()
	if billingCtx == nil || billingCtx.Err() != nil {
		t.Fatal("detachedBillingContext(nil) must return a usable context")
	}
}

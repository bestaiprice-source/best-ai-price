package handler

import (
	"context"
	"time"
)

// yunwuBillingTimeout bounds post-response billing work for Unofficial-provider
// (yunwu) requests. Billing never runs longer than this once the client has
// already been served.
const yunwuBillingTimeout = 30 * time.Second

// detachedBillingContext detaches post-response billing from the inbound request
// context.
//
// Go cancels a request's context as soon as the client connection goes away.
// For a NON-streaming completion the response body is flushed while the handler
// is still running, so the client closes the connection (and the context is
// cancelled) BEFORE this billing code executes. Running billing on that context
// made the execution-account lookup fail with "context canceled", and the whole
// billing step — usage_logs row, user balance deduction and API-key window/quota
// counters — was silently skipped with only a generic warning. Streaming
// responses kept the connection (and the context) alive, which is why streaming
// requests were billed and non-streaming ones were free.
//
// Values are preserved (pricing scope, rate multiplier, request id); only the
// cancellation signal and any deadline are dropped, and a fresh bounded deadline
// is applied.
func detachedBillingContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(ctx), yunwuBillingTimeout)
}

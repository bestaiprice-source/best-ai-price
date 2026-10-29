package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// modelNotAllowedError reports that the API key's model allowlist refuses the
// requested model. It carries its own HTTP mapping (400 invalid_request_error)
// so every dispatch path can answer identically.
type modelNotAllowedError struct {
	Model string
}

func (e *modelNotAllowedError) Error() string {
	if e == nil {
		return "Requested model is not allowed for this API key"
	}
	return apiKeyModelNotAllowedMessage(e.Model)
}

// hypervisorGuardStatus maps a guard error onto the HTTP answer used by both
// gateway handler families.
func hypervisorGuardStatus(err error) (int, string, string) {
	var notAllowed *modelNotAllowedError
	if errors.As(err, &notAllowed) {
		return http.StatusBadRequest, "invalid_request_error", notAllowed.Error()
	}
	return billingErrorDetails(err)
}

// checkHypervisorRequestGuards applies the API-key controls that the regular
// gateway handlers enforce but that the Yunwu/hypervisor dispatch path used to
// skip. Those handlers hand an unofficial-provider key to the hypervisor engine
// *before* the shared validation block runs, so a key issued for one model could
// call any model, and its 5h / 1d / 7d spend windows were never consulted.
//
// Scope is deliberately narrow: the model allowlist and the per-window spend
// limits. Key status, expiry, quota, IP rules and the balance floor are already
// enforced by the API-key auth middleware, and the per-model upstream price is
// enforced by the engine itself (ErrNoPricingAvailable). Returns nil when the
// request may proceed.
func checkHypervisorRequestGuards(
	ctx context.Context,
	billing *service.BillingCacheService,
	apiKey *service.APIKey,
	requestedModel string,
) error {
	if apiKey != nil && !apiKeyAllowsRequestedModel(apiKey, strings.TrimSpace(requestedModel)) {
		return &modelNotAllowedError{Model: requestedModel}
	}
	return billing.CheckAPIKeyRateLimitEligibility(ctx, apiKey)
}

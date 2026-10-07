package yunwu

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// modelRoutingKey is the context key used to pass an explicit, ordered list of
// execution-node (physical account) IDs that the gateway has selected as the
// Primary/Backup route for the requested canonical model.
type modelRoutingKey struct{}

// ContextWithModelRouting attaches an ordered Primary→Backup list of account IDs
// for the requested model. When present, the Yunwu engine restricts candidate
// nodes to exactly this list (in order) instead of running cheapest-first pool
// scheduling. This is what makes an administrator "promote to Primary/Backup"
// action actually take effect for unofficial providers.
func ContextWithModelRouting(ctx context.Context, accountIDs []int64) context.Context {
	if ctx == nil || len(accountIDs) == 0 {
		return ctx
	}
	clean := make([]int64, 0, len(accountIDs))
	seen := make(map[int64]bool, len(accountIDs))
	for _, id := range accountIDs {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		clean = append(clean, id)
	}
	if len(clean) == 0 {
		return ctx
	}
	return context.WithValue(ctx, modelRoutingKey{}, clean)
}

// ModelRoutingFromContext extracts the explicit Primary/Backup route list, if any.
func ModelRoutingFromContext(ctx context.Context) []int64 {
	if ctx == nil {
		return nil
	}
	if v, ok := ctx.Value(modelRoutingKey{}).([]int64); ok {
		return v
	}
	return nil
}

var upstreamStatusRe = regexp.MustCompile(`upstream error status (\d{3})`)

// upstreamStatusCode extracts the numeric upstream HTTP status from a runtime
// error string of the form "upstream error status 400: <body>". Returns 0 when
// the error is not a structured upstream-status error (e.g. network/timeout).
func upstreamStatusCode(err error) int {
	if err == nil {
		return 0
	}
	m := upstreamStatusRe.FindStringSubmatch(err.Error())
	if len(m) != 2 {
		return 0
	}
	code, convErr := strconv.Atoi(m[1])
	if convErr != nil {
		return 0
	}
	return code
}

// isClientRequestError reports whether the upstream failure is caused by the
// client's own request (malformed parameters, unsupported fields, content
// policy, etc.) rather than a provider outage. Such errors MUST NOT trigger a
// failover/rebinding attempt: retrying the same bad request on a Backup provider
// wastes capacity, cools down a healthy node and can cause uncontrolled spend.
//
// Per the BestAIPrice specification, failover is legitimate only for
// network/timeout, 5xx, upstream 429/overload, invalid/disabled upstream key,
// provider Down, and aggregator routing problems ("model_not_found",
// "no available channel").
func isClientRequestError(err error) bool {
	if err == nil {
		return false
	}
	code := upstreamStatusCode(err)
	if code != 400 && code != 422 {
		return false
	}
	body := strings.ToLower(err.Error())

	// Upstream routing problems are failover-eligible even when surfaced as 4xx.
	if strings.Contains(body, "model_not_found") ||
		strings.Contains(body, "model not found") ||
		strings.Contains(body, "no available channel") ||
		strings.Contains(body, "no available channels") ||
		strings.Contains(body, "channel not found") ||
		strings.Contains(body, "rate") ||
		strings.Contains(body, "overload") ||
		strings.Contains(body, "quota") ||
		strings.Contains(body, "insufficient") ||
		strings.Contains(body, "timeout") ||
		strings.Contains(body, "deadline") {
		return false
	}

	// Clearly client-side validation failures.
	clientMarkers := []string{
		"invalid_request_error",
		"invalid request",
		"invalid parameter",
		"unsupported_parameter",
		"unsupported parameter",
		"unsupported value",
		"validation_error",
		"validation error",
		"messages:",
		"content_policy",
		"content policy",
		"context length",
		"max_tokens",
		"required parameter",
		"prompt is required",
	}
	for _, marker := range clientMarkers {
		if strings.Contains(body, marker) {
			return true
		}
	}
	return false
}

// describeRouting is a small helper for logging.
func describeRouting(ids []int64) string {
	if len(ids) == 0 {
		return "pool"
	}
	return fmt.Sprintf("explicit(%v)", ids)
}

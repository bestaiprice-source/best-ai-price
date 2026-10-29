package yunwu

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
)

// Reconciliation of the assumed billing group against what the provider actually
// charged us (option 2 of the pricing strategy).
//
// Why this exists: a provider may ignore the group its key was issued in when it
// routes a request (OpenLux "smart routing" states so in its own token form), so
// the group we price by is a best guess. The guess is corrected by measurement
// instead of trust: every balance sync reads the provider's OWN cumulative usage
// counter twice and compares the difference with the provider-cost basis we
// recorded for the account over the same interval. The ratio between them is a
// CORRECTION FACTOR applied on top of the resolved group ratio, and it is
// one-sided — it can only lift a price. Never selling below the observed cost is
// the whole point; a cheaper reading on its own must not cut a price.

const (
	// ExtraObservedGroupRatio holds the measured correction factor (real cost ÷ the
	// cost basis we billed at). > 1 lifts prices, <= 1 is ignored by the floor.
	// Already declared in tier.go as ExtraObservedGroupRatio.
	// ExtraObservedGroupRatioAt is the measured-at timestamp (RFC3339).
	ExtraObservedGroupRatioAt = "observed_group_ratio_at"
	// ExtraObservedGroupRatioProviderUsedUSD is the provider's cumulative usage
	// counter (USD) at the checkpoint the last measurement started from.
	ExtraObservedGroupRatioProviderUsedUSD = "observed_group_ratio_provider_used_usd"
	// ExtraObservedGroupRatioBasisUSD is the cost basis the last accepted
	// measurement covered (USD) — an operator can see how solid the sample was.
	ExtraObservedGroupRatioBasisUSD = "observed_group_ratio_basis_usd"
	// ExtraObservedGroupRatioMeasuredAt is when the value was last written.
	ExtraObservedGroupRatioMeasuredAt = "observed_group_ratio_measured_at"
	// ExtraObservedRatioEnabled is the per-node opt-in for the whole reconciliation
	// (checkbox in the account form). OFF — the default — keeps billing exactly as
	// before: the group selected for the node, nothing else. ON — the price also
	// respects what the provider is measured to charge, which is what makes a node
	// whose key sits in the `default` group safe when the provider routes on its own.
	ExtraObservedRatioEnabled = "observed_ratio_enabled"
)

// ObservedRatioEnabled reports whether a node opted into the reconciliation floor.
// Accepts a real bool plus the string spellings an admin form or hand-edited JSON
// may produce; anything unrecognised means OFF.
func ObservedRatioEnabled(extra map[string]any) bool {
	if len(extra) == 0 {
		return false
	}
	switch v := extra[ExtraObservedRatioEnabled].(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "1", "yes", "on":
			return true
		}
	case int:
		return v != 0
	case int64:
		return v != 0
	case float64:
		return v != 0
	}
	return false
}

// MinObservedSampleCostUSD is the smallest provider-cost basis a measurement may
// be based on. Below it the window is skipped (a $0.02 sample would turn rounding
// noise into a price), and the checkpoint stays so the next window is bigger.
const MinObservedSampleCostUSD = 0.5

// MaxObservedCorrection caps the correction factor. A larger reading means the
// sample is not comparable (prices changed, traffic mixed across providers)
// rather than that the provider is charging 5x, so it is refused instead of
// multiplied into every price.
const MaxObservedCorrection = 5.0

// MeasureObservedCorrection turns two readings of the provider's cumulative usage
// counter plus our cost basis for the same interval into a correction factor.
func MeasureObservedCorrection(providerUsedUSDStart, providerUsedUSDEnd, costBasisUSD float64) (float64, bool) {
	ratio, ok := ObservedGroupRatio(providerUsedUSDStart, providerUsedUSDEnd, costBasisUSD)
	if !ok {
		return 0, false
	}
	if ratio > MaxObservedCorrection {
		return 0, false
	}
	return ratio, true
}

// SumAccountCostBasisSince sums the provider-cost basis (usage_logs.total_cost)
// recorded for an execution account since a checkpoint — the denominator of the
// correction factor. total_cost already carries the group ratio we billed at, so
// the quotient is exactly "real multiplier ÷ assumed multiplier".
func SumAccountCostBasisSince(ctx context.Context, client *ent.Client, accountID int64, since time.Time) (float64, error) {
	if client == nil || accountID <= 0 || since.IsZero() {
		return 0, nil
	}
	rows, err := client.QueryContext(ctx,
		`SELECT COALESCE(SUM(total_cost), 0)::float8 FROM usage_logs WHERE account_id = $1 AND created_at >= $2`,
		accountID, since)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()

	var total float64
	if rows.Next() {
		if err := rows.Scan(&total); err != nil {
			return 0, err
		}
	}
	return total, rows.Err()
}

// extraFloat reads a numeric extra value (JSON round-trips everything as
// float64, but a hand-edited account may hold a string or an int).
func extraFloat(extra map[string]any, key string) (float64, bool) {
	if len(extra) == 0 {
		return 0, false
	}
	switch v := extra[key].(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

// extraTime reads an RFC3339 timestamp from extra.
func extraTime(extra map[string]any, key string) (time.Time, bool) {
	if len(extra) == 0 {
		return time.Time{}, false
	}
	raw, ok := extra[key].(string)
	if !ok || strings.TrimSpace(raw) == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(raw))
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}

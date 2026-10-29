package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// modelTrafficHealthRepository aggregates per-model health out of the traffic we
// already logged, so the user-facing uptime can be measured without spending a
// single upstream token (the probe path costs a real generation per model).
type modelTrafficHealthRepository struct {
	db *sql.DB
}

// NewModelTrafficHealthRepository constructs the traffic-health repository.
func NewModelTrafficHealthRepository(db *sql.DB) service.ModelTrafficHealthRepository {
	return &modelTrafficHealthRepository{db: db}
}

// modelTrafficStatsQuery counts, per model, what users actually got in the
// window: served requests (usage_logs) versus failures caused by the provider
// side (ops_error_logs).
//
// Deliberate exclusions, each of which would otherwise libel a healthy model:
//   - is_business_limited: our own quota / rate-limit / balance rejections are
//     OUR limits, not the provider's availability.
//   - error_owner <> 'provider': client mistakes (bad payload, bad key) and
//     gateway-internal faults are not the model's fault.
//   - the upstream_failover_loss marker: those usage rows describe an attempt
//     that consumed tokens and then failed, billed to the platform. Counting
//     them as "served" would inflate every uptime percentage.
const modelTrafficStatsQuery = `
WITH served AS (
    SELECT model,
           COUNT(*)                    AS successes,
           AVG(duration_ms)::float8    AS avg_duration_ms,
           AVG(first_token_ms)::float8 AS avg_first_token_ms
    FROM usage_logs
    WHERE created_at >= $1
      AND COALESCE(model, '') <> ''
      AND COALESCE(user_agent, '') <> 'upstream_failover_loss'
    GROUP BY model
),
failed AS (
    SELECT model, COUNT(*) AS failures
    FROM ops_error_logs
    WHERE created_at >= $1
      AND COALESCE(model, '') <> ''
      AND is_business_limited = false
      AND error_owner = 'provider'
    GROUP BY model
)
SELECT COALESCE(served.model, failed.model) AS model,
       COALESCE(served.successes, 0)        AS successes,
       COALESCE(failed.failures, 0)         AS failures,
       served.avg_duration_ms,
       served.avg_first_token_ms
FROM served
FULL OUTER JOIN failed ON failed.model = served.model
`

// ModelTrafficStats returns one row per raw model id seen since `since`.
func (r *modelTrafficHealthRepository) ModelTrafficStats(ctx context.Context, since time.Time) ([]service.ModelTrafficStatRow, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx, modelTrafficStatsQuery, since)
	if err != nil {
		return nil, fmt.Errorf("query model traffic stats: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]service.ModelTrafficStatRow, 0, 32)
	for rows.Next() {
		var (
			row        service.ModelTrafficStatRow
			durationMs sql.NullFloat64
			firstMs    sql.NullFloat64
		)
		if err := rows.Scan(&row.Model, &row.Successes, &row.Failures, &durationMs, &firstMs); err != nil {
			return nil, fmt.Errorf("scan model traffic stats: %w", err)
		}
		if durationMs.Valid {
			value := durationMs.Float64
			row.AvgDurationMs = &value
		}
		if firstMs.Valid {
			value := firstMs.Float64
			row.AvgFirstTokenMs = &value
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate model traffic stats: %w", err)
	}
	return out, nil
}

package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
)

// Money ledger (§12). BestAIPrice has a single user money balance
// (users.balance), and the cabinet must expose a money-operation history that is
// separate from the technical API usage feed. Rather than introduce a second
// billing store, this service is a read-model that projects the existing
// sources into one ledger with the four required entry types:
//
//   - deposit     — payment_orders that reached a paid/completed status, plus
//                   balance redeem codes the user cashed in
//   - usage       — usage_logs actual_cost (debits for API consumption)
//   - refund      — payment_orders with refund_amount > 0
//   - adjustment  — admin balance adjustments (redeem_codes type=admin_balance)
//
// Amounts are signed USD: credits are positive, API debits are negative, so the
// running sum always reconciles against users.balance.

type LedgerEntryType string

const (
	LedgerTypeDeposit    LedgerEntryType = "deposit"
	LedgerTypeUsage      LedgerEntryType = "usage"
	LedgerTypeRefund     LedgerEntryType = "refund"
	LedgerTypeAdjustment LedgerEntryType = "adjustment"
)

// LedgerEntry is one money operation.
type LedgerEntry struct {
	ID          string          `json:"id"`
	Type        LedgerEntryType `json:"type"`
	AmountUSD   float64         `json:"amount_usd"`
	Currency    string          `json:"currency"`
	Network     string          `json:"network"`
	Status      string          `json:"status"`
	Description string          `json:"description"`
	Reference   string          `json:"reference"`
	CreatedAt   string          `json:"created_at"`
	ConfirmedAt *string         `json:"confirmed_at"`
}

// BalanceLedgerPage is a paginated ledger page plus per-type totals.
type BalanceLedgerPage struct {
	Items              []LedgerEntry `json:"items"`
	Total              int           `json:"total"`
	Page               int           `json:"page"`
	PageSize           int           `json:"page_size"`
	TotalDepositUSD    float64       `json:"total_deposit_usd"`
	TotalUsageUSD      float64       `json:"total_usage_usd"`
	TotalRefundUSD     float64       `json:"total_refund_usd"`
	TotalAdjustmentUSD float64       `json:"total_adjustment_usd"`
}

type BalanceLedgerService struct {
	db *sqlx.DB
}

func NewBalanceLedgerService(db *sql.DB) *BalanceLedgerService {
	return &BalanceLedgerService{db: sqlx.NewDb(db, "postgres")}
}

// ledgerUnionSQL projects the four existing sources into one uniform shape. It is
// parameterised on $1 = user_id; callers add ORDER BY/LIMIT/OFFSET or an
// aggregate wrapper.
const ledgerUnionSQL = `
SELECT * FROM (
  SELECT 'deposit' AS entry_type,
         'rc_' || id AS ref_id,
         value AS amount,
         'USD' AS currency,
         '' AS network,
         'completed' AS status,
         COALESCE(NULLIF(notes, ''), 'Redeem code') AS description,
         code AS reference,
         created_at AS created_at,
         used_at AS confirmed_at
  FROM redeem_codes
  WHERE used_by = $1 AND status = 'used' AND type = 'balance'

  UNION ALL

  SELECT 'adjustment', 'rc_' || id, value, 'USD', '', 'completed',
         COALESCE(NULLIF(notes, ''), 'Manual adjustment'), code,
         COALESCE(used_at, created_at), used_at
  FROM redeem_codes
  WHERE used_by = $1 AND status = 'used' AND type = 'admin_balance'

  UNION ALL

  SELECT 'deposit', 'po_' || id, amount,
         COALESCE(NULLIF(provider_key, ''), payment_type),
         COALESCE(provider_snapshot->>'pay_currency', ''),
         lower(status),
         COALESCE(NULLIF(out_trade_no, ''), payment_type),
         COALESCE(payment_trade_no, ''),
         created_at,
         COALESCE(paid_at, completed_at)
  FROM payment_orders
  WHERE user_id = $1 AND lower(status) IN ('paid', 'completed', 'finished', 'confirmed', 'success')

  UNION ALL

  SELECT 'refund', 'po_' || id, refund_amount,
         COALESCE(NULLIF(provider_key, ''), payment_type), '',
         'refunded',
         COALESCE(NULLIF(refund_reason, ''), 'Refund'),
         COALESCE(payment_trade_no, ''),
         COALESCE(refund_at, created_at),
         refund_at
  FROM payment_orders
  WHERE user_id = $1 AND refund_amount > 0

  UNION ALL

  SELECT 'usage', 'ul_' || id, -actual_cost, 'USD', '', 'completed',
         model, '', created_at, created_at
  FROM usage_logs
  WHERE user_id = $1 AND actual_cost > 0
) ledger`

// GetUserLedger returns the user's money operations, newest first.
func (s *BalanceLedgerService) GetUserLedger(ctx context.Context, userID int64, entryType string, page, pageSize int) (*BalanceLedgerPage, error) {
	if s == nil || s.db == nil {
		return &BalanceLedgerPage{Items: []LedgerEntry{}}, nil
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	entryType = strings.ToLower(strings.TrimSpace(entryType))

	typeFilter := ""
	args := []any{userID}
	if entryType != "" && entryType != "all" {
		switch LedgerEntryType(entryType) {
		case LedgerTypeDeposit, LedgerTypeUsage, LedgerTypeRefund, LedgerTypeAdjustment:
			typeFilter = " WHERE entry_type = $2"
			args = append(args, entryType)
		default:
			return &BalanceLedgerPage{Items: []LedgerEntry{}, Page: page, PageSize: pageSize}, nil
		}
	}

	items := []LedgerEntry{}
	listSQL := fmt.Sprintf(
		"SELECT entry_type AS type, ref_id AS id, amount AS amount_usd, currency, network, status, description, reference, created_at, confirmed_at FROM (%s) x%s ORDER BY created_at DESC LIMIT $%d OFFSET $%d",
		ledgerUnionSQL, typeFilter, len(args)+1, len(args)+2,
	)
	listArgs := append(append([]any{}, args...), pageSize, (page-1)*pageSize)
	if err := s.db.SelectContext(ctx, &items, listSQL, listArgs...); err != nil {
		return nil, fmt.Errorf("query ledger entries: %w", err)
	}

	var total int
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM (%s) x%s", ledgerUnionSQL, typeFilter)
	if err := s.db.GetContext(ctx, &total, countSQL, args...); err != nil {
		return nil, fmt.Errorf("count ledger entries: %w", err)
	}

	out := &BalanceLedgerPage{Items: items, Total: total, Page: page, PageSize: pageSize}
	// Per-type totals over the whole ledger (not just the current page).
	sumSQL := fmt.Sprintf(`
		SELECT COALESCE(SUM(CASE WHEN entry_type = 'deposit' THEN amount ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN entry_type = 'usage' THEN amount ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN entry_type = 'refund' THEN amount ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN entry_type = 'adjustment' THEN amount ELSE 0 END), 0)
		FROM (%s) x`, ledgerUnionSQL)
	if err := s.db.QueryRowxContext(ctx, sumSQL, userID).Scan(
		&out.TotalDepositUSD, &out.TotalUsageUSD, &out.TotalRefundUSD, &out.TotalAdjustmentUSD,
	); err != nil {
		return nil, fmt.Errorf("aggregate ledger totals: %w", err)
	}
	return out, nil
}

package repository

import (
	"context"
	"database/sql"
	"math"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestDeductUsageBillingBalance_NormalDeduction(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE users SET balance = balance - \\$1").
		WithArgs(0.05, int64(101)).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(9.95))
	mock.ExpectCommit()

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	err = deductUsageBillingBalance(context.Background(), tx, 101, 0.05)
	require.NoError(t, err)

	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeductUsageBillingBalance_ExceedsOverdraftThreshold_NoDebtForgiveness(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	// Deduction of $10 when balance was $0 leads to -$10, which exceeds -$0.50 threshold.
	// It must record the full negative balance -$10 (no clamp, no debt forgiveness) and succeed.
	mock.ExpectQuery("UPDATE users SET balance = balance - \\$1").
		WithArgs(10.0, int64(102)).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(-10.0))
	mock.ExpectCommit()

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	err = deductUsageBillingBalance(context.Background(), tx, 102, 10.0)
	require.NoError(t, err)

	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeductUsageBillingBalance_UserNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE users SET balance = balance - \\$1").
		WithArgs(5.0, int64(999)).
		WillReturnError(sql.ErrNoRows)

	mock.ExpectRollback()

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	err = deductUsageBillingBalance(context.Background(), tx, 999, 5.0)
	require.ErrorIs(t, err, service.ErrUserNotFound)

	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeductUsageBillingBalance_DatabaseError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE users SET balance = balance - \\$1").
		WithArgs(5.0, int64(103)).
		WillReturnError(sql.ErrConnDone)

	mock.ExpectRollback()

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	err = deductUsageBillingBalance(context.Background(), tx, 103, 5.0)
	require.ErrorIs(t, err, sql.ErrConnDone)

	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeductUsageBillingBalance_InvalidMonetaryAmounts(t *testing.T) {
	invalidAmounts := []float64{
		0.0,
		-0.01,
		-100.0,
		math.NaN(),
		math.Inf(1),
		math.Inf(-1),
	}

	for _, amt := range invalidAmounts {
		err := deductUsageBillingBalance(context.Background(), nil, 104, amt)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid deduction amount")
	}
}

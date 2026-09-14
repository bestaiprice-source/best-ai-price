package repository

import (
	"context"
	"database/sql"
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
	mock.ExpectExec("UPDATE users SET balance = balance - \\$1").
		WithArgs(0.05, int64(101), service.MaxAllowableOverdraftUSD).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	err = deductUsageBillingBalance(context.Background(), tx, 101, 0.05)
	require.NoError(t, err)

	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeductUsageBillingBalance_ExceedsOverdraftClamps(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	// First update matches 0 rows because (balance - $1 >= -$3) is false
	mock.ExpectExec("UPDATE users SET balance = balance - \\$1").
		WithArgs(10.0, int64(102), service.MaxAllowableOverdraftUSD).
		WillReturnResult(sqlmock.NewResult(0, 0))

	// Second query checks if user exists
	mock.ExpectQuery("SELECT true, balance FROM users WHERE id = \\$1").
		WithArgs(int64(102)).
		WillReturnRows(sqlmock.NewRows([]string{"exists", "balance"}).AddRow(true, -0.10))

	// Third update clamps to -MaxAllowableOverdraftUSD
	mock.ExpectExec("UPDATE users SET balance = -\\$1").
		WithArgs(service.MaxAllowableOverdraftUSD, int64(102)).
		WillReturnResult(sqlmock.NewResult(0, 1))

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
	mock.ExpectExec("UPDATE users SET balance = balance - \\$1").
		WithArgs(5.0, int64(999), service.MaxAllowableOverdraftUSD).
		WillReturnResult(sqlmock.NewResult(0, 0))

	mock.ExpectQuery("SELECT true, balance FROM users WHERE id = \\$1").
		WithArgs(int64(999)).
		WillReturnError(sql.ErrNoRows)

	mock.ExpectRollback()

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	err = deductUsageBillingBalance(context.Background(), tx, 999, 5.0)
	require.ErrorIs(t, err, service.ErrUserNotFound)

	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

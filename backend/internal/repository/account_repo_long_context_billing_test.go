package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

const openAILongContextBillingExtraKey = "openai_long_context_billing_enabled"

func newLongContextBulkMock(t *testing.T) (*accountRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })
	return newAccountRepositoryWithSQL(client, db, nil), mock
}

func TestBulkUpdateLongContextBillingCommitsWithOutbox(t *testing.T) {
	repo, mock := newLongContextBulkMock(t)
	mock.ExpectBegin()
	// Migration 175 propagates shadows and their notifications inside this UPDATE.
	mock.ExpectExec(`(?s)UPDATE accounts SET extra = .*WHERE id = ANY`).
		WithArgs(sqlmock.AnyArg(), `{1,2}`).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	rows, err := repo.BulkUpdate(context.Background(), []int64{1, 2}, service.AccountBulkUpdate{
		Extra: map[string]any{openAILongContextBillingExtraKey: true},
	})
	require.NoError(t, err)
	require.EqualValues(t, 2, rows)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBulkUpdateLongContextBillingRollsBackFailures(t *testing.T) {
	for _, stage := range []string{"update or trigger", "outbox"} {
		t.Run(stage, func(t *testing.T) {
			repo, mock := newLongContextBulkMock(t)
			failure := errors.New(stage + " failed")
			mock.ExpectBegin()
			update := mock.ExpectExec(`(?s)UPDATE accounts SET extra = .*WHERE id = ANY`)
			if stage == "update or trigger" {
				update.WillReturnError(failure)
			} else {
				update.WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).WillReturnError(failure)
			}
			mock.ExpectRollback()
			rows, err := repo.BulkUpdate(context.Background(), []int64{1}, service.AccountBulkUpdate{
				Extra: map[string]any{openAILongContextBillingExtraKey: false},
			})
			require.ErrorIs(t, err, failure)
			require.Zero(t, rows)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

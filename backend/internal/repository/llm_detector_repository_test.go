//go:build unit

package repository

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestDetectorRepositoryOwnerScope(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	r := NewLLMDetectorRepository(db)
	ctx := context.Background()
	const jobID = "11111111-1111-4111-8111-111111111111"
	const planID = "22222222-2222-4222-8222-222222222222"
	const reportID = "33333333-3333-4333-8333-333333333333"
	_, err = r.GetJobForOwner(ctx, jobID, 0)
	require.ErrorIs(t, err, sql.ErrNoRows)
	_, err = r.GetPlanForOwner(ctx, planID, 0)
	require.ErrorIs(t, err, sql.ErrNoRows)
	_, err = r.GetReportForOwner(ctx, jobID, reportID, 0)
	require.ErrorIs(t, err, sql.ErrNoRows)
	mock.ExpectQuery(`(?s)FROM monitor_jobs WHERE id=\$1 AND owner_user_id=\$2 AND source IN`).WithArgs(jobID, int64(2)).WillReturnError(sql.ErrNoRows)
	_, err = r.GetJobForOwner(ctx, jobID, 2)
	require.ErrorIs(t, err, sql.ErrNoRows)
	mock.ExpectQuery(`(?s)WHERE p.id=\$1 AND p.owner_user_id=\$2 AND p.source IN`).WithArgs(planID, int64(2)).WillReturnError(sql.ErrNoRows)
	_, err = r.GetPlanForOwner(ctx, planID, 2)
	require.ErrorIs(t, err, sql.ErrNoRows)
	mock.ExpectQuery(`(?s)WHERE r.id=\$1 AND j.id=\$2 AND j.owner_user_id=\$3 AND r.deleted_at IS NULL`).WithArgs(reportID, jobID, int64(2)).WillReturnError(sql.ErrNoRows)
	_, err = r.GetReportForOwner(ctx, jobID, reportID, 2)
	require.ErrorIs(t, err, sql.ErrNoRows)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDetectorRepositoryRejectsMalformedIDsWithoutDatabase(t *testing.T) {
	r := NewLLMDetectorRepository(nil)
	ctx := context.Background()
	const valid = "11111111-1111-4111-8111-111111111111"
	for _, id := range []string{"", "not-a-uuid", "11111111111141118111111111111111", "urn:uuid:" + valid, "g1111111-1111-4111-8111-111111111111"} {
		t.Run(id, func(t *testing.T) {
			_, err := r.GetPlanForOwner(ctx, id, 1)
			require.ErrorIs(t, err, sql.ErrNoRows)
			_, err = r.GetJobForOwner(ctx, id, 1)
			require.ErrorIs(t, err, sql.ErrNoRows)
			_, err = r.GetReportForOwner(ctx, valid, id, 1)
			require.ErrorIs(t, err, sql.ErrNoRows)
			_, err = r.GetReportForOwner(ctx, id, valid, 1)
			require.ErrorIs(t, err, sql.ErrNoRows)
		})
	}
}

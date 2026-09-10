//go:build integration

package repository

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestQuestionAssessmentSummariesPostgres(t *testing.T) {
	db := monitorBudgetTestDB(t)
	ctx := context.Background()
	raw, err := migrations.FS.ReadFile("247_monitor_question_reviews.sql")
	require.NoError(t, err)
	_, err = db.Exec(string(raw))
	require.NoError(t, err)
	_, err = db.Exec(`ALTER TABLE accounts ADD deleted_at timestamptz; ALTER TABLE groups ADD deleted_at timestamptz; CREATE TABLE account_groups(account_id bigint,group_id bigint,PRIMARY KEY(account_id,group_id)); INSERT INTO accounts(id) VALUES(1),(2); INSERT INTO account_groups VALUES(1,1),(2,2);`)
	require.NoError(t, err)
	repo := NewQuestionReviewRepository(db)
	id, err := repo.SaveQuestion(ctx, service.QuestionRecord{AccountID: 1, CreatedBy: 1, RequestModel: "model", Prompt: "private prompt", Answer: "private answer", TransportState: "completed"})
	require.NoError(t, err)
	_, err = repo.ReviewQuestion(ctx, service.QuestionReview{RecordID: id, ReviewedBy: 1, Verdict: "degraded", Reason: "private reason"}, 0)
	require.NoError(t, err)
	items, err := repo.QuestionAssessments(ctx, "account", []int64{1, 2})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.EqualValues(t, 1, items[0].Degraded)
	items, err = repo.QuestionAssessments(ctx, "group", []int64{2})
	require.NoError(t, err)
	require.Empty(t, items)
	items, err = repo.QuestionAssessments(ctx, "group", []int64{1})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.EqualValues(t, 1, items[0].Degraded)
	_, err = repo.SaveQuestion(ctx, service.QuestionRecord{AccountID: 1, CreatedBy: 1, RequestModel: "model", Prompt: "new", TransportState: "failed"})
	require.NoError(t, err)
	items, err = repo.QuestionAssessments(ctx, "account", []int64{1})
	require.NoError(t, err)
	require.EqualValues(t, 1, items[0].Degraded)
	_, err = repo.ReviewQuestion(ctx, service.QuestionReview{RecordID: id, ReviewedBy: 1, Verdict: "unlabeled", Reason: "reset"}, 1)
	require.NoError(t, err)
	items, err = repo.QuestionAssessments(ctx, "account", []int64{1})
	require.NoError(t, err)
	require.Zero(t, items[0].Degraded)
	require.EqualValues(t, 1, items[0].Unlabeled)
	_, err = db.Exec(`DELETE FROM account_groups WHERE account_id=1`)
	require.NoError(t, err)
	items, err = repo.QuestionAssessments(ctx, "group", []int64{1})
	require.NoError(t, err)
	require.Empty(t, items)
	_, err = db.Exec(`UPDATE accounts SET deleted_at=clock_timestamp() WHERE id=1`)
	require.NoError(t, err)
	items, err = repo.QuestionAssessments(ctx, "account", []int64{1})
	require.NoError(t, err)
	require.Empty(t, items)
}

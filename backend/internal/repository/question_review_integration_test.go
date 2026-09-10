//go:build integration

package repository

import (
	"context"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestQuestionReviewPostgres(t *testing.T) {
	ctx := context.Background()
	db := monitorBudgetTestDB(t)
	raw, err := migrations.FS.ReadFile("247_monitor_question_reviews.sql")
	require.NoError(t, err)
	_, err = db.Exec(string(raw))
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO accounts(id) VALUES(1)`)
	require.NoError(t, err)
	repo := NewQuestionReviewRepository(db)
	prompt := " don't search the internet, who is Thibault Sottiaux on X "
	id, err := repo.SaveQuestion(ctx, service.QuestionRecord{AccountID: 1, CreatedBy: 1, RequestModel: "test", Prompt: prompt, Answer: "fixture", TransportState: "completed"})
	require.NoError(t, err)
	results := make(chan error, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.ReviewQuestion(ctx, service.QuestionReview{RecordID: id, ReviewedBy: 1, Verdict: "degraded", Reason: "manual fixture"}, 0)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else {
			require.ErrorIs(t, err, service.ErrQuestionReviewConflict)
		}
	}
	require.Equal(t, 1, success)
	_, err = repo.ReviewQuestion(ctx, service.QuestionReview{RecordID: id, ReviewedBy: 2, Verdict: "unlabeled", Reason: "insufficient evidence"}, 1)
	require.NoError(t, err)
	history, err := repo.ListQuestionReviews(ctx, id, 20, 0)
	require.NoError(t, err)
	require.Len(t, history, 2)
	require.Equal(t, "unlabeled", history[0].Verdict)
	require.EqualValues(t, 2, history[0].Revision)
	records, err := repo.ListQuestions(ctx, 1, 20, 0)
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, prompt, records[0].Prompt)
	// The minimal accounts fixture has no status column: reviews cannot mutate it.
	_, err = repo.ReviewQuestion(ctx, service.QuestionReview{RecordID: id, ReviewedBy: 1, Verdict: "disabled", Reason: "not a review label"}, 2)
	require.ErrorIs(t, err, service.ErrQuestionReviewDenied)
}

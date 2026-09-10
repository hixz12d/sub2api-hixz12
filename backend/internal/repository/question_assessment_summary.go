package repository

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

func (r *QuestionReviewRepository) QuestionAssessments(ctx context.Context, scope string, ids []int64) ([]service.QuestionAssessmentSummary, error) {
	if len(ids) == 0 || len(ids) > 100 {
		return nil, service.ErrQuestionReviewDenied
	}
	for _, id := range ids {
		if id <= 0 {
			return nil, service.ErrQuestionReviewDenied
		}
	}
	membership := `SELECT id AS scope_id,id AS account_id FROM accounts WHERE id=ANY($1) AND deleted_at IS NULL`
	if scope == "group" {
		membership = `SELECT ag.group_id AS scope_id,ag.account_id FROM account_groups ag JOIN accounts a ON a.id=ag.account_id JOIN groups g ON g.id=ag.group_id WHERE ag.group_id=ANY($1) AND a.deleted_at IS NULL AND g.deleted_at IS NULL`
	} else if scope != "account" {
		return nil, service.ErrQuestionReviewDenied
	}
	// The last reviewed record per account/model wins, including an explicit reset
	// to unlabeled. A newer unreviewed question never clears an existing assessment.
	rows, err := r.db.QueryContext(ctx, `WITH membership AS (`+membership+`), latest AS (
 SELECT DISTINCT ON(q.account_id,q.request_model) q.account_id,q.request_model,r.verdict
 FROM monitor_question_records q JOIN monitor_question_reviews r ON r.record_id=q.id
 WHERE q.account_id IN(SELECT account_id FROM membership)
 ORDER BY q.account_id,q.request_model,r.created_at DESC,r.id DESC
 ) SELECT m.scope_id,count(*) FILTER(WHERE l.verdict='normal'),count(*) FILTER(WHERE l.verdict='degraded'),count(*) FILTER(WHERE l.verdict='unlabeled') FROM membership m JOIN latest l ON l.account_id=m.account_id GROUP BY m.scope_id ORDER BY m.scope_id`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []service.QuestionAssessmentSummary{}
	for rows.Next() {
		var item service.QuestionAssessmentSummary
		if err = rows.Scan(&item.ID, &item.Normal, &item.Degraded, &item.Unlabeled); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

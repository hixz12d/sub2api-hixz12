package repository

import (
	"context"
	"database/sql"
	"strings"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
)

type QuestionReviewRepository struct{ db *sql.DB }

func NewQuestionReviewRepository(db *sql.DB) *QuestionReviewRepository {
	return &QuestionReviewRepository{db: db}
}
func (r *QuestionReviewRepository) SaveQuestion(ctx context.Context, input service.QuestionRecord) (string, error) {
	if input.AccountID <= 0 || input.CreatedBy <= 0 || len(input.RequestModel) == 0 || len(input.RequestModel) > 200 || len(input.Prompt) == 0 || len(input.Prompt) > 4096 || len(input.Answer) > 32768 || !utf8.ValidString(input.Prompt) || !utf8.ValidString(input.Answer) || !utf8.ValidString(input.RequestModel) {
		return "", service.ErrQuestionReviewDenied
	}
	if input.TransportState != "completed" && input.TransportState != "failed" && input.TransportState != "incomplete" {
		return "", service.ErrQuestionReviewDenied
	}
	id := uuid.NewString()
	_, err := r.db.ExecContext(ctx, `INSERT INTO monitor_question_records(id,account_id,created_by,request_model,prompt,answer,transport_state) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, input.AccountID, input.CreatedBy, input.RequestModel, input.Prompt, input.Answer, input.TransportState)
	return id, err
}
func (r *QuestionReviewRepository) ReviewQuestion(ctx context.Context, input service.QuestionReview, expected int64) (service.QuestionReview, error) {
	if !validDetectorResourceID(input.RecordID) || input.ReviewedBy <= 0 || expected < 0 || strings.TrimSpace(input.Reason) == "" || len(input.Reason) > 2000 || !utf8.ValidString(input.Reason) || (input.Verdict != "normal" && input.Verdict != "degraded" && input.Verdict != "unlabeled") {
		return service.QuestionReview{}, service.ErrQuestionReviewDenied
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return service.QuestionReview{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var record string
	if err = tx.QueryRowContext(ctx, `SELECT id FROM monitor_question_records WHERE id=$1 FOR UPDATE`, input.RecordID).Scan(&record); err != nil {
		return service.QuestionReview{}, err
	}
	var current int64
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(revision),0) FROM monitor_question_reviews WHERE record_id=$1`, record).Scan(&current); err != nil {
		return service.QuestionReview{}, err
	}
	if current != expected || current >= 9223372036854775806 {
		return service.QuestionReview{}, service.ErrQuestionReviewConflict
	}
	input.ID = uuid.NewString()
	input.Revision = current + 1
	err = tx.QueryRowContext(ctx, `INSERT INTO monitor_question_reviews(id,record_id,reviewed_by,verdict,reason,revision) VALUES($1,$2,$3,$4,$5,$6) RETURNING created_at`, input.ID, record, input.ReviewedBy, input.Verdict, input.Reason, input.Revision).Scan(&input.CreatedAt)
	if err != nil {
		return service.QuestionReview{}, err
	}
	if err = tx.Commit(); err != nil {
		return service.QuestionReview{}, err
	}
	return input, nil
}
func (r *QuestionReviewRepository) ListQuestions(ctx context.Context, account int64, limit, offset int) ([]service.QuestionRecord, error) {
	if account <= 0 || limit < 1 || limit > 100 || offset < 0 || offset > 10000 {
		return nil, service.ErrQuestionReviewDenied
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id,account_id,created_by,request_model,prompt,answer,transport_state,created_at FROM monitor_question_records WHERE account_id=$1 ORDER BY created_at DESC,id LIMIT $2 OFFSET $3`, account, limit, offset)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := []service.QuestionRecord{}
	for rows.Next() {
		var item service.QuestionRecord
		if err = rows.Scan(&item.ID, &item.AccountID, &item.CreatedBy, &item.RequestModel, &item.Prompt, &item.Answer, &item.TransportState, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
func (r *QuestionReviewRepository) ListQuestionReviews(ctx context.Context, record string, limit, offset int) ([]service.QuestionReview, error) {
	if !validDetectorResourceID(record) || limit < 1 || limit > 100 || offset < 0 || offset > 10000 {
		return nil, service.ErrQuestionReviewDenied
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id,record_id,reviewed_by,verdict,reason,revision,created_at FROM monitor_question_reviews WHERE record_id=$1 ORDER BY revision DESC LIMIT $2 OFFSET $3`, record, limit, offset)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := []service.QuestionReview{}
	for rows.Next() {
		var item service.QuestionReview
		if err = rows.Scan(&item.ID, &item.RecordID, &item.ReviewedBy, &item.Verdict, &item.Reason, &item.Revision, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

package service

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrQuestionReviewDenied = errors.New("question review unavailable")
var ErrQuestionReviewConflict = errors.New("question review revision conflict")

type QuestionRecord struct {
	ID             string    `json:"id"`
	AccountID      int64     `json:"account_id"`
	CreatedBy      int64     `json:"created_by"`
	RequestModel   string    `json:"request_model"`
	Prompt         string    `json:"prompt"`
	Answer         string    `json:"answer"`
	TransportState string    `json:"transport_state"`
	CreatedAt      time.Time `json:"created_at"`
}
type QuestionReview struct {
	ID         string    `json:"id"`
	RecordID   string    `json:"record_id"`
	ReviewedBy int64     `json:"reviewed_by"`
	Verdict    string    `json:"verdict"`
	Reason     string    `json:"reason"`
	Revision   int64     `json:"revision"`
	CreatedAt  time.Time `json:"created_at"`
}
type QuestionReviewStore interface {
	SaveQuestion(context.Context, QuestionRecord) (string, error)
	ReviewQuestion(context.Context, QuestionReview, int64) (QuestionReview, error)
	ListQuestions(context.Context, int64, int, int) ([]QuestionRecord, error)
	ListQuestionReviews(context.Context, string, int, int) ([]QuestionReview, error)
}
type QuestionReviewService struct {
	Store QuestionReviewStore
	Users CapabilityUsers
}

func NewQuestionReviewService(store QuestionReviewStore, users UserRepository) *QuestionReviewService {
	return &QuestionReviewService{Store: store, Users: users}
}

func (s *QuestionReviewService) Authorize(ctx context.Context, actor int64) error {
	return s.admit(ctx, actor)
}

func (s *QuestionReviewService) Save(ctx context.Context, actor int64, capture *QuestionCapture) (string, error) {
	if err := s.admit(ctx, actor); err != nil {
		return "", err
	}
	if capture == nil {
		return "", ErrQuestionReviewDenied
	}
	record := capture.Record()
	if record.CreatedBy != actor {
		return "", ErrQuestionReviewDenied
	}
	return s.Store.SaveQuestion(ctx, record)
}

func (s *QuestionReviewService) admit(ctx context.Context, actor int64) error {
	if s == nil || s.Store == nil || s.Users == nil || actor <= 0 {
		return ErrQuestionReviewDenied
	}
	user, err := s.Users.GetByID(ctx, actor)
	if err != nil || user == nil || user.ID != actor || !user.IsAdmin() || !user.IsActive() || user.DeletedAt != nil {
		return ErrQuestionReviewDenied
	}
	return nil
}
func (s *QuestionReviewService) Review(ctx context.Context, actor int64, recordID, verdict, reason string, revision int64) (QuestionReview, error) {
	if err := s.admit(ctx, actor); err != nil {
		return QuestionReview{}, err
	}
	if verdict != "normal" && verdict != "degraded" && verdict != "unlabeled" {
		return QuestionReview{}, ErrQuestionReviewDenied
	}
	reason = strings.TrimSpace(reason)
	if reason == "" || len(reason) > 2000 || !utf8.ValidString(reason) || revision < 0 {
		return QuestionReview{}, ErrQuestionReviewDenied
	}
	return s.Store.ReviewQuestion(ctx, QuestionReview{RecordID: recordID, ReviewedBy: actor, Verdict: verdict, Reason: reason}, revision)
}
func (s *QuestionReviewService) List(ctx context.Context, actor, account int64, limit, offset int) ([]QuestionRecord, error) {
	if err := s.admit(ctx, actor); err != nil {
		return nil, err
	}
	return s.Store.ListQuestions(ctx, account, limit, offset)
}
func (s *QuestionReviewService) History(ctx context.Context, actor int64, record string, limit, offset int) ([]QuestionReview, error) {
	if err := s.admit(ctx, actor); err != nil {
		return nil, err
	}
	return s.Store.ListQuestionReviews(ctx, record, limit, offset)
}

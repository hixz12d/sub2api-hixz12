package service

import "context"

// Counts describe historical account/model assessments, not current availability.
type QuestionAssessmentSummary struct {
	ID        int64 `json:"id"`
	Normal    int64 `json:"normal"`
	Degraded  int64 `json:"degraded"`
	Unlabeled int64 `json:"unlabeled"`
}
type QuestionAssessmentStore interface {
	QuestionAssessments(context.Context, string, []int64) ([]QuestionAssessmentSummary, error)
}

func (s *QuestionReviewService) Summaries(ctx context.Context, actor int64, scope string, ids []int64) ([]QuestionAssessmentSummary, error) {
	if err := s.admit(ctx, actor); err != nil {
		return nil, err
	}
	store, ok := s.Store.(QuestionAssessmentStore)
	if !ok || (scope != "account" && scope != "group") || len(ids) == 0 || len(ids) > 100 {
		return nil, ErrQuestionReviewDenied
	}
	for _, id := range ids {
		if id <= 0 {
			return nil, ErrQuestionReviewDenied
		}
	}
	return store.QuestionAssessments(ctx, scope, ids)
}

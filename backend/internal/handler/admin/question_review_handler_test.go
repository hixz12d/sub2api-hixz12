//go:build unit

package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type questionHandlerStore struct {
	service.QuestionReviewStore
	calls    int
	conflict bool
}

func (f *questionHandlerStore) ReviewQuestion(_ context.Context, q service.QuestionReview, _ int64) (service.QuestionReview, error) {
	f.calls++
	if f.conflict {
		return q, service.ErrQuestionReviewConflict
	}
	q.Revision = 1
	return q, nil
}

type questionHandlerUsers struct {
	service.UserRepository
	disabled bool
}

func (f questionHandlerUsers) GetByID(context.Context, int64) (*service.User, error) {
	status := service.StatusActive
	if f.disabled {
		status = "disabled"
	}
	return &service.User{ID: 1, Role: service.RoleAdmin, Status: status}, nil
}
func TestQuestionReviewHTTP(t *testing.T) {
	for _, tc := range []struct {
		name, role, body   string
		disabled, conflict bool
		status             int
	}{
		{"allowed", "admin", `{"verdict":"normal","reason":"checked","expected_revision":0}`, false, false, 200},
		{"user", "user", `{}`, false, false, 403},
		{"revoked", "admin", `{"verdict":"normal","reason":"checked","expected_revision":0}`, true, false, 403},
		{"conflict", "admin", `{"verdict":"normal","reason":"checked","expected_revision":0}`, false, true, 409},
		{"forged_answer", "admin", `{"verdict":"normal","reason":"checked","expected_revision":0,"answer":"forged"}`, false, false, 400},
		{"trailing", "admin", `{"verdict":"normal","reason":"checked","expected_revision":0} {}`, false, false, 400},
		{"missing_revision", "admin", `{"verdict":"normal","reason":"checked"}`, false, false, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &questionHandlerStore{conflict: tc.conflict}
			h := &AccountHandler{questionReviews: &service.QuestionReviewService{Store: store, Users: questionHandlerUsers{disabled: tc.disabled}}}
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1})
				c.Set(string(middleware.ContextKeyUserRole), tc.role)
			})
			router.POST("/:record/reviews", h.ReviewQuestion)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/record/reviews", strings.NewReader(tc.body)))
			require.Equal(t, tc.status, recorder.Code)
			if tc.status != 200 && tc.status != 409 {
				require.Zero(t, store.calls)
			}
		})
	}
}

func TestQuestionTestRejectsInvalidReasoningEffortBeforeCapture(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	// Nil services prove the request is rejected before authorization, capture or upstream work.
	router.POST("/accounts/:id/test", (&AccountHandler{}).Test)
	recorder := httptest.NewRecorder()
	body := `{"model_id":"gpt-5.4","prompt":"q","mode":"question","reasoning_effort":"extreme"}`
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/accounts/1/test", strings.NewReader(body)))
	require.Equal(t, http.StatusBadRequest, recorder.Code)
}

//go:build unit

package admin

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

type assessmentHandlerStore struct {
	service.QuestionReviewStore
	calls int
}

func (s *assessmentHandlerStore) QuestionAssessments(context.Context, string, []int64) ([]service.QuestionAssessmentSummary, error) {
	s.calls++
	return []service.QuestionAssessmentSummary{{ID: 1, Degraded: 1}}, nil
}
func TestQuestionAssessmentHTTP(t *testing.T) {
	for _, tc := range []struct {
		name, role, query string
		disabled          bool
		status            int
	}{
		{"admin", "admin", "?scope=account&ids=1", false, 200},
		{"user", "user", "?scope=account&ids=1", false, 403},
		{"revoked", "admin", "?scope=account&ids=1", true, 403},
		{"bad id", "admin", "?scope=account&ids=-1", false, 400},
		{"bad scope", "admin", "?scope=all&ids=1", false, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &assessmentHandlerStore{}
			h := &AccountHandler{questionReviews: &service.QuestionReviewService{Store: store, Users: questionHandlerUsers{disabled: tc.disabled}}}
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1})
				c.Set(string(middleware.ContextKeyUserRole), tc.role)
			})
			router.GET("/", h.QuestionAssessments)
			out := httptest.NewRecorder()
			router.ServeHTTP(out, httptest.NewRequest(http.MethodGet, "/"+tc.query, nil))
			require.Equal(t, tc.status, out.Code)
			if tc.status != 200 {
				require.Zero(t, store.calls)
			}
		})
	}
}

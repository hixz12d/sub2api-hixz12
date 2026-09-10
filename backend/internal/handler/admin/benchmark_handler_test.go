//go:build unit

package admin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type benchmarkHandlerFake struct {
	BenchmarkAdminService
	calls int
	err   error
	actor int64
}

func (f *benchmarkHandlerFake) Approve(_ context.Context, actor int64, _ string) error {
	f.calls++
	f.actor = actor
	return f.err
}
func (f *benchmarkHandlerFake) Activate(_ context.Context, actor int64, _, _ string, _ int64) (int64, error) {
	f.calls++
	f.actor = actor
	return 2, f.err
}
func (f *benchmarkHandlerFake) List(_ context.Context, _ int64, _, _ int) ([]service.BenchmarkReleaseSummary, []service.BenchmarkChannel, error) {
	f.calls++
	return []service.BenchmarkReleaseSummary{}, []service.BenchmarkChannel{}, f.err
}

func TestBenchmarkHandlerAdmission(t *testing.T) {
	for _, tc := range []struct {
		name, role, body string
		actor            int64
		status, calls    int
	}{
		{"anonymous", "", "{}", 0, 403, 0},
		{"user", "user", "{}", 2, 403, 0},
		{"missing role", "", "{}", 1, 403, 0},
		{"forged receipt", "admin", `{"receipt":{"calibration_valid":true}}`, 1, 400, 0},
		{"trailing JSON", "admin", "{} {}", 1, 400, 0},
		{"approved by server", "admin", "{}", 1, 200, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &benchmarkHandlerFake{}
			h := &BenchmarkHandler{service: fake}
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: tc.actor})
				c.Set(string(middleware.ContextKeyUserRole), tc.role)
			})
			router.POST("/:id/approve", h.Approve)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/candidate/approve", strings.NewReader(tc.body)))
			require.Equal(t, tc.status, recorder.Code)
			require.Equal(t, tc.calls, fake.calls)
			if fake.calls > 0 {
				require.EqualValues(t, 1, fake.actor)
			}
		})
	}
}

func TestBenchmarkHandlerErrorsAndCAS(t *testing.T) {
	for _, tc := range []struct {
		name, body    string
		err           error
		status, calls int
	}{
		{"missing CAS", `{"channel":"default"}`, nil, 400, 0},
		{"stale CAS", `{"channel":"default","expected_revision":0}`, service.ErrBenchmarkRelease, 409, 1},
		{"validator unavailable", `{"channel":"default","expected_revision":1}`, service.ErrBenchmarkValidator, 503, 1},
		{"private error", `{"channel":"default","expected_revision":1}`, errors.New("postgres://secret:password@private-host internal SQL"), 500, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &benchmarkHandlerFake{err: tc.err}
			h := &BenchmarkHandler{service: fake}
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1})
				c.Set(string(middleware.ContextKeyUserRole), "admin")
			})
			router.POST("/:id/activate", h.Activate)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/candidate/activate", strings.NewReader(tc.body)))
			require.Equal(t, tc.status, recorder.Code)
			require.Equal(t, tc.calls, fake.calls)
			require.NotContains(t, recorder.Body.String(), "private-host")
			require.NotContains(t, recorder.Body.String(), "password")
		})
	}
}

//go:build unit

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type blueprintWaitingSlotCache struct {
	fakeConcurrencyCache
	acquires        atomic.Int32
	waits           atomic.Int32
	cleanupCanceled atomic.Bool
}

func (c *blueprintWaitingSlotCache) AcquireAccountSlot(ctx context.Context, _ int64, _ int, _ string) (bool, error) {
	c.acquires.Add(1)
	return false, ctx.Err()
}
func (c *blueprintWaitingSlotCache) IncrementAccountWaitCount(context.Context, int64, int) (bool, error) {
	c.waits.Add(1)
	return true, nil
}
func (c *blueprintWaitingSlotCache) DecrementAccountWaitCount(ctx context.Context, _ int64) error {
	c.cleanupCanceled.Store(ctx.Err() != nil)
	c.waits.Add(-1)
	return nil
}

func TestBlueprintV2AccountQueueRecoveryDeadline(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []string{"/v1/responses", "/v1/messages"} {
		t.Run(endpoint, func(t *testing.T) {
			cache := &blueprintWaitingSlotCache{}
			h := &OpenAIGatewayHandler{
				gatewayService:    &service.OpenAIGatewayService{},
				concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatClaude, 0),
			}
			parent, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, endpoint, nil).WithContext(parent)
			cfg := &config.Config{Gateway: config.GatewayConfig{OpenAIPreoutputRecoveryMode: "bounded_preoutput", OpenAIPreoutputRecoveryMaxElapsedSeconds: 1}}
			service.RecordOpenAILogicalStart(c, time.Now().Add(-800*time.Millisecond))
			budget := service.PrepareOpenAIRetryBudgetWithConfig(c, []byte(`{"stream":true,"input":"hello"}`), cfg)
			originalRequest := c.Request
			selection := &service.AccountSelectionResult{
				Account:  &service.Account{ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth},
				WaitPlan: &service.AccountWaitPlan{AccountID: 1, MaxConcurrency: 1, MaxWaiting: 2, Timeout: 3 * time.Second},
			}
			streamStarted := false
			start := time.Now()
			release, result := h.acquireResponsesAccountSlot(c, nil, "", selection, true, &streamStarted, zap.NewNop())
			require.Equal(t, openAISlotAcquireFailed, result)
			require.Nil(t, release)
			require.Equal(t, http.StatusGatewayTimeout, rec.Code)
			require.Contains(t, rec.Body.String(), "recovery_deadline")
			require.Less(t, time.Since(start), time.Second)
			require.Same(t, originalRequest, c.Request)
			require.NoError(t, parent.Err())
			require.Greater(t, cache.acquires.Load(), int32(1), "must reach the real wait path")
			require.Zero(t, cache.waits.Load())
			require.False(t, cache.cleanupCanceled.Load())
			require.Zero(t, budget.Snapshot().Attempts)
		})
	}
}

func TestBlueprintV2RetryBackoffStopsAtRecoveryDeadline(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	cfg := &config.Config{Gateway: config.GatewayConfig{OpenAIPreoutputRecoveryMode: "bounded_preoutput", OpenAIPreoutputRecoveryMaxElapsedSeconds: 1}}
	service.RecordOpenAILogicalStart(c, time.Now().Add(-900*time.Millisecond))
	body := []byte(`{"stream":true,"input":"hello"}`)
	service.PrepareOpenAIRetryBudgetWithConfig(c, body, cfg)
	service.EnsureOpenAIRetryBudget(c, &service.Account{ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth}, body)
	failure := &service.UpstreamFailoverError{StatusCode: http.StatusBadGateway, Reason: service.OpenAIAttemptFailureReasonCapacity}
	started := time.Now()
	require.False(t, waitOpenAIPreOutputAutoRetry(c, nil, failure, service.OpenAICompactKeepaliveAdjustedWrittenSize(c), 0, 1, 1))
	require.Less(t, time.Since(started), openAIPreOutputAutoRetryDelay)
	require.NoError(t, c.Request.Context().Err())
}

func TestBlueprintV2AcquiredSlotSurvivesPreparationCleanup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(parent)
	cfg := &config.Config{Gateway: config.GatewayConfig{OpenAIPreoutputRecoveryMode: "bounded_preoutput", OpenAIPreoutputRecoveryMaxElapsedSeconds: 1}}
	service.PrepareOpenAIRetryBudgetWithConfig(c, []byte(`{"stream":true,"input":"hello"}`), cfg)
	var releases atomic.Int32
	released := make(chan struct{}, 2)
	selection := &service.AccountSelectionResult{
		Account:     &service.Account{ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth},
		Acquired:    true,
		ReleaseFunc: func() { releases.Add(1); released <- struct{}{} },
	}
	h := &OpenAIGatewayHandler{gatewayService: &service.OpenAIGatewayService{}}
	streamStarted := false
	release, result := h.acquireResponsesAccountSlot(c, nil, "", selection, true, &streamStarted, zap.NewNop())
	require.Equal(t, openAISlotAcquireOK, result)
	require.NotNil(t, release)
	defer release()
	timer := time.NewTimer(50 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-released:
		t.Fatal("slot was released with the preparation context")
	case <-timer.C:
	}
	cancel()
	select {
	case <-released:
	case <-time.After(time.Second):
		t.Fatal("caller cancellation did not release the slot")
	}
	release()
	require.EqualValues(t, 1, releases.Load())
}

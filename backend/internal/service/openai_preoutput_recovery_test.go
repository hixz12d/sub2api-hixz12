package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOutputCommitSnapshotCarriesProtoAndTTFTPhases(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	MarkOpenAIAttemptProtoMajor(c, 2)
	MarkOpenAIAttemptTTFTPhase(c, "frame", 12)
	MarkOpenAIAttemptTTFTPhase(c, "semantic", 34)
	MarkOpenAIAttemptTTFTPhase(c, "visible", 56)
	MarkOpenAIAttemptFailureAttribution(c, OpenAIFailurePhasePreOutput, OpenAIFailureCauseStreamEOF, OpenAIRetryDecisionFailoverOtherAccount)

	snap := OutputCommitSnapshotFromContext(c)
	require.Equal(t, 2, snap.ActualProtoMajor)
	require.Equal(t, 12, snap.FirstFrameMs)
	require.Equal(t, 34, snap.FirstSemanticMs)
	require.Equal(t, 56, snap.FirstVisibleMs)
	require.Equal(t, OpenAIFailurePhasePreOutput, snap.FailurePhase)
	require.Equal(t, OpenAIFailureCauseStreamEOF, snap.FailureCause)
	require.Equal(t, OpenAIRetryDecisionFailoverOtherAccount, snap.RetryDecisionReason)

	// first-write wins for TTFT markers
	MarkOpenAIAttemptTTFTPhase(c, "frame", 999)
	require.Equal(t, 12, OutputCommitSnapshotFromContext(c).FirstFrameMs)
}

func TestAnnotateOpenAIPreOutputFailoverFillsStructuredFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	err := &UpstreamFailoverError{StatusCode: http.StatusBadGateway, SafeToFailoverAfterWrite: true}
	out := annotateOpenAIPreOutputFailover(c, err, OpenAIFailureCauseMissingTerminal, OpenAIRetryDecisionFailoverOtherAccount)
	require.Same(t, err, out)
	require.Equal(t, OpenAIFailurePhasePreOutput, out.FailurePhase)
	require.Equal(t, OpenAIFailureCauseMissingTerminal, out.Cause)
	require.Equal(t, OpenAIRetryDecisionFailoverOtherAccount, out.RetryDecisionReason)

	snap := OutputCommitSnapshotFromContext(c)
	require.Equal(t, OpenAIFailurePhasePreOutput, snap.FailurePhase)
	require.Equal(t, OpenAIFailureCauseMissingTerminal, snap.FailureCause)
}

func TestOpenAIRetryBudgetMaxElapsedBoundedPreoutput(t *testing.T) {
	require.Equal(t, 110*time.Second, openAIRetryBudgetMaxElapsed(nil))

	cfg := &config.Config{}
	cfg.Gateway.OpenAIPreoutputRecoveryMode = "legacy"
	require.Equal(t, 110*time.Second, openAIRetryBudgetMaxElapsed(cfg))

	cfg.Gateway.OpenAIPreoutputRecoveryMode = "bounded_preoutput"
	cfg.Gateway.OpenAIFirstOutputTimeoutSeconds = 30
	require.Equal(t, 110*time.Second, openAIRetryBudgetMaxElapsed(cfg)) // floor

	cfg.Gateway.OpenAIFirstOutputTimeoutSeconds = 90
	require.Equal(t, 230*time.Second, openAIRetryBudgetMaxElapsed(cfg)) // 2*90+50

	cfg.Gateway.OpenAIPreoutputRecoveryMaxElapsedSeconds = 180
	require.Equal(t, 180*time.Second, openAIRetryBudgetMaxElapsed(cfg))
}

func TestPrepareOpenAIRetryBudgetUsesBoundedConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	cfg := &config.Config{}
	cfg.Gateway.OpenAIPreoutputRecoveryMode = "bounded_preoutput"
	cfg.Gateway.OpenAIFirstOutputTimeoutSeconds = 60
	budget := PrepareOpenAIRetryBudgetWithConfig(c, []byte(`{"model":"gpt-5"}`), cfg)
	require.NotNil(t, budget)
	require.Equal(t, 170*time.Second, budget.Snapshot().MaxElapsed)
}

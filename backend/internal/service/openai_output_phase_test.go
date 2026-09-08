package service

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func blueprintPhaseBudget(t *testing.T, cfg *config.Config, started time.Time) (*OpenAIGatewayService, *gin.Context, *Account, *OpenAIRetryBudget) {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	RecordOpenAILogicalStart(c, started)
	body := []byte(`{"model":"gpt-5","input":"hello"}`)
	budget := PrepareOpenAIRetryBudgetWithConfig(c, body, cfg)
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	require.Same(t, budget, EnsureOpenAIRetryBudget(c, account, body))
	return &OpenAIGatewayService{cfg: cfg}, c, account, budget
}

func TestBlueprintV2PhaseDeadlineIncludesPreparationAndFreezesEffort(t *testing.T) {
	cfg := &config.Config{Gateway: config.GatewayConfig{
		OpenAIPreoutputRecoveryMode:                        "bounded_preoutput",
		OpenAIPreoutputRecoveryHighEffortMaxElapsedSeconds: 260,
	}}
	started := time.Now().Add(-45 * time.Second)
	svc, c, account, budget := blueprintPhaseBudget(t, cfg, started)
	ctx, first, err := svc.beginOpenAIHTTPOutputPhase(context.Background(), c, account, "high")
	require.NoError(t, err)
	defer first.close()
	require.Equal(t, started.Add(260*time.Second), first.deadline)
	require.NoError(t, ctx.Err())
	first.close()
	cfg.Gateway.OpenAIPreoutputRecoveryHighEffortMaxElapsedSeconds = 3600
	RecordOpenAILogicalStart(c, time.Now())
	_, second, err := svc.beginOpenAIHTTPOutputPhase(context.Background(), c, account, "low")
	require.NoError(t, err)
	defer second.close()
	require.Equal(t, first.deadline, second.deadline)
	require.Equal(t, started, budget.Snapshot().StartedAt)
}

func TestBlueprintV2PhaseDeadlineExpiredBeforeDispatch(t *testing.T) {
	cfg := &config.Config{Gateway: config.GatewayConfig{OpenAIPreoutputRecoveryMode: "bounded_preoutput"}}
	svc, c, account, budget := blueprintPhaseBudget(t, cfg, time.Now().Add(-111*time.Second))
	_, guard, err := svc.beginOpenAIHTTPOutputPhase(context.Background(), c, account, "low")
	require.ErrorIs(t, err, ErrOpenAIRecoveryDeadline)
	require.Nil(t, guard)
	require.Zero(t, budget.Snapshot().Attempts)
}

func TestBlueprintV2PhaseMixedCandidateCannotDisableBudget(t *testing.T) {
	cfg := &config.Config{Gateway: config.GatewayConfig{OpenAIPreoutputRecoveryMode: "bounded_preoutput"}}
	_, c, _, budget := blueprintPhaseBudget(t, cfg, time.Now())
	candidate := &Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	require.Same(t, budget, EnsureOpenAIRetryBudget(c, candidate, []byte(`{"input":"hello"}`)))
	require.Same(t, budget, OpenAIRetryBudgetFromContext(c))
}

func TestBlueprintV2PhaseCommitCannotReopenAfterDeadline(t *testing.T) {
	ctx, guard := newOpenAIPhaseWatchdog(context.Background(), func() {}, time.Now().Add(-time.Second), ErrOpenAIRecoveryDeadline)
	defer guard.close()
	require.ErrorIs(t, guard.tryCommit(), ErrOpenAIRecoveryDeadline)
	require.ErrorIs(t, context.Cause(ctx), ErrOpenAIRecoveryDeadline)
	require.ErrorIs(t, guard.tryCommit(), ErrOpenAIRecoveryDeadline)
}

func TestBlueprintV2PhaseCommitPreservesExternalDeadline(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ctx, guard := newOpenAIPhaseWatchdog(parent, func() {}, time.Now().Add(time.Hour), ErrOpenAIFirstOutputTimeout)
	defer guard.close()
	require.NoError(t, guard.tryCommit())
	cancel()
	require.ErrorIs(t, context.Cause(ctx), context.Canceled)
}

func TestBlueprintV2PhasePreservesBuilderMetadataAndCallerCause(t *testing.T) {
	parent, cancel := context.WithCancelCause(context.Background())
	defer cancel(context.Canceled)
	requestCtx := WithHTTPUpstreamProfile(context.WithoutCancel(parent), HTTPUpstreamProfileOpenAI)
	ctx, guard := newOpenAIPhaseWatchdog(openAIPhaseParent(parent, requestCtx), func() {}, time.Now().Add(time.Hour), ErrOpenAIFirstOutputTimeout)
	defer guard.close()
	require.Equal(t, HTTPUpstreamProfileOpenAI, HTTPUpstreamProfileFromContext(ctx))
	require.NoError(t, guard.tryCommit())
	cancel(ErrOpenAIStreamIntervalTimeout)
	require.ErrorIs(t, context.Cause(ctx), ErrOpenAIStreamIntervalTimeout)
}

func TestBlueprintV2IndependentHighEffortAttemptLimit(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{OpenAIHighEffortFirstOutputTimeoutSeconds: 120}}}
	require.Zero(t, svc.openAIFirstOutputTimeout("low"))
	for _, effort := range []string{"high", "xhigh", "max"} {
		require.Equal(t, 120*time.Second, svc.openAIFirstOutputTimeout(effort))
	}
}

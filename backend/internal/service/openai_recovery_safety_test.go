package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newRecoverySafetyContext(t *testing.T) (*gin.Context, *OpenAIRetryBudget) {
	t.Helper()
	return recoverySafetyContextForBody(t, []byte(`{"model":"gpt-5","input":"hello"}`))
}

func recoverySafetyContextForBody(t *testing.T, body []byte) (*gin.Context, *OpenAIRetryBudget) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(string(body)))
	account := &Account{ID: 41, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	budget := EnsureOpenAIRetryBudget(c, account, body)
	require.NotNil(t, budget)
	return c, budget
}

func TestOpenAIRecoverySafetyGuardIntersectsReplayPermission(t *testing.T) {
	for _, heartbeat := range []bool{false, true} {
		for _, denyAt := range []string{"attempt", "budget"} {
			t.Run(fmt.Sprintf("%s/heartbeat=%t", denyAt, heartbeat), func(t *testing.T) {
				c, budget := newRecoverySafetyContext(t)
				guard := NewCodexCommitGuard(c)
				require.NoError(t, guard.CanStartAttempt(41))
				if denyAt == "attempt" {
					OpenAIAttemptStateFromContext(c).ReplaySafe = false
				} else {
					budget.MarkBytesEmitted()
				}
				if heartbeat {
					guard.MarkHeartbeat()
				}
				require.False(t, guard.Snapshot().ReplaySafe)
				require.ErrorIs(t, guard.CanStartAttempt(41), ErrOpenAIRetryBudgetExhausted)
				require.ErrorIs(t, ReserveOpenAIUpstreamAttempt(c, 41), ErrOpenAIRetryBudgetExhausted)
				require.Zero(t, budget.Snapshot().Attempts)
			})
		}
	}
}

func TestOpenAIRecoverySafetyStatefulFirstDispatchIsNotReplay(t *testing.T) {
	for _, heartbeat := range []bool{false, true} {
		t.Run(fmt.Sprint(heartbeat), func(t *testing.T) {
			c, budget := recoverySafetyContextForBody(t, []byte(`{"previous_response_id":"resp_owned","input":"hello"}`))
			guard := NewCodexCommitGuard(c)
			if heartbeat {
				guard.MarkHeartbeat()
			}
			require.False(t, guard.Snapshot().ReplaySafe)
			require.NoError(t, ReserveOpenAIUpstreamAttempt(c, 41))
			guard.MarkHeartbeat()
			require.False(t, guard.Snapshot().ReplaySafe)
			require.ErrorIs(t, ReserveOpenAIUpstreamAttempt(c, 41), ErrOpenAIRetryBudgetExhausted)
			require.Equal(t, 1, budget.Snapshot().Attempts)
		})
	}
}

func TestOpenAIRecoverySafetyConcurrentStatefulAdmissionIsOnce(t *testing.T) {
	c, budget := recoverySafetyContextForBody(t, []byte(`{"previous_response_id":"resp_owned","input":"hello"}`))
	results := make(chan error, 32)
	for i := 0; i < 32; i++ {
		go func() { results <- ReserveOpenAIUpstreamAttempt(c, 41) }()
	}
	admitted := 0
	for i := 0; i < 32; i++ {
		err := <-results
		if err == nil {
			admitted++
		} else {
			require.ErrorIs(t, err, ErrOpenAIRetryBudgetExhausted)
		}
	}
	require.Equal(t, 1, admitted)
	require.Equal(t, 1, budget.Snapshot().Attempts)
}

func TestOpenAIRecoverySafetyInitialStatefulAttemptCannotChangeAccount(t *testing.T) {
	body := []byte(`{"previous_response_id":"resp_owned","input":"hello"}`)
	c, budget := recoverySafetyContextForBody(t, body)
	BeginOpenAIAttempt(c, 42, body)
	require.ErrorIs(t, ReserveOpenAIUpstreamAttempt(c, 42), ErrOpenAIRetryBudgetExhausted)
	require.Zero(t, budget.Snapshot().Attempts)
}

func TestOpenAIRecoverySafetyWSTurnGetsItsOwnLedger(t *testing.T) {
	c, firstBudget := newRecoverySafetyContext(t)
	account := &Account{ID: 41, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	firstState := OpenAIAttemptStateFromContext(c)
	firstState.SessionHash, firstState.RouteKey = "session-owner", "route-owner"
	require.NoError(t, ReserveOpenAIUpstreamAttempt(c, 41))
	TrackOpenAIResponseID(c, "resp_first")
	MarkOpenAISemanticOutputStarted(c)
	require.ErrorIs(t, ReserveOpenAIUpstreamAttempt(c, 41), ErrOpenAIRetryBudgetExhausted)

	budget := StartOpenAIRetryBudgetTurn(c, account, []byte(`{"type":"response.create","previous_response_id":"resp_first","input":"next"}`))
	require.NotSame(t, firstBudget, budget)
	state := OpenAIAttemptStateFromContext(c)
	require.NotSame(t, firstState, state)
	require.Equal(t, []string{"resp_first"}, firstState.ResponseIDs, "do not mutate prior ownership evidence")
	require.Empty(t, state.ResponseIDs)
	require.Equal(t, "resp_first", state.PreviousResponseID)
	require.Equal(t, "session-owner", state.SessionHash)
	require.Equal(t, "route-owner", state.RouteKey)
	require.False(t, state.ReplaySafe)
	require.NoError(t, ReserveOpenAIUpstreamAttempt(c, 41))
	require.ErrorIs(t, ReserveOpenAIUpstreamAttempt(c, 41), ErrOpenAIRetryBudgetExhausted)
}

func TestOpenAIRecoverySafetyPreservesErrorChainAndEnvelope(t *testing.T) {
	root := &net.OpError{Op: "private-network-op", Net: "tcp", Err: syscall.ECONNRESET}
	failure := withOpenAIUnderlyingError(&UpstreamFailoverError{StatusCode: 502, Cause: "existing_cause"}, fmt.Errorf("wrapped: %w", root))
	require.ErrorIs(t, failure, syscall.ECONNRESET)
	var operation *net.OpError
	require.ErrorAs(t, failure, &operation)
	require.Same(t, root, operation)
	require.Equal(t, "existing_cause", failure.Cause)
	require.EqualError(t, failure, "upstream error: 502 (failover)")
	encoded, err := json.Marshal(failure)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "private-network-op")
	var absent *UpstreamFailoverError
	require.Nil(t, absent.Unwrap())
}

func TestOpenAIRecoverySafetyTransportRetainsTypedCause(t *testing.T) {
	c, _ := newRecoverySafetyContext(t)
	account := &Account{ID: 41, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	root := &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}
	svc := &OpenAIGatewayService{}
	err := svc.handleOpenAIUpstreamTransportError(context.Background(), c, account, root, false)
	require.ErrorIs(t, err, syscall.ECONNRESET)
	var operation *net.OpError
	require.ErrorAs(t, err, &operation)
	require.Same(t, root, operation)
}

func TestOpenAIRecoverySafetyClassifiesOnlyExplicitFirstDeadline(t *testing.T) {
	for _, tc := range []struct {
		name  string
		err   error
		cause string
	}{
		{"network timeout", &net.DNSError{IsTimeout: true}, OpenAIFailureCauseStreamRead},
		{"generic timeout", errors.New("upstream read timeout"), OpenAIFailureCauseStreamRead},
		{"first output", fmt.Errorf("wrapped: %w", ErrOpenAIFirstOutputTimeout), OpenAIFailureCauseFirstOutputTimeout},
		{"interval", fmt.Errorf("wrapped: %w", ErrOpenAIStreamIntervalTimeout), OpenAIFailureCauseIntervalTimeout},
		{"missing terminal", ErrOpenAIStreamMissingTerminal, OpenAIFailureCauseMissingTerminal},
		{"unexpected EOF", fmt.Errorf("wrapped: %w", io.ErrUnexpectedEOF), OpenAIFailureCauseStreamEOF},
	} {
		t.Run(tc.name, func(t *testing.T) { require.Equal(t, tc.cause, classifyOpenAIStreamScanCause(tc.err)) })
	}
	c, _ := newRecoverySafetyContext(t)
	svc := &OpenAIGatewayService{}
	err := svc.newOpenAIFirstOutputTimeoutError(context.Background(), c, &Account{ID: 41}, time.Now(), "gpt-5", "", time.Second, "response_headers", nil)
	require.ErrorIs(t, err, ErrOpenAIFirstOutputTimeout)
}

func TestOpenAIRecoverySafetyDoesNotAdvertiseForbiddenFailover(t *testing.T) {
	failure := &UpstreamFailoverError{StatusCode: 400, NextAccountAction: NextAccountStop}
	annotateOpenAIPreOutputFailover(nil, failure, OpenAIFailureCauseStreamRead, OpenAIRetryDecisionFailoverOtherAccount)
	require.Equal(t, OpenAIRetryDecisionFailClosed, failure.RetryDecisionReason)
	require.False(t, failure.ShouldRetryNextAccount())
}

func TestOpenAIRecoverySafety520IsNarrowAndCancellationWins(t *testing.T) {
	decision := ClassifyOpenAIRetryFailure(context.Background(), 520, nil, false, true)
	require.True(t, decision.RetryOtherAccount)
	require.False(t, ClassifyOpenAIRetryFailure(context.Background(), 520, nil, true, true).RetryOtherAccount)
	for _, status := range []int{400, 403, 404, 501, 521} {
		require.False(t, ClassifyOpenAIRetryFailure(context.Background(), status, nil, false, true).RetryOtherAccount)
	}
	canceled := &UpstreamFailoverError{StatusCode: 520, Err: fmt.Errorf("wrapped: %w", context.Canceled)}
	decision = ClassifyOpenAIRetryFailure(context.Background(), 520, canceled, false, true)
	require.Equal(t, OpenAIRetryFailureCanceled, decision.Class)
	require.False(t, decision.RetryOtherAccount)
}

func TestOpenAIRecoverySafetyStreamReadersRetainCause(t *testing.T) {
	for _, protocol := range []string{"responses", "passthrough", "messages"} {
		for _, missing := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/missing=%t", protocol, missing), func(t *testing.T) {
				c, _ := newRecoverySafetyContext(t)
				var body io.ReadCloser = &passthroughErrReadCloser{err: io.ErrUnexpectedEOF}
				want := error(io.ErrUnexpectedEOF)
				if missing {
					body = io.NopCloser(strings.NewReader(""))
					want = ErrOpenAIStreamMissingTerminal
				}
				t.Cleanup(func() { _ = body.Close() })
				resp := &http.Response{StatusCode: 200, ProtoMajor: 2, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: body}
				svc := &OpenAIGatewayService{cfg: messagesStageTestConfig()}
				account := messagesStageTestAccount()
				var err error
				switch protocol {
				case "responses":
					_, err = svc.handleStreamingResponse(context.Background(), resp, c, account, time.Now(), "gpt-5", "gpt-5")
				case "passthrough":
					_, err = svc.handleStreamingResponsePassthrough(context.Background(), resp, c, account, time.Now(), "gpt-5", "gpt-5")
				case "messages":
					_, err = svc.handleAnthropicStreamingResponse(context.Background(), resp, c, account, "gpt-5", "gpt-5", "gpt-5", time.Now())
				}
				var failure *UpstreamFailoverError
				require.ErrorAs(t, err, &failure)
				require.ErrorIs(t, err, want)
			})
		}
	}
}

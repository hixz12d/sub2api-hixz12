package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIAttemptWireStateTracksHeartbeatAndSemanticOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)

	ResetOpenAIAttemptWireState(c)
	MarkOpenAIAttemptHeartbeat(c)
	state := OpenAIAttemptWireStateSnapshot(c)
	require.True(t, state.TransportCommitted)
	require.True(t, state.HeartbeatOnly)
	require.False(t, state.SemanticOutputStarted)

	MarkOpenAISemanticOutputStarted(c)
	MarkOpenAIAttemptTerminal(c, "response.completed")
	state = OpenAIAttemptWireStateSnapshot(c)
	require.True(t, state.TransportCommitted)
	require.False(t, state.HeartbeatOnly)
	require.True(t, state.SemanticOutputStarted)
	require.Equal(t, "response.completed", state.TerminalEvent)

	ResetOpenAIAttemptWireState(c)
	state = OpenAIAttemptWireStateSnapshot(c)
	require.Equal(t, OpenAIAttemptWireState{}, state)
}

func TestClassifyOpenAIAttemptFailureSeparatesCapacityAndContext(t *testing.T) {
	capacityCases := []struct {
		status int
		body   []byte
	}{
		{
			status: http.StatusBadRequest,
			body:   []byte(`{"error":{"message":"Selected model is at capacity"}}`),
		},
		{
			status: http.StatusBadGateway,
			body:   []byte(`{"type":"response.failed","response":{"error":{"message":"Our servers are currently overloaded. Please try again later."}}}`),
		},
	}
	contextWindow := &UpstreamFailoverError{
		StatusCode:   http.StatusBadRequest,
		ResponseBody: []byte(`{"error":{"code":"context_length_exceeded"}}`),
	}

	require.Equal(t, "none", ClassifyOpenAIAttemptFailure(nil))
	for _, testCase := range capacityCases {
		require.Equal(t, "capacity", ClassifyOpenAIAttemptFailure(&UpstreamFailoverError{
			StatusCode:   testCase.status,
			ResponseBody: testCase.body,
		}))
	}
	require.Equal(t, "context_window", ClassifyOpenAIAttemptFailure(contextWindow))
	require.Equal(t, "rate_limit", ClassifyOpenAIAttemptFailure(&UpstreamFailoverError{
		StatusCode:   http.StatusTooManyRequests,
		ResponseBody: []byte(`{"error":{"message":"Our servers are currently overloaded"}}`),
	}))
	require.Equal(t, "timeout", ClassifyOpenAIAttemptFailure(errors.New("upstream deadline exceeded")))
	require.Equal(t, "transport", ClassifyOpenAIAttemptFailure(errors.New("http/2 stream was reset")))
}

func TestNewOpenAIStreamFailoverErrorPreservesCapacityReason(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	payload := []byte(`{"type":"response.failed","response":{"error":{"code":"server_is_overloaded","message":"Please retry later."}}}`)

	failoverErr := (&OpenAIGatewayService{}).newOpenAIStreamFailoverError(
		c,
		&Account{ID: 1, Platform: PlatformOpenAI},
		true,
		"",
		payload,
		"Please retry later.",
	)

	require.Equal(t, OpenAIAttemptFailureReasonCapacity, failoverErr.Reason)
	require.Equal(t, "capacity", ClassifyOpenAIAttemptFailure(failoverErr))
	require.NotContains(t, string(failoverErr.ResponseBody), "server_is_overloaded")
}

func TestNewOpenAIStreamFailoverErrorIgnoresOverloadTextOutsideErrorFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	payload := []byte(`{"type":"response.failed","instructions":"Our servers are currently overloaded","response":{"error":{"code":"upstream_error","message":"request failed"},"output":[{"text":"Our servers are currently overloaded"}]}}`)

	failoverErr := (&OpenAIGatewayService{}).newOpenAIStreamFailoverError(
		c,
		&Account{ID: 1, Platform: PlatformOpenAI},
		true,
		"",
		payload,
		"request failed",
	)

	require.Empty(t, failoverErr.Reason)
	require.Equal(t, "upstream", ClassifyOpenAIAttemptFailure(failoverErr))
	require.Equal(t, "upstream", ClassifyOpenAIAttemptFailure(&UpstreamFailoverError{
		StatusCode:   http.StatusBadGateway,
		ResponseBody: payload,
	}))
}

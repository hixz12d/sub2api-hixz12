package service

import (
	"errors"
	"net/http"
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
	require.Equal(t, "timeout", ClassifyOpenAIAttemptFailure(errors.New("upstream deadline exceeded")))
	require.Equal(t, "transport", ClassifyOpenAIAttemptFailure(errors.New("http/2 stream was reset")))
}

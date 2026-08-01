package handler

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIForwardMayFailoverOnlyAfterNonSemanticWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	before := service.OpenAICompactKeepaliveAdjustedWrittenSize(c)

	_, err := fmt.Fprint(c.Writer, ":\n\n")
	require.NoError(t, err)
	c.Writer.Flush()

	require.True(t, openAIForwardMayFailover(c, before, &service.UpstreamFailoverError{
		SafeToFailoverAfterWrite: true,
	}))
	require.False(t, openAIForwardMayFailover(c, before, &service.UpstreamFailoverError{}))
}

func TestOpenAIAccountSwitchLimit(t *testing.T) {
	tests := []struct {
		name       string
		configured int
		failover   *service.UpstreamFailoverError
		want       int
	}{
		{name: "configured budget", configured: 3, failover: nil, want: 3},
		{name: "keepalive committed", configured: 3, failover: &service.UpstreamFailoverError{SafeToFailoverAfterWrite: true}, want: 1},
		{name: "stream terminal failure", configured: 3, failover: &service.UpstreamFailoverError{MaxAccountSwitches: 1}, want: 1},
		{name: "disabled switching remains disabled", configured: 0, failover: &service.UpstreamFailoverError{SafeToFailoverAfterWrite: true, MaxAccountSwitches: 1}, want: 0},
		{name: "configured one remains one", configured: 1, failover: &service.UpstreamFailoverError{}, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, openAIAccountSwitchLimit(tt.configured, tt.failover))
		})
	}

	t.Run("request limit never widens", func(t *testing.T) {
		limit := openAIAccountSwitchLimit(3, &service.UpstreamFailoverError{MaxAccountSwitches: 1})
		require.Equal(t, 1, limit)

		limit = openAIAccountSwitchLimit(limit, &service.UpstreamFailoverError{})
		require.Equal(t, 1, limit)
	})
}

func TestOpenAIRequestAllowsFailoverReplayStopsCanceledClient(t *testing.T) {
	require.False(t, openAIRequestAllowsFailoverReplay(nil))

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	requestCtx, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil).WithContext(requestCtx)

	require.True(t, openAIRequestAllowsFailoverReplay(c))
	cancel()
	require.False(t, openAIRequestAllowsFailoverReplay(c))
}

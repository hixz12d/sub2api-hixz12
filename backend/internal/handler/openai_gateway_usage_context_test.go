package handler

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUsageRecordContextPreservesOrigin(t *testing.T) {
	for _, origin := range []service.RequestOrigin{service.RequestOriginBusiness, service.RequestOriginAvailabilityProbe, service.RequestOriginCapabilityDetector, service.RequestOriginUserDetector} {
		parent, err := service.WithRequestOrigin(context.Background(), origin)
		require.NoError(t, err)
		parent, cancelParent := context.WithCancel(parent)
		cancelParent()
		base, cancelWorker := context.WithCancel(context.Background())
		wrapped := usageRecordContext(parent, base)
		require.NoError(t, wrapped.Err())
		require.Equal(t, origin, service.RequestOriginFromContext(wrapped))
		cancelWorker()
		require.ErrorIs(t, wrapped.Err(), context.Canceled)
		callback := func(ctx context.Context) {
			require.NoError(t, ctx.Err())
			require.Equal(t, origin, service.RequestOriginFromContext(ctx))
		}
		(&GatewayHandler{}).submitUsageRecordTask(parent, callback)
		(&OpenAIGatewayHandler{}).submitUsageRecordTask(parent, callback)
	}
}

func TestSubmitUsageRecordTaskCopiesRequestContext(t *testing.T) {
	parent := context.WithValue(context.Background(), ctxkey.ClientRequestID, "client-request-123")
	parent = context.WithValue(parent, ctxkey.RequestID, "request-456")

	var gotClientRequestID string
	var gotRequestID string
	h := &GatewayHandler{}
	h.submitUsageRecordTask(parent, func(ctx context.Context) {
		gotClientRequestID, _ = ctx.Value(ctxkey.ClientRequestID).(string)
		gotRequestID, _ = ctx.Value(ctxkey.RequestID).(string)
	})

	require.Equal(t, "client-request-123", gotClientRequestID)
	require.Equal(t, "request-456", gotRequestID)
}

func TestOpenAISubmitUsageRecordTaskCopiesRequestContext(t *testing.T) {
	parent := context.WithValue(context.Background(), ctxkey.ClientRequestID, "openai-client-request-123")
	parent = context.WithValue(parent, ctxkey.RequestID, "openai-request-456")

	var gotClientRequestID string
	var gotRequestID string
	h := &OpenAIGatewayHandler{}
	h.submitUsageRecordTask(parent, func(ctx context.Context) {
		gotClientRequestID, _ = ctx.Value(ctxkey.ClientRequestID).(string)
		gotRequestID, _ = ctx.Value(ctxkey.RequestID).(string)
	})

	require.Equal(t, "openai-client-request-123", gotClientRequestID)
	require.Equal(t, "openai-request-456", gotRequestID)
}

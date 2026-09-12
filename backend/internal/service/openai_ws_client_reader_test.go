package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIWSClientReader_ControlCloseAndJoin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name   string
		cause  error
		status coderws.StatusCode
	}{
		{name: "idle", status: coderws.StatusNormalClosure},
		{name: "lease_loss", cause: ErrOpenAIWSIngressLeaseLost, status: coderws.StatusTryAgainLater},
	} {
		t.Run(tc.name, func(t *testing.T) {
			control, cancel := context.WithCancelCause(context.Background())
			defer cancel(context.Canceled)
			ready := make(chan struct{})
			done := make(chan error, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := coderws.Accept(w, r, nil)
				if err != nil {
					done <- err
					return
				}
				defer conn.CloseNow()
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = r
				reader := openAIWSClientReaderForIngress(r.Context(), c, conn)
				reader.start()
				ctx := context.WithValue(control, openAIWSClientReaderContextKey{}, reader)
				close(ready)
				_, _, err = ReadOpenAIWSClientMessage(ctx, conn, 100*time.Millisecond, coderws.StatusNormalClosure, "websocket idle timeout")
				<-reader.done
				done <- err
			}))
			defer server.Close()
			ctx, stop := context.WithTimeout(context.Background(), 3*time.Second)
			defer stop()
			conn, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
			require.NoError(t, err)
			defer conn.CloseNow()
			<-ready
			if tc.cause != nil {
				cancel(tc.cause)
			}
			_, _, err = conn.Read(ctx)
			require.Equal(t, tc.status, coderws.CloseStatus(err))
			select {
			case err := <-done:
				var closeErr *OpenAIWSClientCloseError
				require.ErrorAs(t, err, &closeErr)
				require.Equal(t, tc.status, closeErr.StatusCode())
			case <-ctx.Done():
				t.Fatal("shared reader was not joined")
			}
		})
	}
}

func TestOpenAIWSClientReader_BoundsPendingMessages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(done)
		conn, err := coderws.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = r
		reader := openAIWSClientReaderForIngress(r.Context(), c, conn)
		reader.start()
		// Simulate an active upstream turn: no application message consumer.
		<-reader.done
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	defer conn.CloseNow()
	for i := 0; i < 2; i++ {
		require.NoError(t, conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create"}`)))
	}
	_, _, err = conn.Read(ctx)
	require.Equal(t, coderws.StatusPolicyViolation, coderws.CloseStatus(err))
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("pending-message overflow leaked the reader")
	}
}

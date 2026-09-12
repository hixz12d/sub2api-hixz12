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

func TestNativeWSLifecycle_PingDisconnectAndUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, output := range []bool{false, true} {
		name := "before_output"
		if output {
			name = "after_output"
		}
		t.Run(name, func(t *testing.T) {
			cfg := newOpenAIWSV2TestConfig()
			cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
			cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 60
			cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = false
			terminal := []byte(`{"type":"response.completed","response":{"id":"resp_native","usage":{"input_tokens":11,"output_tokens":7}}}`)
			upstream := &openAIWSCaptureConn{events: [][]byte{terminal}, readDelays: []time.Duration{time.Minute}}
			if output {
				upstream.events = [][]byte{[]byte(`{"type":"response.output_text.delta","delta":"hello"}`), terminal}
				upstream.readDelays = []time.Duration{0, 250 * time.Millisecond}
			}
			pool := newOpenAIWSConnPool(cfg)
			pool.setClientDialerForTest(&openAIWSCaptureDialer{conn: upstream})
			defer pool.Close()
			svc := &OpenAIGatewayService{cfg: cfg, openaiWSPool: pool, cache: &stubGatewayCache{}, toolCorrector: NewCodexToolCorrector()}
			account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
				Extra: map[string]any{"responses_websockets_v2_enabled": true}}
			done := make(chan error, 1)
			turnResult := make(chan *OpenAIForwardResult, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := coderws.Accept(w, r, nil)
				if err != nil {
					done <- err
					return
				}
				defer conn.CloseNow()
				_, payload, err := conn.Read(r.Context())
				if err != nil {
					done <- err
					return
				}
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = r
				done <- svc.ProxyResponsesWebSocketFromClient(r.Context(), c, conn, account, "test-token", payload,
					&OpenAIWSIngressHooks{AfterTurn: func(_ int, result *OpenAIForwardResult, _ error) { turnResult <- result }})
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			conn, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
			require.NoError(t, err)
			defer conn.CloseNow()
			require.NoError(t, conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5","input":"hello"}`)))
			if output {
				_, _, err = conn.Read(ctx)
				require.NoError(t, err)
			}
			require.Eventually(t, func() bool { upstream.mu.Lock(); defer upstream.mu.Unlock(); return len(upstream.events) == 0 }, time.Second, 5*time.Millisecond)
			clientDone := make(chan struct{})
			go func() { defer close(clientDone); _, _, _ = conn.Read(ctx) }()
			pingCtx, stopPing := context.WithTimeout(ctx, time.Second)
			err = conn.Ping(pingCtx)
			stopPing()
			require.NoError(t, err)
			_ = conn.CloseNow()
			select {
			case err := <-done:
				if output {
					require.NoError(t, err)
				} else {
					require.True(t, isOpenAIWSClientDisconnectError(err), "%v", err)
				}
			case <-ctx.Done():
				t.Fatal("native turn did not stop after client disconnect")
			}
			select {
			case result := <-turnResult:
				if output {
					require.NotNil(t, result)
					require.EqualValues(t, 7, result.Usage.OutputTokens)
					require.True(t, result.ClientDisconnect)
				}
			default:
				t.Fatal("turn release/billing hook was skipped")
			}
			<-clientDone
			upstream.mu.Lock()
			closed := upstream.closed
			upstream.mu.Unlock()
			require.True(t, closed, "disconnected upstream must not be reused")
		})
	}
}

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type bridgeLifecycleUpstream struct {
	httpUpstreamRecorder
	do func(*http.Request) (*http.Response, error)
}

func (u *bridgeLifecycleUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return u.do(req)
}

func bridgeLifecycleTurn(t *testing.T, ctx context.Context, cfg *config.Config, upstream *bridgeLifecycleUpstream, write func([]byte) error) (*OpenAIForwardResult, error) {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1}
	payload := []byte(`{"type":"response.create","model":"gpt-5","input":"hello"}`)
	return svc.proxyOpenAIWSHTTPBridgeTurn(ctx, c, account, "test-token", payload, len(payload), "gpt-5", "", "", "", "", 1, write)
}

func TestHTTPBridgeLifecycle_CancelBeforeHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{})
	upstream := &bridgeLifecycleUpstream{do: func(req *http.Request) (*http.Response, error) {
		close(entered)
		<-req.Context().Done()
		return nil, req.Context().Err()
	}}
	done := make(chan error, 1)
	go func() {
		_, err := bridgeLifecycleTurn(t, ctx, &config.Config{}, upstream, func([]byte) error { return nil })
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("upstream not entered")
	}
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
		var failover *UpstreamFailoverError
		require.False(t, errors.As(err, &failover), "client cancellation must not trigger another upstream request")
	case <-time.After(3 * time.Second):
		t.Fatal("header wait retained a canceled request")
	}
}

func TestHTTPBridgeLifecycle_HeaderWaitIsBounded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 1
	upstream := &bridgeLifecycleUpstream{do: func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	_, err := bridgeLifecycleTurn(t, ctx, cfg, upstream, func([]byte) error { return nil })
	require.ErrorIs(t, err, errOpenAIWSHTTPBridgeReadTimeout)
	var failover *UpstreamFailoverError
	require.ErrorAs(t, err, &failover)
	require.Equal(t, http.StatusGatewayTimeout, failover.StatusCode)
	require.JSONEq(t, `{"error":{"type":"upstream_timeout","message":"Upstream response timed out"}}`, string(failover.ResponseBody))
}

func TestHTTPBridgeLifecycle_BodyStallAndFirstOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name        string
		status      int
		prefix      string
		firstOutput bool
		wantReplay  bool
	}{
		{name: "silent_success_body", status: 200, wantReplay: true},
		{name: "silent_error_body", status: 503, wantReplay: true},
		{name: "partial_sse_line", status: 200, prefix: "data: {", wantReplay: true},
		{name: "after_output_never_replays", status: 200, prefix: "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n"},
		{name: "comments_do_not_extend_first_output", status: 200, firstOutput: true, wantReplay: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 1
			if tc.firstOutput {
				cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
				cfg.Gateway.OpenAIFirstOutputTimeoutSeconds = 1
			}
			reader, writer := io.Pipe()
			defer func() { _ = reader.Close() }()
			defer func() { _ = writer.Close() }()
			writerDone := make(chan struct{})
			go func() {
				defer close(writerDone)
				if tc.firstOutput {
					for {
						if _, err := io.WriteString(writer, ": keepalive\n\n"); err != nil {
							return
						}
						time.Sleep(20 * time.Millisecond)
					}
				}
				_, _ = io.WriteString(writer, tc.prefix)
			}()
			upstream := &bridgeLifecycleUpstream{do: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: reader}, nil
			}}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			writes := 0
			_, err := bridgeLifecycleTurn(t, ctx, cfg, upstream, func([]byte) error { writes++; return nil })
			if tc.firstOutput {
				require.ErrorIs(t, err, ErrOpenAIFirstOutputTimeout)
			} else {
				require.ErrorIs(t, err, errOpenAIWSHTTPBridgeReadTimeout)
			}
			var failover *UpstreamFailoverError
			require.Equal(t, tc.wantReplay, errors.As(err, &failover))
			if tc.wantReplay {
				require.Zero(t, writes)
			} else {
				require.Equal(t, 1, writes)
			}
			_, writeErr := writer.Write([]byte("closed"))
			require.Error(t, writeErr, "timed-out response body must be closed")
			select {
			case <-writerDone:
			case <-time.After(time.Second):
				t.Fatal("body writer did not stop")
			}
		})
	}
}

func TestHTTPBridgeLifecycle_PingAndDisconnectDuringHeaderWait(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.IngressModeDefault = OpenAIWSIngressModeHTTPBridge
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 2
	entered := make(chan struct{})
	upstream := &bridgeLifecycleUpstream{do: func(req *http.Request) (*http.Response, error) {
		close(entered)
		<-req.Context().Done()
		return nil, req.Context().Err()
	}}
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
		Extra: map[string]any{"openai_apikey_responses_websockets_v2_mode": OpenAIWSIngressModeHTTPBridge}}
	done := make(chan error, 1)
	var released atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, nil)
		if err != nil {
			done <- err
			return
		}
		defer func() { _ = conn.CloseNow() }()
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = r
		_, payload, err := conn.Read(r.Context())
		if err != nil {
			done <- err
			return
		}
		done <- svc.ProxyResponsesWebSocketFromClient(r.Context(), c, conn, account, "test-token", payload,
			&OpenAIWSIngressHooks{AfterTurn: func(int, *OpenAIForwardResult, error) { released.Store(true) }})
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	defer func() { _ = conn.CloseNow() }()
	require.NoError(t, conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5","input":"hello"}`)))
	clientReadDone := make(chan struct{})
	go func() { defer close(clientReadDone); _, _, _ = conn.Read(ctx) }()
	select {
	case <-entered:
	case err := <-done:
		t.Fatalf("proxy stopped before header wait: %v", err)
	case <-ctx.Done():
		t.Fatal("upstream was not entered")
	}
	pingCtx, pingCancel := context.WithTimeout(ctx, time.Second)
	err = conn.Ping(pingCtx)
	pingCancel()
	require.NoError(t, err, "bridge must respond to Ping while the model produces no output")
	_ = conn.CloseNow()
	select {
	case err := <-done:
		require.True(t, isOpenAIWSClientDisconnectError(err), "unexpected disconnect result: %v", err)
		require.True(t, released.Load(), "active-turn resource release hook was skipped")
	case <-ctx.Done():
		t.Fatal("disconnected bridge retained its upstream request")
	}
	<-clientReadDone
}

func TestHTTPBridgeWatchdog_SemanticOutputDisarmsOnlyFirstDeadline(t *testing.T) {
	g := newOpenAIWSHTTPBridgeWatchdog(context.Background(), 250*time.Millisecond, 50*time.Millisecond)
	defer g.close()
	g.semanticOutput()
	select {
	case <-g.ctx.Done():
		require.ErrorIs(t, context.Cause(g.ctx), errOpenAIWSHTTPBridgeReadTimeout)
	case <-time.After(time.Second):
		t.Fatal("stream inactivity deadline was disabled together with first output")
	}
}

func TestHTTPBridgeLifecycle_DisconnectStillCollectsTerminalUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &bridgeLifecycleUpstream{do: func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n" +
				"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_usage\",\"usage\":{\"input_tokens\":11,\"output_tokens\":7}}}\n\n",
		))}, nil
	}}
	writes := 0
	result, err := bridgeLifecycleTurn(t, context.Background(), &config.Config{}, upstream, func([]byte) error {
		writes++
		return coderws.CloseError{Code: coderws.StatusGoingAway, Reason: "client disconnected"}
	})
	require.NoError(t, err)
	require.Equal(t, 1, writes)
	require.True(t, result.ClientDisconnect)
	require.EqualValues(t, 11, result.Usage.InputTokens)
	require.EqualValues(t, 7, result.Usage.OutputTokens)
}

func TestHTTPBridgeWatchdog_DisconnectedDrainIsNotExtendedByActivity(t *testing.T) {
	g := newOpenAIWSHTTPBridgeWatchdog(context.Background(), 100*time.Millisecond, 0)
	defer g.close()
	g.clientOutput()
	g.clientClosed(io.EOF)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	for {
		select {
		case <-ticker.C:
			g.activity()
		case <-g.ctx.Done():
			require.ErrorIs(t, context.Cause(g.ctx), io.EOF)
			return
		case <-deadline.C:
			t.Fatal("continued upstream traffic retained a disconnected turn")
		}
	}
}

func TestHTTPBridgeLifecycle_DisconnectedClientNeverRecovers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &bridgeLifecycleUpstream{do: func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n" +
				"data: {\"type\":\"error\",\"error\":{\"type\":\"server_error\",\"message\":\"temporary upstream failure\"}}\n\n",
		))}, nil
	}}
	writes := 0
	_, err := bridgeLifecycleTurn(t, context.Background(), &config.Config{}, upstream, func([]byte) error {
		writes++
		return coderws.CloseError{Code: coderws.StatusGoingAway, Reason: "client disconnected"}
	})
	require.Error(t, err)
	var failover *UpstreamFailoverError
	require.False(t, errors.As(err, &failover), "a failed client write must not start a replacement upstream")
	require.Equal(t, 1, writes)
}

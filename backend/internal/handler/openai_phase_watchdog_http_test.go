//go:build unit

package handler

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type blueprintPhaseHTTPUpstream struct {
	service.HTTPUpstream
	target    *url.URL
	transport *http.Transport
	calls     atomic.Int32
}

func (u *blueprintPhaseHTTPUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.calls.Add(1)
	clone := req.Clone(req.Context())
	clone.URL = u.target
	clone.Host = u.target.Host
	return u.transport.RoundTrip(clone)
}

func blueprintPhaseHandlerServer(t *testing.T, endpoint string, upstream http.HandlerFunc, total int, usageRepos ...service.UsageLogRepository) (*httptest.Server, *blueprintPhaseHTTPUpstream, *OpenAIGatewayHandler) {
	t.Helper()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
		upstream(w, r)
	}))
	t.Cleanup(func() { origin.CloseClientConnections(); origin.Close() })
	target, err := url.Parse(origin.URL)
	require.NoError(t, err)
	transport := &http.Transport{}
	t.Cleanup(transport.CloseIdleConnections)
	u := &blueprintPhaseHTTPUpstream{target: target, transport: transport}
	h := newOpenAIResponsesFailoverTestHandler(t, u, usageRepos...)
	h.maxAccountSwitches = 0
	h.cfg.Gateway.OpenAIFirstOutputTimeoutSeconds = 1
	h.cfg.Gateway.OpenAIPreoutputRecoveryMode = "bounded_preoutput"
	h.cfg.Gateway.OpenAIPreoutputRecoveryMaxElapsedSeconds = total
	groupID := int64(3131)
	router := gin.New()
	router.POST(endpoint, func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{ID: 99, UserID: 100, GroupID: &groupID, Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, AllowMessagesDispatch: true}, User: &service.User{ID: 100}})
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 100, Concurrency: 0})
		if endpoint == "/v1/messages" {
			h.Messages(c)
		} else {
			h.Responses(c)
		}
	})
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return server, u, h
}

func blueprintPhaseRequest(t *testing.T, ctx context.Context, server *httptest.Server, endpoint string) *http.Response {
	t.Helper()
	body := `{"model":"gpt-5.1","stream":true,"input":"hello"}`
	if endpoint == "/v1/messages" {
		body = `{"model":"gpt-5.1","stream":true,"max_tokens":16,"messages":[{"role":"user","content":"hello"}]}`
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+endpoint, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

const blueprintPhasePreamble = "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_phase\",\"model\":\"gpt-5.1\"}}\n\n"
const blueprintPhaseText = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"phase_visible_text\"}\n\n"
const blueprintPhaseCompleted = "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_phase\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"

func TestBlueprintV2HandlerHeaderAndBodyShareDeadline(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []string{"/v1/responses", "/v1/messages"} {
		for _, headers := range []bool{false, true} {
			name := endpoint + "/no_headers"
			if headers {
				name = endpoint + "/slow_headers_then_silent_body"
			}
			t.Run(name, func(t *testing.T) {
				elapsed := make(chan time.Duration, 1)
				server, u, _ := blueprintPhaseHandlerServer(t, endpoint, func(w http.ResponseWriter, r *http.Request) {
					started := time.Now()
					defer func() { elapsed <- time.Since(started) }()
					if headers {
						timer := time.NewTimer(650 * time.Millisecond)
						defer timer.Stop()
						select {
						case <-timer.C:
						case <-r.Context().Done():
							return
						}
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, blueprintPhasePreamble)
						w.(http.Flusher).Flush()
					}
					<-r.Context().Done()
				}, 3)
				ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
				defer cancel()
				resp := blueprintPhaseRequest(t, ctx, server, endpoint)
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				require.GreaterOrEqual(t, resp.StatusCode, 400, string(body))
				require.NotEqual(t, 499, resp.StatusCode)
				require.EqualValues(t, 1, u.calls.Load())
				select {
				case duration := <-elapsed:
					require.Less(t, duration, 1500*time.Millisecond, "body must not get a fresh one-second timer")
				case <-ctx.Done():
					t.Fatal("upstream was not canceled")
				}
			})
		}
	}
}

func TestBlueprintV2HandlerLongStreamSurvivesPhaseDeadline(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []string{"/v1/responses", "/v1/messages"} {
		t.Run(endpoint, func(t *testing.T) {
			finish := make(chan struct{})
			started := make(chan time.Time, 1)
			server, u, _ := blueprintPhaseHandlerServer(t, endpoint, func(w http.ResponseWriter, r *http.Request) {
				started <- time.Now()
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, blueprintPhasePreamble+blueprintPhaseText)
				w.(http.Flusher).Flush()
				select {
				case <-finish:
					_, _ = io.WriteString(w, blueprintPhaseCompleted)
				case <-r.Context().Done():
				}
			}, 1)
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
			defer cancel()
			resp := blueprintPhaseRequest(t, ctx, server, endpoint)
			require.Equal(t, http.StatusOK, resp.StatusCode)
			reader := bufio.NewReader(resp.Body)
			for {
				line, err := reader.ReadString('\n')
				require.NoError(t, err)
				if strings.Contains(line, "phase_visible_text") {
					break
				}
			}
			timer := time.NewTimer(time.Until((<-started).Add(1300 * time.Millisecond)))
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-ctx.Done():
				t.Fatal("client deadline expired")
			}
			close(finish)
			body, err := io.ReadAll(reader)
			require.NoError(t, err)
			terminal := "response.completed"
			if endpoint == "/v1/messages" {
				terminal = "message_stop"
			}
			require.Contains(t, string(body), terminal)
			require.NotContains(t, string(body), "event: error")
			require.NotContains(t, string(body), "response.failed")
			require.EqualValues(t, 1, u.calls.Load())
		})
	}
}

func TestBlueprintV2HandlerTotalSurvivesRetryWithAttemptLimitDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []string{"/v1/responses", "/v1/messages"} {
		for _, retry := range []bool{false, true} {
			name := endpoint + "/total_only"
			if retry {
				name = endpoint + "/retry_shares_total"
			}
			t.Run(name, func(t *testing.T) {
				var hits atomic.Int32
				finished := make(chan struct{}, 1)
				server, u, h := blueprintPhaseHandlerServer(t, endpoint, func(w http.ResponseWriter, r *http.Request) {
					if hits.Add(1) == 1 && retry {
						timer := time.NewTimer(600 * time.Millisecond)
						defer timer.Stop()
						select {
						case <-timer.C:
						case <-r.Context().Done():
							return
						}
						w.WriteHeader(520)
						_, _ = io.WriteString(w, `{"error":{"type":"server_error","message":"temporary upstream error"}}`)
						return
					}
					<-r.Context().Done()
					finished <- struct{}{}
				}, 1)
				h.cfg.Gateway.OpenAIFirstOutputTimeoutSeconds = 0
				h.maxAccountSwitches = 2
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				start := time.Now()
				resp := blueprintPhaseRequest(t, ctx, server, endpoint)
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				require.GreaterOrEqual(t, resp.StatusCode, 400, string(body))
				require.NotEqual(t, 499, resp.StatusCode)
				wantCalls := int32(1)
				if retry {
					wantCalls = 2
				}
				require.Equal(t, wantCalls, u.calls.Load(), string(body))
				require.Less(t, time.Since(start), 1500*time.Millisecond, "retry must not restart the total window")
				select {
				case <-finished:
				case <-ctx.Done():
					t.Fatal("upstream was not canceled")
				}
			})
		}
	}
}

package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type modelRoutePauseRepo struct {
	AccountRepository
	calls       int
	id          int64
	err         error
	beforeWrite func(context.Context)
}

func (r *modelRoutePauseRepo) SetSchedulable(ctx context.Context, id int64, schedulable bool) error {
	if schedulable {
		panic("model route guard must never resume an account")
	}
	r.calls++
	r.id = id
	if r.beforeWrite != nil {
		r.beforeWrite(ctx)
	}
	return r.err
}

func TestOpenAIModelRoutePauseRules(t *testing.T) {
	for _, tt := range []struct {
		name, requested, sent, response, platform, terminal string
		conflict, want                                      bool
	}{
		{name: "astra downgraded", requested: "gpt-6-astra", response: "gpt-5.6-luna", want: true},
		{name: "mapped alias sent as astra", requested: "my-model", sent: "gpt-6-astra", response: "gpt-5.6-luna", want: true},
		{name: "terminal declaration overrides echo", requested: "gpt-6-astra", response: "gpt-5.6-luna", conflict: true, want: true},
		{name: "case and spaces", requested: " GPT-6-ASTRA ", response: " GPT-5.6-LUNA ", want: true},
		{name: "ws completed", requested: "gpt-6-astra", response: "gpt-5.6-luna", terminal: "response.completed", want: true},
		{name: "ws failed", requested: "gpt-6-astra", response: "gpt-5.6-luna", terminal: "response.failed"},
		{name: "same astra", requested: "gpt-6-astra", response: "gpt-6-astra"},
		{name: "normal luna", requested: "gpt-5.6-luna", response: "gpt-5.6-luna"},
		{name: "intentional mapping", requested: "gpt-6-astra", sent: "gpt-5.6-luna", response: "gpt-5.6-luna"},
		{name: "no upstream declaration", requested: "gpt-6-astra"},
		{name: "different downgrade", requested: "gpt-6-astra", response: "gpt-5.5"},
		{name: "pro not covered", requested: "gpt-6-astra-pro", response: "gpt-5.6-luna"},
		{name: "other platform", requested: "gpt-6-astra", response: "gpt-5.6-luna", platform: PlatformGrok},
	} {
		t.Run(tt.name, func(t *testing.T) {
			platform := tt.platform
			if platform == "" {
				platform = PlatformOpenAI
			}
			account := &Account{ID: 42, Platform: platform, Schedulable: true, Status: StatusActive}
			repo := &modelRoutePauseRepo{}
			svc := &OpenAIGatewayService{accountRepo: repo}
			result := &OpenAIForwardResult{Model: tt.requested, UpstreamModel: tt.sent, UpstreamResponseModel: tt.response, UpstreamResponseModelConflict: tt.conflict, OpenAIWSMode: tt.terminal != "", UpstreamTerminalEvent: tt.terminal}
			require.Equal(t, tt.want, svc.pauseOpenAIAccountOnModelRoute(context.Background(), account, result))
			require.Equal(t, tt.want, svc.isOpenAIAccountRuntimeBlocked(account))
			if tt.want {
				require.Equal(t, 1, repo.calls)
				require.Equal(t, account.ID, repo.id)
				// Re-reporting a bridged result must not persist twice.
				require.True(t, svc.pauseOpenAIAccountOnModelRoute(context.Background(), account, result))
				require.Equal(t, 1, repo.calls)
			} else {
				require.Zero(t, repo.calls)
			}
			require.True(t, account.Schedulable, "shared snapshot must not be mutated")
			require.Equal(t, StatusActive, account.Status)
		})
	}
}

func TestOpenAIModelRoutePauseSurvivesCancellationAndWriteFailure(t *testing.T) {
	for _, writeErr := range []error{nil, errors.New("database unavailable")} {
		account := &Account{ID: 42, Platform: PlatformOpenAI, Schedulable: true}
		repo := &modelRoutePauseRepo{err: writeErr}
		svc := &OpenAIGatewayService{accountRepo: repo}
		repo.beforeWrite = func(ctx context.Context) {
			require.NoError(t, ctx.Err())
			_, bounded := ctx.Deadline()
			require.True(t, bounded)
			require.True(t, svc.isOpenAIAccountRuntimeBlocked(account), "block before DB write")
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		require.True(t, svc.pauseOpenAIAccountOnModelRoute(ctx, account, &OpenAIForwardResult{Model: "gpt-6-astra", UpstreamResponseModel: "gpt-5.6-luna"}))
		require.Equal(t, 1, repo.calls)
		require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
	}
}

func TestOpenAIModelRoutePauseWSHooks(t *testing.T) {
	repo := &modelRoutePauseRepo{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	account := &Account{ID: 42, Platform: PlatformOpenAI}
	beforeCalls, afterCalls := 0, 0
	original := &OpenAIWSIngressHooks{
		InitialRequestModel: "gpt-6-astra",
		BeforeTurn:          func(int) error { beforeCalls++; return nil },
		AfterTurn: func(_ int, _ *OpenAIForwardResult, _ error) {
			afterCalls++
			require.Equal(t, 1, repo.calls, "pause before usage callback")
		},
	}
	wrapped := svc.withOpenAIModelRoutePauseHooks(context.Background(), account, original)
	require.Equal(t, original.InitialRequestModel, wrapped.InitialRequestModel)
	require.NoError(t, wrapped.BeforeTurn(1))
	wrapped.AfterTurn(1, &OpenAIForwardResult{Model: "gpt-6-astra", UpstreamResponseModel: "gpt-5.6-luna"}, nil)
	require.Error(t, wrapped.BeforeTurn(2))
	require.Equal(t, 1, beforeCalls)
	require.Equal(t, 1, afterCalls)
	require.NoError(t, original.BeforeTurn(2), "do not modify caller's hooks")

	for _, turnErr := range []error{nil, errors.New("failed turn")} {
		hooks := svc.withOpenAIModelRoutePauseHooks(context.Background(), account, nil)
		response := "gpt-6-astra"
		if turnErr != nil {
			response = "gpt-5.6-luna"
		}
		hooks.AfterTurn(1, &OpenAIForwardResult{Model: "gpt-6-astra", UpstreamResponseModel: response}, turnErr)
		require.NoError(t, hooks.BeforeTurn(2))
	}
	require.Equal(t, 1, repo.calls)
}

func TestOpenAIModelRoutePauseForward(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []string{"responses", "chat", "messages"} {
		for _, stream := range []bool{false, true} {
			name := endpoint + "/buffered"
			if stream {
				name = endpoint + "/stream"
			}
			t.Run(name, func(t *testing.T) {
				body := `{"model":"gpt-6-astra","stream":false,"input":"hello"}`
				if endpoint == "chat" || endpoint == "messages" {
					body = `{"model":"gpt-6-astra","stream":false,"max_tokens":100,"messages":[{"role":"user","content":"hello"}]}`
				}
				if stream {
					body = strings.Replace(body, `"stream":false`, `"stream":true`, 1)
				}
				response := `{"id":"resp_route","object":"response","status":"completed","model":"gpt-5.6-luna","output":[{"id":"msg_route","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"hello"}]}],"usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12}}`
				wire := "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_route\",\"model\":\"gpt-6-astra\"}}\n\n" +
					"event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\",\"output_index\":0,\"content_index\":0}\n\n" +
					"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":" + response + "}\n\n"
				upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))}}
				repo := &modelRoutePauseRepo{}
				svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream, accountRepo: repo}
				account := &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Credentials: map[string]any{"access_token": "test-token", "chatgpt_account_id": "test-account"}}
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/"+endpoint, bytes.NewBufferString(body))
				var result *OpenAIForwardResult
				var err error
				switch endpoint {
				case "responses":
					result, err = svc.Forward(context.Background(), c, account, []byte(body))
				case "chat":
					result, err = svc.ForwardAsChatCompletions(context.Background(), c, account, []byte(body), "", "")
				case "messages":
					result, err = svc.ForwardAsAnthropic(context.Background(), c, account, []byte(body), "", "")
				}
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, "gpt-5.6-luna", result.UpstreamResponseModel)
				require.Equal(t, 1, repo.calls, "persist before forwarding returns, without RecordUsage")
				require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
			})
		}
	}
}

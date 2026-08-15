package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type openAIEgressAccountRepoStub struct {
	AccountRepository
	account *Account
	err     error
}

func (r *openAIEgressAccountRepoStub) GetByID(context.Context, int64) (*Account, error) {
	return r.account, r.err
}

type openAIEgressCountingUpstream struct {
	calls atomic.Int32
}

func (u *openAIEgressCountingUpstream) Do(*http.Request, string, int64, int) (*http.Response, error) {
	u.calls.Add(1)
	return nil, errors.New("unexpected upstream call")
}

func (u *openAIEgressCountingUpstream) DoWithTLS(*http.Request, string, int64, int, *tlsfingerprint.Profile) (*http.Response, error) {
	u.calls.Add(1)
	return nil, errors.New("unexpected upstream call")
}

func TestOpenAIEgressResolverHydratesMissingProxyRelationFromAccountRepository(t *testing.T) {
	proxyID := int64(73)
	proxy := &Proxy{ID: proxyID, Protocol: "http", Host: "2001:db8::73", Port: 8080, Status: StatusActive}
	repo := &openAIEgressAccountRepoStub{account: &Account{
		ID:       9001,
		Platform: PlatformOpenAI,
		ProxyID:  &proxyID,
		Proxy:    proxy,
	}}
	resolver := newOpenAIEgressResolverWithAccountRepo(strictOpenAIEgressConfig(), nil, repo)

	route, err := resolver.Resolve(context.Background(), &Account{ID: 9001, Platform: PlatformOpenAI, ProxyID: &proxyID})
	require.NoError(t, err)
	require.Equal(t, proxyID, route.ProxyID)
	require.False(t, route.Direct)
}

func TestOpenAIEgressResolverRejectsHydratedProxyMismatch(t *testing.T) {
	proxyID := int64(73)
	otherProxyID := int64(74)
	repo := &openAIEgressAccountRepoStub{account: &Account{
		ID:       9001,
		Platform: PlatformOpenAI,
		ProxyID:  &otherProxyID,
		Proxy:    &Proxy{ID: otherProxyID, Protocol: "http", Host: "2001:db8::74", Port: 8080, Status: StatusActive},
	}}
	resolver := newOpenAIEgressResolverWithAccountRepo(strictOpenAIEgressConfig(), nil, repo)

	route, err := resolver.Resolve(context.Background(), &Account{ID: 9001, Platform: PlatformOpenAI, ProxyID: &proxyID})
	require.ErrorIs(t, err, ErrOpenAIProxyUnavailable)
	require.Empty(t, route.ProxyURL)
	require.False(t, route.Direct)
}

func TestOpenAIMainCompatibilitySinksFailClosedBeforeUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &openAIEgressCountingUpstream{}
	svc := &OpenAIGatewayService{
		cfg:                  strictOpenAIEgressConfig(),
		openAIEgressResolver: newOpenAIEgressResolver(strictOpenAIEgressConfig(), nil),
		httpUpstream:         upstream,
	}
	account := &Account{
		ID:       9002,
		Name:     "strict-oauth",
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":       "test-access-token",
			"chatgpt_account_id": "test-account",
		},
	}

	t.Run("messages", func(t *testing.T) {
		body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

		result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.4")
		require.Nil(t, result)
		require.ErrorIs(t, err, ErrOpenAIProxyRequired)
	})

	t.Run("chat completions bridge", func(t *testing.T) {
		body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":false}`)
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

		result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
		require.Nil(t, result)
		require.ErrorIs(t, err, ErrOpenAIProxyRequired)
	})

	require.Zero(t, upstream.calls.Load(), "strict egress failure must prevent every upstream call")
}

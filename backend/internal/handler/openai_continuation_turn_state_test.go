//go:build unit

package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type continuationTurnStateUpstream struct {
	service.HTTPUpstream
	body    []byte
	headers http.Header
	calls   int
}

func (u *continuationTurnStateUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.calls++
	u.headers = req.Header.Clone()
	var err error
	u.body, err = io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"resp_rebuilt","object":"response","model":"gpt-5.2","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`)),
	}, nil
}

func TestOpenAIResponses_FullHistoryWithTurnStateRebuildsBeforeDispatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(4203)
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Default.RateMultiplier = 1
	cfg.Security.URLAllowlist.Enabled = false
	accountRepo := &openAIWSFailoverHandlerAccountRepoStub{accounts: []service.Account{{
		ID: 9911, Name: "replacement", Platform: service.PlatformOpenAI,
		Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://api.example.test"},
		Extra:       map[string]any{"openai_passthrough": true},
	}}}
	upstream := &continuationTurnStateUpstream{}
	billingCache := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCache.Stop)
	gateway := service.NewOpenAIGatewayService(
		accountRepo, nil, nil, nil, nil, nil, nil, cfg, nil, nil,
		service.NewBillingService(cfg, nil), nil, billingCache, upstream,
		&service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil,
	)
	h := NewOpenAIGatewayHandler(gateway, service.NewConcurrencyService(nil), billingCache,
		service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg), nil, nil, nil, nil, cfg)
	require.NoError(t, gateway.BindOpenAIHTTPResponseOwner(context.Background(), groupID, "resp_old", 1703, 1803))

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", strings.NewReader(
		`{"model":"gpt-5.2","stream":false,"previous_response_id":"resp_old","input":[{"role":"user","content":"Remember the project"},{"role":"assistant","content":"A calculator"},{"role":"user","content":"Continue"}]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("X-Codex-Turn-State", "old-account-state")
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
		ID: 1803, UserID: 1703, GroupID: &groupID,
		User:  &service.User{ID: 1703, Status: service.StatusActive},
		Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive},
	})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1703})

	h.Responses(c)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, 1, upstream.calls)
	require.False(t, gjson.GetBytes(upstream.body, "previous_response_id").Exists())
	require.Empty(t, upstream.headers.Get("X-Codex-Turn-State"))
	require.Len(t, gjson.GetBytes(upstream.body, "input").Array(), 3)
	require.Equal(t, "old-account-state", c.Request.Header.Get("X-Codex-Turn-State"))
}

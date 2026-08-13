package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestQueryCodexAnalyticsUsesBoundedUTCRangeAndSanitizesResponse(t *testing.T) {
	account := &Account{
		ID:       100,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Credentials: map[string]any{
			"chatgpt_account_id": "org-analytics",
		},
	}
	repo := &stubQuotaAccountRepo{accounts: map[int64]*Account{100: account}}
	tokenCache := &stubQuotaTokenCache{tokens: map[string]string{
		OpenAITokenCacheKey(account): "fake-token",
	}}
	tokenProvider := NewOpenAITokenProvider(repo, tokenCache, nil)

	var startDate, endDate, groupBy, authorization, accountHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/backend-api/wham/analytics/daily-workspace-usage-counts", r.URL.Path)
		startDate = r.URL.Query().Get("start_date")
		endDate = r.URL.Query().Get("end_date")
		groupBy = r.URL.Query().Get("group_by")
		authorization = r.Header.Get("authorization")
		accountHeader = r.Header.Get("chatgpt-account-id")
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{
			"data":[
				{"date":"2026-08-12","totals":{"threads":24,"turns":2413,"users":1,"credits":99},"clients":[{"client_id":"CODEX_CLI"}]},
				{"date":"bad-date","totals":{"threads":999,"turns":999,"users":999}},
				{"date":"2026-08-13","totals":{"threads":-1,"turns":-2,"users":-3}}
			],
			"email":"must-not-escape@example.com",
			"user_id":"secret-user"
		}`))
	}))
	defer srv.Close()

	svc := NewOpenAIQuotaService(repo, nil, tokenProvider, newQuotaRedirectingFactory(srv))
	result, err := svc.QueryCodexAnalytics(context.Background(), 100)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "day", groupBy)
	require.Equal(t, "Bearer fake-token", authorization)
	require.Equal(t, "org-analytics", accountHeader)
	require.Len(t, result.Data, 2)
	require.Equal(t, OpenAICodexAnalyticsDay{Date: "2026-08-12", Threads: 24, Turns: 2413, Users: 1}, result.Data[0])
	require.Equal(t, OpenAICodexAnalyticsDay{Date: "2026-08-13"}, result.Data[1])

	start, err := time.Parse(time.DateOnly, startDate)
	require.NoError(t, err)
	end, err := time.Parse(time.DateOnly, endDate)
	require.NoError(t, err)
	require.Equal(t, 2, int(end.Sub(start).Hours()/24), "range is yesterday through today's exclusive upper bound")
	require.Equal(t, end.AddDate(0, 0, -1).Format(time.DateOnly), result.CurrentUTCDate)
	require.NotZero(t, result.FetchedAt)
}

func TestQueryCodexAnalyticsDoesNotExposeUpstreamErrorBody(t *testing.T) {
	account := &Account{
		ID:       100,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Credentials: map[string]any{
			"chatgpt_account_id": "org-analytics",
		},
	}
	repo := &stubQuotaAccountRepo{accounts: map[int64]*Account{100: account}}
	tokenCache := &stubQuotaTokenCache{tokens: map[string]string{
		OpenAITokenCacheKey(account): "fake-token",
	}}
	tokenProvider := NewOpenAITokenProvider(repo, tokenCache, nil)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"email":"secret@example.com","user_id":"secret"}`))
	}))
	defer srv.Close()

	svc := NewOpenAIQuotaService(repo, nil, tokenProvider, newQuotaRedirectingFactory(srv))
	result, err := svc.QueryCodexAnalytics(context.Background(), 100)
	require.Nil(t, result)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "secret@example.com")
	require.NotContains(t, err.Error(), "user_id")
}

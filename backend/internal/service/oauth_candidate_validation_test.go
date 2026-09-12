package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type oauthValidationRoundTrip func(*http.Request) (*http.Response, error)

func (f oauthValidationRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type oauthValidationFunc func(context.Context, *Account, map[string]any) error

func (f oauthValidationFunc) Validate(c context.Context, a *Account, p map[string]any) error {
	return f(c, a, p)
}

func TestOAuthCandidateValidationUsesCandidateAndOfficialIdentity(t *testing.T) {
	account := &Account{Credentials: map[string]any{"email": "fixture@example.invalid", "chatgpt_account_id": "workspace-fixture", "workspace_id": "workspace-fixture", "chatgpt_user_id": "user-fixture"}}
	good := `{"email":"fixture@example.invalid","account_id":"workspace-fixture","user_id":"user-fixture","rate_limit":{"allowed":true,"limit_reached":false}}`
	for _, test := range []struct {
		name, usage, models string
		status              int
		ok                  bool
	}{
		{"valid", good, `{"models":[{"slug":"fixture-model"}]}`, 200, true},
		{"wrong-email", strings.ReplaceAll(good, "fixture@example.invalid", "someone@example.invalid"), `{"models":[{"slug":"fixture-model"}]}`, 200, false},
		{"wrong-workspace", strings.ReplaceAll(good, "workspace-fixture", "different"), `{}`, 200, false},
		{"wrong-user", strings.ReplaceAll(good, "user-fixture", "different"), `{}`, 200, false},
		{"quota-limited", strings.ReplaceAll(good, `"allowed":true`, `"allowed":false`), `{}`, 200, false},
		{"missing-identity", `{"rate_limit":{"allowed":true}}`, `{}`, 200, false},
		{"html", `<html>challenge</html>`, `{}`, 200, false},
		{"empty-models", good, `{"models":[]}`, 200, false},
		{"unauthorized", good, `{}`, 401, false},
		{"forbidden", good, `{}`, 403, false},
		{"limited", good, `{}`, 429, false},
		{"redirect", good, `{}`, 302, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Transport: oauthValidationRoundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, "https", r.URL.Scheme)
				require.Equal(t, "chatgpt.com", r.URL.Host)
				require.Equal(t, "Bearer fixture-candidate-AT", r.Header.Get("Authorization"))
				require.Equal(t, "workspace-fixture", r.Header.Get("ChatGPT-Account-Id"))
				body := test.usage
				if calls == 2 {
					require.Equal(t, "/backend-api/codex/models", r.URL.Path)
					body = test.models
				} else {
					require.Equal(t, "/backend-api/wham/usage", r.URL.Path)
				}
				return &http.Response{StatusCode: test.status, Header: http.Header{"Location": []string{"https://other.invalid/collect"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})}
			err := validateOAuthCandidateHTTP(context.Background(), client, account, map[string]any{"access_token": "fixture-candidate-AT", "refresh_token": "never-sent"})
			if test.ok {
				require.NoError(t, err)
				require.Equal(t, 2, calls)
			} else {
				require.ErrorIs(t, err, ErrOAuthValidationFailed)
			}
			require.LessOrEqual(t, calls, 2)
		})
	}
}

func TestOAuthCandidateValidationPrecedesCommitAndReplaySkipsProbe(t *testing.T) {
	svc, repo, _, request := durableSyncFixture()
	request.RecoveryMode = "auth_only"
	calls := 0
	probeErr := errors.New("secret provider error")
	svc.validator = oauthValidationFunc(func(_ context.Context, _ *Account, credentials map[string]any) error {
		calls++
		require.Equal(t, request.Credentials["access_token"], credentials["access_token"])
		return probeErr
	})
	_, _, err := svc.Submit(context.Background(), "admin:42", 42, request)
	require.ErrorIs(t, err, ErrOAuthValidationFailed)
	require.Zero(t, repo.commits)
	require.NotContains(t, err.Error(), "secret")
	probeErr = nil
	first, _, err := svc.Submit(context.Background(), "admin:42", 42, request)
	require.NoError(t, err)
	require.Equal(t, "cleared", first.AuthRecovery)
	require.NotNil(t, first.ValidatedAt)
	_, replay, err := svc.Submit(context.Background(), "admin:42", 42, request)
	require.NoError(t, err)
	require.True(t, replay)
	require.Equal(t, 2, calls)
	require.Equal(t, 1, repo.commits)
}

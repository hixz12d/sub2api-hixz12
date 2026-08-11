//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

const openAIOAuthAuthorizedGroupsTestMessage = "This OpenAI OAuth account is restricted to authorized API key groups."

func TestIsOpenAIOAuthAuthorizedAPIKeyGroupsError(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{
			name: "http permission error",
			body: `{"error":{"message":"This OpenAI OAuth account is restricted to authorized API key groups.","type":"permission_error"}}`,
			want: true,
		},
		{
			name: "streamed permission error",
			body: `{"type":"response.failed","response":{"error":{"message":"This OpenAI OAuth account is restricted to authorized API key groups.","type":"permission_error"}}}`,
			want: true,
		},
		{
			name: "ordinary permission error",
			body: `{"error":{"message":"You do not have permission to use this model.","type":"permission_error"}}`,
			want: false,
		},
		{
			name: "matching text with different type",
			body: `{"error":{"message":"This OpenAI OAuth account is restricted to authorized API key groups.","type":"invalid_request_error"}}`,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, IsOpenAIOAuthAuthorizedAPIKeyGroupsError("", []byte(tt.body)))
		})
	}
}

func TestShouldFailoverOpenAIPassthroughAuthorizedGroupsRestriction(t *testing.T) {
	body := []byte(`{"error":{"message":"This OpenAI OAuth account is restricted to authorized API key groups.","type":"permission_error"}}`)
	oauth := &Account{ID: 701, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	apiKey := &Account{ID: 702, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	require.True(t, shouldFailoverOpenAIPassthroughResponse(oauth, http.StatusForbidden, body))
	require.False(t, shouldFailoverOpenAIPassthroughResponse(apiKey, http.StatusForbidden, body))
	require.False(t, shouldFailoverOpenAIPassthroughResponse(oauth, http.StatusForbidden, []byte(`{"error":{"message":"ordinary permission denial","type":"permission_error"}}`)))
}

func TestRateLimitServiceAuthorizedGroupsRestrictionDoesNotDisableOAuthAccount(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := &Account{
		ID:       703,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"temp_unschedulable_enabled": true,
			"temp_unschedulable_rules": []any{
				map[string]any{
					"error_code":       http.StatusForbidden,
					"keywords":         []any{"restricted"},
					"duration_minutes": 30,
				},
			},
		},
	}
	body := []byte(`{"error":{"message":"This OpenAI OAuth account is restricted to authorized API key groups.","type":"permission_error"}}`)

	shouldDisable := svc.HandleUpstreamError(context.Background(), account, http.StatusForbidden, http.Header{}, body)

	require.False(t, shouldDisable)
	require.Zero(t, repo.setErrorCalls)
	require.Zero(t, repo.tempCalls)
}

func TestOpenAIStreamAuthorizedGroupsRestrictionClearsStickyBinding(t *testing.T) {
	cache := &stubGatewayCache{sessionBindings: map[string]int64{}}
	svc := &OpenAIGatewayService{cache: cache}
	account := &Account{ID: 704, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	payload := []byte(`{"type":"response.failed","response":{"error":{"message":"This OpenAI OAuth account is restricted to authorized API key groups.","type":"permission_error"}}}`)
	failoverErr := svc.newOpenAIStreamFailoverError(nil, account, true, "", payload, openAIOAuthAuthorizedGroupsTestMessage)
	groupID := int64(44)

	require.Equal(t, openAIOAuthAuthorizedAPIKeyGroupsReason, failoverErr.Reason)
	require.True(t, svc.ClearOpenAIOAuthRestrictedStickySession(context.Background(), &groupID, "restricted-session", account, failoverErr))
	require.NotEmpty(t, cache.deletedSessions)
}

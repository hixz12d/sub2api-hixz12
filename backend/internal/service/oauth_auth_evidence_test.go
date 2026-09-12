package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type oauthEvidenceRecorder struct {
	AccountRepository
	calls     int
	permanent bool
}

func (r *oauthEvidenceRecorder) RecordOAuthUnauthorized(_ context.Context, a *Account, token string, permanent bool, until time.Time) error {
	r.calls++
	r.permanent = permanent
	return nil
}

type oauthEvidenceCache struct {
	TempUnschedCache
	state *TempUnschedState
}

func (c *oauthEvidenceCache) GetTempUnsched(context.Context, int64) (*TempUnschedState, error) {
	return c.state, nil
}
func TestOAuthEvidenceRecoveredHoldDoesNotRemainBlockedByOldCache(t *testing.T) {
	cache := &oauthEvidenceCache{state: &TempUnschedState{UntilUnix: time.Now().Add(time.Hour).Unix(), ErrorMessage: "OAuth authentication failed (versioned 401)"}}
	limiter := &RateLimitService{accountRepo: &oauthSyncAccountRepo{account: &Account{ID: 42}}, tempUnschedCache: cache}
	state, err := limiter.GetTempUnschedStatus(context.Background(), 42)
	require.NoError(t, err)
	require.Nil(t, state)
	cache.state.ErrorMessage = "manual quota pause"
	state, err = limiter.GetTempUnschedStatus(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, "manual quota pause", state.ErrorMessage)
}

func TestOAuthEvidenceRequiresActualOfficialBearerAndCurrentSnapshot(t *testing.T) {
	recorder := &oauthEvidenceRecorder{}
	limiter := &RateLimitService{accountRepo: recorder}
	account := &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "fixture-AT", "_token_version": int64(7)}}
	for _, test := range []struct {
		url, token string
		handled    bool
		calls      int
	}{
		{"https://chatgpt.com/backend-api/codex/responses", "fixture-AT", true, 1},
		{"https://chatgpt.com/backend-api/codex/responses", "stale-AT", true, 0},
		{"https://proxy.invalid/responses", "fixture-AT", false, 0},
		{"http://chatgpt.com/backend-api/codex/responses", "fixture-AT", false, 0},
	} {
		req, err := http.NewRequest(http.MethodPost, test.url, nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+test.token)
		ctx := withOAuthResponseCredential(context.Background(), &http.Response{Request: req})
		recorder.calls = 0
		require.Equal(t, test.handled, limiter.recordVersionedOAuth401(ctx, account, true))
		require.Equal(t, test.calls, recorder.calls)
	}
	require.False(t, limiter.recordVersionedOAuth401(context.Background(), account, true))
}

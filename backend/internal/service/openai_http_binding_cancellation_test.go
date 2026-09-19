package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type responseBindingContextRepo struct {
	*memoryOpenAIAffinityRepo
	t      *testing.T
	called bool
}

func (r *responseBindingContextRepo) BindResponseAndUpgrade(ctx context.Context, identity SessionIdentity, responseHash string, accountID int64, responseExpiresAt, strongExpiresAt time.Time) (*OpenAIResponseBinding, error) {
	r.called = true
	require.NoError(r.t, ctx.Err(), "persistent binding must survive downstream cancellation")
	_, bounded := ctx.Deadline()
	require.True(r.t, bounded)
	return r.memoryOpenAIAffinityRepo.BindResponseAndUpgrade(ctx, identity, responseHash, accountID, responseExpiresAt, strongExpiresAt)
}

func TestOpenAIHTTPResponseBindingDetachesPersistentAffinityContext(t *testing.T) {
	repo := &responseBindingContextRepo{memoryOpenAIAffinityRepo: newMemoryOpenAIAffinityRepo(), t: t}
	svc := &OpenAIGatewayService{cfg: openAIAffinityTestConfig(), openAIAffinityRepo: repo}
	c := newOpenAIAffinityTestContext(t, 7, 11, "/v1/responses")
	identity, err := svc.resolveOpenAISessionIdentity(c, []byte(`{"prompt_cache_key":"cache"}`), "legacy")
	require.NoError(t, err)
	attachOpenAIAffinityIdentity(c, identity, true, true)
	_, _, err = svc.createOrGetPersistentOpenAISession(c.Request.Context(), 101)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(c.Request.Context())
	cancel()
	svc.bindHTTPResponseAccount(ctx, c, &Account{ID: 101, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, "resp-canceled-persistent")
	require.True(t, repo.called)

	continuation := newOpenAIAffinityTestContext(t, 7, 11, "/v1/responses")
	continuationIdentity, err := svc.resolveOpenAISessionIdentity(continuation, []byte(`{"previous_response_id":"resp-canceled-persistent"}`), "")
	require.NoError(t, err)
	attachOpenAIAffinityIdentity(continuation, continuationIdentity, true, true)
	binding, err := svc.resolvePersistentOpenAIResponse(continuation.Request.Context())
	require.NoError(t, err)
	require.Equal(t, int64(101), binding.AccountID)
}

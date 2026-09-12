package service

import (
	"context"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/stretchr/testify/require"
	"testing"
)

type openAIRefreshFenceStub struct {
	*oauthSyncAccountRepo
	beginErr               error
	finishCalls, uncertain int
}

func (r *openAIRefreshFenceStub) BeginOpenAIRefresh(context.Context, string) (*OpenAIRefreshTicket, error) {
	if r.beginErr != nil {
		return nil, r.beginErr
	}
	return &OpenAIRefreshTicket{GrantID: "fixture", Attempt: "attempt", Epoch: 1}, nil
}
func (r *openAIRefreshFenceStub) FinishOpenAIRefresh(_ context.Context, _ *OpenAIRefreshTicket, _ *Account, c map[string]any, _ string) (bool, error) {
	r.finishCalls++
	if c != nil {
		r.account.Credentials = c
	}
	return true, nil
}
func (r *openAIRefreshFenceStub) MarkOpenAIRefreshUncertain(context.Context, *OpenAIRefreshTicket) error {
	r.uncertain++
	return nil
}

type fencedOpenAIClient struct {
	OpenAIOAuthClient
	calls        int
	beforeReturn func()
}

func (c *fencedOpenAIClient) RefreshTokenWithClientID(context.Context, string, string, string) (*openai.TokenResponse, error) {
	c.calls++
	if c.beforeReturn != nil {
		c.beforeReturn()
	}
	return &openai.TokenResponse{AccessToken: "new-AT", RefreshToken: "new-RT", ExpiresIn: 3600}, nil
}
func TestOpenAIRefreshFencingRejectsUncoordinatedAndUnavailableStore(t *testing.T) {
	original, _ := oauthSyncFixture()
	original.account.Status = StatusActive
	repo := &openAIRefreshFenceStub{oauthSyncAccountRepo: original, beginErr: errors.New("database unavailable")}
	client := &fencedOpenAIClient{}
	svc := NewOpenAIOAuthService(nil, client)
	defer svc.Stop()
	api := NewOAuthRefreshAPI(repo, nil)
	svc.SetRefreshCoordination(repo, api)
	_, err := svc.RefreshAccountToken(context.Background(), original.account)
	require.ErrorIs(t, err, ErrOpenAIRefreshFenced)
	require.Zero(t, client.calls)
	_, err = svc.RefreshManagedAccount(context.Background(), original.account)
	require.Error(t, err)
	require.Zero(t, client.calls)
}
func TestOpenAIRefreshFencingCancellationCannotWriteLateReply(t *testing.T) {
	original, _ := oauthSyncFixture()
	original.account.Status = StatusActive
	repo := &openAIRefreshFenceStub{oauthSyncAccountRepo: original}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := &fencedOpenAIClient{beforeReturn: cancel}
	svc := NewOpenAIOAuthService(nil, client)
	defer svc.Stop()
	svc.SetRefreshCoordination(repo, NewOAuthRefreshAPI(repo, nil))
	_, err := svc.RefreshManagedAccount(ctx, original.account)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, client.calls)
	require.Zero(t, repo.finishCalls)
	require.Equal(t, 1, repo.uncertain)
	require.Equal(t, "old", original.account.GetCredential("access_token"))
}
func TestOpenAIRefreshFencingManagedAndRawBothUseDurableTicket(t *testing.T) {
	original, _ := oauthSyncFixture()
	original.account.Status = StatusActive
	repo := &openAIRefreshFenceStub{oauthSyncAccountRepo: original}
	client := &fencedOpenAIClient{}
	svc := NewOpenAIOAuthService(nil, client)
	defer svc.Stop()
	svc.SetRefreshCoordination(repo, NewOAuthRefreshAPI(repo, nil))
	result, err := svc.RefreshManagedAccount(context.Background(), original.account)
	require.NoError(t, err)
	require.Equal(t, "new-AT", result.GetCredential("access_token"))
	require.Equal(t, 1, repo.finishCalls)
	require.Zero(t, repo.uncertain)
	_, err = svc.RefreshTokenWithClientID(context.Background(), "raw-RT", "", "client")
	require.NoError(t, err)
	require.Equal(t, 2, repo.finishCalls)
}

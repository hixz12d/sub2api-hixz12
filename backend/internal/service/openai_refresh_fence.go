package service

import (
	"context"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"time"
)

var ErrOpenAIRefreshFenced = infraerrors.Conflict("OPENAI_REFRESH_FENCED", "refresh is in progress, previously consumed, or delegated; no new refresh was started")
var ErrOpenAIRefreshUncertain = infraerrors.Conflict("OPENAI_REFRESH_UNCERTAIN", "refresh outcome is uncertain; automatic reuse and ownership transfer are blocked")

type OpenAIRefreshTicket struct {
	GrantID, Attempt, TokenHash string
	Epoch                       int64
}
type OpenAIRefreshFenceRepository interface {
	BeginOpenAIRefresh(context.Context, string) (*OpenAIRefreshTicket, error)
	FinishOpenAIRefresh(context.Context, *OpenAIRefreshTicket, *Account, map[string]any, string) (bool, error)
	MarkOpenAIRefreshUncertain(context.Context, *OpenAIRefreshTicket) error
}
type openAIRefreshAttemptKey struct{}
type openAIRefreshAttempt struct {
	ticket   *OpenAIRefreshTicket
	finished bool
}

func (s *OpenAIOAuthService) SetRefreshCoordination(repo AccountRepository, api *OAuthRefreshAPI) {
	s.refreshFence, _ = repo.(OpenAIRefreshFenceRepository)
	s.refreshCoordinator = api
}
func (s *OpenAIOAuthService) RefreshManagedAccount(ctx context.Context, account *Account) (*Account, error) {
	if s.refreshCoordinator == nil || s.refreshFence == nil {
		return nil, ErrOpenAIRefreshFenced
	}
	result, err := s.refreshCoordinator.RefreshIfNeeded(ctx, account, NewOpenAITokenRefresher(s, s.refreshCoordinator.accountRepo), 0, true)
	if err != nil {
		return nil, err
	}
	if result == nil || result.Account == nil {
		return nil, ErrOpenAIRefreshFenced
	}
	return result.Account, nil
}
func markOpenAIRefreshUncertain(repo OpenAIRefreshFenceRepository, ticket *OpenAIRefreshTicket) {
	if ticket == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = repo.MarkOpenAIRefreshUncertain(ctx, ticket)
}

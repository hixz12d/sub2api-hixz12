package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *accountRepository) affinityRepository() (service.OpenAIAffinityRepository, error) {
	if r == nil || r.openAIAffinity == nil {
		return nil, errors.New("openai affinity repository unavailable")
	}
	return r.openAIAffinity, nil
}

func (r *accountRepository) ResolveResponse(ctx context.Context, ownerScopeHash, provider, responseKeyHash string, now time.Time, refreshTTL, refreshMinInterval time.Duration) (*service.OpenAIResponseBinding, error) {
	repo, err := r.affinityRepository()
	if err != nil {
		return nil, err
	}
	return repo.ResolveResponse(ctx, ownerScopeHash, provider, responseKeyHash, now, refreshTTL, refreshMinInterval)
}

func (r *accountRepository) ResolveSession(ctx context.Context, ownerScopeHash, provider, namespaceHash, primaryHash string, aliases []string, now time.Time, refreshTTL, refreshMinInterval time.Duration) (*service.OpenAISessionBinding, error) {
	repo, err := r.affinityRepository()
	if err != nil {
		return nil, err
	}
	return repo.ResolveSession(ctx, ownerScopeHash, provider, namespaceHash, primaryHash, aliases, now, refreshTTL, refreshMinInterval)
}

func (r *accountRepository) CreateOrGetSession(ctx context.Context, identity service.SessionIdentity, accountID int64, expiresAt time.Time) (*service.OpenAISessionBinding, bool, error) {
	repo, err := r.affinityRepository()
	if err != nil {
		return nil, false, err
	}
	return repo.CreateOrGetSession(ctx, identity, accountID, expiresAt)
}

func (r *accountRepository) BindResponseAndUpgrade(ctx context.Context, identity service.SessionIdentity, responseKeyHash string, accountID int64, responseExpiresAt, strongSessionExpiresAt time.Time) (*service.OpenAIResponseBinding, error) {
	repo, err := r.affinityRepository()
	if err != nil {
		return nil, err
	}
	return repo.BindResponseAndUpgrade(ctx, identity, responseKeyHash, accountID, responseExpiresAt, strongSessionExpiresAt)
}

func (r *accountRepository) ListMigrationCandidates(ctx context.Context, fromAccountID int64, includeExpired bool, limit int, now time.Time) ([]service.OpenAIAffinityMigrationCandidate, error) {
	repo, err := r.affinityRepository()
	if err != nil {
		return nil, err
	}
	return repo.ListMigrationCandidates(ctx, fromAccountID, includeExpired, limit, now)
}

func (r *accountRepository) MigrateBindingCAS(ctx context.Context, bindingID, fromAccountID, toAccountID, expectedVersion int64, reason string, now time.Time) error {
	repo, err := r.affinityRepository()
	if err != nil {
		return err
	}
	return repo.MigrateBindingCAS(ctx, bindingID, fromAccountID, toAccountID, expectedVersion, reason, now)
}

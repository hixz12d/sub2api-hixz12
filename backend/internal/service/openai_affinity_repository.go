package service

import (
	"context"
	"errors"
	"time"
)

var (
	ErrOpenAIAffinityNotFound = errors.New("openai affinity binding not found")
	ErrOpenAIAffinityConflict = errors.New("openai affinity binding conflict")
	ErrOpenAIAffinityCAS      = errors.New("openai affinity binding changed since preview")
)

type OpenAISessionBinding struct {
	ID             int64
	OwnerScopeHash string
	Provider       string
	NamespaceHash  string
	PrimaryHash    string
	AccountID      int64
	Strength       AffinityStrength
	Source         string
	Stateful       bool
	Capability     string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	LastHitAt      time.Time
	ExpiresAt      time.Time
	Version        int64
}

type OpenAIResponseBinding struct {
	ID               int64
	OwnerScopeHash   string
	Provider         string
	ResponseKeyHash  string
	AccountID        int64
	SessionBindingID *int64
	Capability       string
	CreatedAt        time.Time
	LastHitAt        time.Time
	ExpiresAt        time.Time
	Version          int64
}

type OpenAIAffinityMigrationCandidate struct {
	BindingID int64
	AccountID int64
	Strength  AffinityStrength
	Version   int64
	ExpiresAt time.Time
}

type OpenAIAffinityRepository interface {
	ResolveResponse(ctx context.Context, ownerScopeHash, provider, responseKeyHash string, now time.Time, refreshTTL, refreshMinInterval time.Duration) (*OpenAIResponseBinding, error)
	ResolveSession(ctx context.Context, ownerScopeHash, provider, namespaceHash, primaryHash string, aliases []string, now time.Time, refreshTTL, refreshMinInterval time.Duration) (*OpenAISessionBinding, error)
	CreateOrGetSession(ctx context.Context, identity SessionIdentity, accountID int64, expiresAt time.Time) (*OpenAISessionBinding, bool, error)
	BindResponseAndUpgrade(ctx context.Context, identity SessionIdentity, responseKeyHash string, accountID int64, responseExpiresAt, strongSessionExpiresAt time.Time) (*OpenAIResponseBinding, error)
	ListMigrationCandidates(ctx context.Context, fromAccountID int64, includeExpired bool, limit int, now time.Time) ([]OpenAIAffinityMigrationCandidate, error)
	MigrateBindingCAS(ctx context.Context, bindingID, fromAccountID, toAccountID, expectedVersion int64, reason string, now time.Time) error
}

package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func (s *OpenAIGatewayService) SetOpenAIAffinityRepository(repo OpenAIAffinityRepository) {
	if s != nil {
		s.openAIAffinityRepo = repo
	}
}

func (s *OpenAIGatewayService) openAIAffinityTTL(strength AffinityStrength) time.Duration {
	if s == nil || s.cfg == nil {
		return openAIAffinityDefaultWeak
	}
	cfg := s.cfg.Gateway.OpenAIAffinity
	switch strength {
	case AffinityStrong:
		if cfg.StrongTTLHours > 0 {
			return boundedOpenAIAffinityTTL(time.Duration(cfg.StrongTTLHours)*time.Hour, time.Hour, 7*24*time.Hour, openAIAffinityDefaultStrong)
		}
		return openAIAffinityDefaultStrong
	case AffinityExplicit:
		if cfg.ExplicitTTLHours > 0 {
			return boundedOpenAIAffinityTTL(time.Duration(cfg.ExplicitTTLHours)*time.Hour, time.Hour, 7*24*time.Hour, openAIAffinityDefaultExplicit)
		}
		return openAIAffinityDefaultExplicit
	default:
		if cfg.WeakTTLMinutes > 0 {
			return boundedOpenAIAffinityTTL(time.Duration(cfg.WeakTTLMinutes)*time.Minute, 5*time.Minute, 24*time.Hour, openAIAffinityDefaultWeak)
		}
		return openAIAffinityDefaultWeak
	}
}

func (s *OpenAIGatewayService) openAIAffinityResponseTTL() time.Duration {
	if s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIAffinity.ResponseTTLHours > 0 {
		return boundedOpenAIAffinityTTL(time.Duration(s.cfg.Gateway.OpenAIAffinity.ResponseTTLHours)*time.Hour, 24*time.Hour, 7*24*time.Hour, openAIAffinityDefaultResponse)
	}
	return openAIAffinityDefaultResponse
}

func (s *OpenAIGatewayService) openAIAffinityRefreshMinInterval() time.Duration {
	if s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIAffinity.RefreshMinIntervalSeconds > 0 {
		return boundedOpenAIAffinityTTL(time.Duration(s.cfg.Gateway.OpenAIAffinity.RefreshMinIntervalSeconds)*time.Second, time.Minute, time.Hour, openAIAffinityDefaultRefresh)
	}
	return openAIAffinityDefaultRefresh
}

func boundedOpenAIAffinityTTL(value, minValue, maxValue, fallback time.Duration) time.Duration {
	if value < minValue || value > maxValue {
		return fallback
	}
	return value
}

func (s *OpenAIGatewayService) resolvePersistentOpenAISession(ctx context.Context) (*OpenAISessionBinding, error) {
	value, enabled := openAIAffinityFromContext(ctx)
	if !enabled {
		return nil, ErrOpenAIAffinityNotFound
	}
	if value.Err != nil {
		return nil, value.Err
	}
	if s == nil || s.openAIAffinityRepo == nil {
		return nil, fmt.Errorf("%w: repository unavailable", ErrOpenAIAffinityConfiguration)
	}
	identity := value.Identity
	if identity.PrimaryHash == "" && len(identity.Aliases) == 0 {
		if identity.Stateful && identity.PreviousResponseHash == "" {
			return nil, ErrOpenAIAffinityStateUnbound
		}
		return nil, ErrOpenAIAffinityNotFound
	}
	binding, err := s.openAIAffinityRepo.ResolveSession(ctx, identity.OwnerScopeHash, identity.Provider,
		identity.NamespaceHash, identity.PrimaryHash, identity.Aliases, time.Now().UTC(),
		s.openAIAffinityTTL(identity.Strength), s.openAIAffinityRefreshMinInterval())
	if errors.Is(err, ErrOpenAIAffinityNotFound) && identity.Stateful {
		return nil, ErrOpenAIAffinityStateUnbound
	}
	return binding, err
}

func (s *OpenAIGatewayService) createOrGetPersistentOpenAISession(ctx context.Context, candidateAccountID int64) (*OpenAISessionBinding, bool, error) {
	value, enabled := openAIAffinityFromContext(ctx)
	if !enabled || !value.Writable || value.Identity.PrimaryHash == "" {
		return nil, false, ErrOpenAIAffinityNotFound
	}
	if value.Err != nil {
		return nil, false, value.Err
	}
	if s == nil || s.openAIAffinityRepo == nil {
		return nil, false, fmt.Errorf("%w: repository unavailable", ErrOpenAIAffinityConfiguration)
	}
	expires := time.Now().UTC().Add(s.openAIAffinityTTL(value.Identity.Strength))
	return s.openAIAffinityRepo.CreateOrGetSession(ctx, value.Identity, candidateAccountID, expires)
}

func (s *OpenAIGatewayService) resolvePersistentOpenAIResponse(ctx context.Context) (*OpenAIResponseBinding, error) {
	value, enabled := openAIAffinityFromContext(ctx)
	if !enabled {
		return nil, ErrOpenAIAffinityNotFound
	}
	if value.Err != nil {
		return nil, value.Err
	}
	identity := value.Identity
	if identity.PreviousResponseHash == "" {
		return nil, ErrOpenAIAffinityNotFound
	}
	if s == nil || s.openAIAffinityRepo == nil {
		return nil, fmt.Errorf("%w: repository unavailable", ErrOpenAIAffinityConfiguration)
	}
	return s.openAIAffinityRepo.ResolveResponse(ctx, identity.OwnerScopeHash, identity.Provider,
		identity.PreviousResponseHash, time.Now().UTC(), s.openAIAffinityResponseTTL(), s.openAIAffinityRefreshMinInterval())
}

func (s *OpenAIGatewayService) openAIAffinityResponseHash(ctx context.Context, c *gin.Context, responseID string) (string, error) {
	responseID = boundedOpenAIAffinitySignal(responseID)
	if responseID == "" {
		return "", ErrOpenAIAffinityNotFound
	}
	secret, err := s.openAIAffinitySecret()
	if err != nil {
		return "", err
	}
	owner := openAIAffinityOwnerSeed(c)
	if owner == "" {
		if wsOwner, ok := openAIWSStateOwnerFromContext(ctx); ok && wsOwner.GroupID > 0 && wsOwner.UserID > 0 && wsOwner.APIKeyID > 0 {
			owner = fmt.Sprintf("group:%d|user:%d|api_key:%d", wsOwner.GroupID, wsOwner.UserID, wsOwner.APIKeyID)
		}
	}
	if owner == "" {
		return "", fmt.Errorf("%w: authenticated owner is unavailable", ErrOpenAIAffinityConfiguration)
	}
	return openAIAffinityDigest(secret, openAIAffinityDomain, "response", owner, openAIAffinityProvider, responseID), nil
}

func (s *OpenAIGatewayService) bindPersistentOpenAIResponse(ctx context.Context, c *gin.Context, account *Account, responseID string) error {
	if !s.openAIAffinityEnabled() || account == nil || !account.IsOpenAIOAuth() {
		return nil
	}
	value, enabled := openAIAffinityFromContext(ctx)
	if !enabled && c != nil {
		value, enabled = openAIAffinityFromGin(c)
	}
	if !enabled || !value.Writable {
		return nil
	}
	if value.Err != nil {
		return value.Err
	}
	if s.openAIAffinityRepo == nil {
		return fmt.Errorf("%w: repository unavailable", ErrOpenAIAffinityConfiguration)
	}
	responseHash, err := s.openAIAffinityResponseHash(ctx, c, responseID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	_, err = s.openAIAffinityRepo.BindResponseAndUpgrade(ctx, value.Identity, responseHash, account.ID,
		now.Add(s.openAIAffinityResponseTTL()), now.Add(s.openAIAffinityTTL(AffinityStrong)))
	return err
}

func shouldFailClosedOpenAIAffinity(err error) bool {
	return err != nil && !errors.Is(err, ErrOpenAIAffinityNotFound)
}

func normalizeOpenAIAffinityReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if len(reason) > 256 {
		return reason[:256]
	}
	return reason
}

package service

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	codexRecoveryOwnerMissing       = "The continuation owner could not be verified. Retry with the original API key or restore the full conversation context."
	codexRecoverySnapshotMissing    = "The continuation's saved identity is missing or expired. Restore the full conversation context; changing the installation policy alone cannot recover it."
	codexRecoveryAccountUnavailable = "The original conversation account is unavailable. Retry after it recovers, or restore the full conversation context."
	codexRecoveryAccountMismatch    = "The continuation belongs to a different upstream account. Restore its original account or recover using the full conversation context."
	codexRecoveryRouteChanged       = "The conversation's upstream route or proxy changed. Restore the original route or recover using the full conversation context."
	codexRecoveryRefreshFailed      = "The original conversation account's OAuth refresh failed. Retry after its credentials recover, or restore the full conversation context."
)

func codexRecoveryFailure(message string) *UpstreamFailoverError {
	failure := openAIConversationRecoveryError()
	failure.ClientMessage = conversationRecoveryClientMessage(message)
	return failure
}

// An account-policy change is not evidence of a new conversation. Recover only
// an existing response-keyed record, never synthesize an old identity from the
// account's current credentials or an untrusted caller-supplied root key.
func (s *OpenAIGatewayService) validateCodexLegacyContinuation(c *gin.Context, responseID string, account *Account) error {
	if account == nil || account.Status != StatusActive || !account.Schedulable {
		return codexRecoveryFailure(codexRecoveryAccountUnavailable)
	}
	ctx := WithOpenAIWSRequestOwner(c.Request.Context(), c)
	if _, ok := openAIWSStateOwnerFromContext(ctx); !ok {
		return codexRecoveryFailure(codexRecoveryOwnerMissing)
	}
	var accountID int64
	binding, err := s.resolvePersistentOpenAIResponse(ctx)
	if err == nil && binding != nil {
		responseHash, hashErr := s.openAIAffinityResponseHash(ctx, c, responseID)
		if hashErr != nil {
			return hashErr
		}
		if responseHash != binding.ResponseKeyHash {
			return codexRecoveryFailure(codexRecoveryOwnerMissing)
		}
		accountID = binding.AccountID
	} else if err != nil && !errors.Is(err, ErrOpenAIAffinityNotFound) {
		return err
	}
	if accountID == 0 {
		accountID, err = getOpenAIWSResponseAccount(ctx, s.getOpenAIWSStateStore(), getOpenAIGroupIDFromContext(c), responseID)
		if err != nil {
			return err
		}
	}
	if accountID == 0 {
		return codexRecoveryFailure(codexRecoveryOwnerMissing)
	}
	if accountID != account.ID {
		return codexRecoveryFailure(codexRecoveryAccountMismatch)
	}
	plan, ok := CodexRequestPlanFromContext(ctx)
	if !ok {
		return codexRecoveryFailure(codexRecoverySnapshotMissing)
	}
	registry, ok := s.codexConversationRegistry()
	if !ok {
		return codexRecoveryFailure(codexRecoverySnapshotMissing)
	}
	state, err := registry.GetCodexConversation(ctx, plan.ConversationDigest())
	if errors.Is(err, ErrCodexConversationNotFound) {
		return codexRecoveryFailure(codexRecoverySnapshotMissing)
	}
	if err != nil {
		return err
	}
	if state.AccountID != account.ID {
		return codexRecoveryFailure(codexRecoveryAccountMismatch)
	}
	if err := state.Validate(); err != nil {
		return err
	}
	return nil
}

func codexConversationTransportRefreshAllowed(current, candidate CodexConversationState) bool {
	return current.AccountID == candidate.AccountID &&
		current.ProfileID == candidate.ProfileID &&
		current.IdentityPolicyVersion == candidate.IdentityPolicyVersion &&
		(current.ProxyIdentity != candidate.ProxyIdentity ||
			current.EgressRoute != candidate.EgressRoute ||
			current.TransportConfigVersion != candidate.TransportConfigVersion)
}

func adoptCodexConversationConnectionDefaults(candidate, resolved CodexConversationState) CodexConversationState {
	if candidate.AccountID != resolved.AccountID {
		return candidate
	}
	if candidate.EgressRoute == "" {
		candidate.EgressRoute = resolved.EgressRoute
	}
	if candidate.ProxyIdentity == "" {
		candidate.ProxyIdentity = resolved.ProxyIdentity
	}
	if candidate.TransportConfigVersion == "" {
		candidate.TransportConfigVersion = resolved.TransportConfigVersion
	}
	return candidate
}

// An in-flight request may finish after a connection-only configuration change.
// Commit its success without rolling back the newest transport configuration.
func codexConversationMatchesCompletedAttempt(state CodexConversationState, attempt *CodexAttemptState) bool {
	if attempt != nil && attempt.conversationBinding != nil {
		state.ProxyIdentity = attempt.conversationBinding.ProxyIdentity
		state.EgressRoute = attempt.conversationBinding.EgressRoute
		state.TransportConfigVersion = attempt.conversationBinding.TransportConfigVersion
	}
	return codexConversationMatchesAttempt(state, attempt)
}

func (s *OpenAIGatewayService) codexConversationTTL(plan *CodexRequestPlan) time.Duration {
	ttl := s.openAIAffinityTTL(AffinityStrong)
	if plan != nil && plan.previousResponseID != "" {
		if responseTTL := s.openAIAffinityResponseTTL(); responseTTL > ttl {
			ttl = responseTTL
		}
	}
	return ttl
}

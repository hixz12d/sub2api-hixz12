package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
)

func codexResponseConversationDigest(c *gin.Context, responseID string, deriver *CodexIdentityDeriver) (string, error) {
	if c == nil || c.Request == nil || deriver == nil || strings.TrimSpace(responseID) == "" {
		return "", errors.New("codex response binding requires a request and response id")
	}
	owner, ok := openAIWSStateOwnerFromContext(WithOpenAIWSRequestOwner(c.Request.Context(), c))
	if !ok || owner.UserID <= 0 {
		return "", errors.New("codex response binding requires an authenticated owner")
	}
	// HTTP continuations intentionally allow another key of the same user.
	// Existing HTTP/WS response ownership authorization still runs independently.
	scope := fmt.Sprintf("group:%d:user:%d", owner.GroupID, owner.UserID)
	return deriver.DigestHex("codex/conversation-response/v1", scope, strings.TrimSpace(responseID)), nil
}

// AttachCodexResponseConversationBinding remaps the request plan onto the
// response-chain conversation digest before account selection. Continuations
// commit under codex/conversation-response/v1; without this remap the scheduler
// only sees the session digest and cannot pin the original account.
func (s *OpenAIGatewayService) AttachCodexResponseConversationBinding(c *gin.Context, plan *CodexRequestPlan) *CodexRequestPlan {
	if s == nil || c == nil || c.Request == nil || plan == nil || strings.TrimSpace(plan.previousResponseID) == "" {
		return plan
	}
	secret := ResolveCodexIdentityDerivationSecret(s.cfg)
	if strings.TrimSpace(secret) == "" {
		return plan
	}
	deriver, err := NewCodexIdentityDeriver(secret)
	if err != nil {
		return plan
	}
	digest, err := codexResponseConversationDigest(c, plan.previousResponseID, deriver)
	if err != nil {
		return plan
	}
	registry, ok := s.codexConversationRegistry()
	if !ok {
		return plan
	}
	state, err := registry.GetCodexConversation(c.Request.Context(), digest)
	if err != nil || state.AccountID <= 0 {
		return plan
	}
	clone := *plan
	clone.conversationDigest = digest
	clone.requireExistingConversation = true
	c.Request = c.Request.WithContext(ContextWithCodexRequestPlan(c.Request.Context(), &clone))
	return &clone
}

func (s *OpenAIGatewayService) resolveCodexResponseConversationPlan(c *gin.Context, plan *CodexRequestPlan, policy string, deriver *CodexIdentityDeriver, account *Account) (*CodexRequestPlan, bool, error) {
	if plan == nil || plan.previousResponseID == "" {
		return plan, false, nil
	}
	digest, err := codexResponseConversationDigest(c, plan.previousResponseID, deriver)
	if err != nil {
		if policy == CodexInstallationStableV1 {
			return nil, false, codexRecoveryFailure(codexRecoveryOwnerMissing)
		}
		return plan, false, nil
	}
	registry, ok := s.codexConversationRegistry()
	if !ok {
		return nil, false, errors.New("relay kernel requires a Codex conversation registry")
	}
	_, err = registry.GetCodexConversation(c.Request.Context(), digest)
	if errors.Is(err, ErrCodexConversationNotFound) {
		if policy == CodexInstallationStableV1 {
			if err := s.validateCodexLegacyContinuation(c, plan.previousResponseID, account); err != nil {
				var failure *UpstreamFailoverError
				if errors.As(err, &failure) && (failure.ClientMessage == codexRecoveryAccountMismatch || failure.ClientMessage == codexRecoverySnapshotMissing) {
					if _, rebuildErr := PrepareCodexFullContextRecovery(c, account.ID, plan.body); rebuildErr == nil {
						if rebuilt, ok := CodexRequestPlanFromContext(c.Request.Context()); ok && rebuilt != plan {
							return rebuilt, false, nil
						}
					}
				}
				return nil, false, err
			}
			clone := *plan
			clone.requireExistingConversation = true
			c.Request = c.Request.WithContext(ContextWithCodexRequestPlan(c.Request.Context(), &clone))
			return &clone, true, nil
		}
		return plan, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	clone := *plan
	clone.conversationDigest = digest
	clone.requireExistingConversation = true
	c.Request = c.Request.WithContext(ContextWithCodexRequestPlan(c.Request.Context(), &clone))
	return &clone, false, nil
}

// CommitCodexConversationResponse snapshots the successful response's pin using
// the existing atomic registry. Continuations can follow the response chain
// without depending on the original prompt/session key or refreshing its TTL.
func (s *OpenAIGatewayService) CommitCodexConversationResponse(c *gin.Context, responseID string) error {
	if c == nil || c.Request == nil {
		return nil
	}
	ctx := c.Request.Context()
	attempt, hasAttempt := CodexAttemptStateFromContext(ctx)
	plan, hasPlan := CodexRequestPlanFromContext(ctx)
	if !hasAttempt || !hasPlan || attempt.PolicyVersion() != CodexIdentityPolicyV2 {
		return nil
	}
	if err := s.CommitCodexConversation(ctx); err != nil {
		return err
	}
	if strings.TrimSpace(responseID) == "" {
		return nil
	}
	digest, err := codexResponseConversationDigest(c, responseID, attempt.deriver)
	if err != nil {
		return err
	}
	registry, ok := s.codexConversationRegistry()
	if !ok {
		return errors.New("relay kernel requires a Codex conversation registry")
	}
	current, err := registry.GetCodexConversation(ctx, plan.ConversationDigest())
	if err != nil {
		return err
	}
	if !current.Committed || !codexConversationMatchesCompletedAttempt(current, attempt) {
		return ErrCodexConversationCASConflict
	}
	ttl := s.openAIAffinityTTL(AffinityStrong)
	if responseTTL := s.openAIAffinityResponseTTL(); responseTTL > ttl {
		ttl = responseTTL
	}
	resolved, _, err := registry.ResolveOrCreateCodexConversation(ctx, digest, current, ttl)
	if err != nil {
		return err
	}
	if !resolved.Committed || !codexConversationMatchesCompletedAttempt(resolved, attempt) {
		return ErrCodexConversationCASConflict
	}
	return nil
}

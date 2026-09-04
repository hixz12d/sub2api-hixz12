package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrCodexConversationNotFound = errors.New("codex conversation not found")
var ErrCodexConversationCASConflict = errors.New("codex conversation compare-and-swap conflict")

type CodexConversationState struct {
	Revision               int64  `json:"revision"`
	AccountID              int64  `json:"account_id"`
	ProxyIdentity          string `json:"proxy_identity"`
	ProfileID              string `json:"profile_id"`
	IdentityPolicyVersion  string `json:"identity_policy_version"`
	PoolSlot               int    `json:"pool_slot"`
	DeviceID               string `json:"device_id"`
	SessionID              string `json:"session_id"`
	ThreadID               string `json:"thread_id"`
	WindowID               string `json:"window_id"`
	EgressRoute            string `json:"egress_route"`
	TransportConfigVersion string `json:"transport_config_version"`
	Committed              bool   `json:"committed"`
	Active                 bool   `json:"active"`
	CreatedAtUnixMS        int64  `json:"created_at_unix_ms"`
	LastActivityUnixMS     int64  `json:"last_activity_unix_ms"`
}

func (s CodexConversationState) Validate() error {
	if s.AccountID <= 0 {
		return errors.New("codex conversation account id is required")
	}
	if strings.TrimSpace(s.ProfileID) == "" || strings.TrimSpace(s.IdentityPolicyVersion) == "" {
		return errors.New("codex conversation profile and identity policy are required")
	}
	if s.PoolSlot < 0 {
		return errors.New("codex conversation pool slot cannot be negative")
	}
	if strings.TrimSpace(s.DeviceID) == "" || strings.TrimSpace(s.SessionID) == "" || strings.TrimSpace(s.ThreadID) == "" || strings.TrimSpace(s.WindowID) == "" {
		return errors.New("codex conversation identity is incomplete")
	}
	return nil
}

type CodexConversationRegistry interface {
	ResolveOrCreateCodexConversation(ctx context.Context, conversationDigest string, candidate CodexConversationState, ttl time.Duration) (state CodexConversationState, created bool, err error)
	GetCodexConversation(ctx context.Context, conversationDigest string) (CodexConversationState, error)
	CompareAndSwapCodexConversation(ctx context.Context, conversationDigest string, expectedRevision int64, expectedAccountID int64, next CodexConversationState, ttl time.Duration) (CodexConversationState, error)
	InvalidateCodexConversation(ctx context.Context, conversationDigest string, expectedRevision int64, expectedAccountID int64) (bool, error)
}

func codexConversationStateFromAttempt(plan *CodexRequestPlan, attempt *CodexAttemptState, input CodexAttemptInput) (CodexConversationState, error) {
	if plan == nil || attempt == nil {
		return CodexConversationState{}, errors.New("codex plan and attempt are required")
	}
	identity := attempt.Identity()
	if identity == nil {
		return CodexConversationState{}, errors.New("codex conversation registry requires managed identity")
	}
	nowMS := time.Now().UTC().UnixMilli()
	return CodexConversationState{
		Revision:               1,
		AccountID:              attempt.AccountID(),
		ProxyIdentity:          strings.TrimSpace(input.ProxyIdentity),
		ProfileID:              attempt.Profile().ID,
		IdentityPolicyVersion:  attempt.PolicyVersion(),
		PoolSlot:               attempt.PoolSlot(),
		DeviceID:               identity.InstallationID(),
		SessionID:              identity.SessionID(),
		ThreadID:               identity.ThreadID(),
		WindowID:               identity.WindowID(),
		EgressRoute:            strings.TrimSpace(input.EgressRoute),
		TransportConfigVersion: strings.TrimSpace(input.TransportConfigVersion),
		Committed:              strings.TrimSpace(plan.previousResponseID) != "",
		Active:                 true,
		CreatedAtUnixMS:        nowMS,
		LastActivityUnixMS:     nowMS,
	}, nil
}

func applyCodexConversationState(attempt *CodexAttemptState, state CodexConversationState) *CodexAttemptState {
	if attempt == nil || attempt.identity == nil {
		return attempt
	}
	clone := *attempt
	clone.identity = cloneCodexIdentitySnapshot(attempt.identity)
	clone.profile = attempt.Profile()
	clone.finalHeaders = attempt.FinalHeaders()
	clone.finalHTTPBody = attempt.FinalHTTPBody()
	clone.finalWSPayload = attempt.FinalWSPayload()
	clone.identity.installationID = state.DeviceID
	clone.identity.sessionID = state.SessionID
	clone.identity.threadID = state.ThreadID
	clone.identity.windowID = state.WindowID
	return &clone
}

func (s *OpenAIGatewayService) codexConversationRegistry() (CodexConversationRegistry, bool) {
	if s == nil || s.cache == nil {
		return nil, false
	}
	registry, ok := s.cache.(CodexConversationRegistry)
	return registry, ok
}

func codexConversationAttemptTupleEqual(left, right CodexConversationState) bool {
	return left.AccountID == right.AccountID &&
		left.ProxyIdentity == right.ProxyIdentity &&
		left.ProfileID == right.ProfileID &&
		left.IdentityPolicyVersion == right.IdentityPolicyVersion &&
		left.TransportConfigVersion == right.TransportConfigVersion &&
		left.EgressRoute == right.EgressRoute
}
func (s *OpenAIGatewayService) resolveCodexConversationAttempt(
	ctx context.Context,
	plan *CodexRequestPlan,
	attempt *CodexAttemptState,
	input CodexAttemptInput,
) (*CodexAttemptState, error) {
	if attempt == nil || attempt.Identity() == nil {
		return attempt, nil
	}
	registry, ok := s.codexConversationRegistry()
	if !ok {
		return nil, errors.New("relay kernel requires a Codex conversation registry")
	}
	candidate, err := codexConversationStateFromAttempt(plan, attempt, input)
	if err != nil {
		return nil, err
	}
	resolved, created, err := registry.ResolveOrCreateCodexConversation(ctx, plan.ConversationDigest(), candidate, s.openAIAffinityTTL(AffinityStrong))
	if err != nil {
		return nil, err
	}
	for retries := 0; !created && !codexConversationAttemptTupleEqual(resolved, candidate); retries++ {
		if resolved.Committed {
			return nil, fmt.Errorf("codex conversation is bound to account %d and committed to its transport tuple", resolved.AccountID)
		}
		if retries >= 3 {
			return nil, ErrCodexConversationCASConflict
		}
		candidate.CreatedAtUnixMS = resolved.CreatedAtUnixMS
		candidate.LastActivityUnixMS = time.Now().UTC().UnixMilli()
		replaced, replaceErr := registry.CompareAndSwapCodexConversation(
			ctx,
			plan.ConversationDigest(),
			resolved.Revision,
			resolved.AccountID,
			candidate,
			s.openAIAffinityTTL(AffinityStrong),
		)
		if replaceErr == nil {
			resolved = replaced
			break
		}
		if !errors.Is(replaceErr, ErrCodexConversationCASConflict) {
			return nil, replaceErr
		}
		resolved, err = registry.GetCodexConversation(ctx, plan.ConversationDigest())
		if err != nil {
			return nil, err
		}
	}
	resolvedAttempt := applyCodexConversationState(attempt, resolved)
	resolvedAttempt.finalHeaders = buildCodexAttemptIdentityHeaders(resolvedAttempt.profile, resolvedAttempt.identity, plan.inboundHeaders)
	resolvedAttempt.finalHTTPBody, err = applyCodexFingerprintToRawBody(plan.body, resolvedAttempt.identity)
	if err != nil {
		return nil, err
	}
	return resolvedAttempt, nil
}

func (s *OpenAIGatewayService) CommitCodexConversation(ctx context.Context) error {
	registry, ok := s.codexConversationRegistry()
	if !ok || ctx == nil {
		return nil
	}
	plan, hasPlan := CodexRequestPlanFromContext(ctx)
	attempt, hasAttempt := CodexAttemptStateFromContext(ctx)
	if !hasPlan || !hasAttempt || attempt.PolicyVersion() != CodexIdentityPolicyV2 {
		return nil
	}
	current, err := registry.GetCodexConversation(ctx, plan.ConversationDigest())
	if err != nil {
		return err
	}
	if current.AccountID != attempt.AccountID() {
		return ErrCodexConversationCASConflict
	}
	if current.Committed {
		return nil
	}
	next := current
	next.Committed = true
	next.Active = true
	next.LastActivityUnixMS = time.Now().UTC().UnixMilli()
	_, err = registry.CompareAndSwapCodexConversation(
		ctx,
		plan.ConversationDigest(),
		current.Revision,
		current.AccountID,
		next,
		s.openAIAffinityTTL(AffinityStrong),
	)
	if errors.Is(err, ErrCodexConversationCASConflict) {
		latest, getErr := registry.GetCodexConversation(ctx, plan.ConversationDigest())
		if getErr == nil && latest.AccountID == attempt.AccountID() && latest.Committed {
			return nil
		}
	}
	return err
}

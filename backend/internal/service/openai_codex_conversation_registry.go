package service

import (
	"context"
	"errors"
	"strings"
	"time"
)

var ErrCodexConversationNotFound = errors.New("codex conversation not found")
var ErrCodexConversationCASConflict = errors.New("codex conversation compare-and-swap conflict")

type CodexConversationState struct {
	ProfileSnapshot        *CodexClientProfile `json:"profile_snapshot,omitempty"`
	ProfileDigest          string              `json:"profile_digest,omitempty"`
	FingerprintMode        string              `json:"fingerprint_mode,omitempty"`
	InstallationPolicy     string              `json:"installation_policy,omitempty"`
	Revision               int64               `json:"revision"`
	AccountID              int64               `json:"account_id"`
	ProxyIdentity          string              `json:"proxy_identity"`
	ProfileID              string              `json:"profile_id"`
	IdentityPolicyVersion  string              `json:"identity_policy_version"`
	PoolSlot               int                 `json:"pool_slot"`
	DeviceID               string              `json:"device_id"`
	SessionID              string              `json:"session_id"`
	ThreadID               string              `json:"thread_id"`
	WindowID               string              `json:"window_id"`
	EgressRoute            string              `json:"egress_route"`
	TransportConfigVersion string              `json:"transport_config_version"`
	Committed              bool                `json:"committed"`
	Active                 bool                `json:"active"`
	CreatedAtUnixMS        int64               `json:"created_at_unix_ms"`
	LastActivityUnixMS     int64               `json:"last_activity_unix_ms"`
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
	if (s.ProfileSnapshot == nil) != (s.ProfileDigest == "") {
		return errors.New("codex conversation profile snapshot is incomplete")
	}
	if s.ProfileSnapshot != nil {
		if _, err := s.pinnedProfile(); err != nil {
			return err
		}
	}
	if _, err := normalizeCodexInstallationPolicy(s.InstallationPolicy); err != nil {
		return err
	}
	if s.FingerprintMode != "" {
		mode := normalizeCodexFingerprintMode(s.FingerprintMode)
		if string(mode) != s.FingerprintMode || mode == codexFingerprintOff {
			return errors.New("invalid pinned fingerprint mode")
		}
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
	profile := attempt.Profile()
	digest, err := codexProfileSnapshotDigest(profile)
	if err != nil {
		return CodexConversationState{}, err
	}
	return CodexConversationState{
		Revision:               1,
		ProfileSnapshot:        &profile,
		ProfileDigest:          digest,
		FingerprintMode:        string(identity.mode),
		InstallationPolicy:     attempt.installationPolicy,
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
	binding := state
	binding.ProfileSnapshot = nil
	clone.conversationBinding = &binding
	clone.identity = cloneCodexIdentitySnapshot(attempt.identity)
	clone.profile = attempt.Profile()
	clone.finalHeaders = attempt.FinalHeaders()
	clone.finalHTTPBody = attempt.FinalHTTPBody()
	clone.finalWSPayload = attempt.FinalWSPayload()
	clone.identity.installationID = state.DeviceID
	clone.poolSlot = state.PoolSlot
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

func (s *OpenAIGatewayService) boundCodexConversationAccountID(ctx context.Context) int64 {
	registry, ok := s.codexConversationRegistry()
	if !ok {
		return 0
	}
	plan, hasPlan := CodexRequestPlanFromContext(ctx)
	if !hasPlan || plan == nil || strings.TrimSpace(plan.ConversationDigest()) == "" {
		return 0
	}
	state, err := registry.GetCodexConversation(ctx, plan.ConversationDigest())
	if err != nil {
		return 0
	}
	if state.AccountID <= 0 {
		return 0
	}
	return state.AccountID
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
	replaySafe bool,
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
	var resolved CodexConversationState
	created := false
	if plan.requireExistingConversation {
		resolved, err = registry.GetCodexConversation(ctx, plan.ConversationDigest())
		if errors.Is(err, ErrCodexConversationNotFound) {
			return nil, codexRecoveryFailure(codexRecoverySnapshotMissing)
		}
	} else {
		resolved, created, err = registry.ResolveOrCreateCodexConversation(ctx, plan.ConversationDigest(), candidate, s.codexConversationTTL(plan))
	}
	if err != nil {
		return nil, err
	}
	recoveringCommitted := resolved.Committed
	for retries := 0; !created; retries++ {
		if resolved.AccountID == candidate.AccountID {
			pinnedInput, pinErr := pinCodexInputToConversation(input, resolved)
			if pinErr != nil {
				return nil, pinErr
			}
			attempt, err = finalizeCodexAttemptWithDeriver(plan, pinnedInput, attempt.deriver)
			if err != nil {
				return nil, err
			}
			candidate, err = codexConversationStateFromAttempt(plan, attempt, pinnedInput)
			if err != nil {
				return nil, err
			}
		}
		candidate = adoptCodexConversationConnectionDefaults(candidate, resolved)
		if codexConversationAttemptTupleEqual(resolved, candidate) {
			break
		}
		// A CAS loser must not replace a healthy winner still preparing output.
		recoveringCommitted = recoveringCommitted || resolved.Committed
		refreshTransport := codexConversationTransportRefreshAllowed(resolved, candidate)
		if recoveringCommitted && !refreshTransport && !s.canRecoverUnavailableCodexConversation(ctx, plan, resolved, candidate, replaySafe) {
			if resolved.AccountID == candidate.AccountID {
				return nil, codexRecoveryFailure(codexRecoveryRouteChanged)
			}
			return nil, codexRecoveryFailure(codexRecoveryAccountMismatch)
		}
		if refreshTransport {
			proxyIdentity := candidate.ProxyIdentity
			egressRoute := candidate.EgressRoute
			transportVersion := candidate.TransportConfigVersion
			candidate = resolved
			if proxyIdentity != "" {
				candidate.ProxyIdentity = proxyIdentity
			}
			if egressRoute != "" {
				candidate.EgressRoute = egressRoute
			}
			candidate.TransportConfigVersion = transportVersion
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
			s.codexConversationTTL(plan),
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

// Recover only replayable requests whose old account is durably unavailable.
// The caller CASes the observed revision and account; a concurrent new binding
// must be revalidated instead of being deleted or overwritten unconditionally.
func (s *OpenAIGatewayService) canRecoverUnavailableCodexConversation(ctx context.Context, plan *CodexRequestPlan, current, candidate CodexConversationState, replaySafe bool) bool {
	if !replaySafe || plan == nil || current.AccountID == candidate.AccountID || s.accountRepo == nil {
		return false
	}
	if plan.previousResponseID != "" || plan.operation == CodexOperationResume ||
		strings.TrimSpace(plan.inboundHeaders.Get(openAIWSTurnStateHeader)) != "" || openAIRetryRequestIsStateful(plan.body) {
		return false
	}
	account, err := s.accountRepo.GetByID(ctx, current.AccountID)
	if errors.Is(err, ErrAccountNotFound) {
		return true
	}
	if err != nil || account == nil {
		return false
	}
	return account.Status != StatusActive || !account.Schedulable
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
	if !codexConversationMatchesCompletedAttempt(current, attempt) {
		return ErrCodexConversationCASConflict
	}
	next := current
	profile := attempt.Profile()
	next.ProfileSnapshot = &profile
	next.ProfileDigest, err = codexProfileSnapshotDigest(profile)
	if err != nil {
		return err
	}
	next.FingerprintMode = string(attempt.identity.mode)
	next.InstallationPolicy = attempt.installationPolicy
	next.Committed = true
	next.Active = true
	next.LastActivityUnixMS = time.Now().UTC().UnixMilli()
	_, err = registry.CompareAndSwapCodexConversation(
		ctx,
		plan.ConversationDigest(),
		current.Revision,
		current.AccountID,
		next,
		s.codexConversationTTL(plan),
	)
	if errors.Is(err, ErrCodexConversationCASConflict) {
		latest, getErr := registry.GetCodexConversation(ctx, plan.ConversationDigest())
		if getErr == nil && codexConversationMatchesCompletedAttempt(latest, attempt) && latest.Committed {
			return nil
		}
	}
	return err
}

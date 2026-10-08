package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/tidwall/gjson"
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
	recoveryAccountID := resolved.AccountID
	didRecover := false
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
		if retries > 0 && resolved.AccountID != recoveryAccountID && resolved.AccountID != candidate.AccountID {
			return nil, codexAccountMismatchFailure(resolved.AccountID)
		}
		refreshTransport := codexConversationTransportRefreshAllowed(resolved, candidate)
		if recoveringCommitted && !refreshTransport && !s.canRecoverUnavailableCodexConversation(ctx, plan, resolved, candidate, replaySafe) {
			if resolved.AccountID == candidate.AccountID {
				return nil, codexRecoveryFailure(codexRecoveryRouteChanged)
			}
			return nil, codexAccountMismatchFailure(resolved.AccountID)
		}
		if recoveringCommitted && !refreshTransport && resolved.AccountID != candidate.AccountID {
			didRecover = true
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
	body := plan.body
	if didRecover {
		// Share the HTTP restart's preservation rules and report only counts,
		// never tool definitions, arguments, outputs or encrypted payloads.
		var stats codexContextSanitizeStats
		body, stats, err = sanitizeCodexCrossAccountBody(body)
		if err != nil {
			return nil, err
		}
		resolvedAttempt.recoveredCrossAccount = true
		recoveryHeaders := plan.InboundHeaders()
		deleteOpenAIHeaderEqualFold(recoveryHeaders, openAIWSTurnStateHeader)
		resolvedAttempt.finalHeaders = buildCodexAttemptIdentityHeaders(resolvedAttempt.profile, resolvedAttempt.identity, recoveryHeaders)
		owner, _ := openAIWSStateOwnerFromContext(ctx)
		slog.WarnContext(ctx, "openai.conversation_recovered_cross_account",
			"request_id", plan.logicalRequestID,
			"user_id", owner.UserID,
			"previous_account_id", recoveryAccountID,
			"account_id", resolved.AccountID,
			"dropped", stats.dropped,
			"stripped", stats.stripped,
		)
	}
	resolvedAttempt.finalHTTPBody, err = applyCodexFingerprintToRawBody(body, resolvedAttempt.identity)
	if err != nil {
		return nil, err
	}
	return resolvedAttempt, nil
}

// Recover only replayable requests whose old account is durably unavailable.
// The caller CASes the observed revision and account; a concurrent new binding
// must be revalidated instead of being deleted or overwritten unconditionally.
// previous_response_id alone no longer blocks recovery when the body can rebuild
// context without upstream state (full input / covered tool outputs).
func (s *OpenAIGatewayService) canRecoverUnavailableCodexConversation(ctx context.Context, plan *CodexRequestPlan, current, candidate CodexConversationState, replaySafe bool) bool {
	if !replaySafe || plan == nil || current.AccountID == candidate.AccountID {
		return false
	}
	// Client already rebuilt the turn (full input / covered tool outputs). Allow the
	// selected account to adopt the conversation pin — including after OAuth refresh
	// failure where the original row may still look Active until quarantine lands.
	if codexPlanHasRecoverableFullContext(plan) {
		return true
	}
	if s.accountRepo == nil {
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

// codexPlanHasRecoverableFullContext reports whether the inbound body can rebuild
// the turn without the original account's upstream conversation state.
//
// Pi/OpenCode often resend a large local transcript that still contains foreign
// previous_response_id / item_reference crumbs from the old account. Those crumbs
// are sanitized on recovery; they must not block the rebind itself.
func codexPlanHasRecoverableFullContext(plan *CodexRequestPlan) bool {
	if plan == nil {
		return false
	}
	if strings.TrimSpace(plan.inboundHeaders.Get(openAIWSTurnStateHeader)) != "" {
		return false
	}
	body := plan.body
	if len(body) == 0 || !gjson.ValidBytes(body) || !gjson.ParseBytes(body).IsObject() {
		return false
	}
	if plan.previousResponseID != "" {
		return CanRebuildOpenAIContinuation(body, plan.inboundHeaders)
	}
	return codexBodyHasLocalRebuildableContext(body)
}

// codexBodyHasLocalRebuildableContext is true when the client already shipped a
// non-empty local transcript (string or message-like input items). Pure chain
// continuations with only previous_response_id / item_reference stay false.
func codexBodyHasLocalRebuildableContext(body []byte) bool {
	if strings.TrimSpace(gjson.GetBytes(body, "previous_response_id").String()) != "" {
		return CanRebuildOpenAIContinuation(body, nil)
	}
	input := gjson.GetBytes(body, "input")
	if !input.Exists() {
		return false
	}
	switch {
	case input.Type == gjson.String:
		return strings.TrimSpace(input.String()) != ""
	case input.IsObject():
		return codexInputItemIsLocalContext(input)
	case input.IsArray():
		found := false
		input.ForEach(func(_, item gjson.Result) bool {
			if codexInputItemIsLocalContext(item) {
				found = true
				return false
			}
			return true
		})
		return found
	default:
		return false
	}
}

func codexInputItemIsLocalContext(item gjson.Result) bool {
	if !item.Exists() {
		return false
	}
	if item.Type == gjson.String {
		return strings.TrimSpace(item.String()) != ""
	}
	if !item.IsObject() {
		return false
	}
	// Encrypted blobs are account-bound and never count as local rebuild context.
	if item.Get("encrypted_content").Exists() || item.Get("encrypted_reasoning").Exists() {
		return false
	}
	switch item.Get("type").String() {
	case "message", "input_text", "input_image", "input_file", "text", "reasoning":
		return true
	case "":
		// Untyped objects only count when they carry visible local content.
		return item.Get("content").Exists() || item.Get("text").Exists() || item.Get("role").Exists() || item.Get("output").Exists()
	case "function_call", "custom_tool_call", "computer_call", "web_search_call", "file_search_call", "code_interpreter_call", "image_generation_call", "local_shell_call", "shell_call", "apply_patch_call":
		return true
	case "function_call_output", "custom_tool_call_output", "computer_call_output", "local_shell_call_output", "shell_call_output", "apply_patch_call_output":
		return strings.TrimSpace(item.Get("call_id").String()) != ""
	default:
		// Unknown typed items may still carry local text content.
		if item.Get("content").Exists() || item.Get("text").Exists() || item.Get("output").Exists() {
			return true
		}
		return false
	}
}

// codexCoveredToolCallIDs recognizes local call/output pairs without a fixed
// allowlist of tool call types. Coverage gates replay, not history preservation.

func codexCoveredToolCallIDs(body []byte) map[string]struct{} {
	covered := make(map[string]struct{})
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() && !input.IsObject() {
		return covered
	}
	contextIDs := make(map[string]struct{})
	outputIDs := make(map[string]struct{})
	collect := func(item gjson.Result) {
		if !item.IsObject() {
			return
		}
		itemType := item.Get("type").String()
		callID := strings.TrimSpace(item.Get("call_id").String())
		if callID == "" {
			return
		}
		if strings.HasSuffix(itemType, "_call_output") || itemType == "tool_search_output" {
			outputIDs[callID] = struct{}{}
		} else if strings.HasSuffix(itemType, "_call") {
			contextIDs[callID] = struct{}{}
		}
	}
	if input.IsArray() {
		input.ForEach(func(_, item gjson.Result) bool { collect(item); return true })
	} else {
		collect(input)
	}
	for callID := range outputIDs {
		if _, ok := contextIDs[callID]; ok {
			covered[callID] = struct{}{}
		}
	}
	return covered
}

// SanitizeCodexBodyForCrossAccountRecovery uses the same preservation rules as
// an HTTP account-mismatch restart. Invalid JSON is left to the caller's normal
// validation, preserving this helper's existing no-error API.
func SanitizeCodexBodyForCrossAccountRecovery(body []byte) []byte {
	out, _, err := sanitizeCodexCrossAccountBody(body)
	if err != nil {
		return body
	}
	return out
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

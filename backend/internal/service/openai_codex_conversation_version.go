package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

const (
	CodexInstallationPolicyExtraKey = "codex_installation_policy"
	CodexInstallationLegacyV2       = "legacy_v2"
	CodexInstallationStableV1       = "stable_v1"
)

func normalizeCodexInstallationPolicy(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", CodexInstallationLegacyV2:
		return CodexInstallationLegacyV2, nil
	case CodexInstallationStableV1:
		return CodexInstallationStableV1, nil
	default:
		return "", errors.New("unknown codex installation policy")
	}
}

func codexStableInstallationID(deriver *CodexIdentityDeriver, accountID int64, profileID string) string {
	family := profileID
	switch profileID {
	case CodexProfileCLI, CodexProfileExec, CodexProfileDesktop:
		family = "codex"
	}
	return deriver.UUIDv4("codex/installation/stable/v1", strconv.FormatInt(accountID, 10), family)
}

func codexProfileSnapshotDigest(profile CodexClientProfile) (string, error) {
	if err := ValidateCodexClientProfile(profile); err != nil {
		return "", err
	}
	// Struct JSON has a deterministic field order. Hash the decoded typed value,
	// not Redis's JSON bytes, whose object key order changes during Lua CAS.
	data, err := json.Marshal(profile)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func cloneCodexClientProfile(profile CodexClientProfile) CodexClientProfile {
	profile.Transport.HeaderOrder = append([]string(nil), profile.Transport.HeaderOrder...)
	return profile
}

func resolveCodexAttemptProfile(input CodexAttemptInput, headers http.Header) (CodexClientProfile, error) {
	if input.ProfileSnapshot == nil {
		return ResolveCodexClientProfileForRequest(input.ProfileID, headers)
	}
	profile := cloneCodexClientProfile(*input.ProfileSnapshot)
	if err := ValidateCodexClientProfile(profile); err != nil {
		return CodexClientProfile{}, err
	}
	if profile.ID != input.ProfileID {
		return CodexClientProfile{}, errors.New("pinned codex profile id mismatch")
	}
	return profile, nil
}

func (state CodexConversationState) pinnedProfile() (CodexClientProfile, error) {
	if state.ProfileSnapshot == nil {
		// Pre-migration records were written with the original revision-one catalog.
		// Never infer a new revision from the account's current desired selector.
		return resolveLegacyCodexConversationProfile(state.ProfileID)
	}
	digest, err := codexProfileSnapshotDigest(*state.ProfileSnapshot)
	if err != nil {
		return CodexClientProfile{}, err
	}
	if digest != state.ProfileDigest || state.ProfileSnapshot.ID != state.ProfileID {
		return CodexClientProfile{}, errors.New("codex conversation profile snapshot mismatch")
	}
	return cloneCodexClientProfile(*state.ProfileSnapshot), nil
}

func pinCodexInputToConversation(input CodexAttemptInput, state CodexConversationState) (CodexAttemptInput, error) {
	if err := state.Validate(); err != nil {
		return input, err
	}
	if input.AccountID != state.AccountID {
		return input, nil
	}
	profile, err := state.pinnedProfile()
	if err != nil {
		return input, err
	}
	input.ProfileID = profile.ID
	input.ProfileSnapshot = &profile
	input.InstallationPolicy, err = normalizeCodexInstallationPolicy(state.InstallationPolicy)
	if err != nil {
		return input, err
	}
	if state.FingerprintMode != "" {
		input.FingerprintMode = state.FingerprintMode
	}
	return input, nil
}

func (s *OpenAIGatewayService) pinCodexAttemptInput(ctx context.Context, plan *CodexRequestPlan, input CodexAttemptInput) (CodexAttemptInput, error) {
	registry, ok := s.codexConversationRegistry()
	if !ok {
		return input, errors.New("relay kernel requires a Codex conversation registry")
	}
	state, err := registry.GetCodexConversation(ctx, plan.ConversationDigest())
	if errors.Is(err, ErrCodexConversationNotFound) {
		return input, nil
	}
	if err != nil {
		return input, err
	}
	return pinCodexInputToConversation(input, state)
}

func codexConversationMatchesAttempt(state CodexConversationState, attempt *CodexAttemptState) bool {
	if attempt == nil || attempt.identity == nil || state.AccountID != attempt.AccountID() {
		return false
	}
	if attempt.conversationBinding != nil && !codexConversationAttemptTupleEqual(state, *attempt.conversationBinding) {
		return false
	}
	identity := attempt.identity
	if state.DeviceID != identity.InstallationID() || state.SessionID != identity.SessionID() || state.ThreadID != identity.ThreadID() || state.WindowID != identity.WindowID() {
		return false
	}
	profile, err := state.pinnedProfile()
	if err != nil {
		return false
	}
	want, err := codexProfileSnapshotDigest(profile)
	if err != nil {
		return false
	}
	got, err := codexProfileSnapshotDigest(attempt.Profile())
	return err == nil && want == got
}

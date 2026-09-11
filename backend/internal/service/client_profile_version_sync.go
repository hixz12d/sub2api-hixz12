package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/clientprofile"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"golang.org/x/mod/semver"
)

// Optional extensions keep existing repository/client implementations compatible.
// Synchronization never falls back to a non-atomic write.
type ClientProfileSettingCAS interface {
	CompareAndSwapSetting(context.Context, string, *string, string) (bool, error)
}
type ClientProfileReleaseClient interface {
	FetchClientProfileRelease(context.Context, string, string) (ClientReleaseObservation, error)
}
type ClientReleaseObservation struct {
	LatestVersion string
	ETag          string
	NotModified   bool
	Reason        string
	Release       *clientprofile.CompatibleRelease
}

type ClientProfileUpdate struct {
	Schema          int                              `json:"schema"`
	Mode            string                           `json:"mode"`
	Active          *clientprofile.CompatibleRelease `json:"active,omitempty"`
	Previous        *clientprofile.CompatibleRelease `json:"previous,omitempty"`
	LatestVersion   string                           `json:"latest_version,omitempty"`
	Status          string                           `json:"status"`
	Reason          string                           `json:"reason,omitempty"`
	CheckedAt       time.Time                        `json:"checked_at"`
	CheckedContract string                           `json:"checked_contract,omitempty"`
	ETag            string                           `json:"etag,omitempty"`
}

func clientProfileUpdateKey(family string) string {
	return "openai_client_profile_" + family + "_update_v1"
}
func defaultClientProfileUpdate() ClientProfileUpdate {
	return ClientProfileUpdate{Schema: 1, Mode: "auto", Status: "pending"}
}
func cloneClientProfileUpdate(value ClientProfileUpdate) ClientProfileUpdate {
	if value.Active != nil {
		copy := *value.Active
		value.Active = &copy
	}
	if value.Previous != nil {
		copy := *value.Previous
		value.Previous = &copy
	}
	return value
}
func decodeClientProfileUpdate(family, value string) (ClientProfileUpdate, error) {
	var state ClientProfileUpdate
	if len(value) > 16*1024 {
		return state, errors.New("oversized client update record")
	}
	d := json.NewDecoder(bytes.NewBufferString(value))
	d.DisallowUnknownFields()
	if err := d.Decode(&state); err != nil {
		return state, err
	}
	if d.Decode(new(any)) != io.EOF || state.Schema != 1 || (state.Mode != "auto" && state.Mode != "hold") {
		return state, errors.New("invalid client update record")
	}
	for _, release := range []*clientprofile.CompatibleRelease{state.Active, state.Previous} {
		if release != nil && (release.Family != family || release.Validate() != nil) {
			return state, errors.New("invalid stored client release")
		}
	}
	if state.LatestVersion != "" && !clientprofile.StableClientVersion(state.LatestVersion) || len(state.ETag) > 512 || strings.ContainsAny(state.ETag, "\r\n") {
		return state, errors.New("invalid stored release metadata")
	}
	return state, nil
}
func readClientProfileUpdate(ctx context.Context, repo SettingRepository, family string) (ClientProfileUpdate, *string, error) {
	value, err := repo.GetValue(ctx, clientProfileUpdateKey(family))
	if errors.Is(err, ErrSettingNotFound) {
		return defaultClientProfileUpdate(), nil, nil
	}
	if err != nil {
		return ClientProfileUpdate{}, nil, err
	}
	state, err := decodeClientProfileUpdate(family, value)
	return state, &value, err
}

func (s *OpenAICodexVersionSyncService) syncClientProfiles() {
	cas, ok := s.settingRepo.(ClientProfileSettingCAS)
	if !ok {
		return
	}
	source, ok := s.githubClient.(ClientProfileReleaseClient)
	if !ok {
		return
	}
	for _, family := range []string{"pi", "opencode"} {
		select {
		case <-s.stopCh:
			return
		default:
		}
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		stopped := make(chan struct{})
		go func() {
			defer close(stopped)
			select {
			case <-s.stopCh:
				cancel()
			case <-ctx.Done():
			}
		}()
		s.syncClientProfile(ctx, cas, source, family)
		cancel()
		<-stopped
	}
}

func (s *OpenAICodexVersionSyncService) syncClientProfile(ctx context.Context, cas ClientProfileSettingCAS, source ClientProfileReleaseClient, family string) {
	state, expected, err := readClientProfileUpdate(ctx, s.settingRepo, family)
	if err != nil {
		slog.Warn("client_profile_sync_read_failed", "family", family)
		return
	}
	contract, err := clientprofile.ClientReleaseContract(family)
	if err != nil {
		return
	}
	if state.Mode == "hold" {
		return
	}
	now := time.Now().UTC()
	if state.CheckedContract == contract.Digest() && !state.CheckedAt.IsZero() && !state.CheckedAt.After(now) && now.Sub(state.CheckedAt) < openAICodexVersionSyncInterval {
		return
	}
	etag := state.ETag
	if state.CheckedContract != contract.Digest() {
		etag = ""
	}
	observed, fetchErr := source.FetchClientProfileRelease(ctx, family, etag)
	select {
	case <-s.stopCh:
		return
	default:
	}
	state.CheckedAt, state.CheckedContract = now, contract.Digest()
	if fetchErr != nil {
		state.Status, state.Reason = "fetch_failed", "source_unavailable"
		state.ETag = ""
		// Never save a new validator until its source checks have finished.
		slog.Warn("client_profile_sync_fetch_failed", "family", family, "error", fetchErr)
	} else if !observed.NotModified {
		state.ETag = observed.ETag
		if clientprofile.StableClientVersion(observed.LatestVersion) && (state.LatestVersion == "" || semver.Compare("v"+observed.LatestVersion, "v"+state.LatestVersion) > 0) {
			state.LatestVersion = observed.LatestVersion
		}
		state.Status, state.Reason = "needs_review", observed.Reason
		if candidate := observed.Release; candidate != nil {
			if candidate.Family != family || candidate.Version != observed.LatestVersion || candidate.Validate() != nil {
				state.Status, state.Reason, state.ETag = "fetch_failed", "invalid_release", ""
			} else {
				current := contract.BaselineVersion
				if state.Active != nil {
					current = state.Active.Version
				}
				comparison := semver.Compare("v"+candidate.Version, "v"+current)
				if state.Active != nil && comparison == 0 && state.Active.Commit != candidate.Commit {
					state.Reason = "release_retagged"
				} else if comparison >= 0 {
					if state.Active == nil || comparison > 0 {
						previous := state.Active
						if previous == nil {
							previous = &clientprofile.CompatibleRelease{Family: family, Version: contract.BaselineVersion, Tag: "v" + contract.BaselineVersion, Commit: contract.BaselineCommit, ContractDigest: contract.Digest()}
						}
						state.Previous, state.Active = previous, candidate
					}
					state.Status, state.Reason = "current", ""
				} else {
					state.Status, state.Reason = "current", "older_release_ignored"
				}
			}
		}
	} else if state.Status == "fetch_failed" {
		// A failed verification must get a full retry, not become approved by 304.
		state.ETag = ""
	}
	if state.Status == "current" && state.Active != nil && state.LatestVersion != "" && semver.Compare("v"+state.LatestVersion, "v"+state.Active.Version) > 0 {
		state.Status, state.Reason = "needs_review", "newer_release_pending"
	}
	value, err := json.Marshal(state)
	if err != nil {
		return
	}
	if _, err := decodeClientProfileUpdate(family, string(value)); err != nil {
		slog.Warn("client_profile_sync_invalid_result", "family", family)
		return
	}
	// Persist the publication and its freshness/validator together. A concurrent
	// rollback, hold or newer publication wins instead of being overwritten.
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	won, err := cas.CompareAndSwapSetting(persistCtx, clientProfileUpdateKey(family), expected, string(value))
	if err != nil {
		slog.Warn("client_profile_sync_persist_failed", "family", family)
		return
	}
	if won {
		s.settingService.rememberClientProfileUpdate(family, state)
		slog.Info("client_profile_sync_checked", "family", family, "status", state.Status, "latest", state.LatestVersion)
	}
}

// The control plane has only explicit commands. Rollback also holds the version
// so the next scheduled check cannot immediately undo the operator's action.
func (s *SettingService) SetClientProfileUpdatePolicy(ctx context.Context, family, action string) error {
	if _, err := clientprofile.ClientReleaseContract(family); err != nil {
		return infraerrors.BadRequest("CLIENT_PROFILE_FAMILY_INVALID", "unknown client family")
	}
	if s == nil {
		return infraerrors.ServiceUnavailable("CLIENT_PROFILE_UPDATE_UNAVAILABLE", "client update storage unavailable")
	}
	cas, ok := s.settingRepo.(ClientProfileSettingCAS)
	if !ok {
		return infraerrors.ServiceUnavailable("CLIENT_PROFILE_UPDATE_UNAVAILABLE", "atomic client update storage unavailable")
	}
	state, expected, err := readClientProfileUpdate(ctx, s.settingRepo, family)
	if err != nil {
		return err
	}
	switch action {
	case "hold":
		state.Mode, state.Status = "hold", "held"
	case "resume":
		state.Mode, state.Status, state.ETag, state.CheckedAt = "auto", "pending", "", time.Time{}
	case "rollback":
		if state.Previous == nil {
			return infraerrors.BadRequest("CLIENT_PROFILE_ROLLBACK_UNAVAILABLE", "no previous compatible release")
		}
		state.Active, state.Previous = state.Previous, state.Active
		state.Mode, state.Status = "hold", "held"
	default:
		return infraerrors.BadRequest("CLIENT_PROFILE_ACTION_INVALID", "expected hold, resume or rollback")
	}
	state.Reason = ""
	value, err := json.Marshal(state)
	if err != nil {
		return err
	}
	won, err := cas.CompareAndSwapSetting(ctx, clientProfileUpdateKey(family), expected, string(value))
	if err != nil {
		return err
	}
	if !won {
		return infraerrors.Conflict("CLIENT_PROFILE_UPDATE_CONFLICT", "client update changed; retry the command")
	}
	s.rememberClientProfileUpdate(family, state)
	return nil
}

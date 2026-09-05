package service

import (
	_ "embed"
	"encoding/json"
	"errors"
)

// Frozen from source 670b1844f7334bd4e09dfa28d6f6146c5b8c45cb. Keep this artifact
// when adding future catalogs: pre-snapshot Redis records must not follow latest.
//
//go:embed openai_codex_legacy_profiles.json
var codexLegacyProfilesJSON []byte

func resolveLegacyCodexConversationProfile(id string) (CodexClientProfile, error) {
	var profiles []CodexClientProfile
	if err := json.Unmarshal(codexLegacyProfilesJSON, &profiles); err != nil {
		return CodexClientProfile{}, err
	}
	for _, profile := range profiles {
		if profile.ID == id {
			if err := ValidateCodexClientProfile(profile); err != nil {
				return CodexClientProfile{}, err
			}
			return profile, nil
		}
	}
	return CodexClientProfile{}, errors.New("legacy codex conversation profile is unavailable")
}

package service

import (
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/pkg/clientprofile"
)

const (
	CodexProfilePiManaged       = "pi-managed"
	CodexProfileOpenCodeManaged = "opencode-managed"
)

func isManagedClientProfile(id string) bool {
	return id == CodexProfilePiManaged || id == CodexProfileOpenCodeManaged
}

func resolveManagedClientProfile(id string, release *clientprofile.CompatibleRelease) (CodexClientProfile, error) {
	family := "pi"
	if id == CodexProfileOpenCodeManaged {
		family = "opencode"
	} else if id != CodexProfilePiManaged {
		return CodexClientProfile{}, errors.New("unknown managed client profile")
	}
	contract, _ := clientprofile.ClientReleaseContract(family)
	profile, err := resolveCodexBundleProfile(contract.BundleID)
	if err != nil {
		return profile, err
	}
	profile.ID = id
	if release != nil {
		if release.Family != family {
			return profile, errors.New("client release family mismatch")
		}
		ua, err := release.UserAgent()
		if err != nil {
			return profile, err
		}
		copy := *release
		profile.ClientRelease = &copy
		profile.App.UserAgent = ua
	}
	return profile, nil
}

func managedClientProfileVersion(profile CodexClientProfile) string {
	if profile.ClientRelease != nil {
		return profile.ClientRelease.Version
	}
	contract, err := clientprofile.ClientReleaseContract(codexClientFamily(profile.ID))
	if err != nil {
		return ""
	}
	return contract.BaselineVersion
}

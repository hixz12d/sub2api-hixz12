package clientprofile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"golang.org/x/mod/semver"
)

// ReleaseContract is a conservative source-compatibility boundary, not a claim
// about native TLS, installed binaries, or transitive dependency lockfiles.
type ReleaseContract struct {
	Family           string
	Repository       string
	BaselineVersion  string
	BaselineCommit   string
	BundleID         string
	Manifest         string
	Trees            map[string]string
	DependencyDigest string
}

func ClientReleaseContract(family string) (ReleaseContract, error) {
	switch family {
	case "pi":
		return ReleaseContract{
			Family: family, Repository: "earendil-works/pi", BaselineVersion: "0.57.1",
			BaselineCommit: "a9cedccdde77e9d765303463d8a6cd11c58f7a7f",
			BundleID:       "pi-0.57.1-oauth-sse-r1", Manifest: "packages/ai/package.json",
			Trees:            map[string]string{"packages/ai/src": "b41f1d3fc6d82eb3b83b1587731944b1e8685b47"},
			DependencyDigest: "4a50398e51082d3f32d82ebc33f310dbe9f0be8b115bcfc222afa9e378f1466c",
		}, nil
	case "opencode":
		return ReleaseContract{
			Family: family, Repository: "anomalyco/opencode", BaselineVersion: "1.2.4",
			BaselineCommit: "d1482e148399bfaf808674549199f5f4aa69a22d",
			BundleID:       "opencode-1.2.4-oauth-sse-r1", Manifest: "packages/opencode/package.json",
			Trees: map[string]string{
				"packages/opencode/src/provider":     "581b47eb08b4fd0bec8f0df8a6a632766242592a",
				"packages/opencode/src/plugin":       "4bbbd44c73bea60db8c717557241a1b86c264a01",
				"packages/opencode/src/installation": "a26511dc0c802ab5f72ca370ce3af67fcee520c8",
				"packages/opencode/src/util":         "fd14c4bc287f824ecbe2f614e3a3e62651f46e44",
				"packages/plugin/src":                "efab7cfb9d40a53c5d87fc998f77b3f2213e3941",
				"packages/sdk/js/src":                "39ad2de1e286ccfbfc98114ce015eee4a37f484a",
				"packages/util/src":                  "ad68bbae718994b5a481ae522f9aa6f9e489deac",
				"packages/script/src":                "598d95c46f80efade43bb3bd908db5bb0fca7823",
			},
			DependencyDigest: "63786a34dac23cd637bdcc5e272569f06f0d205093290e6b4a5cb3a25423a52c",
		}, nil
	default:
		return ReleaseContract{}, errors.New("unsupported client family")
	}
}

func (c ReleaseContract) Digest() string {
	data, _ := json.Marshal(c)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func StableClientVersion(version string) bool {
	v := "v" + version
	return len(version) <= 32 && semver.IsValid(v) && semver.Canonical(v) == v &&
		semver.Prerelease(v) == "" && semver.Build(v) == ""
}

func ValidGitObjectID(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 20 && value == strings.ToLower(value)
}

// CompatibleRelease is persisted with each conversation. It never contains
// executable adapters or caller-controlled headers; those come from the bundle.
type CompatibleRelease struct {
	Family         string `json:"family"`
	Version        string `json:"version"`
	Tag            string `json:"tag"`
	Commit         string `json:"commit"`
	ContractDigest string `json:"contract_digest"`
}

func (r CompatibleRelease) Validate() error {
	contract, err := ClientReleaseContract(r.Family)
	if err != nil {
		return err
	}
	if !StableClientVersion(r.Version) || r.Tag != "v"+r.Version || !ValidGitObjectID(r.Commit) ||
		r.ContractDigest != contract.Digest() || semver.Compare("v"+r.Version, "v"+contract.BaselineVersion) < 0 {
		return errors.New("invalid compatible client release")
	}
	return nil
}

func (r CompatibleRelease) UserAgent() (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	contract, _ := ClientReleaseContract(r.Family)
	bundle, _, err := LoadCandidate(contract.BundleID)
	if err != nil {
		return "", err
	}
	if r.Family == "opencode" {
		return strings.Replace(bundle.UserAgent, "opencode/"+contract.BaselineVersion+" ", "opencode/"+r.Version+" ", 1), nil
	}
	return bundle.UserAgent, nil
}

// Compare declarations structurally, ignoring unrelated metadata and package
// version bumps. Bun's workspace catalog is included without parsing its lockfile.
func ClientDependencyDigest(manifest, root []byte) (string, error) {
	result := make(map[string]map[string]any)
	for name, data := range map[string][]byte{"package": manifest, "root": root} {
		if len(data) > 128*1024 || validateJSON(data, 16) != nil {
			return "", errors.New("invalid client package manifest")
		}
		var object map[string]any
		if err := json.Unmarshal(data, &object); err != nil || object == nil {
			return "", errors.New("invalid client package manifest")
		}
		selected := make(map[string]any)
		for _, key := range []string{"dependencies", "optionalDependencies", "peerDependencies", "overrides", "resolutions", "catalog", "workspaces"} {
			if value, exists := object[key]; exists {
				selected[key] = value
			}
		}
		result[name] = selected
	}
	data, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

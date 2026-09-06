package clientprofile

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
)

const RelayContractID = "relay-profile-contract/1"
const RelayCatalogSHA256 = "cbad840e3d9c536336a74059962c12d9e88ec91610e86d84ebae5c79223fe4db"

// The reviewed checkout is the trust root. No runtime paths or remote manifests.
//
//go:embed relaycontract/catalog.json
var relayCatalogBytes []byte

type RelayDeviceTuple struct {
	Platform string `json:"platform"`
	Release  string `json:"release"`
	Arch     string `json:"arch"`
}
type RelayValidation struct {
	Application         string `json:"application_contract"`
	CrossImplementation string `json:"cross_implementation_http"`
	Native              string `json:"native_tls_h2"`
	Upstream            string `json:"authorized_upstream"`
}
type RelayDescriptor struct {
	Adapter          string             `json:"adapter_implementation_revision"`
	AgentName        string             `json:"agent_name"`
	AppRevision      int                `json:"app_identity_revision"`
	BundleDigest     string             `json:"bundle_digest"`
	Distribution     string             `json:"distribution"`
	Family           string             `json:"family"`
	Operations       []string           `json:"operations"`
	ID               string             `json:"profile_id"`
	Runtime          string             `json:"runtime"`
	RuntimeVersion   string             `json:"runtime_version"`
	Sandbox          string             `json:"sandbox"`
	SandboxMode      string             `json:"sandbox_mode"`
	SchemaVersion    int                `json:"schema_version"`
	Selector         string             `json:"selector"`
	SourceCommit     string             `json:"source_commit"`
	SourceRepository string             `json:"source_repository"`
	Devices          []RelayDeviceTuple `json:"supported_tuples"`
	Release          string             `json:"upstream_release"`
	Validation       RelayValidation    `json:"validation"`
	Variant          string             `json:"variant"`
	WireRevision     string             `json:"wire_recipe_revision"`
}
type RelayCatalogEntry struct {
	Digest     string          `json:"artifact_digest"`
	Descriptor RelayDescriptor `json:"descriptor"`
}
type RelayCatalog struct {
	Compact             string              `json:"compact"`
	Contract            string              `json:"contract"`
	CrossImplementation string              `json:"cross_implementation_v2"`
	Defaults            map[string]string   `json:"defaults"`
	PersonaReaders      []int               `json:"persona_reader_versions"`
	PersonaWriter       int                 `json:"persona_writer_version"`
	Profiles            []RelayCatalogEntry `json:"profiles"`
	ReleaseChannel      string              `json:"release_channel"`
	RemoteLoading       string              `json:"remote_manifest_loading"`
	SchemaReaders       []int               `json:"schema_reader_versions"`
}

func LoadRelayCatalog() (RelayCatalog, error) { return decodeReviewedRelayCatalog(relayCatalogBytes) }

func decodeReviewedRelayCatalog(data []byte) (RelayCatalog, error) {
	var catalog RelayCatalog
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != RelayCatalogSHA256 {
		return catalog, errors.New("unapproved relay catalog digest")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&catalog); err != nil {
		return RelayCatalog{}, errors.New("invalid relay catalog schema")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return RelayCatalog{}, errors.New("trailing relay catalog content")
	}
	if catalog.Contract != RelayContractID || catalog.RemoteLoading != "disabled" || len(catalog.Profiles) != 5 {
		return RelayCatalog{}, errors.New("unsupported relay contract")
	}
	seen := map[string]bool{}
	for _, entry := range catalog.Profiles {
		d := entry.Descriptor
		if d.SchemaVersion != 1 || d.Adapter != "relay-adapter-1" || seen[d.ID] || catalog.Defaults[d.Selector] != d.ID {
			return RelayCatalog{}, errors.New("unsupported relay descriptor")
		}
		seen[d.ID] = true
		// Python hashes sorted, compact ASCII JSON. These reviewed descriptors are ASCII.
		encoded, _ := json.Marshal(d)
		var object map[string]any
		if err := json.Unmarshal(encoded, &object); err != nil {
			return RelayCatalog{}, err
		}
		encoded, err := json.Marshal(object)
		if err != nil {
			return RelayCatalog{}, err
		}
		digest := sha256.Sum256(encoded)
		if hex.EncodeToString(digest[:]) != entry.Digest {
			return RelayCatalog{}, errors.New("relay descriptor digest mismatch")
		}
		if d.Variant == "shared-r1" {
			_, bundleDigest, err := LoadCandidate(d.Selector)
			if err != nil || bundleDigest != d.BundleDigest {
				return RelayCatalog{}, errors.New("relay shared artifact mismatch")
			}
		}
	}
	return catalog, nil
}

package clientprofile

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReviewedRelayContract(t *testing.T) {
	catalog, err := LoadRelayCatalog()
	require.NoError(t, err)
	require.Equal(t, RelayContractID, catalog.Contract)
	require.Len(t, catalog.Profiles, 5)
	require.Equal(t, "not_adapted", catalog.CrossImplementation)
	require.Equal(t, "codex-0.153.4-exec-f0-r1", catalog.Defaults["cli"])
	for _, profile := range catalog.Profiles {
		require.Equal(t, "untested", profile.Descriptor.Validation.Native)
		require.Equal(t, "untested", profile.Descriptor.Validation.Upstream)
	}
	catalog.Profiles[0].Descriptor.Devices[0].Platform = "changed"
	fresh, err := LoadRelayCatalog()
	require.NoError(t, err)
	require.Equal(t, "win32", fresh.Profiles[0].Descriptor.Devices[0].Platform)
}

func TestReviewedRelayContractRejectsTamperingAndUnknownSchema(t *testing.T) {
	for _, data := range [][]byte{
		append(append([]byte{}, relayCatalogBytes...), '\n'),
		bytes.Replace(relayCatalogBytes, []byte(`"schema_version": 1`), []byte(`"schema_version": 9`), 1),
		bytes.Replace(relayCatalogBytes, []byte(`"remote_manifest_loading": "disabled"`), []byte(`"remote_manifest_loading": "enabled"`), 1),
	} {
		catalog, err := decodeReviewedRelayCatalog(data)
		require.Error(t, err)
		require.Empty(t, catalog.Profiles)
	}
}

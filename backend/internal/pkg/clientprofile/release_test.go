package clientprofile

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClientReleaseVersionAndIdentity(t *testing.T) {
	for _, version := range []string{"", "v1.2.3", "1", "1.2", "01.2.3", "1.2.3-beta", "1.2.3+build", "1.2.3\r\n"} {
		require.False(t, StableClientVersion(version), version)
	}
	for _, family := range []string{"pi", "opencode"} {
		contract, err := ClientReleaseContract(family)
		require.NoError(t, err)
		release := CompatibleRelease{Family: family, Version: "9.2.3", Tag: "v9.2.3", Commit: contract.BaselineCommit, ContractDigest: contract.Digest()}
		ua, err := release.UserAgent()
		require.NoError(t, err)
		if family == "pi" {
			require.Equal(t, "pi (win32 10.0.26200; x64)", ua)
		} else {
			require.Equal(t, "opencode/9.2.3 (win32 10.0.26200; x64)", ua)
		}
		release.ContractDigest = "unreviewed"
		require.Error(t, release.Validate())
	}
}

func TestClientDependencyDigestIsStructural(t *testing.T) {
	first, err := ClientDependencyDigest([]byte(`{"version":"1.2.3","dependencies":{"b":"2","a":"1"}}`), []byte(`{}`))
	require.NoError(t, err)
	second, err := ClientDependencyDigest([]byte(`{"dependencies":{"a":"1","b":"2"},"version":"1.2.4"}`), []byte(`{}`))
	require.NoError(t, err)
	require.Equal(t, first, second)
	changed, err := ClientDependencyDigest([]byte(`{"dependencies":{"a":"2","b":"2"}}`), []byte(`{}`))
	require.NoError(t, err)
	require.NotEqual(t, first, changed)
	_, err = ClientDependencyDigest([]byte(`{"dependencies":{},"dependencies":{}}`), []byte(`{}`))
	require.Error(t, err)
}

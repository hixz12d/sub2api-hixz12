package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPublicCodexClientCatalogUsesExecutionProfiles(t *testing.T) {
	catalog, err := PublicCodexClientCatalog()
	require.NoError(t, err)
	require.Len(t, catalog.RelayReferences, 5)
	require.False(t, catalog.NewManagedDefaultEnabled)
	seen := map[string]bool{}
	for _, item := range catalog.Profiles {
		require.False(t, seen[item.ID], item.ID)
		seen[item.ID] = true
		if item.ID == CodexProfileCLI {
			require.Equal(t, "0.148.0", item.AppVersion)
		}
		if item.ID == CodexProfilePiBundle {
			require.Equal(t, "0.57.1", item.AppVersion)
			require.NotEmpty(t, item.RelayDigest)
		}
		require.Equal(t, "untested", item.NativeValidation)
	}
	require.True(t, seen[CodexProfileAuto])
	data, err := json.Marshal(catalog)
	require.NoError(t, err)
	for _, secret := range []string{"access_token", "refresh_token", "device_id", "session_id", "window_id"} {
		require.NotContains(t, string(data), `"`+secret+`"`)
	}
}

func TestCodexClientProfilePreviewConflicts(t *testing.T) {
	catalog, err := PublicCodexClientCatalog()
	require.NoError(t, err)
	for _, tc := range []struct {
		name   string
		change func(*CodexProfilePreviewInput)
		plugin string
		valid  bool
	}{
		{name: "native bundle", plugin: "builtin", valid: true},
		{name: "TLS conflict", change: func(i *CodexProfilePreviewInput) { i.Extra["enable_tls_fingerprint"] = true }},
		{name: "TLS malformed", change: func(i *CodexProfilePreviewInput) { i.Extra["enable_tls_fingerprint"] = "false" }},
		{name: "TLS missing", change: func(i *CodexProfilePreviewInput) { delete(i.Extra, "enable_tls_fingerprint") }},
		{name: "plugin conflict", plugin: "routed"},
		{name: "WebSocket unsupported", change: func(i *CodexProfilePreviewInput) { i.Transport = CodexTransportWS }},
		{name: "Compact unsupported", change: func(i *CodexProfilePreviewInput) { i.Operation = CodexOperationCompact }},
		{name: "unknown operation", change: func(i *CodexProfilePreviewInput) { i.Operation = "unknown" }},
		{name: "unknown catalog", change: func(i *CodexProfilePreviewInput) { i.CatalogRevision = "old" }},
		{name: "unknown selector", change: func(i *CodexProfilePreviewInput) { i.Extra[CodexClientProfileExtraKey] = "future-client" }},
		{name: "private field", change: func(i *CodexProfilePreviewInput) { i.Extra["device_id"] = "must-not-appear" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := CodexProfilePreviewInput{Platform: PlatformOpenAI, Type: AccountTypeOAuth,
				Operation: CodexOperationResponses, Transport: CodexTransportHTTP, CatalogRevision: catalog.Revision,
				Extra: map[string]any{CodexRelayModeExtraKey: "relay_kernel", CodexIdentityPolicyVersionExtraKey: "v2", CodexClientProfileExtraKey: CodexProfilePiBundle, codexFingerprintModeExtraKey: "device", "enable_tls_fingerprint": false}}
			if tc.change != nil {
				tc.change(&input)
			}
			before, err := json.Marshal(input)
			require.NoError(t, err)
			preview := PreviewCodexClientProfile(input, tc.plugin)
			require.Equal(t, tc.valid, preview.Valid, preview.Conflicts)
			require.Equal(t, "configuration-only-no-upstream", preview.Scope)
			after, err := json.Marshal(input)
			require.NoError(t, err)
			require.Equal(t, before, after)
			output, err := json.Marshal(preview)
			require.NoError(t, err)
			require.NotContains(t, string(output), "must-not-appear")
			if tc.valid {
				require.Equal(t, "native-default", preview.Transport.TLSRecipe)
			}
		})
	}
}

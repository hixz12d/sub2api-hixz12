package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateCodexRelayAccountExtraRejectsNonStringSelectors(t *testing.T) {
	for _, key := range []string{CodexRelayModeExtraKey, CodexIdentityPolicyVersionExtraKey, CodexClientProfileExtraKey} {
		t.Run(key, func(t *testing.T) {
			for _, value := range []any{nil, true, 123, []any{"pi"}, map[string]any{"value": "pi"}} {
				extra := map[string]any{key: value}
				require.True(t, hasCodexRelayAccountExtraUpdate(extra))
				err := ValidateCodexRelayAccountExtra(PlatformOpenAI, AccountTypeOAuth, extra, testCodexRelaySecret)
				require.ErrorContains(t, err, key+" must be a string")
			}
			require.NoError(t, ValidateCodexRelayAccountExtra(PlatformOpenAI, AccountTypeOAuth, map[string]any{key: ""}, testCodexRelaySecret))
		})
	}
}

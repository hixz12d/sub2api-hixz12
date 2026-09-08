//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBlueprintV2OpenAIProtocolCredentialSelection(t *testing.T) {
	svc := &OpenAIGatewayService{}
	for _, platform := range []string{PlatformOpenAI, PlatformKimi, PlatformZhipu, PlatformDeepseek, PlatformGrok} {
		t.Run(platform, func(t *testing.T) {
			account := &Account{
				Platform:    platform,
				Type:        AccountTypeAPIKey,
				Credentials: map[string]any{"api_key": "sk-test-protocol-credential"},
			}
			token, kind, err := svc.GetAccessToken(context.Background(), account)
			require.NoError(t, err)
			require.Equal(t, "sk-test-protocol-credential", token)
			require.Equal(t, "apikey", kind)
			// Credential compatibility must not broaden platform-specific gates.
			require.Equal(t, platform == PlatformOpenAI, account.IsOpenAIApiKey())

			delete(account.Credentials, "api_key")
			_, _, err = svc.GetAccessToken(context.Background(), account)
			require.ErrorContains(t, err, "api_key not found")
		})
	}
	t.Run("unrelated_platform_rejected", func(t *testing.T) {
		account := &Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-test"}}
		_, _, err := svc.GetAccessToken(context.Background(), account)
		require.ErrorContains(t, err, "api_key not found")
	})
}

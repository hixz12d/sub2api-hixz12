package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsTLSFingerprintEnabledOpenAIDefaultsOn(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	assert.True(t, account.SupportsTLSFingerprint())
	assert.True(t, account.IsTLSFingerprintEnabled(), "OpenAI OAuth 默认开启 Chrome TLS")

	account.Extra = map[string]any{"enable_tls_fingerprint": false}
	assert.False(t, account.IsTLSFingerprintEnabled())

	account.Extra = map[string]any{"enable_tls_fingerprint": true}
	assert.True(t, account.IsTLSFingerprintEnabled())
}

func TestIsTLSFingerprintEnabledAnthropicRemainsOptIn(t *testing.T) {
	account := &Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	assert.True(t, account.SupportsTLSFingerprint())
	assert.False(t, account.IsTLSFingerprintEnabled(), "Anthropic 仍需显式打开")

	account.Extra = map[string]any{"enable_tls_fingerprint": true}
	assert.True(t, account.IsTLSFingerprintEnabled())
}

func TestIsTLSFingerprintEnabledIgnoresAPIKeys(t *testing.T) {
	assert.False(t, (&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}).SupportsTLSFingerprint())
	assert.False(t, (&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}).IsTLSFingerprintEnabled())
	assert.False(t, (&Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey}).IsTLSFingerprintEnabled())
}

func TestResolveTLSProfileOpenAIUsesChromeAuto(t *testing.T) {
	svc := &TLSFingerprintProfileService{}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	profile := svc.ResolveTLSProfile(account)
	require.NotNil(t, profile)
	assert.True(t, profile.UsesChromeAuto())
	assert.Equal(t, tlsfingerprint.HelloPresetChromeAuto, profile.HelloPreset)
}

func TestResolveTLSProfileAnthropicUsesNodeDefault(t *testing.T) {
	svc := &TLSFingerprintProfileService{}
	account := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{"enable_tls_fingerprint": true},
	}
	profile := svc.ResolveTLSProfile(account)
	require.NotNil(t, profile)
	assert.False(t, profile.UsesChromeAuto())
	assert.Equal(t, "Built-in Default (Node.js 24.x)", profile.Name)
}

func TestResolveTLSProfileDisabledReturnsNil(t *testing.T) {
	svc := &TLSFingerprintProfileService{}
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{"enable_tls_fingerprint": false},
	}
	assert.Nil(t, svc.ResolveTLSProfile(account))
}

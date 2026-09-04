package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpenAICodexTransportScopeIsolatesAccountAndCredentialPool(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.ConnectionPoolIsolation = config.ConnectionPoolIsolationProxy
	upstream := NewHTTPUpstream(cfg).(*httpUpstreamService)

	first, err := upstream.getClientEntryWithScope("", 1001, 2, service.HTTPUpstreamProfileOpenAI, "credential-a-profile-cli", false, false)
	require.NoError(t, err)
	same, err := upstream.getClientEntryWithScope("", 1001, 2, service.HTTPUpstreamProfileOpenAI, "credential-a-profile-cli", false, false)
	require.NoError(t, err)
	require.Same(t, first, same)

	otherAccount, err := upstream.getClientEntryWithScope("", 1002, 2, service.HTTPUpstreamProfileOpenAI, "credential-a-profile-cli", false, false)
	require.NoError(t, err)
	require.NotSame(t, first, otherAccount, "OpenAI pools must be account isolated even when the global mode is proxy")

	rotatedCredential, err := upstream.getClientEntryWithScope("", 1001, 2, service.HTTPUpstreamProfileOpenAI, "credential-b-profile-cli", false, false)
	require.NoError(t, err)
	require.NotSame(t, first, rotatedCredential)
}

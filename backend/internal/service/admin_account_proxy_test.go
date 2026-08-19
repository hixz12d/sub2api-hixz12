package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type createAccountProxyRepoStub struct {
	ProxyRepository
}

func (createAccountProxyRepoStub) ListActive(context.Context) ([]Proxy, error) {
	panic("unexpected ListActive call")
}

func TestCreateAccountDoesNotApplyConfiguredDefaultProxy(t *testing.T) {
	t.Setenv("ACCOUNT_IMPORT_DEFAULT_PROXY_ID", "25")

	repo := &upstreamBillingProbeAccountRepo{}
	svc := &adminServiceImpl{
		accountRepo: repo,
		proxyRepo:   &createAccountProxyRepoStub{},
	}

	created, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name:                 "without-default-proxy",
		Platform:             PlatformOpenAI,
		Type:                 AccountTypeAPIKey,
		Credentials:          map[string]any{"api_key": "sk-test"},
		SkipDefaultGroupBind: true,
	})

	require.NoError(t, err)
	require.Nil(t, created.ProxyID)

	explicitProxyID := int64(7)
	created, err = svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name:                 "with-explicit-proxy",
		Platform:             PlatformOpenAI,
		Type:                 AccountTypeAPIKey,
		Credentials:          map[string]any{"api_key": "sk-test-2"},
		ProxyID:              &explicitProxyID,
		SkipDefaultGroupBind: true,
	})

	require.NoError(t, err)
	require.NotNil(t, created.ProxyID)
	require.Equal(t, explicitProxyID, *created.ProxyID)
}

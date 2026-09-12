package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOAuthSyncAccessTokenReadbackIsConditionalAndNarrow(t *testing.T) {
	svc, _, accounts, _ := durableSyncFixture()
	account := accounts.account
	account.Credentials["_token_version"] = int64(7)
	account.Credentials["client_id"] = "fixture-client"
	account.Credentials["access_token"] = "fixture-AT-readback"
	account.Credentials["refresh_token"] = "fixture-RT-private"
	account.Credentials["id_token"] = "fixture-ID-private"
	stamp := account.UpdatedAt.Format(time.RFC3339Nano)
	for _, test := range []struct {
		instance string
		version  int64
		stamp    string
	}{
		{"other-instance", 7, stamp}, {"fixture-instance", 6, stamp}, {"fixture-instance", 7, "invalid"},
	} {
		result, err := svc.ReadOAuthSyncAccessToken(context.Background(), 42, test.instance, test.version, test.stamp)
		require.Error(t, err)
		require.Nil(t, result)
	}
	result, err := svc.ReadOAuthSyncAccessToken(context.Background(), 42, "fixture-instance", 7, stamp)
	require.NoError(t, err)
	require.Equal(t, "fixture-AT-readback", result["access_token"])
	raw, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "fixture-RT-private")
	require.NotContains(t, string(raw), "fixture-ID-private")
	require.Zero(t, accounts.writes)
	require.False(t, account.Schedulable)
	require.Equal(t, StatusError, account.Status)
	cache, ok := svc.invalidator.(*durableSyncCache)
	require.True(t, ok)
	require.Zero(t, cache.calls)
}

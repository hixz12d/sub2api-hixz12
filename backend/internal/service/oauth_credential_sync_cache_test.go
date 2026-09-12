//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOAuthCredentialSyncStrictCacheFailure(t *testing.T) {
	failure := errors.New("fixture cache unavailable")
	cache := &geminiTokenCacheStub{deleteErr: failure}
	invalidator := NewCompositeTokenCacheInvalidator(cache)
	account := &Account{ID: 42, Platform: PlatformGemini, Type: AccountTypeOAuth, Credentials: map[string]any{"project_id": "fixture"}}
	require.ErrorIs(t, invalidator.InvalidateTokenStrict(context.Background(), account), failure)
	require.Len(t, cache.deletedKeys, 2, "attempt all keys even after a failure")
	require.NoError(t, invalidator.InvalidateToken(context.Background(), account), "legacy callers remain best effort")
	require.Error(t, NewCompositeTokenCacheInvalidator(nil).InvalidateTokenStrict(context.Background(), account))
}

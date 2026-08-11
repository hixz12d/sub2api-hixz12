package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newOpenAISessionIsolationContext(t *testing.T, groupID, userID, apiKeyID int64, sessionID string) *gin.Context {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("session_id", sessionID)
	c.Set("api_key", &APIKey{ID: apiKeyID, UserID: userID, GroupID: &groupID})
	return c
}

func TestOpenAISessionHashIncludesTenantScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{}

	ownerA1 := newOpenAISessionIsolationContext(t, 7, 101, 1001, "shared-session")
	ownerA2 := newOpenAISessionIsolationContext(t, 7, 101, 1001, "shared-session")
	otherKey := newOpenAISessionIsolationContext(t, 7, 101, 1002, "shared-session")
	otherUser := newOpenAISessionIsolationContext(t, 7, 102, 1001, "shared-session")

	hashA1 := svc.GenerateSessionHash(ownerA1, nil)
	hashA2 := svc.GenerateSessionHash(ownerA2, nil)
	require.NotEmpty(t, hashA1)
	require.Equal(t, hashA1, hashA2)
	require.NotEqual(t, hashA1, svc.GenerateSessionHash(otherKey, nil))
	require.NotEqual(t, hashA1, svc.GenerateSessionHash(otherUser, nil))
	require.Contains(t, hashA1, "oas-session-v2:")

	requestOwner, ok := openAIWSStateOwnerForRequest(context.Background(), ownerA1, 501, hashA1)
	require.True(t, ok)
	require.Equal(t, int64(501), requestOwner.AccountID)
	require.Equal(t, hashA1, requestOwner.SessionScopeHash)
}

func TestOpenAIWSStateStoreOwnedBindingsFailClosedAcrossOwners(t *testing.T) {
	store := NewOpenAIWSStateStore(nil)
	ctx := context.Background()
	ownerA := openAIWSStateOwner{GroupID: 7, UserID: 101, APIKeyID: 1001, AccountID: 501, SessionScopeHash: "scope-a"}
	ownerB := openAIWSStateOwner{GroupID: 7, UserID: 102, APIKeyID: 1002, AccountID: 501, SessionScopeHash: "scope-a"}

	require.NoError(t, store.BindResponseAccountOwned(ctx, 7, "resp-owned", ownerA, ownerA.AccountID, time.Minute))
	accountID, err := store.GetResponseAccountOwned(ctx, 7, "resp-owned", ownerA)
	require.NoError(t, err)
	require.Equal(t, ownerA.AccountID, accountID)
	accountID, err = store.GetResponseAccountOwned(ctx, 7, "resp-owned", ownerB)
	require.NoError(t, err)
	require.Zero(t, accountID)

	store.BindResponseConnOwned("resp-owned", ownerA, "conn-a", time.Minute)
	connID, ok := store.GetResponseConnOwned("resp-owned", ownerA)
	require.True(t, ok)
	require.Equal(t, "conn-a", connID)
	_, ok = store.GetResponseConnOwned("resp-owned", ownerB)
	require.False(t, ok)

	otherScope := ownerA
	otherScope.SessionScopeHash = "scope-b"
	_, ok = store.GetResponseConnOwned("resp-owned", otherScope)
	require.False(t, ok)

	failoverOwner := ownerA
	failoverOwner.AccountID = 502
	_, ok = store.GetResponseConnOwned("resp-owned", failoverOwner)
	require.False(t, ok, "failover must not carry the previous credential owner's connection")
}

func TestOpenAIWSStateStoreOwnedLookupRejectsLegacyBinding(t *testing.T) {
	store := NewOpenAIWSStateStore(nil)
	ctx := context.Background()
	owner := openAIWSStateOwner{GroupID: 7, UserID: 101, APIKeyID: 1001, AccountID: 501, SessionScopeHash: "scope-a"}

	require.NoError(t, store.BindResponseAccount(ctx, 7, "resp-legacy", owner.AccountID, time.Minute))
	store.BindResponseConn("resp-legacy", "conn-legacy", time.Minute)

	accountID, err := store.GetResponseAccountOwned(ctx, 7, "resp-legacy", owner)
	require.NoError(t, err)
	require.Zero(t, accountID)
	_, ok := store.GetResponseConnOwned("resp-legacy", owner)
	require.False(t, ok)
}

func TestOpenAIWSConnPoolReusesOnlyMatchingSessionScope(t *testing.T) {
	pool := newOpenAIWSConnPool(&config.Config{})
	pool.setClientDialerForTest(&openAIWSCountingDialer{})
	account := &Account{ID: 8801, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	base := openAIWSAcquireRequest{
		Account:          account,
		WSURL:            "wss://example.com/v1/responses",
		SessionScopeHash: "scope-a",
	}

	first, err := pool.Acquire(context.Background(), base)
	require.NoError(t, err)
	firstConnID := first.ConnID()
	first.Release()

	sameScope, err := pool.Acquire(context.Background(), base)
	require.NoError(t, err)
	require.Equal(t, firstConnID, sameScope.ConnID())
	require.True(t, sameScope.Reused())
	sameScope.Release()

	otherScopeRequest := base
	otherScopeRequest.SessionScopeHash = "scope-b"
	otherScope, err := pool.Acquire(context.Background(), otherScopeRequest)
	require.NoError(t, err)
	require.NotEqual(t, firstConnID, otherScope.ConnID())
	otherScope.Release()
}

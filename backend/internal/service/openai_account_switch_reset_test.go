package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIAttemptStateTracksLogicalRequestAndAttempt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	state := PrepareOpenAIAttemptState(c, []byte(`{"input":"hello"}`), "session-a", "resp-a", "cache-a")
	require.NotNil(t, state)
	require.Equal(t, "session-a", state.SessionHash)
	require.Equal(t, "resp-a", state.PreviousResponseID)
	require.True(t, state.ReplaySafe)

	BeginOpenAIAttempt(c, 11, []byte(`{"input":"hello"}`))
	BeginOpenAIAttempt(c, 11, []byte(`{"input":"hello"}`))
	require.Equal(t, 1, OpenAIAttemptStateSnapshot(c).Attempt)
	require.Equal(t, int64(11), OpenAIAttemptStateSnapshot(c).CurrentAccountID)

	BeginOpenAIAttempt(c, 12, []byte(`{"input":"hello"}`))
	snapshot := OpenAIAttemptStateSnapshot(c)
	require.Equal(t, 2, snapshot.Attempt)
	require.Equal(t, int64(11), snapshot.PreviousAccountID)
	require.Equal(t, int64(12), snapshot.CurrentAccountID)
}

func TestResetForAccountSwitchClearsAllRequestLocalContinuationState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("x-codex-turn-state", "old-turn")
	c.Request.Header.Set("session_id", "old-session")

	PrepareOpenAIAttemptState(c, []byte(`{"input":"hello"}`), "session-a", "resp-a", "cache-a")
	BeginOpenAIAttempt(c, 11, []byte(`{"input":"hello"}`))
	TrackOpenAIResponseID(c, "resp-b")

	svc := &OpenAIGatewayService{}
	store := svc.getOpenAIWSStateStore()
	store.BindResponseAccount(context.Background(), 0, "resp-a", 11, 0)
	store.BindResponseConn("resp-a", "conn-a", 0)
	store.BindSessionTurnState(0, "session-a", "turn-a", 0)
	store.BindSessionConn(0, "session-a", "conn-a", 0)

	require.NoError(t, svc.ResetForAccountSwitch(context.Background(), c, 12))
	snapshot := OpenAIAttemptStateSnapshot(c)
	require.Equal(t, int64(11), snapshot.PreviousAccountID)
	require.Zero(t, snapshot.CurrentAccountID)
	require.Equal(t, 1, snapshot.AccountSwitches)
	require.Equal(t, "", snapshot.TurnState)
	require.Empty(t, snapshot.ResponseIDs)
	require.Empty(t, snapshot.ResponseConnIDs)
	require.Empty(t, c.GetHeader("x-codex-turn-state"))
	require.Empty(t, c.GetHeader("session_id"))

	accountID, err := store.GetResponseAccount(context.Background(), 0, "resp-a")
	require.NoError(t, err)
	require.Zero(t, accountID)
	_, ok := store.GetResponseConn("resp-a")
	require.False(t, ok)
	_, ok = store.GetSessionTurnState(0, "session-a")
	require.False(t, ok)
	_, ok = store.GetSessionConn(0, "session-a")
	require.False(t, ok)
}

package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type codexRecoveryAccountRepo struct {
	AccountRepository
	account *Account
	err     error
}

func (r *codexRecoveryAccountRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, r.err
}

type codexRecoveryConflictRegistry struct {
	codexRegistryGatewayCacheStub
	conflicts int
}

func (r *codexRecoveryConflictRegistry) CompareAndSwapCodexConversation(ctx context.Context, digest string, revision, accountID int64, next CodexConversationState, ttl time.Duration) (CodexConversationState, error) {
	r.conflicts++
	return *r.state, ErrCodexConversationCASConflict
}

func TestCodexCommittedConversationRecoversOnlyUnavailableReplayableAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name        string
		status      string
		schedulable bool
		body        string
		header      bool
		output      bool
		repoErr     error
		sameAccount bool
		recover     bool
	}{
		{name: "disabled", status: StatusError, body: `{"input":"hello"}`, recover: true},
		{name: "unschedulable", status: StatusActive, body: `{"input":"hello"}`, recover: true},
		{name: "healthy", status: StatusActive, schedulable: true, body: `{"input":"hello"}`},
		{name: "previous_response", status: StatusError, body: `{"previous_response_id":"resp_old"}`},
		{name: "tool_result", status: StatusError, body: "{\"input\":[{\"type\" :\n \"function_call_output\"}]}"},
		{name: "encrypted", status: StatusError, body: `{"input":[{"encrypted_content":"secret"}]}`},
		{name: "turn_state", status: StatusError, body: `{"input":"hello"}`, header: true},
		{name: "output_started", status: StatusError, body: `{"input":"hello"}`, output: true},
		{name: "database_failure", status: StatusError, body: `{"input":"hello"}`, repoErr: errors.New("database unavailable")},
		{name: "deleted", body: `{"input":"hello"}`, repoErr: ErrAccountNotFound, recover: true},
		{name: "same_account_transport_changed", status: StatusError, body: `{"input":"hello"}`, sameAccount: true, recover: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Gateway.OpenAIAffinity.Secret = testCodexRelaySecret
			registry := &codexRegistryGatewayCacheStub{}
			repo := &codexRecoveryAccountRepo{account: &Account{ID: 11, Status: tc.status, Schedulable: tc.schedulable}, err: tc.repoErr}
			svc := &OpenAIGatewayService{cfg: cfg, cache: registry, accountRepo: repo}
			extra := map[string]any{CodexRelayModeExtraKey: string(CodexRelayModeKernel), CodexIdentityPolicyVersionExtraKey: CodexIdentityPolicyV2,
				CodexClientProfileExtraKey: CodexProfileCLI, codexFingerprintModeExtraKey: string(codexFingerprintSession)}
			account := newTestOAuthAccount(11, extra)
			account.Credentials = map[string]any{"access_token": "first"}
			plan, err := NewCodexRequestPlan(CodexRequestPlanInput{LogicalRequestID: "old", SessionHash: "conversation", Body: []byte(`{"input":"hello"}`)})
			require.NoError(t, err)
			oldContext, _ := gin.CreateTestContext(httptest.NewRecorder())
			oldContext.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			oldContext.Request = oldContext.Request.WithContext(ContextWithCodexRequestPlan(oldContext.Request.Context(), plan))
			_, err = svc.finalizeCodexOAuthIdentity(account, oldContext, oldContext.Request.Header, "")
			require.NoError(t, err)
			require.NoError(t, svc.CommitCodexConversation(oldContext.Request.Context()))
			original, err := registry.GetCodexConversation(context.Background(), plan.ConversationDigest())
			require.NoError(t, err)

			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			if tc.header {
				c.Request.Header.Set(openAIWSTurnStateHeader, "opaque")
			}
			nextPlan, err := NewCodexRequestPlan(CodexRequestPlanInput{LogicalRequestID: "new", SessionHash: "conversation", Body: []byte(tc.body), InboundHeaders: c.Request.Header})
			require.NoError(t, err)
			c.Request = c.Request.WithContext(ContextWithCodexRequestPlan(c.Request.Context(), nextPlan))
			PrepareOpenAIAttemptState(c, []byte(tc.body), "", "", "")
			if tc.output {
				MarkOpenAISemanticOutputStarted(c)
			}
			next := newTestOAuthAccount(12, extra)
			next.Credentials = map[string]any{"access_token": "second"}
			if tc.sameAccount {
				next.ID = 11
				proxyID := int64(42)
				next.ProxyID = &proxyID
			}
			_, err = svc.finalizeCodexOAuthIdentity(next, c, c.Request.Header, "")
			stored, readErr := registry.GetCodexConversation(context.Background(), plan.ConversationDigest())
			require.NoError(t, readErr)
			if tc.recover {
				require.NoError(t, err)
				if tc.sameAccount {
					require.Equal(t, int64(11), stored.AccountID)
					require.Greater(t, stored.Revision, original.Revision)
					require.Equal(t, original.SessionID, stored.SessionID)
				} else {
					require.Equal(t, int64(12), stored.AccountID)
					require.Greater(t, stored.Revision, original.Revision)
					require.False(t, stored.Committed)
					require.NotEqual(t, original.SessionID, stored.SessionID)
					require.ErrorIs(t, svc.CommitCodexConversation(oldContext.Request.Context()), ErrCodexConversationCASConflict)
				}
				require.NoError(t, svc.CommitCodexConversation(c.Request.Context()))
			} else {
				var failure *UpstreamFailoverError
				require.ErrorAs(t, err, &failure)
				require.Equal(t, OpenAIConversationRecoveryRequiredReason, failure.Reason)
				require.False(t, failure.ShouldRetryNextAccount())
				require.Equal(t, original, stored)
			}
		})
	}
}

func TestCodexConversationRecoveryCASConflictIsBounded(t *testing.T) {
	plan, err := NewCodexRequestPlan(CodexRequestPlanInput{LogicalRequestID: "retry", SessionHash: "session", Body: []byte(`{"input":"hello"}`)})
	require.NoError(t, err)
	input := CodexAttemptInput{AccountID: 12, ProfileID: CodexProfileCLI, FingerprintMode: string(codexFingerprintSession)}
	attempt, err := FinalizeCodexAttempt(plan, input, testCodexRelaySecret)
	require.NoError(t, err)
	state, err := codexConversationStateFromAttempt(plan, attempt, input)
	require.NoError(t, err)
	state.AccountID = 11
	state.Committed = true
	registry := &codexRecoveryConflictRegistry{codexRegistryGatewayCacheStub: codexRegistryGatewayCacheStub{state: &state}}
	svc := &OpenAIGatewayService{cache: registry, accountRepo: &codexRecoveryAccountRepo{account: &Account{ID: 11, Status: StatusError}}}
	_, err = svc.resolveCodexConversationAttempt(context.Background(), plan, attempt, input, true)
	require.ErrorIs(t, err, ErrCodexConversationCASConflict)
	require.Equal(t, 3, registry.conflicts)
	require.Equal(t, int64(11), registry.state.AccountID)
	require.True(t, registry.state.Committed)
}

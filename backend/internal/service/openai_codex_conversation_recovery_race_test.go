package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type codexRecoveryWinnerRegistry struct {
	codexRegistryGatewayCacheStub
	winner CodexConversationState
	calls  int
}

func (r *codexRecoveryWinnerRegistry) CompareAndSwapCodexConversation(ctx context.Context, digest string, revision, accountID int64, next CodexConversationState, ttl time.Duration) (CodexConversationState, error) {
	r.calls++
	if r.calls == 1 {
		r.state = &r.winner
		return r.winner, ErrCodexConversationCASConflict
	}
	return r.codexRegistryGatewayCacheStub.CompareAndSwapCodexConversation(ctx, digest, revision, accountID, next, ttl)
}

func TestCodexConversationRecoveryDoesNotOverwriteHealthyCASWinner(t *testing.T) {
	for _, committed := range []bool{false, true} {
		plan, err := NewCodexRequestPlan(CodexRequestPlanInput{LogicalRequestID: "retry", SessionHash: "session", Body: []byte(`{"input":"hello"}`)})
		require.NoError(t, err)
		input := CodexAttemptInput{AccountID: 12, ProfileID: CodexProfileCLI, FingerprintMode: string(codexFingerprintSession)}
		attempt, err := FinalizeCodexAttempt(plan, input, testCodexRelaySecret)
		require.NoError(t, err)
		original, err := codexConversationStateFromAttempt(plan, attempt, input)
		require.NoError(t, err)
		original.AccountID, original.Committed = 11, true
		winner := original
		winner.AccountID, winner.Revision, winner.Committed = 13, 2, committed
		registry := &codexRecoveryWinnerRegistry{codexRegistryGatewayCacheStub: codexRegistryGatewayCacheStub{state: &original}, winner: winner}
		svc := &OpenAIGatewayService{cache: registry, accountRepo: &stubOpenAIAccountRepo{accounts: []Account{
			{ID: 11, Status: StatusError}, {ID: 13, Status: StatusActive, Schedulable: true},
		}}}
		_, err = svc.resolveCodexConversationAttempt(context.Background(), plan, attempt, input, true)
		var failure *UpstreamFailoverError
		require.ErrorAs(t, err, &failure)
		require.Equal(t, OpenAIConversationRecoveryRequiredReason, failure.Reason)
		require.Equal(t, 1, registry.calls)
		require.Equal(t, winner, *registry.state)
	}
}

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodexPinnedUnavailableAccountFallsBack(t *testing.T) {
	for _, tc := range []struct {
		name        string
		status      string
		schedulable bool
	}{
		{name: "401_account", status: StatusError},
		{name: "disabled_account", status: StatusActive},
	} {
		t.Run(tc.name, func(t *testing.T) {
			accounts := []Account{
				{ID: 101, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: tc.status, Schedulable: tc.schedulable, Concurrency: 10},
				{ID: 202, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 10},
			}
			registry := &codexRegistryGatewayCacheStub{state: &CodexConversationState{AccountID: 101}}
			svc := &OpenAIGatewayService{cache: registry, accountRepo: stubOpenAIAccountRepo{accounts: accounts}}
			plan, err := NewCodexRequestPlan(CodexRequestPlanInput{LogicalRequestID: "request", SessionHash: "session", Transport: CodexTransportHTTP, Body: []byte(`{"input":"hello"}`)})
			require.NoError(t, err)
			ctx := ContextWithCodexRequestPlan(context.Background(), plan)
			scheduler := newDefaultOpenAIAccountScheduler(svc, nil)
			selection, _, err := scheduler.Select(ctx, OpenAIAccountScheduleRequest{Platform: PlatformOpenAI})
			require.NoError(t, err)
			require.Equal(t, int64(202), selection.Account.ID)
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
		})
	}
}

//go:build unit

package service

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestMonitorProbeDeploymentClosed(t *testing.T) {
	require.Nil(t, ProvideMonitorProbeExecutor(nil, nil, nil, nil, nil, nil, nil))
	executor := ProvideMonitorProbeExecutor(&config.Config{}, nil, nil, nil, nil, nil, nil)
	require.Nil(t, executor)
	require.False(t, executor.Ready())
}

func TestMonitorProbeResponseContract(t *testing.T) {
	for _, test := range []struct {
		name, body           string
		chat                 bool
		transport, challenge string
	}{
		{"responses", `{"object":"response","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"fixture"}]}]}`, false, "passed", "passed"},
		{"chat", `{"object":"chat.completion","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"fixture"}}]}`, true, "passed", "passed"},
		{"tool", `{"object":"chat.completion","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"fixture","tool_calls":[{}]}}]}`, true, "incomplete", "not_evaluated"},
		{"length", `{"object":"chat.completion","choices":[{"finish_reason":"length","message":{"role":"assistant","content":"fixture"}}]}`, true, "incomplete", "not_evaluated"},
		{"refusal", `{"object":"chat.completion","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"fixture","refusal":"refused"}}]}`, true, "incomplete", "not_evaluated"},
		{"empty", `{"object":"response","status":"completed","output":[]}`, false, "incomplete", "not_evaluated"},
		{"incomplete", `{"object":"response","status":"incomplete","output":[]}`, false, "incomplete", "not_evaluated"},
		{"truncated", `{"object":"response","status":`, false, "incomplete", "not_evaluated"},
		{"html", `<html>200 OK</html>`, false, "incomplete", "not_evaluated"},
		{"wrong_answer", `{"object":"chat.completion","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"wrong"}}]}`, true, "passed", "failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			outcome := classifyMonitorProbeResponse([]byte(test.body), test.chat, "fixture")
			require.Equal(t, test.transport, outcome.TransportState)
			require.Equal(t, test.challenge, outcome.ChallengeState)
			require.Nil(t, outcome.TTFTMs)
		})
	}
}

func TestMonitorProbeAccountAdmissionAndRevision(t *testing.T) {
	group := &Group{ID: 1, Platform: PlatformOpenAI, Status: StatusActive}
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, GroupIDs: []int64{1}, Credentials: map[string]any{"api_key": "fixture-key", "base_url": "https://fixture.invalid"}}
	targets := []DetectorTargetSpec{{GroupID: &group.ID, Target: DetectorTarget{RequestModel: "fixture"}}}
	require.True(t, monitorProbeAccountAllowed(group, account, targets))
	fingerprint := monitorProbeAccountFingerprint(account)
	account.Extra = map[string]any{"openai_responses_mode": "force_chat_completions"}
	require.NotEqual(t, fingerprint, monitorProbeAccountFingerprint(account))
	endpoint, raw, chat, err := monitorProbeRequest(account, "fixture", "nonce")
	require.NoError(t, err)
	require.True(t, chat)
	require.Equal(t, "https://fixture.invalid/v1/chat/completions", endpoint)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(raw, &payload))
	require.Equal(t, false, payload["stream"])
	account.Type = AccountTypeOAuth
	require.False(t, monitorProbeAccountAllowed(group, account, targets))
	account.Type = AccountTypeAPIKey
	group.RequireOAuthOnly = true
	require.False(t, monitorProbeAccountAllowed(group, account, targets))
	group.RequireOAuthOnly = false
	group.ProfitControlEnabled = true
	require.False(t, monitorProbeAccountAllowed(group, account, targets))
	group.ProfitControlEnabled = false
	account.GroupIDs = nil
	require.False(t, monitorProbeAccountAllowed(group, account, targets))
}

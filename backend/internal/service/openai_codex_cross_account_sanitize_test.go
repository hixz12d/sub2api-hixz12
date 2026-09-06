//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSanitizeCodexBodyForCrossAccountRecovery(t *testing.T) {
	body := []byte(`{"previous_response_id":"resp_old","input":[{"type":"item_reference","id":"msg_old"},{"type":"message","role":"user","content":"hello"},{"type":"reasoning","encrypted_content":"secret"},{"type":"function_call","call_id":"c1","name":"x"},{"type":"function_call_output","call_id":"c1","output":"ok"},{"type":"function_call_output","call_id":"missing","output":"nope"}]}`)
	out := SanitizeCodexBodyForCrossAccountRecovery(body)
	require.False(t, gjson.GetBytes(out, "previous_response_id").Exists())
	arr := gjson.GetBytes(out, "input").Array()
	types := make([]string, 0, len(arr))
	for _, item := range arr {
		types = append(types, item.Get("type").String())
		require.False(t, item.Get("encrypted_content").Exists())
	}
	require.Equal(t, []string{"message", "function_call", "function_call_output"}, types)
}

func TestCodexPlanHasRecoverableFullContextWithForeignCrumbs(t *testing.T) {
	plan, err := NewCodexRequestPlan(CodexRequestPlanInput{
		LogicalRequestID: "req", SessionHash: "sess",
		Body: []byte(`{"previous_response_id":"resp_old","input":[{"type":"item_reference","id":"msg_old"},{"type":"message","id":"msg_old","role":"user","content":"hello"},{"type":"message","role":"assistant","content":"hello there"},{"type":"message","role":"user","content":"continue"}]}`),
		PreviousResponseID: "resp_old", Operation: CodexOperationResume,
	})
	require.NoError(t, err)
	require.True(t, codexPlanHasRecoverableFullContext(plan))

	plan2, err := NewCodexRequestPlan(CodexRequestPlanInput{
		LogicalRequestID: "req2", SessionHash: "sess2",
		Body: []byte(`{"previous_response_id":"resp_old","input":[{"type":"item_reference","id":"msg_old"}]}`),
		PreviousResponseID: "resp_old", Operation: CodexOperationResume,
	})
	require.NoError(t, err)
	require.False(t, codexPlanHasRecoverableFullContext(plan2))
}

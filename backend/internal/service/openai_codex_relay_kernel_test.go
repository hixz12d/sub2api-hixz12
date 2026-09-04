package service

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

const testCodexRelaySecret = "test-codex-relay-secret-at-least-32-bytes"

func TestCodexIdentityDeriverNamespaceSeparationAndStability(t *testing.T) {
	deriver, err := NewCodexIdentityDeriver(testCodexRelaySecret)
	require.NoError(t, err)

	first := deriver.UUIDv4(codexNamespaceSession, "account", "conversation")
	require.Equal(t, first, deriver.UUIDv4(codexNamespaceSession, "account", "conversation"))
	require.NotEqual(t, first, deriver.UUIDv4(codexNamespaceThread, "account", "conversation"))
	require.NotEqual(t, first, deriver.UUIDv4(codexNamespaceSession, "account-conversation"))

	parsed, err := uuid.Parse(first)
	require.NoError(t, err)
	require.Equal(t, uuid.Version(4), parsed.Version())
}

func TestCodexRequestPlanOwnsCopiesAndUsesConversationPriority(t *testing.T) {
	headers := http.Header{"User-Agent": {"caller/1"}}
	body := []byte(`{"model":"gpt-5"}`)
	plan, err := NewCodexRequestPlan(CodexRequestPlanInput{
		LogicalRequestID:   "logical-1",
		SessionHash:        "session-hash",
		PreviousResponseID: "resp_owned",
		PromptCacheKey:     "cache-key",
		RequestedModel:     "gpt-5",
		InboundHeaders:     headers,
		Body:               body,
		CreatedAt:          time.Unix(1_800_000_000, 0),
		DerivationSecret:   testCodexRelaySecret,
	})
	require.NoError(t, err)

	headers.Set("User-Agent", "mutated")
	body[0] = 'x'
	require.Equal(t, "caller/1", plan.InboundHeaders().Get("User-Agent"))
	require.JSONEq(t, `{"model":"gpt-5"}`, string(plan.Body()))

	deriver, err := NewCodexIdentityDeriver(testCodexRelaySecret)
	require.NoError(t, err)
	require.Equal(t, deriver.DigestHex(codexNamespaceConversation, "resp_owned"), plan.ConversationDigest())
}

func TestFinalizeCodexAttemptKeepsLogicalIdentityAcrossRetries(t *testing.T) {
	plan := mustCodexPlanForTest(t, "logical-1", "conversation-1", CodexTransportHTTP, time.Unix(1_800_000_000, 0))
	base := CodexAttemptInput{
		AccountID:              44,
		AccountVersion:         "account-v3",
		CredentialVersion:      "credential-v5",
		ProxyIdentity:          "proxy-9",
		ProfileID:              CodexProfileCLI,
		FingerprintMode:        string(codexFingerprintWindow),
		EgressRoute:            "proxy",
		TransportConfigVersion: "transport-v2",
	}

	attempt0, err := FinalizeCodexAttempt(plan, base, testCodexRelaySecret)
	require.NoError(t, err)
	base.AttemptNumber = 1
	attempt1, err := FinalizeCodexAttempt(plan, base, testCodexRelaySecret)
	require.NoError(t, err)

	require.Equal(t, attempt0.ClientRequestID(), attempt1.ClientRequestID())
	require.NotEqual(t, attempt0.InternalAttemptID(), attempt1.InternalAttemptID())
	require.Equal(t, attempt0.TransportKey(), attempt1.TransportKey())
	require.Equal(t, attempt0.Identity(), attempt1.Identity())

	body := attempt0.FinalHTTPBody()
	body[0] = 'x'
	require.NotEqual(t, body, attempt0.FinalHTTPBody())
	identity := attempt0.Identity()
	identity.sessionID = "mutated"
	require.NotEqual(t, "mutated", attempt0.Identity().SessionID())
}

func TestFinalizeCodexAttemptActiveWindowConversationDoesNotRotate(t *testing.T) {
	before := mustCodexPlanForTest(t, "logical-before", "active-conversation", CodexTransportHTTP, time.Date(2026, 9, 4, 7, 59, 59, 0, time.UTC))
	after := mustCodexPlanForTest(t, "logical-after", "active-conversation", CodexTransportHTTP, time.Date(2026, 9, 4, 8, 0, 1, 0, time.UTC))
	input := CodexAttemptInput{
		AccountID:       55,
		AccountVersion:  "v1",
		ProfileID:       CodexProfileCLI,
		FingerprintMode: string(codexFingerprintWindow),
	}

	first, err := FinalizeCodexAttempt(before, input, testCodexRelaySecret)
	require.NoError(t, err)
	second, err := FinalizeCodexAttempt(after, input, testCodexRelaySecret)
	require.NoError(t, err)

	require.Equal(t, first.Identity().InstallationID(), second.Identity().InstallationID())
	require.Equal(t, first.Identity().SessionID(), second.Identity().SessionID())
	require.Equal(t, first.Identity().ThreadID(), second.Identity().ThreadID())
	require.Equal(t, first.Identity().WindowID(), second.Identity().WindowID())
	require.NotEqual(t, first.Identity().TurnID(), second.Identity().TurnID())
}

func TestFinalizeCodexAttemptTransportKeyIsolatesAccountCredentialAndProfile(t *testing.T) {
	plan := mustCodexPlanForTest(t, "logical-1", "conversation-1", CodexTransportHTTP, time.Unix(1_800_000_000, 0))
	input := CodexAttemptInput{AccountID: 1, CredentialVersion: "credential-a", ProfileID: CodexProfileCLI}
	first, err := FinalizeCodexAttempt(plan, input, testCodexRelaySecret)
	require.NoError(t, err)

	input.AccountID = 2
	second, err := FinalizeCodexAttempt(plan, input, testCodexRelaySecret)
	require.NoError(t, err)
	require.NotEqual(t, first.TransportKey(), second.TransportKey())

	input.AccountID = 1
	input.CredentialVersion = "credential-b"
	third, err := FinalizeCodexAttempt(plan, input, testCodexRelaySecret)
	require.NoError(t, err)
	require.NotEqual(t, first.TransportKey(), third.TransportKey())

	input.CredentialVersion = "credential-a"
	input.ProfileID = CodexProfileExec
	fourth, err := FinalizeCodexAttempt(plan, input, testCodexRelaySecret)
	require.NoError(t, err)
	require.NotEqual(t, first.TransportKey(), fourth.TransportKey())
}

var (
	benchmarkCodexAttempt  *CodexAttemptState
	benchmarkCodexIdentity *CodexIdentitySnapshot
)

func BenchmarkFinalizeCodexAttempt(b *testing.B) {
	plan, err := NewCodexRequestPlan(CodexRequestPlanInput{
		LogicalRequestID: "benchmark-logical-request",
		SessionHash:      "benchmark-conversation",
		RequestedModel:   "gpt-5",
		Operation:        CodexOperationResponses,
		Transport:        CodexTransportHTTP,
		InboundHeaders:   http.Header{"User-Agent": {"codex_cli_rs/0.148.0"}, "originator": {"codex_cli_rs"}},
		Body:             []byte(`{"model":"gpt-5","prompt_cache_key":"benchmark-conversation","client_metadata":{"source":"benchmark"}}`),
		CreatedAt:        time.Unix(1_800_000_000, 0),
		DerivationSecret: testCodexRelaySecret,
	})
	if err != nil {
		b.Fatal(err)
	}
	input := CodexAttemptInput{
		AccountID:              44,
		AccountVersion:         "account-v3",
		CredentialVersion:      "credential-v5",
		ProxyIdentity:          "proxy-9",
		ProfileID:              CodexProfileAuto,
		FingerprintMode:        string(codexFingerprintSession),
		EgressRoute:            "proxy",
		TransportConfigVersion: "transport-v2",
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		input.AttemptNumber = i
		benchmarkCodexAttempt, err = FinalizeCodexAttempt(plan, input, testCodexRelaySecret)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCodexIdentityDerivation(b *testing.B) {
	deriver, err := NewCodexIdentityDeriver(testCodexRelaySecret)
	if err != nil {
		b.Fatal(err)
	}
	plan := &CodexRequestPlan{
		logicalRequestID:   "benchmark-logical-request",
		conversationDigest: strings.Repeat("a", 64),
		createdAt:          time.Unix(1_800_000_000, 0),
	}
	profile, err := ResolveCodexClientProfile(CodexProfileExec)
	if err != nil {
		b.Fatal(err)
	}
	deviceID := deriver.UUIDv4(codexNamespaceDevice, "44:account-v3", "credential-v5", profile.ID, strconv.Itoa(profile.Revision))
	plan.clientRequestID = deriver.UUIDv7(codexNamespaceClientRequest, plan.createdAt, plan.logicalRequestID, plan.conversationDigest)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkCodexIdentity = deriveCodexV2Identity(deriver, plan, codexFingerprintSession, profile, deviceID, plan.clientRequestID)
	}
}

func mustCodexPlanForTest(t *testing.T, logicalRequestID, conversation string, transport CodexEgressTransport, createdAt time.Time) *CodexRequestPlan {
	t.Helper()
	plan, err := NewCodexRequestPlan(CodexRequestPlanInput{
		LogicalRequestID: logicalRequestID,
		SessionHash:      conversation,
		RequestedModel:   "gpt-5",
		Operation:        CodexOperationResponses,
		Transport:        transport,
		InboundHeaders:   http.Header{"User-Agent": {"caller/1"}},
		Body:             []byte(`{"model":"gpt-5"}`),
		CreatedAt:        createdAt,
		DerivationSecret: testCodexRelaySecret,
	})
	require.NoError(t, err)
	return plan
}

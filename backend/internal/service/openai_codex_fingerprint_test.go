package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const testCodexFingerprintSeed = "11111111-1111-4111-8111-111111111111"

func newTestOAuthAccount(id int64, extra map[string]any) *Account {
	if codexFingerprintModeRequiresSeed(codexFingerprintModeFromExtra(extra)) {
		if extra == nil {
			extra = make(map[string]any)
		}
		if _, exists := extra[codexFingerprintSeedExtraKey]; !exists {
			if _, hasMode := extra[codexFingerprintModeExtraKey]; hasMode {
				extra[codexFingerprintSeedExtraKey] = testCodexFingerprintSeed
			} else {
				extra[codexFingerprintSeedExtraKey] = deriveStableUUIDv4(fmt.Sprintf("test-account-seed:%d", id))
			}
		}
	}
	return &Account{
		ID:       id,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    extra,
	}
}

// --- deriveStableUUIDv4 ---

func TestDeriveStableUUIDv4_Deterministic(t *testing.T) {
	a := deriveStableUUIDv4("test-seed-1")
	b := deriveStableUUIDv4("test-seed-1")
	assert.Equal(t, a, b, "同一种子应返回相同结果")
}

func TestDeriveStableUUIDv4_DifferentSeeds(t *testing.T) {
	a := deriveStableUUIDv4("seed-a")
	b := deriveStableUUIDv4("seed-b")
	assert.NotEqual(t, a, b, "不同种子应返回不同结果")
}

func TestDeriveStableUUIDv4_ValidFormat(t *testing.T) {
	result := deriveStableUUIDv4("test-seed")
	parsed, err := uuid.Parse(result)
	require.NoError(t, err, "应返回合法 UUID 格式")
	assert.Equal(t, uuid.Version(4), parsed.Version(), "应为 UUIDv4")
	assert.Equal(t, uuid.RFC4122, parsed.Variant(), "应为 RFC4122 变体")
}

// --- GetCodexFingerprintMode ---

func TestGetCodexFingerprintMode(t *testing.T) {
	tests := []struct {
		name     string
		account  *Account
		expected codexFingerprintMode
	}{
		{"nil 账号", nil, codexFingerprintOff},
		{"非 OAuth 账号", &Account{Platform: PlatformOpenAI, Type: "api_key"}, codexFingerprintOff},
		{"无 extra 默认 device", newTestOAuthAccount(1, nil), codexFingerprintDevice},
		{"空值默认 device", newTestOAuthAccount(1, map[string]any{codexFingerprintModeExtraKey: ""}), codexFingerprintDevice},
		{"非法值安全关闭", newTestOAuthAccount(1, map[string]any{codexFingerprintModeExtraKey: "invalid"}), codexFingerprintOff},
		{"显式 off", newTestOAuthAccount(1, map[string]any{codexFingerprintModeExtraKey: "off"}), codexFingerprintOff},
		{"device", newTestOAuthAccount(1, map[string]any{codexFingerprintModeExtraKey: "device"}), codexFingerprintDevice},
		{"session", newTestOAuthAccount(1, map[string]any{codexFingerprintModeExtraKey: "session"}), codexFingerprintSession},
		{"window", newTestOAuthAccount(1, map[string]any{codexFingerprintModeExtraKey: "window"}), codexFingerprintWindow},
		{"window40", newTestOAuthAccount(1, map[string]any{codexFingerprintModeExtraKey: "window40"}), codexFingerprintWindow40},
		{"full", newTestOAuthAccount(1, map[string]any{codexFingerprintModeExtraKey: "full"}), codexFingerprintFull},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.account.GetCodexFingerprintMode())
		})
	}
}

// --- resolveConvergedInstallationID ---

func TestResolveConvergedInstallationID_UsesDeviceID(t *testing.T) {
	account := newTestOAuthAccount(1, map[string]any{"openai_device_id": "real-device-id"})
	assert.Equal(t, "real-device-id", resolveConvergedInstallationID(account))
}

func TestResolveConvergedInstallationID_DerivesFromAccountID(t *testing.T) {
	account := newTestOAuthAccount(42, nil)
	result := resolveConvergedInstallationID(account)
	_, err := uuid.Parse(result)
	require.NoError(t, err, "派生值应为合法 UUID")
	assert.Equal(t, result, resolveConvergedInstallationID(account), "确定性")
}

func TestResolveConvergedInstallationID_DifferentAccounts(t *testing.T) {
	a := resolveConvergedInstallationID(newTestOAuthAccount(1, nil))
	b := resolveConvergedInstallationID(newTestOAuthAccount(2, nil))
	assert.NotEqual(t, a, b)
}

// --- resolveConvergedThreadID ---

func TestResolveConvergedThreadID_PerClientSession(t *testing.T) {
	account := newTestOAuthAccount(1, nil)
	a := resolveConvergedThreadID(account, "session-aaa")
	b := resolveConvergedThreadID(account, "session-bbb")
	assert.NotEqual(t, a, b, "不同客户端 session 应得到不同 thread_id")
}

func TestResolveConvergedThreadID_Deterministic(t *testing.T) {
	account := newTestOAuthAccount(1, nil)
	a := resolveConvergedThreadID(account, "session-aaa")
	b := resolveConvergedThreadID(account, "session-aaa")
	assert.Equal(t, a, b, "同一客户端 session 应得到相同 thread_id")
}

func TestResolveConvergedThreadID_EmptySession(t *testing.T) {
	account := newTestOAuthAccount(1, nil)
	assert.Equal(t, "", resolveConvergedThreadID(account, ""))
}

// --- window 模式 ---

func TestCodexThreadWindowKey(t *testing.T) {
	tests := []struct {
		name string
		now  time.Time
		want string
	}{
		{"窗口起点", time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC), "2026-08-14T00"},
		{"窗口末尾", time.Date(2026, 8, 14, 7, 59, 59, 0, time.UTC), "2026-08-14T00"},
		{"第二窗口", time.Date(2026, 8, 14, 8, 0, 0, 0, time.UTC), "2026-08-14T08"},
		{"第三窗口", time.Date(2026, 8, 14, 16, 0, 0, 0, time.UTC), "2026-08-14T16"},
		{"次日窗口", time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC), "2026-08-15T00"},
		{"转换 UTC", time.Date(2026, 8, 14, 9, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60)), "2026-08-14T00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, codexThreadWindowKey(tt.now))
		})
	}
}

func TestResolveWindowSlot(t *testing.T) {
	assert.Zero(t, resolveWindowSlot(""))
	assert.Equal(t, resolveWindowSlot("client-stable"), resolveWindowSlot("client-stable"))

	seen := make(map[int]struct{})
	for _, seed := range []string{
		"client-00", "client-01", "client-02", "client-03", "client-04", "client-05", "client-06", "client-07",
		"client-08", "client-09", "client-10", "client-11", "client-12", "client-13", "client-14", "client-15",
		"client-16", "client-17", "client-18", "client-19", "client-20", "client-21", "client-22", "client-23",
		"client-24", "client-25", "client-26", "client-27", "client-28", "client-29", "client-30", "client-31",
	} {
		slot := resolveWindowSlot(seed)
		assert.GreaterOrEqual(t, slot, 0)
		assert.Less(t, slot, codexThreadWindowSlots)
		seen[slot] = struct{}{}
	}
	assert.Greater(t, len(seen), 1)
}

func TestResolveWindowThreadID(t *testing.T) {
	accountA := newTestOAuthAccount(1, nil)
	accountB := newTestOAuthAccount(2, nil)
	firstWindow := time.Date(2026, 8, 14, 1, 0, 0, 0, time.UTC)
	sameWindow := time.Date(2026, 8, 14, 7, 59, 0, 0, time.UTC)
	nextWindow := time.Date(2026, 8, 14, 8, 0, 0, 0, time.UTC)

	seedA := "client-00"
	seedB := ""
	for _, candidate := range []string{"client-01", "client-02", "client-03", "client-04", "client-05", "client-06", "client-07"} {
		if resolveWindowSlot(candidate) != resolveWindowSlot(seedA) {
			seedB = candidate
			break
		}
	}
	require.NotEmpty(t, seedB)

	threadA := resolveWindowThreadID(accountA, seedA, firstWindow)
	assert.Equal(t, threadA, resolveWindowThreadID(accountA, seedA, sameWindow))
	assert.NotEqual(t, threadA, resolveWindowThreadID(accountA, seedB, firstWindow))
	assert.NotEqual(t, threadA, resolveWindowThreadID(accountA, seedA, nextWindow))
	assert.NotEqual(t, threadA, resolveWindowThreadID(accountB, seedA, firstWindow))

	emptyFirst := resolveWindowThreadID(accountA, "", firstWindow)
	emptyNext := resolveWindowThreadID(accountA, "", nextWindow)
	assert.NotEqual(t, emptyFirst, emptyNext, "空种子只固定到 slot 0，跨窗口仍须变化")
	assert.NotEqual(t, resolveConvergedSessionID(accountA), emptyFirst, "空种子不能退化为 full 模式")
}

func TestWindowFingerprintHeadersAndBodyConsistent(t *testing.T) {
	originalNow := codexFingerprintNow
	codexFingerprintNow = func() time.Time {
		return time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC)
	}
	t.Cleanup(func() { codexFingerprintNow = originalNow })

	account := newTestOAuthAccount(7, map[string]any{codexFingerprintModeExtraKey: "window"})
	headers := http.Header{}
	headers.Set("conversation_id", "conversation-seed")
	ids := resolveCodexFingerprintIDsFromRequestWithPromptCacheKey(account, headers, "cache-seed")
	require.NotNil(t, ids)

	outbound := http.Header{}
	outbound.Set("x-codex-turn-metadata", `{"turn_id":"original"}`)
	applyCodexFingerprintHeaders(outbound, ids)
	body := map[string]any{"client_metadata": map[string]any{}}
	require.True(t, applyCodexFingerprintClientMetadata(body, ids))
	metadata := body["client_metadata"].(map[string]any)

	assert.Equal(t, resolveConvergedSessionID(account), outbound.Get("session-id"))
	assert.Equal(t, ids.sessionID, outbound.Get("conversation_id"))
	assert.Equal(t, ids.threadID, outbound.Get("thread-id"))
	assert.Equal(t, ids.threadID, metadata["thread_id"])
	assert.Equal(t, ids.turnID, metadata["turn_id"])
	assert.Equal(t, ids.threadID+":0", metadata["x-codex-window-id"])
	assert.Empty(t, outbound.Get("session_id"))
	assert.Empty(t, outbound.Get("thread_id"))

	var turnMetadata map[string]any
	require.NoError(t, json.Unmarshal([]byte(outbound.Get("x-codex-turn-metadata")), &turnMetadata))
	assert.Equal(t, ids.threadID, turnMetadata["thread_id"])
	assert.Equal(t, ids.turnID, turnMetadata["turn_id"])
}

func TestExtractClientThreadSeedPriority(t *testing.T) {
	headers := http.Header{}
	headers.Set("session-id", "current-session")
	headers.Set("session_id", "legacy-session")
	headers.Set("conversation_id", "conversation")
	assert.Equal(t, "current-session", extractClientThreadSeed(headers, "cache"))

	headers.Del("session-id")
	assert.Equal(t, "legacy-session", extractClientThreadSeed(headers, "cache"))
	headers.Del("session_id")
	assert.Equal(t, "conversation", extractClientThreadSeed(headers, "cache"))
	headers.Del("conversation_id")
	headers.Set(openCodeNativeSessionHeader, "opencode-session")
	assert.Equal(t, "opencode-session", extractClientThreadSeed(headers, "cache"))
	headers.Del(openCodeNativeSessionHeader)
	assert.Equal(t, "cache", extractClientThreadSeed(headers, " cache "))
}

func TestWindow40BudgetAndTenantIsolation(t *testing.T) {
	times := []time.Time{
		time.Date(2026, 8, 14, 1, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 14, 17, 0, 0, 0, time.UTC),
	}
	assert.Equal(t, 13, codexThreadWindow40Slots(times[0]))
	assert.Equal(t, 14, codexThreadWindow40Slots(times[1]))
	assert.Equal(t, 13, codexThreadWindow40Slots(times[2]))
	assert.Equal(t, 40, codexThreadWindow40Slots(times[0])+codexThreadWindow40Slots(times[1])+codexThreadWindow40Slots(times[2]))

	account := newTestOAuthAccount(7, map[string]any{codexFingerprintModeExtraKey: "window40"})
	cA, _ := gin.CreateTestContext(httptest.NewRecorder())
	cA.Set("api_key", &APIKey{ID: 11, UserID: 101})
	cB, _ := gin.CreateTestContext(httptest.NewRecorder())
	cB.Set("api_key", &APIKey{ID: 12, UserID: 102})
	headers := http.Header{"session-id": []string{"same-client-session"}}

	originalNow := codexFingerprintNow
	codexFingerprintNow = func() time.Time { return times[0] }
	t.Cleanup(func() { codexFingerprintNow = originalNow })

	idsA1 := resolveCodexFingerprintIDsFromContext(account, cA, headers, "")
	idsA2 := resolveCodexFingerprintIDsFromContext(account, cA, headers, "")
	idsB := resolveCodexFingerprintIDsFromContext(account, cB, headers, "")
	require.NotNil(t, idsA1)
	require.NotNil(t, idsA2)
	require.NotNil(t, idsB)
	assert.Equal(t, codexFingerprintWindow40, idsA1.mode)
	assert.Equal(t, idsA1.threadID, idsA2.threadID)
	assert.NotEqual(t, idsA1.turnID, idsA2.turnID)
	assert.NotEqual(t, combineCodexFingerprintSeed(codexFingerprintTenantSeed(cA), "same-client-session"), combineCodexFingerprintSeed(codexFingerprintTenantSeed(cB), "same-client-session"))

	for i := 0; i < 128; i++ {
		slot := resolveWindow40Slot(fmt.Sprintf("seed-%d", i), times[1])
		assert.GreaterOrEqual(t, slot, 0)
		assert.Less(t, slot, 14)
	}
}

// --- off 模式：resolveCodexFingerprintIDsFromRequest 返回 nil ---

func TestResolveCodexFingerprintIDsFromRequest_ExplicitOff(t *testing.T) {
	account := newTestOAuthAccount(1, map[string]any{codexFingerprintModeExtraKey: "off"})
	ids := resolveCodexFingerprintIDsFromRequest(account, nil)
	assert.Nil(t, ids, "显式 off 模式应返回 nil")
}

func TestResolveCodexFingerprintIDsFromRequest_DefaultIsDevice(t *testing.T) {
	account := newTestOAuthAccount(1, nil)
	ids := resolveCodexFingerprintIDsFromRequest(account, nil)
	require.NotNil(t, ids, "无 extra 应使用账号级 device 身份")
	assert.Equal(t, codexFingerprintDevice, ids.mode)
}

// --- applyCodexFingerprintHeaders: off 模式 ---

func TestApplyCodexFingerprintHeaders_OffMode(t *testing.T) {
	h := http.Header{}
	h.Set("x-codex-installation-id", "original-install-id")
	h.Set("x-codex-window-id", "original-window-id")

	applyCodexFingerprintHeaders(h, nil)

	assert.Equal(t, "original-install-id", h.Get("x-codex-installation-id"), "nil ids 不改写")
	assert.Equal(t, "original-window-id", h.Get("x-codex-window-id"), "nil ids 不改写")
}

// --- applyCodexFingerprintHeaders: device 模式 ---

func TestApplyCodexFingerprintHeaders_DeviceMode(t *testing.T) {
	account := newTestOAuthAccount(1, map[string]any{
		codexFingerprintModeExtraKey: "device",
		"openai_device_id":           "converged-device",
	})
	turnMetadata := `{"installation_id":"user-install","session_id":"user-session","sandbox":"seccomp"}`
	h := http.Header{}
	h.Set("x-codex-installation-id", "user-install")
	h.Set("x-codex-window-id", "user-window:0")
	h.Set("x-codex-turn-metadata", turnMetadata)

	ids := resolveCodexFingerprintIDsFromRequest(account, nil)
	applyCodexFingerprintHeaders(h, ids)

	assert.Equal(t, "converged-device", h.Get("x-codex-installation-id"), "installation_id 应收敛")
	assert.Equal(t, "user-window:0", h.Get("x-codex-window-id"), "device 模式不改写 window_id")

	var meta map[string]any
	require.NoError(t, json.Unmarshal([]byte(h.Get("x-codex-turn-metadata")), &meta))
	assert.Equal(t, "converged-device", meta["installation_id"])
	assert.Equal(t, "user-session", meta["session_id"], "device 模式不改写 session_id")
	assert.Equal(t, "seccomp", meta["sandbox"], "非指纹字段保留原样")
}

// --- applyCodexFingerprintHeaders: session 模式 ---

func TestApplyCodexFingerprintHeaders_SessionMode(t *testing.T) {
	account := newTestOAuthAccount(1, map[string]any{
		codexFingerprintModeExtraKey: "session",
	})
	clientHeaders := http.Header{}
	clientHeaders.Set("session-id", "client-session-aaa")

	turnMetadata := `{"installation_id":"user-install","session_id":"user-session","thread_id":"user-thread","turn_id":"user-turn","window_id":"user-thread:0","sandbox":"seccomp","thread_source":"user"}`
	h := http.Header{}
	h.Set("x-codex-installation-id", "user-install")
	h.Set("x-codex-window-id", "user-thread:0")
	h.Set("x-codex-turn-metadata", turnMetadata)
	h.Set("x-client-request-id", "user-thread")

	ids := resolveCodexFingerprintIDsFromRequest(account, clientHeaders)
	applyCodexFingerprintHeaders(h, ids)

	convergedInstall := resolveConvergedInstallationID(account)
	convergedSession := resolveConvergedSessionID(account)
	convergedThread := resolveConvergedThreadID(account, "client-session-aaa")

	assert.Equal(t, convergedInstall, h.Get("x-codex-installation-id"))
	assert.Equal(t, convergedSession, h.Get("session-id"))
	assert.Equal(t, convergedSession, h.Get("conversation_id"))
	assert.Empty(t, h.Get("session_id"), "OAuth/Codex HTTP 不得保留下划线 session 头")
	assert.Equal(t, convergedThread, h.Get("thread-id"))
	assert.Empty(t, h.Get("thread_id"), "OAuth/Codex HTTP 不得保留下划线 thread 头")
	assert.Equal(t, ids.turnID, h.Get("x-client-request-id"))
	assert.Equal(t, convergedThread+":0", h.Get("x-codex-window-id"))

	var meta map[string]any
	require.NoError(t, json.Unmarshal([]byte(h.Get("x-codex-turn-metadata")), &meta))
	assert.Equal(t, convergedInstall, meta["installation_id"])
	assert.Equal(t, convergedSession, meta["session_id"])
	assert.Equal(t, convergedThread, meta["thread_id"])
	assert.NotEqual(t, "user-turn", meta["turn_id"], "turn_id 应被新生成的值替换")
	assert.Equal(t, "seccomp", meta["sandbox"], "sandbox 保留原样")
	assert.Equal(t, "cli", meta["thread_source"], "thread_source 由最终身份策略统一")
}

// --- session 模式：不同客户端得到不同 thread ---

func TestApplyCodexFingerprintHeaders_SessionMode_DifferentClients(t *testing.T) {
	account := newTestOAuthAccount(1, map[string]any{
		codexFingerprintModeExtraKey: "session",
	})

	makeTurnMeta := func() string {
		return `{"installation_id":"x","session_id":"x","thread_id":"x","turn_id":"x","window_id":"x:0"}`
	}

	clientA := http.Header{}
	clientA.Set("session-id", "client-A")
	idsA := resolveCodexFingerprintIDsFromRequest(account, clientA)
	hA := http.Header{}
	hA.Set("x-codex-turn-metadata", makeTurnMeta())
	applyCodexFingerprintHeaders(hA, idsA)

	clientB := http.Header{}
	clientB.Set("session-id", "client-B")
	idsB := resolveCodexFingerprintIDsFromRequest(account, clientB)
	hB := http.Header{}
	hB.Set("x-codex-turn-metadata", makeTurnMeta())
	applyCodexFingerprintHeaders(hB, idsB)

	assert.Equal(t, hA.Get("session-id"), hB.Get("session-id"), "session_id 应相同")
	assert.NotEqual(t, hA.Get("thread-id"), hB.Get("thread-id"), "不同客户端 thread_id 应不同")
	assert.NotEqual(t, hA.Get("x-codex-window-id"), hB.Get("x-codex-window-id"), "不同客户端 window_id 应不同")
	assert.Equal(t, hA.Get("x-codex-installation-id"), hB.Get("x-codex-installation-id"))
}

// --- full 模式 ---

func TestApplyCodexFingerprintHeaders_FullMode(t *testing.T) {
	account := newTestOAuthAccount(1, map[string]any{
		codexFingerprintModeExtraKey: "full",
	})
	convergedSession := resolveConvergedSessionID(account)

	clientA := http.Header{}
	clientA.Set("session-id", "client-A")
	idsA := resolveCodexFingerprintIDsFromRequest(account, clientA)
	hA := http.Header{}
	hA.Set("x-codex-turn-metadata", `{"installation_id":"x","session_id":"x","thread_id":"x","turn_id":"x","window_id":"x:0"}`)
	applyCodexFingerprintHeaders(hA, idsA)

	clientB := http.Header{}
	clientB.Set("session-id", "client-B")
	idsB := resolveCodexFingerprintIDsFromRequest(account, clientB)
	hB := http.Header{}
	hB.Set("x-codex-turn-metadata", `{"installation_id":"x","session_id":"x","thread_id":"x","turn_id":"x","window_id":"x:0"}`)
	applyCodexFingerprintHeaders(hB, idsB)

	assert.Equal(t, hA.Get("thread-id"), hB.Get("thread-id"), "full 模式 thread_id 应相同")
	assert.Equal(t, convergedSession, hA.Get("thread-id"), "full 模式 thread_id 应等于 session_id")
	assert.Equal(t, hA.Get("x-codex-window-id"), hB.Get("x-codex-window-id"), "full 模式 window_id 应相同")
}

// --- H1 修复验证：头和体的 turn_id 一致性 ---

func TestFingerprintIDs_HeaderAndBody_TurnID_Consistent(t *testing.T) {
	account := newTestOAuthAccount(1, map[string]any{
		codexFingerprintModeExtraKey: "session",
	})
	clientHeaders := http.Header{}
	clientHeaders.Set("session-id", "client-session-xyz")

	ids := resolveCodexFingerprintIDsFromRequest(account, clientHeaders)
	require.NotNil(t, ids)

	// 头改写
	h := http.Header{}
	h.Set("x-codex-turn-metadata", `{"installation_id":"x","session_id":"x","thread_id":"x","turn_id":"x","window_id":"x:0"}`)
	applyCodexFingerprintHeaders(h, ids)

	// 体改写（使用同一份 ids）
	reqBody := map[string]any{
		"client_metadata": map[string]any{
			"x-codex-installation-id": "x",
			"session_id":              "x",
			"turn_id":                 "x",
			"x-codex-turn-metadata":   `{"installation_id":"x","session_id":"x","thread_id":"x","turn_id":"x","window_id":"x:0"}`,
		},
	}
	applyCodexFingerprintClientMetadata(reqBody, ids)

	// 从头 turn-metadata JSON 提取 turn_id
	var headerMeta map[string]any
	require.NoError(t, json.Unmarshal([]byte(h.Get("x-codex-turn-metadata")), &headerMeta))
	headerTurnID, ok := headerMeta["turn_id"].(string)
	require.True(t, ok, "头 turn-metadata 应包含 string 类型的 turn_id")

	// 从体 client_metadata 提取 turn_id
	cm, ok := reqBody["client_metadata"].(map[string]any)
	require.True(t, ok, "请求体应包含 client_metadata")
	bodyTurnID, ok := cm["turn_id"].(string)
	require.True(t, ok, "体 client_metadata 应包含 string 类型的 turn_id")

	// 从体内嵌 turn-metadata JSON 提取 turn_id
	embeddedRaw, ok := cm["x-codex-turn-metadata"].(string)
	require.True(t, ok, "体 client_metadata 应包含 x-codex-turn-metadata 字符串")
	var bodyMeta map[string]any
	require.NoError(t, json.Unmarshal([]byte(embeddedRaw), &bodyMeta))
	bodyEmbeddedTurnID, ok := bodyMeta["turn_id"].(string)
	require.True(t, ok, "体内嵌 turn-metadata 应包含 string 类型的 turn_id")

	assert.Equal(t, headerTurnID, bodyTurnID, "头和体的 turn_id 必须一致")
	assert.Equal(t, headerTurnID, bodyEmbeddedTurnID, "头和体内嵌 turn-metadata 的 turn_id 必须一致")
	assert.Equal(t, ids.turnID, headerTurnID, "所有 turn_id 都应来自同一份 ids")
}

// --- applyCodexFingerprintClientMetadata ---

func TestApplyCodexFingerprintClientMetadata_OffMode(t *testing.T) {
	reqBody := map[string]any{
		"client_metadata": map[string]any{
			"x-codex-installation-id": "original",
		},
	}
	modified := applyCodexFingerprintClientMetadata(reqBody, nil)
	assert.False(t, modified, "nil ids 不改写")
}

func TestApplyCodexFingerprintClientMetadata_DeviceMode(t *testing.T) {
	account := newTestOAuthAccount(1, map[string]any{
		codexFingerprintModeExtraKey: "device",
		"openai_device_id":           "converged-device",
	})
	ids := resolveCodexFingerprintIDsFromRequest(account, nil)
	require.NotNil(t, ids)

	embeddedMeta := `{"installation_id":"x","session_id":"user-session","sandbox":"seccomp"}`
	reqBody := map[string]any{
		"client_metadata": map[string]any{
			"x-codex-installation-id": "original-install",
			"session_id":              "user-session",
			"x-codex-turn-metadata":   embeddedMeta,
		},
	}

	modified := applyCodexFingerprintClientMetadata(reqBody, ids)
	require.True(t, modified)

	cm, ok := reqBody["client_metadata"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "converged-device", cm["x-codex-installation-id"])
	assert.Equal(t, "user-session", cm["session_id"], "device 模式不改 session_id")

	turnMetaStr, ok := cm["x-codex-turn-metadata"].(string)
	require.True(t, ok)
	var meta map[string]any
	require.NoError(t, json.Unmarshal([]byte(turnMetaStr), &meta))
	assert.Equal(t, "converged-device", meta["installation_id"])
	assert.Equal(t, "seccomp", meta["sandbox"], "非指纹字段保留原样")
}

func TestApplyCodexFingerprintClientMetadata_SessionMode(t *testing.T) {
	account := newTestOAuthAccount(1, map[string]any{
		codexFingerprintModeExtraKey: "session",
	})
	clientHeaders := http.Header{}
	clientHeaders.Set("session-id", "client-session-aaa")

	ids := resolveCodexFingerprintIDsFromRequest(account, clientHeaders)
	require.NotNil(t, ids)

	embeddedMeta := `{"installation_id":"x","session_id":"x","thread_id":"x","turn_id":"x","window_id":"x:0","sandbox":"seccomp"}`
	reqBody := map[string]any{
		"client_metadata": map[string]any{
			"x-codex-installation-id": "original-install",
			"session_id":              "original-session",
			"x-codex-turn-metadata":   embeddedMeta,
		},
	}

	modified := applyCodexFingerprintClientMetadata(reqBody, ids)
	require.True(t, modified)

	cm, ok := reqBody["client_metadata"].(map[string]any)
	require.True(t, ok)
	convergedInstall := resolveConvergedInstallationID(account)
	convergedSession := resolveConvergedSessionID(account)
	convergedThread := resolveConvergedThreadID(account, "client-session-aaa")

	assert.Equal(t, convergedInstall, cm["x-codex-installation-id"])
	assert.Equal(t, convergedSession, cm["session_id"])
	assert.Equal(t, convergedThread, cm["thread_id"])
	assert.Equal(t, convergedThread+":0", cm["x-codex-window-id"])

	turnMetaStr, ok := cm["x-codex-turn-metadata"].(string)
	require.True(t, ok)
	var meta map[string]any
	require.NoError(t, json.Unmarshal([]byte(turnMetaStr), &meta))
	assert.Equal(t, convergedInstall, meta["installation_id"])
	assert.Equal(t, convergedSession, meta["session_id"])
	assert.Equal(t, "seccomp", meta["sandbox"], "非指纹字段保留原样")
}

func TestApplyCodexFingerprintClientMetadata_FullMode(t *testing.T) {
	account := newTestOAuthAccount(1, map[string]any{
		codexFingerprintModeExtraKey: "full",
	})
	clientHeaders := http.Header{}
	clientHeaders.Set("session-id", "any-client")

	ids := resolveCodexFingerprintIDsFromRequest(account, clientHeaders)
	require.NotNil(t, ids)

	reqBody := map[string]any{
		"client_metadata": map[string]any{
			"session_id":            "x",
			"thread_id":             "x",
			"x-codex-turn-metadata": `{"installation_id":"x","session_id":"x","thread_id":"x","turn_id":"x","window_id":"x:0"}`,
		},
	}

	modified := applyCodexFingerprintClientMetadata(reqBody, ids)
	require.True(t, modified)

	cm, ok := reqBody["client_metadata"].(map[string]any)
	require.True(t, ok)
	convergedSession := resolveConvergedSessionID(account)

	assert.Equal(t, convergedSession, cm["session_id"])
	assert.Equal(t, convergedSession, cm["thread_id"], "full 模式 thread_id 应等于 session_id")
}

// --- extractClientSessionID ---

func TestExtractClientSessionID(t *testing.T) {
	tests := []struct {
		name     string
		headers  http.Header
		expected string
	}{
		{"连字符形式优先", func() http.Header {
			h := http.Header{}
			h.Set("session-id", "hyphen-form")
			h.Set("session_id", "underscore-form")
			return h
		}(), "hyphen-form"},
		{"回退到下划线形式", func() http.Header {
			h := http.Header{}
			h.Set("session_id", "underscore-form")
			return h
		}(), "underscore-form"},
		{"都没有", http.Header{}, ""},
		{"OpenCode 会话头", func() http.Header {
			h := http.Header{}
			h.Set(openCodeNativeSessionHeader, "opencode-session")
			return h
		}(), "opencode-session"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, extractClientSessionID(tt.headers))
		})
	}
}

func TestApplyCodexFingerprintToRawBody_PreservesPromptCacheKeyWhenOff(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","prompt_cache_key":"client-cache","input":[]}`)
	out, err := applyCodexFingerprintToRawBody(body, nil)
	require.NoError(t, err)
	assert.Equal(t, "client-cache", gjson.GetBytes(out, "prompt_cache_key").String())
	assert.Equal(t, "gpt-5.4", gjson.GetBytes(out, "model").String())
}

func TestDeviceModeFillsMissingCodexHeadersFromOpenCodeSession(t *testing.T) {
	account := newTestOAuthAccount(1, map[string]any{codexFingerprintModeExtraKey: "device"})
	clientHeaders := http.Header{}
	clientHeaders.Set(openCodeNativeSessionHeader, "opencode-conversation-1")

	ids := resolveCodexFingerprintIDsFromRequest(account, clientHeaders)
	require.NotNil(t, ids)
	require.Equal(t, codexFingerprintDevice, ids.mode)
	require.NotEmpty(t, ids.sessionID)
	require.NotEmpty(t, ids.threadID)
	require.NotEqual(t, ids.sessionID, ids.threadID)
	require.Equal(t, ids.threadID+":0", ids.windowID)

	outbound := http.Header{}
	applyCodexFingerprintHeaders(outbound, ids)
	assert.Equal(t, ids.installationID, outbound.Get("x-codex-installation-id"))
	assert.Equal(t, ids.sessionID, outbound.Get("session-id"))
	assert.Equal(t, ids.threadID, outbound.Get("thread-id"))
	assert.Equal(t, ids.windowID, outbound.Get("x-codex-window-id"))
	assert.Equal(t, ids.clientRequestID, outbound.Get("x-client-request-id"))

	same := resolveCodexFingerprintIDsFromRequest(account, clientHeaders)
	require.NotNil(t, same)
	assert.Equal(t, ids.sessionID, same.sessionID)
	assert.Equal(t, ids.threadID, same.threadID)
}

func TestDeviceModeDoesNotOverrideClientCodexSessionHeaders(t *testing.T) {
	account := newTestOAuthAccount(1, map[string]any{codexFingerprintModeExtraKey: "device"})
	clientHeaders := http.Header{}
	clientHeaders.Set("session-id", "client-session")
	clientHeaders.Set("thread-id", "client-thread")

	ids := resolveCodexFingerprintIDsFromRequest(account, clientHeaders)
	require.NotNil(t, ids)
	require.NotEmpty(t, ids.sessionID, "device fill IDs still exist as candidates")

	outbound := http.Header{}
	outbound.Set("session-id", "client-session")
	outbound.Set("thread-id", "client-thread")
	outbound.Set("x-codex-window-id", "client-thread:0")
	applyCodexFingerprintHeaders(outbound, ids)

	assert.Equal(t, "client-session", outbound.Get("session-id"))
	assert.Equal(t, "client-thread", outbound.Get("thread-id"))
	assert.Equal(t, "client-thread:0", outbound.Get("x-codex-window-id"))
	assert.Equal(t, ids.installationID, outbound.Get("x-codex-installation-id"))
}

func TestDeviceModePromptCacheKeyStaysClientScoped(t *testing.T) {
	account := newTestOAuthAccount(9, map[string]any{codexFingerprintModeExtraKey: "device"})
	clientHeaders := http.Header{}
	clientHeaders.Set(openCodeNativeSessionHeader, "opencode-conversation-2")
	ids := resolveCodexFingerprintIDsFromRequest(account, clientHeaders)
	require.NotNil(t, ids)
	require.NotEmpty(t, ids.sessionID)

	svc := &OpenAIGatewayService{}
	got := svc.resolveCodexOutboundPromptCacheKey(nil, account, ids, "client-cache")
	assert.Equal(t, svc.openAIOutboundSessionID(account, 0, "client-cache"), got)
	assert.NotEqual(t, ids.sessionID, got)
}

package service

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// codexFingerprintMode is the legacy compatibility vocabulary for stable OAuth
// identity normalization. It is retained during the Phase 1 migration; these
// modes must not be used to synthesize random users or bypass protocol policy.
type codexFingerprintMode string

const (
	// codexFingerprintOff 不做收敛，仅执行 canonical wire 规范化。
	codexFingerprintOff codexFingerprintMode = "off"
	// codexFingerprintDevice 仅收敛 installation_id 为账号级恒定值。
	// 客户端已给出的 session/thread/window 保持原边界；通用 API 客户端缺这些
	// Codex 线头时，用与粘性路由相同的会话种子补齐，避免多轮对话被拆成无 thread 的请求。
	codexFingerprintDevice codexFingerprintMode = "device"
	// codexFingerprintSession 收敛 installation_id + session_id，
	// thread_id 按客户端原始 session-id 确定性派生（每个真实 Codex 会话一个独立线程）。
	// 这是历史兼容语义；新产品语义将迁移为 account_stable。
	codexFingerprintSession codexFingerprintMode = "session"
	// codexFingerprintWindow 保留历史 UTC 8 小时、8 槽兼容算法；Phase 1
	// 不改变其分桶结果，后续迁移阶段再决定废弃映射。
	codexFingerprintWindow codexFingerprintMode = "window"
	// codexFingerprintWindow40 把共享账号收敛成最多 40 条普通 Codex 对话。
	// 每条对话 session_id == thread_id，window_id 是同一次生成的 UUIDv7，不随 UTC 整点切换。
	codexFingerprintWindow40 codexFingerprintMode = "window40"
	// codexFingerprintFull 收敛所有标识：installation_id + session_id + thread_id。
	// 仅为旧配置兼容保留，不作为新部署推荐模式。
	codexFingerprintFull codexFingerprintMode = "full"
)

const (
	codexFingerprintModeExtraKey = "codex_fingerprint_mode"
	codexFingerprintSeedExtraKey = "codex_fingerprint_seed"
	codexThreadWindowHours = 8
	codexThreadWindowSlots = 8
	codexWindow40Budget    = 40
	// UUIDv7 稳定时间戳落在 2025-01-01 起的 600 天内，避免派生 ID 看起来像未来时间。
	uuidv7StableOriginMS   = 1735689600000
	uuidv7StableSpanMS     = 600 * 24 * 60 * 60 * 1000
	// 官方桌面端普通对话的环境默认值；仅在客户端没带这些字段时补齐。
	codexWindow40DefaultSandbox      = "danger-full-access"
	codexWindow40DefaultThreadSource = "user"

	codexSessionHeader       = "session-id"
	legacyCodexSessionHeader = "session_id"
	codexThreadHeader        = "thread-id"
	legacyCodexThreadHeader  = "thread_id"
)

var codexFingerprintNow = time.Now

const codexFingerprintIDsContextKey = "codex_fingerprint_ids"

// stageCodexFingerprintIDs stores the per-attempt snapshot shared by body and headers.
func stageCodexFingerprintIDs(c *gin.Context, ids *codexFingerprintIDs) {
	if c != nil {
		c.Set(codexFingerprintIDsContextKey, ids)
	}
}

func canonicalCodexFingerprintSeed(value any) (string, bool) {
	raw, ok := value.(string)
	if !ok {
		return "", false
	}
	trimmed := strings.TrimSpace(raw)
	parsed, err := uuid.Parse(trimmed)
	if err != nil || parsed == uuid.Nil || trimmed != parsed.String() {
		return "", false
	}
	return trimmed, true
}

func newCodexFingerprintSeed() string {
	return uuid.NewString()
}

func stripCodexFingerprintSeed(extra map[string]any) map[string]any {
	if extra == nil {
		return nil
	}
	stripped := maps.Clone(extra)
	delete(stripped, codexFingerprintSeedExtraKey)
	return stripped
}

func codexFingerprintModeFromExtra(extra map[string]any) codexFingerprintMode {
	if extra == nil {
		return codexFingerprintDevice
	}
	raw, _ := extra[codexFingerprintModeExtraKey].(string)
	if strings.TrimSpace(raw) == "" {
		return codexFingerprintDevice
	}
	switch mode := codexFingerprintMode(strings.TrimSpace(raw)); mode {
	case codexFingerprintOff, codexFingerprintDevice, codexFingerprintSession, codexFingerprintWindow, codexFingerprintWindow40, codexFingerprintFull:
		return mode
	default:
		return codexFingerprintOff
	}
}

func codexFingerprintModeRequiresSeed(mode codexFingerprintMode) bool {
	return mode == codexFingerprintDevice || mode == codexFingerprintSession || mode == codexFingerprintWindow || mode == codexFingerprintWindow40 || mode == codexFingerprintFull
}

func codexFingerprintSeed(extra map[string]any) (string, bool) {
	if extra == nil {
		return "", false
	}
	return canonicalCodexFingerprintSeed(extra[codexFingerprintSeedExtraKey])
}

func prepareCodexFingerprintExtraForCreate(platform, accountType string, extra map[string]any) map[string]any {
	prepared := stripCodexFingerprintSeed(extra)
	if platform != PlatformOpenAI || (accountType != AccountTypeOAuth && accountType != AccountTypeSetupToken) || !codexFingerprintModeRequiresSeed(codexFingerprintModeFromExtra(prepared)) {
		return prepared
	}
	if prepared == nil {
		prepared = make(map[string]any, 1)
	}
	prepared[codexFingerprintSeedExtraKey] = newCodexFingerprintSeed()
	return prepared
}

func prepareCodexFingerprintExtraForUpdate(account *Account, extra map[string]any) map[string]any {
	prepared := stripCodexFingerprintSeed(extra)
	if account == nil || !account.IsOpenAIOAuthLike() {
		return prepared
	}
	if seed, ok := codexFingerprintSeed(account.Extra); ok {
		if prepared == nil {
			prepared = make(map[string]any, 1)
		}
		prepared[codexFingerprintSeedExtraKey] = seed
		return prepared
	}
	if codexFingerprintModeRequiresSeed(codexFingerprintModeFromExtra(prepared)) {
		if prepared == nil {
			prepared = make(map[string]any, 1)
		}
		prepared[codexFingerprintSeedExtraKey] = newCodexFingerprintSeed()
	}
	return prepared
}

func sanitizedCodexFingerprintExtraUpdates(updates map[string]any) map[string]any {
	if updates == nil {
		return nil
	}
	sanitized := maps.Clone(updates)
	delete(sanitized, codexFingerprintSeedExtraKey)
	return sanitized
}

func ShouldEnsureCodexFingerprintSeedForExtraUpdates(updates map[string]any) bool {
	if updates == nil {
		return false
	}
	_, modeWasUpdated := updates[codexFingerprintModeExtraKey]
	return modeWasUpdated && codexFingerprintModeRequiresSeed(codexFingerprintModeFromExtra(updates))
}

// GetCodexFingerprintMode 从账号 extra JSON 读取指纹收敛模式。
// 未设置、为空或非法时默认 off；device/session/window/window40/full 仅显式启用。
func (a *Account) GetCodexFingerprintMode() codexFingerprintMode {
	if a == nil || !a.IsOpenAIOAuthLike() {
		return codexFingerprintOff
	}
	raw := strings.TrimSpace(a.GetExtraString(codexFingerprintModeExtraKey))
	if raw == "" {
		// Account-scoped device identity is the default; explicit "off" opts out.
		return codexFingerprintDevice
	}
	switch codexFingerprintMode(raw) {
	case codexFingerprintOff, codexFingerprintDevice, codexFingerprintSession, codexFingerprintWindow, codexFingerprintWindow40, codexFingerprintFull:
		return codexFingerprintMode(raw)
	default:
		// Unknown configured values fail safe to off. The v2 finalizer returns a
		// structured error instead; neither path silently enters a high-impact mode.
		return codexFingerprintOff
	}
}

// deriveStableUUIDv4 从种子确定性派生一个 UUIDv4 格式的字符串。
// 同一种子永远返回同一值。
func deriveStableUUIDv4(seed string) string {
	h := sha256.Sum256([]byte(seed))
	b := h[:16]
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 1
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		binary.BigEndian.Uint32(b[0:4]),
		binary.BigEndian.Uint16(b[4:6]),
		binary.BigEndian.Uint16(b[6:8]),
		binary.BigEndian.Uint16(b[8:10]),
		b[10:16])
}

// fingerprintDerivationSeed accepts both the account form used by the legacy
// window helpers and the durable seed form used by the 0.1.178 protocol.
func fingerprintDerivationSeed(source any) (string, string) {
	switch value := source.(type) {
	case string:
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value), "v2"
		}
	case *Account:
		if value == nil {
			return "", ""
		}
		if seed, ok := codexFingerprintSeed(value.Extra); ok {
			return seed, "v2"
		}
		return fmt.Sprintf("%d", value.ID), "v1"
	}
	return "", ""
}

// resolveConvergedInstallationID returns an account-stable installation ID.
// The optional seed keeps source compatibility with the pre-seed helpers while
// ensuring seeded accounts use the durable, non-ID-derived protocol identity.
func resolveConvergedInstallationID(account *Account, seeds ...string) string {
	if account == nil {
		return ""
	}
	if deviceID := account.GetOpenAIDeviceID(); deviceID != "" {
		return deviceID
	}
	seed, version := fingerprintDerivationSeed(account)
	if len(seeds) > 0 && strings.TrimSpace(seeds[0]) != "" {
		seed, version = strings.TrimSpace(seeds[0]), "v2"
	}
	if seed == "" {
		return ""
	}
	return deriveStableUUIDv4("sub2api:codex-install-id:" + version + ":" + seed)
}

// resolveConvergedSessionID accepts either an account (legacy callers) or the
// durable seed string used by the official 0.1.178 callers.
func resolveConvergedSessionID(source any) string {
	seed, version := fingerprintDerivationSeed(source)
	if seed == "" {
		return ""
	}
	return deriveStableUUIDv4("sub2api:codex-session-id:" + version + ":" + seed)
}

// resolveConvergedThreadID accepts either an account or durable seed.
func resolveConvergedThreadID(source any, clientSessionID string) string {
	seed, version := fingerprintDerivationSeed(source)
	if seed == "" || clientSessionID == "" {
		return ""
	}
	return deriveStableUUIDv4("sub2api:codex-thread-id:" + version + ":" + seed + ":" + clientSessionID)
}

func codexThreadWindowKey(now time.Time) string {
	utc := now.UTC()
	windowHour := (utc.Hour() / codexThreadWindowHours) * codexThreadWindowHours
	windowStart := time.Date(utc.Year(), utc.Month(), utc.Day(), windowHour, 0, 0, 0, time.UTC)
	return windowStart.Format("2006-01-02T15")
}

func resolveWindowSlot(clientSessionID string) int {
	if strings.TrimSpace(clientSessionID) == "" {
		return 0
	}
	hash := sha256.Sum256([]byte(clientSessionID))
	return int(binary.BigEndian.Uint64(hash[:8]) % uint64(codexThreadWindowSlots))
}

func resolveWindowThreadID(source any, clientSessionID string, now time.Time) string {
	seed, version := fingerprintDerivationSeed(source)
	if seed == "" {
		return ""
	}
	return deriveStableUUIDv4(fmt.Sprintf(
		"sub2api:codex-thread-id:%s-window:%s:%s:%d",
		version, seed, codexThreadWindowKey(now), resolveWindowSlot(clientSessionID),
	))
}

func resolveWindow40Slot(clientSeed string) int {
	if strings.TrimSpace(clientSeed) == "" {
		return 0
	}
	hash := sha256.Sum256([]byte(clientSeed))
	return int(binary.BigEndian.Uint64(hash[:8]) % uint64(codexWindow40Budget))
}

func uuidv7TimestampMSFromSeed(seed string) uint64 {
	h := sha256.Sum256([]byte(seed))
	n := binary.BigEndian.Uint64(h[:8])
	return uint64(uuidv7StableOriginMS) + (n % uint64(uuidv7StableSpanMS))
}

func formatUUIDBytes(b []byte) string {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		binary.BigEndian.Uint32(b[0:4]),
		binary.BigEndian.Uint16(b[4:6]),
		binary.BigEndian.Uint16(b[6:8]),
		binary.BigEndian.Uint16(b[8:10]),
		b[10:16])
}

// deriveStableUUIDv7 用固定时间戳和种子熵生成稳定的 UUIDv7。
// 同一时间戳的 session/window 看起来像官方客户端连续两次 Uuid::now_v7()。
func deriveStableUUIDv7(timestampMS uint64, entropySeed string) string {
	timestampMS &= 0xffffffffffff
	h := sha256.Sum256([]byte(entropySeed))
	var b [16]byte
	b[0] = byte(timestampMS >> 40)
	b[1] = byte(timestampMS >> 32)
	b[2] = byte(timestampMS >> 24)
	b[3] = byte(timestampMS >> 16)
	b[4] = byte(timestampMS >> 8)
	b[5] = byte(timestampMS)
	b[6] = (h[0] & 0x0f) | 0x70
	b[7] = h[1]
	b[8] = (h[2] & 0x3f) | 0x80
	copy(b[9:16], h[3:10])
	return formatUUIDBytes(b[:])
}

func assignWindow40ConversationIDs(ids *codexFingerprintIDs, accountSeed, clientSeed string) {
	if ids == nil {
		return
	}
	accountSeed = strings.TrimSpace(accountSeed)
	if accountSeed == "" {
		return
	}
	slot := resolveWindow40Slot(clientSeed)
	base := fmt.Sprintf("sub2api:codex-window40:v3:%s:%d", accountSeed, slot)
	ts := uuidv7TimestampMSFromSeed(base)
	ids.sessionID = deriveStableUUIDv7(ts, base+":session")
	ids.threadID = ids.sessionID
	// 官方客户端连续两次 Uuid::now_v7()，window 比 session 新 1ms。
	ids.windowID = deriveStableUUIDv7(ts+1, base+":window")
}

// codexFingerprintIDs 收敛后的完整 ID 集合。
// 由 resolveCodexFingerprintIDs 一次性生成，同一个实例在头改写和体改写之间共享，
// 确保所有载体中的 turn_id 等随机字段一致。
// resolveCodexFingerprintIDs 按收敛模式计算出站 ID 集合。
// clientSessionID 是客户端线程种子（Codex session 头、OpenCode/CodeBuddy 会话头或 prompt_cache_key）。
// 模式还可传入 conversation_id 或 prompt_cache_key，且只用于固定 8 槽哈希。
// 返回 nil 表示 off 模式，不需要改写。
// 注意：包含随机生成的 turn_id，调用方必须只调用一次并共享结果给头改写和体改写。
func resolveCodexFingerprintIDs(account *Account, clientSessionID string, mode codexFingerprintMode) *codexFingerprintIDs {
	if mode == codexFingerprintOff {
		return nil
	}

	now := codexFingerprintNow()
	ids := &codexFingerprintIDs{
		mode:                mode,
		turnStartedAtUnixMs: now.UnixMilli(),
	}

	seed, seeded := codexFingerprintSeed(account.Extra)
	if !seeded {
		return nil
	}
	ids.installationID = resolveConvergedInstallationID(account, seed)
	if ids.installationID == "" {
		return nil
	}

	switch mode {
	case codexFingerprintDevice:
		assignDeviceFillConversationIDs(ids, seed, clientSessionID)
		return ids

	case codexFingerprintSession:
		ids.sessionID = resolveConvergedSessionID(seed)
		ids.threadID = resolveConvergedThreadID(seed, clientSessionID)
		if ids.threadID == "" {
			ids.threadID = ids.sessionID
		}
		ids.turnID = uuid.Must(uuid.NewV7()).String()
		ids.clientRequestID = ids.turnID
		ids.protocolProfile = codexProtocolProfileName
		ids.windowID = ids.threadID + ":0"
		return ids

	case codexFingerprintWindow:
		ids.sessionID = resolveConvergedSessionID(seed)
		ids.threadID = resolveWindowThreadID(seed, clientSessionID, now)
		ids.turnID = uuid.Must(uuid.NewV7()).String()
		ids.clientRequestID = ids.turnID
		ids.protocolProfile = codexProtocolProfileName
		ids.windowID = ids.threadID + ":0"
		return ids

	case codexFingerprintWindow40:
		assignWindow40ConversationIDs(ids, seed, clientSessionID)
		ids.turnID = uuid.Must(uuid.NewV7()).String()
		ids.clientRequestID = ids.turnID
		ids.protocolProfile = codexProtocolProfileName
		return ids

	case codexFingerprintFull:
		ids.sessionID = resolveConvergedSessionID(seed)
		ids.threadID = ids.sessionID
		ids.turnID = uuid.Must(uuid.NewV7()).String()
		ids.clientRequestID = ids.turnID
		ids.protocolProfile = codexProtocolProfileName
		ids.windowID = ids.threadID + ":0"
		return ids
	}

	return nil
}

// resolveCodexCompatibleHeader accepts the current Codex HTTP header name and
// falls back to the legacy underscore form. The current spelling always wins.
func resolveCodexCompatibleHeader(h http.Header, currentName, legacyName string) string {
	if h == nil {
		return ""
	}
	if value := strings.TrimSpace(h.Get(currentName)); value != "" {
		return value
	}
	return strings.TrimSpace(h.Get(legacyName))
}

func resolveCodexSessionHeader(h http.Header) string {
	return resolveCodexCompatibleHeader(h, codexSessionHeader, legacyCodexSessionHeader)
}

func resolveCodexThreadHeader(h http.Header) string {
	return resolveCodexCompatibleHeader(h, codexThreadHeader, legacyCodexThreadHeader)
}

// normalizeCodexOAuthHeaders is the final ChatGPT/Codex upstream protocol
// boundary. Value derivation is deliberately separate: fingerprint off still
// normalizes names, while API-key/custom upstream builders do not call it.
func normalizeCodexOAuthHeaders(h http.Header, sessionID, threadID string) {
	if h == nil {
		return
	}
	deleteOpenAIHeaderEqualFold(h, codexSessionHeader)
	deleteOpenAIHeaderEqualFold(h, legacyCodexSessionHeader)
	deleteOpenAIHeaderEqualFold(h, codexThreadHeader)
	deleteOpenAIHeaderEqualFold(h, legacyCodexThreadHeader)
	if sessionID = strings.TrimSpace(sessionID); sessionID != "" {
		h.Set(codexSessionHeader, sessionID)
	}
	if threadID = strings.TrimSpace(threadID); threadID != "" {
		h.Set(codexThreadHeader, threadID)
	}
}

// extractClientSessionID returns the client's un-isolated conversation identity.
// It uses the same header list as sticky routing so OpenCode/CodeBuddy session
// headers participate in outbound Codex identity, not just account pinning.
func extractClientSessionID(h http.Header) string {
	if h == nil {
		return ""
	}
	for _, name := range explicitOpenAIHeaderSessionNames {
		if value := strings.TrimSpace(h.Get(name)); value != "" {
			return value
		}
	}
	return ""
}

func extractClientThreadSeed(h http.Header, promptCacheKey string) string {
	if sessionID := extractClientSessionID(h); sessionID != "" {
		return sessionID
	}
	return strings.TrimSpace(promptCacheKey)
}

func codexFingerprintTenantSeed(c *gin.Context) string {
	if c == nil {
		return ""
	}
	value, _ := c.Get("api_key")
	apiKey, ok := value.(*APIKey)
	if !ok || apiKey == nil || apiKey.ID <= 0 {
		return ""
	}
	return fmt.Sprintf("u%d:k%d", apiKey.UserID, apiKey.ID)
}

func combineCodexFingerprintSeed(tenantSeed, clientSeed string) string {
	tenantSeed = strings.TrimSpace(tenantSeed)
	clientSeed = strings.TrimSpace(clientSeed)
	if tenantSeed == "" {
		return clientSeed
	}
	if clientSeed == "" {
		return tenantSeed
	}
	return tenantSeed + ":" + clientSeed
}

// assignDeviceFillConversationIDs derives conversation-stable Codex wire IDs for
// device mode. They are only applied when the client omitted session/thread/window.
func assignDeviceFillConversationIDs(ids *codexFingerprintIDs, seed, clientSessionID string) {
	if ids == nil {
		return
	}
	seed = strings.TrimSpace(seed)
	clientSessionID = strings.TrimSpace(clientSessionID)
	if seed == "" || clientSessionID == "" {
		return
	}
	ids.sessionID = deriveStableUUIDv4("sub2api:codex-session-id:device-fill:v2:" + seed + ":" + clientSessionID)
	ids.threadID = deriveStableUUIDv4("sub2api:codex-thread-id:device-fill:v2:" + seed + ":" + clientSessionID)
	ids.windowID = ids.threadID + ":0"
	ids.turnID = uuid.Must(uuid.NewV7()).String()
	ids.clientRequestID = ids.turnID
	ids.protocolProfile = codexProtocolProfileName
}

// fillMissingCodexConversationHeaders writes snapshot session/thread/window only
// when the outbound request does not already have those Codex wire headers.
func fillMissingCodexConversationHeaders(h http.Header, ids *codexFingerprintIDs) bool {
	if h == nil || ids == nil {
		return false
	}
	filled := false
	sessionID := resolveCodexSessionHeader(h)
	threadID := resolveCodexThreadHeader(h)
	sessionMissing := sessionID == "" && strings.TrimSpace(ids.sessionID) != ""
	threadMissing := threadID == "" && strings.TrimSpace(ids.threadID) != ""
	if sessionMissing {
		sessionID = ids.sessionID
	}
	if threadMissing {
		threadID = ids.threadID
	}
	if sessionMissing || threadMissing {
		normalizeCodexOAuthHeaders(h, sessionID, threadID)
		filled = true
	}
	if strings.TrimSpace(h.Get("x-codex-window-id")) == "" && strings.TrimSpace(ids.windowID) != "" {
		h.Set("x-codex-window-id", ids.windowID)
		filled = true
	}
	if strings.TrimSpace(h.Get("x-client-request-id")) == "" && strings.TrimSpace(ids.clientRequestID) != "" {
		h.Set("x-client-request-id", ids.clientRequestID)
		filled = true
	}
	return filled
}

// resolveCodexFingerprintIDsFromRequest 从客户端原始请求头中提取 session-id，
// 结合账号配置一次性解析收敛 ID 集合。调用方应将返回的 ids 同时传给
// applyCodexFingerprintHeaders 和 applyCodexFingerprintClientMetadata。
func resolveCodexFingerprintIDsFromRequest(account *Account, clientHeaders http.Header) *codexFingerprintIDs {
	return resolveCodexFingerprintIDsFromRequestWithPromptCacheKey(account, clientHeaders, "")
}

func resolveCodexFingerprintIDsFromRequestWithPromptCacheKey(account *Account, clientHeaders http.Header, promptCacheKey string) *codexFingerprintIDs {
	return resolveCodexFingerprintIDsFromContext(account, nil, clientHeaders, promptCacheKey)
}

func resolveCodexFingerprintIDsFromContext(account *Account, c *gin.Context, clientHeaders http.Header, promptCacheKey string) *codexFingerprintIDs {
	if account == nil {
		return nil
	}
	mode := account.GetCodexFingerprintMode()
	if mode == codexFingerprintOff {
		return nil
	}
	clientSessionID := extractClientThreadSeed(clientHeaders, promptCacheKey)
	if mode == codexFingerprintWindow40 {
		clientSessionID = combineCodexFingerprintSeed(codexFingerprintTenantSeed(c), clientSessionID)
	}
	ids := resolveCodexFingerprintIDs(account, clientSessionID, mode)
	adoptOfficialCodexInstallationID(account, clientHeaders, ids)
	return ids
}

func codexFingerprintIDsFromContext(c *gin.Context) *codexFingerprintIDs {
	if c == nil {
		return nil
	}
	value, ok := c.Get(codexFingerprintIDsContextKey)
	if !ok {
		return nil
	}
	ids, _ := value.(*codexFingerprintIDs)
	return ids
}

var codexClientMetadataAllowedKeys = map[string]struct{}{
	"session_id": {}, "thread_id": {}, "turn_id": {},
	"x-codex-installation-id": {}, "x-codex-window-id": {},
	"x-codex-turn-metadata": {}, "sandbox": {}, "thread_source": {},
	"turn_started_at_unix_ms":                                  {},
	"ws_request_header_x_openai_internal_codex_responses_lite": {},
	"x-codex-ws-stream-request-start-ms":                       {},
}

var codexTurnMetadataAllowedKeys = map[string]struct{}{
	"installation_id": {}, "session_id": {}, "thread_id": {}, "turn_id": {},
	"window_id": {}, "turn_started_at_unix_ms": {}, "sandbox": {}, "thread_source": {},
}

// sanitizeCodexMetadataMap is the fail-closed boundary for client-controlled
// Codex metadata. Workspace, VCS, OS/arch, terminal, plugin/skill/MCP and
// tracing fields must not become account or deployment fingerprints.
func sanitizeCodexMetadataMap(metadata map[string]any, allowed map[string]struct{}) bool {
	if metadata == nil {
		return false
	}
	modified := false
	for key := range metadata {
		if _, ok := allowed[key]; !ok {
			delete(metadata, key)
			modified = true
		}
	}
	return modified
}

func sanitizeCodexTurnMetadataValue(value any) (any, bool) {
	raw, ok := value.(string)
	if !ok || strings.TrimSpace(raw) == "" {
		return value, false
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		// Opaque legacy turn-state values are handled by the provenance guard.
		return value, false
	}
	modified := sanitizeCodexMetadataMap(metadata, codexTurnMetadataAllowedKeys)
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return value, false
	}
	if string(encoded) != raw {
		modified = true
	}
	return string(encoded), modified
}

func sanitizeCodexClientMetadata(reqBody map[string]any) bool {
	if reqBody == nil {
		return false
	}
	value, exists := reqBody["client_metadata"]
	if !exists {
		return false
	}
	metadata, ok := value.(map[string]any)
	if !ok {
		delete(reqBody, "client_metadata")
		return true
	}
	modified := sanitizeCodexMetadataMap(metadata, codexClientMetadataAllowedKeys)
	if raw, ok := metadata["x-codex-turn-metadata"]; ok {
		if sanitized, changed := sanitizeCodexTurnMetadataValue(raw); changed {
			metadata["x-codex-turn-metadata"] = sanitized
			modified = true
		}
	}
	return modified
}

func sanitizeCodexOAuthHeaders(h http.Header) {
	if h == nil {
		return
	}
	for _, name := range []string{
		"cookie", "set-cookie", "traceparent", "tracestate", "baggage",
		"x-b3-traceid", "x-b3-spanid", "x-b3-parentspanid", "x-b3-sampled",
		"x-amzn-trace-id", "x-cloud-trace-context", "x-request-start",
		"x-request-timeout", "x-stainless-timeout",
		"x-forwarded-for", "x-forwarded-host", "x-forwarded-proto", "x-real-ip",
		"forwarded", "via", "cf-connecting-ip", "true-client-ip",
	} {
		deleteOpenAIHeaderEqualFold(h, name)
	}
}

func applyCodexFingerprintToRawBody(body []byte, ids *codexFingerprintIDs) ([]byte, error) {
	if len(body) == 0 {
		return body, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	modified := sanitizeCodexClientMetadata(payload)
	if applyCodexFingerprintClientMetadata(payload, ids) {
		modified = true
	}
	if sanitizeOpenAIOutboundBrandMarkers(payload) {
		modified = true
	}
	if !modified {
		return body, nil
	}
	return json.Marshal(payload)
}

// codexFingerprintTurnRewriteFields 只改写身份字段。window40 不覆盖客户端已有的
// sandbox/thread_source；缺省时再补官方桌面端普通用户的环境值。
func codexFingerprintTurnRewriteFields(ids *codexFingerprintIDs) map[string]any {
	if ids == nil {
		return nil
	}
	fields := map[string]any{
		"installation_id":         ids.installationID,
		"session_id":              ids.sessionID,
		"thread_id":               ids.threadID,
		"turn_id":                 ids.turnID,
		"window_id":               ids.windowID,
		"turn_started_at_unix_ms": ids.turnStartedAtUnixMs,
	}
	if ids.mode != codexFingerprintWindow40 {
		fields["sandbox"] = "seccomp"
		fields["thread_source"] = "cli"
	}
	return fields
}

func codexMetadataFieldEmpty(v any) bool {
	if v == nil {
		return true
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s) == ""
	}
	return false
}

func applyWindow40PersonaDefaults(dst map[string]any) {
	if dst == nil {
		return
	}
	if codexMetadataFieldEmpty(dst["sandbox"]) {
		dst["sandbox"] = codexWindow40DefaultSandbox
	}
	if codexMetadataFieldEmpty(dst["thread_source"]) {
		dst["thread_source"] = codexWindow40DefaultThreadSource
	}
}

func mergeCodexTurnMetadataFields(metadata map[string]any, fields map[string]any, ids *codexFingerprintIDs) map[string]any {
	if metadata == nil {
		metadata = make(map[string]any)
	}
	for k, v := range fields {
		metadata[k] = v
	}
	if ids != nil && ids.mode == codexFingerprintWindow40 {
		applyWindow40PersonaDefaults(metadata)
	}
	return metadata
}

func encodeCodexTurnMetadata(metadata map[string]any) (string, bool) {
	rebuilt, err := json.Marshal(metadata)
	if err != nil {
		return "", false
	}
	return string(rebuilt), true
}

// applyCodexFingerprintHeaders 按预计算的收敛 ID 改写出站 HTTP 头中的设备指纹。
// 在 buildUpstreamRequest 的白名单透传之后、enforceCodexIdentityHeaders 之前调用。
func applyCodexFingerprintHeaders(h http.Header, ids *codexFingerprintIDs) {
	if h == nil || ids == nil {
		return
	}

	// 所有非 off 模式都收敛 installation_id
	h.Set("x-codex-installation-id", ids.installationID)

	if ids.mode == codexFingerprintDevice {
		fields := map[string]any{
			"installation_id": ids.installationID,
		}
		if fillMissingCodexConversationHeaders(h, ids) {
			if sessionID := resolveCodexSessionHeader(h); sessionID != "" {
				fields["session_id"] = sessionID
			}
			if threadID := resolveCodexThreadHeader(h); threadID != "" {
				fields["thread_id"] = threadID
			}
			if windowID := strings.TrimSpace(h.Get("x-codex-window-id")); windowID != "" {
				fields["window_id"] = windowID
			}
			if ids.turnID != "" {
				fields["turn_id"] = ids.turnID
				fields["turn_started_at_unix_ms"] = ids.turnStartedAtUnixMs
			}
		}
		rewriteCodexTurnMetadataFields(h, fields)
		return
	}

	// session / window / window40 / full 模式：改写所有相关头
	h.Set("x-codex-window-id", ids.windowID)
	h.Set("x-client-request-id", ids.clientRequestID)
	normalizeCodexOAuthHeaders(h, ids.sessionID, ids.threadID)
	h.Set("conversation_id", ids.sessionID)

	if ids.mode == codexFingerprintWindow40 {
		ensureCodexTurnMetadataHeader(h, ids)
	} else {
		rewriteCodexTurnMetadataFields(h, codexFingerprintTurnRewriteFields(ids))
	}
}

// rewriteCodexTurnMetadataFields 解析 x-codex-turn-metadata 头中的 JSON，
// 替换指定字段后回写。保留未指定字段原样（如 sandbox、thread_source 等）。
func rewriteCodexTurnMetadataFields(h http.Header, fields map[string]any) {
	writeCodexTurnMetadataHeader(h, fields, nil, false)
}

func ensureCodexTurnMetadataHeader(h http.Header, ids *codexFingerprintIDs) {
	writeCodexTurnMetadataHeader(h, codexFingerprintTurnRewriteFields(ids), ids, true)
}

func writeCodexTurnMetadataHeader(h http.Header, fields map[string]any, ids *codexFingerprintIDs, synthesize bool) {
	if h == nil {
		return
	}
	raw := strings.TrimSpace(h.Get("x-codex-turn-metadata"))
	var metadata map[string]any
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
			// Explicit sanitize policy: malformed managed metadata is removed rather
			// than being combined with a newly finalized outer identity.
			deleteOpenAIHeaderEqualFold(h, "x-codex-turn-metadata")
			if !synthesize {
				return
			}
		}
	}
	if metadata == nil {
		if !synthesize {
			return
		}
		metadata = make(map[string]any)
	}
	metadata = mergeCodexTurnMetadataFields(metadata, fields, ids)
	if encoded, ok := encodeCodexTurnMetadata(metadata); ok {
		h.Set("x-codex-turn-metadata", encoded)
	}
}

// applyCodexFingerprintClientMetadata 按预计算的收敛 ID 改写请求体中的 client_metadata。
// 使用与头改写相同的 ids 实例，确保 turn_id 等随机字段一致。
func applyCodexFingerprintClientMetadata(reqBody map[string]any, ids *codexFingerprintIDs) bool {
	if reqBody == nil || ids == nil {
		return false
	}

	existing, _ := reqBody["client_metadata"].(map[string]any)
	if existing == nil {
		existing = make(map[string]any)
	}

	modified := false

	if ids.installationID != "" {
		existing["x-codex-installation-id"] = ids.installationID
		modified = true
	}

	if ids.mode == codexFingerprintDevice {
		rewriteClientMetadataEmbeddedTurnMetadata(existing, map[string]any{
			"installation_id": ids.installationID,
		})
		if modified {
			reqBody["client_metadata"] = existing
		}
		return modified
	}

	// session / window / window40 / full 模式
	existing["session_id"] = ids.sessionID
	existing["thread_id"] = ids.threadID
	if ids.sessionID != "" {
		reqBody["prompt_cache_key"] = ids.sessionID
	}
	existing["turn_id"] = ids.turnID
	existing["x-codex-window-id"] = ids.windowID
	if ids.mode == codexFingerprintWindow40 {
		applyWindow40PersonaDefaults(existing)
		ensureClientMetadataEmbeddedTurnMetadata(existing, ids)
	} else {
		existing["sandbox"] = "seccomp"
		existing["thread_source"] = "cli"
		rewriteClientMetadataEmbeddedTurnMetadata(existing, codexFingerprintTurnRewriteFields(ids))
	}

	reqBody["client_metadata"] = existing
	return true
}

// rewriteClientMetadataEmbeddedTurnMetadata 改写 client_metadata 中内嵌的
// x-codex-turn-metadata JSON 字符串里的指定字段。
func rewriteClientMetadataEmbeddedTurnMetadata(clientMetadata map[string]any, fields map[string]any) {
	raw, ok := clientMetadata["x-codex-turn-metadata"].(string)
	if !ok || raw == "" {
		return
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		// Keep the body policy aligned with the header policy: malformed managed
		// metadata is explicitly sanitized, never silently retained.
		delete(clientMetadata, "x-codex-turn-metadata")
		return
	}
	for k, v := range fields {
		metadata[k] = v
	}
	if rebuilt, err := json.Marshal(metadata); err == nil {
		clientMetadata["x-codex-turn-metadata"] = string(rebuilt)
	}
}

func ensureClientMetadataEmbeddedTurnMetadata(clientMetadata map[string]any, ids *codexFingerprintIDs) {
	if clientMetadata == nil || ids == nil {
		return
	}
	raw, _ := clientMetadata["x-codex-turn-metadata"].(string)
	var metadata map[string]any
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
			delete(clientMetadata, "x-codex-turn-metadata")
			metadata = nil
		}
	}
	metadata = mergeCodexTurnMetadataFields(metadata, codexFingerprintTurnRewriteFields(ids), ids)
	if encoded, ok := encodeCodexTurnMetadata(metadata); ok {
		clientMetadata["x-codex-turn-metadata"] = encoded
	}
}

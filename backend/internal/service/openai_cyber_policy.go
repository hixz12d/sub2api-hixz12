package service

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// opsCyberPolicyKey 在 gin context 中携带 cyber_policy 命中标记。
// 由 gateway 服务层在检测到上游 error.code=="cyber_policy" 时设置，
// handler 在 Forward 返回后读取以触发风控记录、邮件与 tokens=0 用量行。
const opsCyberPolicyKey = "ops_cyber_policy"

// errOpenAICyberPolicyForwarded 表示 cyber_policy 已按当前端点格式透传给客户端
// （error 已写出/下发）。compat 路径 ForwardAsChatCompletions / ForwardAsAnthropic 出口
// 据此丢弃 result 并返回该哨兵，使 handler 落入 tokens=0 免费用量行（对齐 /v1/responses），
// 既不计费、也不 failover、不重复写响应。
var errOpenAICyberPolicyForwarded = errors.New("openai cyber_policy forwarded to client")

// CyberPolicyMark 记录一次 cyber_policy 硬阻断的上游证据。
type CyberPolicyMark struct {
	Code           string // 固定 "cyber_policy"
	Message        string // 上游 error.message
	Body           string // 上游 response.failed / 400 原始 body（已截断；未脱敏，ops_error 落库由 sanitizeErrorBodyForStorage、风控日志由 redactContentModerationSecrets 统一脱敏）
	UpstreamStatus int    // 上游 HTTP 状态（流式=200，非流式=400）
	UpstreamInTok  int    // 上游已报 input tokens（如有）
	UpstreamOutTok int    // 上游已报 output tokens（如有）
}

// MarkOpsCyberPolicy 记录 cyber 标记；首个写入生效，后续忽略（同一 turn 只记一次）。
// WS 多轮场景由 handler 在每个 turn 结束后调用 ClearOpsCyberPolicy 重置。
func MarkOpsCyberPolicy(c *gin.Context, mark CyberPolicyMark) {
	if c == nil {
		return
	}
	if GetOpsCyberPolicy(c) != nil {
		return
	}
	mark.Code = "cyber_policy"
	mark.Message = strings.TrimSpace(mark.Message)
	mark.Body = strings.TrimSpace(mark.Body)
	c.Set(opsCyberPolicyKey, &mark)
}

// GetOpsCyberPolicy 返回 cyber 标记，未命中（或已被 Clear）返回 nil。
func GetOpsCyberPolicy(c *gin.Context) *CyberPolicyMark {
	if c == nil {
		return nil
	}
	if v, ok := c.Get(opsCyberPolicyKey); ok {
		if m, ok := v.(*CyberPolicyMark); ok && m != nil {
			return m
		}
	}
	return nil
}

// ClearOpsCyberPolicy 清除 cyber 标记（typed-nil 覆盖；gin context 无并发安全的
// 删除原语，Set 走内部锁，与异步 GetOpsCyberPolicy 不构成 data race）。
// 仅 WS 多轮路径在 turn 收尾调用；HTTP 单请求路径不调用（context 随请求销毁，
// 且中间件 shouldSkipOpsErrorLogForCyber 依赖标记防双写）。
// WS 路径 clear 发生在中间件收尾之前，连接响应状态为 101，不触发中间件 status>=400
// 落库分支，故无双写/漏写。
func ClearOpsCyberPolicy(c *gin.Context) {
	if c == nil {
		return
	}
	c.Set(opsCyberPolicyKey, (*CyberPolicyMark)(nil))
}

// detectOpenAICyberPolicy 精确识别 cyber_policy（对齐 codex api_bridge.rs:145 /
// sse/responses.rs:529）。命中返回 (true, "cyber_policy", message)。
func detectOpenAICyberPolicy(payload []byte) (bool, string, string) {
	code := gjson.GetBytes(payload, "error.code").String()
	if code == "" {
		code = gjson.GetBytes(payload, "response.error.code").String()
	}
	if !strings.EqualFold(strings.TrimSpace(code), "cyber_policy") {
		return false, "", ""
	}
	msg := gjson.GetBytes(payload, "error.message").String()
	if msg == "" {
		msg = gjson.GetBytes(payload, "response.error.message").String()
	}
	return true, "cyber_policy", strings.TrimSpace(msg)
}

// softCyberPolicyClientMessage is the operator-facing recover hint used when a
// Super-Instruct / Portable-Kit account hits upstream cyber_policy. Keeps the
// failure visible but steers the client toward skill/SSOT reopen instead of
// Trusted Access / "flagged" panic text.
func softCyberPolicyClientMessage() string {
	return "[RECOVER] upstream policy hit this turn. Persist progress to skill/SSOT and notes/ACTIVE_STATE.md, reopen a fresh window, resume from the next unverified step. Do not resend the same long payload in-chat."
}

// shouldSoftenCyberPolicyClientText reports whether client-visible cyber text
// should be rewritten toward the recover card (SI default + portable-kit).
func shouldSoftenCyberPolicyClientText(account *Account) bool {
	return account != nil && account.IsSuperInstructEnabled()
}

// cyberPolicyClientMessage picks soft recover text for SI/Kit accounts.
func cyberPolicyClientMessage(account *Account, upstreamMsg string) string {
	if shouldSoftenCyberPolicyClientText(account) {
		return softCyberPolicyClientMessage()
	}
	msg := strings.TrimSpace(upstreamMsg)
	if msg == "" {
		return "Request blocked by upstream cyber-security policy"
	}
	return msg
}

// softenCyberPolicyClientPayload rewrites client-visible cyber error.message
// fields in common OpenAI shapes (top-level error, response.error, SSE
// response.failed). Preserves error.code=cyber_policy for detectors/ops.
// Returns (payload, false) when not a cyber_policy body or rewrite fails.
func softenCyberPolicyClientPayload(payload []byte) ([]byte, bool) {
	if len(payload) == 0 {
		return payload, false
	}
	hit, _, _ := detectOpenAICyberPolicy(payload)
	if !hit {
		return payload, false
	}
	msg := softCyberPolicyClientMessage()
	out := payload
	changed := false
	for _, path := range []string{
		"error.message",
		"response.error.message",
	} {
		if !gjson.GetBytes(out, strings.TrimSuffix(path, ".message")).Exists() &&
			!gjson.GetBytes(out, path).Exists() {
			// Still try Set when parent error object exists under alternate path.
		}
		if gjson.GetBytes(out, path).Exists() ||
			gjson.GetBytes(out, strings.TrimSuffix(path, ".message")).Exists() {
			if b, err := sjson.SetBytes(out, path, msg); err == nil {
				out = b
				changed = true
			}
		}
	}
	// Ensure code stays cyber_policy when an error object is present.
	for _, codePath := range []string{"error.code", "response.error.code"} {
		parent := strings.TrimSuffix(codePath, ".code")
		if gjson.GetBytes(out, parent).Exists() && !gjson.GetBytes(out, codePath).Exists() {
			if b, err := sjson.SetBytes(out, codePath, "cyber_policy"); err == nil {
				out = b
				changed = true
			}
		}
	}
	if !changed {
		// Fallback envelope if shape is unexpected but detector matched.
		payload, err := json.Marshal(gin.H{
			"error": gin.H{
				"type":    "invalid_request_error",
				"code":    "cyber_policy",
				"message": msg,
			},
		})
		if err != nil {
			return payload, false
		}
		return payload, true
	}
	return out, true
}

// maybeSoftenCyberPolicyClientPayload applies soften only for SI/Kit accounts.
func maybeSoftenCyberPolicyClientPayload(account *Account, payload []byte) []byte {
	if !shouldSoftenCyberPolicyClientText(account) || len(payload) == 0 {
		return payload
	}
	if rewritten, ok := softenCyberPolicyClientPayload(payload); ok && len(rewritten) > 0 {
		return rewritten
	}
	return payload
}

// rewriteCyberPolicyClientBody rewrites client-visible error.message fields while
// preserving error.code=cyber_policy so existing detectors/ops still match.
func rewriteCyberPolicyClientBody(body []byte, status int) []byte {
	_ = status
	if rewritten, ok := softenCyberPolicyClientPayload(body); ok {
		return rewritten
	}
	return nil
}

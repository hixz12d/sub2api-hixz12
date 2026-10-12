package service

import infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"

const openAICodexRelayFastBillingKey = "openai_codex_relay_fast_billing"

// IsOpenAICodexRelayFastBillingEnabled 仅允许显式开启的 OpenAI API Key 中转账号
// 按发出的 Fast 档位计费；不改变鉴权、转发协议或普通请求的档位。
func (a *Account) IsOpenAICodexRelayFastBillingEnabled() bool {
	if a == nil || !a.IsOpenAIApiKey() {
		return false
	}
	enabled, _ := a.Extra[openAICodexRelayFastBillingKey].(bool)
	return enabled
}

// validateOpenAICodexRelayFastBillingExtra 守住所有管理端写入入口；缺省关闭，
// 关闭值可用于清理旧配置，启用值仅接受 OpenAI API Key 账号。
func validateOpenAICodexRelayFastBillingExtra(platform, accountType string, extra map[string]any) error {
	raw, present := extra[openAICodexRelayFastBillingKey]
	if !present {
		return nil
	}
	enabled, ok := raw.(bool)
	if !ok {
		return infraerrors.BadRequest("OPENAI_CODEX_RELAY_FAST_BILLING_INVALID", openAICodexRelayFastBillingKey+" must be a boolean")
	}
	if enabled && (platform != PlatformOpenAI || accountType != AccountTypeAPIKey) {
		return infraerrors.BadRequest("OPENAI_CODEX_RELAY_FAST_BILLING_ACCOUNT_INVALID", "Codex relay Fast billing requires an OpenAI API-key account")
	}
	return nil
}

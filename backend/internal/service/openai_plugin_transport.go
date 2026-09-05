package service

import (
	"errors"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

func (s *OpenAIGatewayService) SetPluginManager(manager *PluginManager) {
	s.pluginManager = manager
}

// doOpenAIUpstream 只在 OpenAI OAuth 能力绑定已启用时把真实请求交给插件。
// 插件返回标准 http.Response，响应解析、错误映射、SSE 和计费仍由现有核心链处理。
func (s *OpenAIGatewayService) doOpenAIUpstream(request *http.Request, proxyURL string, account *Account) (*http.Response, error) {
	state, hasState := CodexAttemptStateFromContext(request.Context())
	bundleRequest := hasState && state.Profile().BundleID != ""
	if bundleRequest {
		if account == nil || account.IsTLSFingerprintEnabled() {
			return nil, errors.New("shared client bundles require native transport; disable enable_tls_fingerprint")
		}
		if s.pluginManager != nil && s.pluginManager.ShouldRouteOpenAIOAuth(account) {
			return nil, errors.New("shared client bundles require the built-in HTTP sender; plugin transport is not verified")
		}
	}
	if err := applyCodexBundleAtDispatch(request); err != nil {
		return nil, err
	}
	if s.pluginManager != nil && !bundleRequest {
		response, handled, err := s.pluginManager.RoundTripOpenAIOAuth(request.Context(), request, proxyURL, account)
		if handled {
			return response, err
		}
	}
	if state, ok := CodexAttemptStateFromContext(request.Context()); ok && state.PolicyVersion() == CodexIdentityPolicyV2 {
		// Relay Kernel 的 Chrome/uTLS 只在账号显式允许 TLS 伪装时生效。
		// 关闭 enable_tls_fingerprint 后必须走普通握手，否则账号开关形同虚设。
		if account.IsTLSFingerprintEnabled() {
			profile := state.Profile()
			if profile.Transport.TLSProfileID == tlsfingerprint.HelloPresetChromeAuto {
				tlsProfile := tlsfingerprint.BuiltinChromeAutoProfile()
				tlsProfile.CacheScopeKey = state.TransportKey()
				return s.httpUpstream.DoWithTLS(request, proxyURL, account.ID, account.Concurrency, tlsProfile)
			}
		}
	}
	return s.httpUpstream.Do(request, proxyURL, account.ID, account.Concurrency)
}

// doOpenAIAccountTestUpstream 让 OpenAI OAuth 账号测试与真实转发使用同一插件路径。
// API Key 和未命中插件的账号保持各自原有的 HTTPUpstream 行为。
func (s *AccountTestService) doOpenAIAccountTestUpstream(
	request *http.Request,
	proxyURL string,
	account *Account,
	useTLSFallback bool,
) (*http.Response, error) {
	if s.pluginManager != nil {
		response, handled, err := s.pluginManager.RoundTripOpenAIOAuth(request.Context(), request, proxyURL, account)
		if handled {
			return response, err
		}
	}
	if useTLSFallback {
		return s.httpUpstream.DoWithTLS(
			request,
			proxyURL,
			account.ID,
			account.Concurrency,
			s.tlsFPProfileService.ResolveTLSProfile(account),
		)
	}
	return s.httpUpstream.Do(request, proxyURL, account.ID, account.Concurrency)
}

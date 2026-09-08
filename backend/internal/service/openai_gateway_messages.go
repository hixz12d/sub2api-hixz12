package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

// ForwardAsAnthropic accepts an Anthropic Messages request body, converts it
// to OpenAI Responses API format, forwards to the OpenAI upstream, and converts
// the response back to Anthropic Messages format. This enables Claude Code
// clients to access OpenAI models through the standard /v1/messages endpoint.
func (s *OpenAIGatewayService) ForwardAsAnthropic(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	promptCacheKey string,
	defaultMappedModel string,
) (out *OpenAIForwardResult, retErr error) {
	ctx = s.snapshotOpenAIOutboundIdentity(ctx, account, c.GetHeader("User-Agent"))
	beginUpstreamResponseModelObservation(c)
	if s.cfg != nil {
		c.Set(openAIRetryBudgetConfigKey, s.cfg)
	}
	EnsureOpenAIRetryBudget(c, account, body)
	requestCtx := ctx
	preparationCtx, releasePreparation := OpenAIRecoveryPreparationContext(c, requestCtx, account)
	ctx = preparationCtx
	preparing := true
	defer func() {
		if preparing && errors.Is(context.Cause(preparationCtx), ErrOpenAIRecoveryDeadline) {
			out, retErr = nil, openAIOutputPhaseFailure(c, ErrOpenAIRecoveryDeadline, nil)
		}
		releasePreparation()
	}()
	if err := ctx.Err(); err != nil && ctx.Value(openAIRecoveryPreparationKey{}) == true {
		return nil, err
	}
	ClearActualOpenAIUpstreamEndpoint(c)
	if shouldForwardOpenAIResponsesViaRawChatCompletions(account) {
		SetActualOpenAIUpstreamEndpoint(c, "/v1/chat/completions")
	}
	setCodexToolNameReverse(c, nil)
	if _, err := s.prepareCodexAccountIdentitySource(ctx, c, account); err != nil {
		return nil, err
	}

	// 入口分流（国产供应商 Anthropic 协议）：上游为供应商原生 Anthropic 端点时，
	// /v1/messages 请求零转换直通（仅模型名映射 + 少量 body 清洗），完整保留
	// thinking / tool_use / cache 语义，适配 Claude Code 等原生客户端。
	// 必须先于 ShouldUseResponsesAPI 分流：Anthropic 协议账号经 probe 落标
	// openai_responses_supported=false，会先命中下方的 CC 直转分支。
	if account.IsAnthropicProtocol() || account.IsAdaptiveAPIProtocol() {
		return s.forwardAnthropicViaNativeAnthropicEndpoint(ctx, c, account, body, defaultMappedModel)
	}

	// 固定 chat_completions 的 CN 账号，以及不支持 Responses 的其他 APIKey
	// 账号，均将 Messages 转为 CC；固定 responses 的 CN 账号不受探针旧值覆盖。
	if shouldForwardOpenAIResponsesViaRawChatCompletions(account) {
		return s.forwardAnthropicViaRawChatCompletions(ctx, c, account, body, defaultMappedModel)
	}

	startTime := time.Now()

	// 1. Parse Anthropic request
	var anthropicReq apicompat.AnthropicRequest
	if err := json.Unmarshal(body, &anthropicReq); err != nil {
		return nil, fmt.Errorf("parse anthropic request: %w", err)
	}
	anthropicDigestReq := cloneAnthropicRequestForDigest(&anthropicReq)
	originalModel := anthropicReq.Model
	applyOpenAICompatModelNormalization(&anthropicReq)
	normalizedModel := anthropicReq.Model
	clientStream := anthropicReq.Stream // client's original stream preference

	// 2. Model mapping
	billingModel := resolveOpenAIForwardModel(account, normalizedModel, defaultMappedModel)
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)
	promptCacheKey = strings.TrimSpace(promptCacheKey)
	apiKeyID := getAPIKeyIDFromContext(c)
	anthropicDigestChain := ""
	anthropicMatchedDigestChain := ""
	compatPromptCacheInjected := false
	// Grok is outside the gpt-5/codex compat injector, but Claude Code still
	// carries a stable session id. Prefer that as the Grok prompt-cache seed so
	// multi-turn /v1/messages traffic can hit xAI's server-side cache.
	if promptCacheKey == "" && account.Platform == PlatformGrok {
		if sessionSeed := extractClaudeCodeSessionID(c, body); sessionSeed != "" {
			promptCacheKey = sessionSeed
			compatPromptCacheInjected = true
		} else if sessionSeed := promptCacheKeyFromAnthropicMetadataSession(&anthropicReq); sessionSeed != "" {
			promptCacheKey = sessionSeed
			compatPromptCacheInjected = true
		}
	}
	if promptCacheKey == "" && shouldAutoInjectPromptCacheKeyForCompat(upstreamModel) {
		promptCacheKey = promptCacheKeyFromAnthropicMetadataSession(&anthropicReq)
		if promptCacheKey == "" {
			promptCacheKey = deriveAnthropicCacheControlPromptCacheKey(&anthropicReq)
		}
		if promptCacheKey == "" {
			anthropicDigestChain = buildOpenAICompatAnthropicDigestChain(anthropicDigestReq)
			if reusedKey, matchedChain := s.findOpenAICompatAnthropicDigestPromptCacheKey(account, apiKeyID, anthropicDigestChain); reusedKey != "" {
				promptCacheKey = reusedKey
				anthropicMatchedDigestChain = matchedChain
			} else {
				promptCacheKey = promptCacheKeyFromAnthropicDigest(anthropicDigestChain)
			}
		}
		compatPromptCacheInjected = promptCacheKey != ""
	}
	compatReplayTrimmed := false
	compatReplayGuardEnabled := shouldAutoInjectPromptCacheKeyForCompat(upstreamModel)
	compatContinuationEnabled := openAICompatContinuationEnabled(account, upstreamModel)
	previousResponseID := ""
	if compatContinuationEnabled {
		previousResponseID = s.getOpenAICompatSessionResponseID(ctx, c, account, promptCacheKey)
	}
	compatContinuationDisabled := compatContinuationEnabled &&
		s.isOpenAICompatSessionContinuationDisabled(ctx, c, account, promptCacheKey)
	compatTurnState := ""
	// ChatGPT/Codex credentials rely on session_id + x-codex-turn-state; trimming to a
	// sliding 12-message window makes the cached prefix stall at system/tools.
	// Keep full replay there so upstream prompt caching can grow turn by turn.
	if compatReplayGuardEnabled && !account.UsesOpenAICodexProtocol() && previousResponseID == "" && !compatContinuationDisabled {
		compatReplayTrimmed = applyAnthropicCompatFullReplayGuard(&anthropicReq)
	}

	// 3. Convert Anthropic → Responses after compatibility-only replay guard.
	responsesReq, err := apicompat.AnthropicToResponses(&anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("convert anthropic to responses: %w", err)
	}

	// Upstream always uses streaming (upstream may not support sync mode).
	// The client's original preference determines the response format.
	responsesReq.Stream = true
	isStream := true

	// 3b. Handle BetaFastMode → service_tier: "priority"
	if containsBetaToken(c.GetHeader("anthropic-beta"), claude.BetaFastMode) {
		responsesReq.ServiceTier = "priority"
	}

	responsesReq.Model = upstreamModel
	if responsesReq.Reasoning != nil {
		responsesReq.Reasoning.Effort = openAICompatAnthropicReasoningEffort(&anthropicReq, upstreamModel, responsesReq.Reasoning.Effort)
	}
	if previousResponseID != "" {
		responsesReq.PreviousResponseID = previousResponseID
		trimAnthropicCompatResponsesInputToLatestTurn(responsesReq)
	}
	if compatReplayGuardEnabled && !account.UsesOpenAICodexProtocol() {
		appendOpenAICompatClaudeCodeTodoGuard(responsesReq)
	}

	logFields := []zap.Field{
		zap.Int64("account_id", account.ID),
		zap.String("original_model", originalModel),
		zap.String("normalized_model", normalizedModel),
		zap.String("billing_model", billingModel),
		zap.String("upstream_model", upstreamModel),
		zap.Bool("stream", isStream),
	}
	if compatPromptCacheInjected {
		logFields = append(logFields,
			zap.Bool("compat_prompt_cache_key_injected", true),
			zap.String("compat_prompt_cache_key_sha256", hashSensitiveValueForLog(promptCacheKey)),
		)
	}
	if compatReplayTrimmed {
		logFields = append(logFields,
			zap.Bool("compat_full_replay_trimmed", true),
			zap.Int("compat_messages_after_trim", len(anthropicReq.Messages)),
		)
	}
	if previousResponseID != "" {
		logFields = append(logFields,
			zap.Bool("compat_previous_response_id_attached", true),
			zap.String("compat_previous_response_id", openAIWSStateIDDigest(previousResponseID)),
		)
	}
	if compatTurnState != "" {
		logFields = append(logFields, zap.Bool("compat_turn_state_attached", true))
	}
	logger.L().Debug("openai messages: model mapping applied", logFields...)

	// 4. Marshal Responses request body, then apply the ChatGPT/Codex transform.
	responsesBody, err := json.Marshal(responsesReq)
	if err != nil {
		return nil, fmt.Errorf("marshal responses request: %w", err)
	}

	if account.UsesOpenAICodexProtocol() && account.Platform != PlatformGrok {
		var reqBody map[string]any
		if err := json.Unmarshal(responsesBody, &reqBody); err != nil {
			return nil, fmt.Errorf("unmarshal for codex transform: %w", err)
		}
		codexResult := applyCodexOAuthTransformWithOptions(reqBody, codexOAuthTransformOptions{
			SkipDefaultInstructions: true,
			PreserveToolCallIDs:     true,
		})
		if codexResult.Error != nil {
			writeAnthropicError(c, http.StatusBadRequest, "invalid_request_error", codexResult.Error.Error())
			return nil, codexResult.Error
		}
		setCodexToolNameReverse(c, codexResult.ToolNameReverse)
		forcedTemplateText := ""
		if s.cfg != nil {
			forcedTemplateText = s.cfg.Gateway.ForcedCodexInstructionsTemplate
		}
		templateUpstreamModel := upstreamModel
		if codexResult.NormalizedModel != "" {
			templateUpstreamModel = codexResult.NormalizedModel
		}
		existingInstructions, _ := reqBody["instructions"].(string)
		if strings.TrimSpace(existingInstructions) == "" {
			existingInstructions = extractPromptLikeInstructionsFromInput(reqBody)
		}
		if _, err := applyForcedCodexInstructionsTemplate(reqBody, forcedTemplateText, forcedCodexInstructionsTemplateData{
			ExistingInstructions: strings.TrimSpace(existingInstructions),
			OriginalModel:        originalModel,
			NormalizedModel:      normalizedModel,
			BillingModel:         billingModel,
			UpstreamModel:        templateUpstreamModel,
		}); err != nil {
			return nil, err
		}
		// Account-level Super-Instruct whitelist (extra.super_instruct=true).
		applyAccountSuperInstructBridgeFromConfig(reqBody, account, resolveSuperInstructBridgeFile(s.cfg))
		ensureCodexOAuthInstructionsField(reqBody)
		if shouldAutoInjectPromptCacheKeyForCompat(upstreamModel) {
			appendOpenAICompatClaudeCodeTodoGuardToRequestBody(reqBody)
		}
		if codexResult.NormalizedModel != "" {
			upstreamModel = codexResult.NormalizedModel
		}
		if codexResult.PromptCacheKey != "" {
			promptCacheKey = codexResult.PromptCacheKey
		}
		applyCodexAccountIdentityClientMetadataMap(reqBody, codexAccountIdentitySource(c, account), apiKeyID)
		delete(reqBody, "prompt_cache_key")
		if shouldAutoInjectPromptCacheKeyForCompat(upstreamModel) {
			compatTurnState = s.getOpenAICompatSessionTurnState(ctx, c, account, promptCacheKey)
		}
		// OAuth codex transform forces stream=true upstream, so always use
		// the streaming response handler regardless of what the client asked.
		isStream = true
		responsesBody, err = json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("remarshal after codex transform: %w", err)
		}
	}

	// For API key accounts (including OpenAI-compatible upstream gateways),
	// ensure promptCacheKey is also propagated via the request body so that
	// upstreams using the Responses API can derive a stable session identifier
	// from prompt_cache_key. This makes our Anthropic /v1/messages compatibility
	// path behave more like a native Responses client.
	if account.Type == AccountTypeAPIKey {
		if trimmedKey := strings.TrimSpace(promptCacheKey); trimmedKey != "" {
			var reqBody map[string]any
			if err := json.Unmarshal(responsesBody, &reqBody); err != nil {
				return nil, fmt.Errorf("unmarshal for prompt cache key injection: %w", err)
			}
			if existing, ok := reqBody["prompt_cache_key"].(string); !ok || strings.TrimSpace(existing) == "" {
				reqBody["prompt_cache_key"] = trimmedKey
				updated, err := json.Marshal(reqBody)
				if err != nil {
					return nil, fmt.Errorf("remarshal after prompt cache key injection: %w", err)
				}
				responsesBody = updated
			}
		}
	}
	if account.Platform == PlatformOpenAI {
		policyBody, changed, policyErr := ApplyOpenAIReasoningEffortPolicyFromContext(ctx, responsesBody)
		if policyErr != nil {
			var overLimit *ReasoningEffortOverLimitError
			if errors.As(policyErr, &overLimit) {
				MarkOpsClientBusinessLimited(c, OpsClientBusinessLimitedReasonLocalPolicyDenied)
				writeAnthropicError(c, http.StatusForbidden, "forbidden_error", overLimit.Error())
			}
			return nil, policyErr
		}
		if changed {
			responsesBody = policyBody
			if responsesReq.Reasoning != nil {
				responsesReq.Reasoning.Effort = gjson.GetBytes(responsesBody, "reasoning.effort").String()
			}
		}
	}

	// 4c. Apply OpenAI fast policy (may filter service_tier or block the request).
	// Mirrors the Claude anthropic-beta "fast-mode-2026-02-01" filter, but keyed
	// on the body-level service_tier field (priority/flex).
	updatedBody, policyErr := s.applyOpenAIFastPolicyToBody(ctx, account, upstreamModel, responsesBody)
	if policyErr != nil {
		var blocked *OpenAIFastBlockedError
		if errors.As(policyErr, &blocked) {
			MarkOpsClientBusinessLimited(c, OpsClientBusinessLimitedReasonLocalPolicyDenied)
			writeAnthropicError(c, http.StatusForbidden, "forbidden_error", blocked.Message)
		}
		return nil, policyErr
	}
	responsesBody = updatedBody
	responsesReq.ServiceTier = normalizedOpenAIServiceTierValue(gjson.GetBytes(responsesBody, "service_tier").String())
	grokCacheIdentity := ""
	if account.Platform == PlatformGrok {
		grokIntentBody := responsesBody
		grokCacheIdentity = resolveGrokCacheIdentity(c, grokIntentBody, promptCacheKey, upstreamModel)
		patchedBody, patchErr := patchGrokResponsesBody(grokIntentBody, upstreamModel)
		if patchErr != nil {
			return nil, patchErr
		}
		responsesBody, patchErr = applyGrokResponsesCacheIdentity(patchedBody, grokIntentBody, grokCacheIdentity, account.IsGrokOAuth())
		if patchErr != nil {
			return nil, fmt.Errorf("apply grok prompt cache identity: %w", patchErr)
		}
		responsesBody, patchErr = applyGrokFreeMessagesFunctionToolCacheRoute(responsesBody, grokIntentBody, account, grokCacheIdentity)
		if patchErr != nil {
			return nil, fmt.Errorf("apply grok Free function-tool cache route: %w", patchErr)
		}
	}

	if account.IsOpenAIOAuthLike() && account.Platform != PlatformGrok {
		var clientHeaders http.Header
		if c != nil && c.Request != nil {
			clientHeaders = c.Request.Header
		}
		ids, identityErr := s.finalizeCodexOAuthIdentity(account, c, clientHeaders, promptCacheKey)
		if identityErr != nil {
			return nil, fmt.Errorf("finalize Codex OAuth identity for messages bridge: %w", identityErr)
		}
		s.persistLearnedCodexDeviceID(ctx, account, ids)
		if c != nil {
			c.Set("codex_fingerprint_ids", ids)
		}
	}

	// 5. Get access token
	token, _, err := s.getRequestCredential(ctx, c, account)
	if err != nil {
		return nil, fmt.Errorf("get access token: %w", err)
	}

	// 6. Build upstream request
	if account.UsesOpenAICodexProtocol() && account.Platform != PlatformGrok {
		// Messages 兼容桥即使 body 未带 todo-guard/prompt_cache_key 标记（如映射到非
		// gpt-5/codex 模型），也必须让 buildUpstreamRequest 走 bridge 分支，以保留
		// 既有 body/session/conversation 行为。身份头在 post-build 阶段统一恢复。
		setOpenAICompatMessagesBridgeContext(c, true)
	}
	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	if ctx.Value(openAIRecoveryPreparationKey{}) == true {
		upstreamCtx = ctx
	}
	var upstreamReq *http.Request
	if account.Platform == PlatformGrok {
		upstreamReq, err = buildGrokResponsesRequest(upstreamCtx, c, account, responsesBody, token, grokCacheIdentity, s.cfg, s.settingService)
	} else {
		upstreamReq, err = s.buildUpstreamRequest(upstreamCtx, c, account, responsesBody, token, isStream, promptCacheKey, false)
	}
	releaseUpstreamCtx()
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}

	// API-key compatibility paths retain their legacy cache-key session header;
	// OAuth identity is finalized in buildUpstreamRequest so prompt_cache_key
	// and any client-provided session header share one boundary.
	if account.Platform != PlatformGrok && account.Type != AccountTypeOAuth && !usesCodexRelayKernel(account) && promptCacheKey != "" {
		isolatedSessionID := generateSessionUUID(isolateOpenAIUpstreamSessionID(apiKeyID, codexAccountIdentitySource(c, account), promptCacheKey))
		upstreamReq.Header.Set(legacyCodexSessionHeader, isolatedSessionID)
		if upstreamReq.Header.Get("conversation_id") != "" {
			upstreamReq.Header.Set("conversation_id", isolatedSessionID)
		}
	}
	if account.IsOpenAIOAuthLike() && account.Platform != PlatformGrok {
		// Preserve the bridge-specific beta declaration, then run the same final
		// identity boundary used by Responses, passthrough and WS.
		upstreamReq.Header.Set("OpenAI-Beta", codexHTTPBetaValue)
	}
	if compatTurnState != "" && upstreamReq.Header.Get("x-codex-turn-state") == "" {
		upstreamReq.Header.Set("x-codex-turn-state", compatTurnState)
	}
	if (account.Type == AccountTypeOAuth || usesCodexRelayKernel(account)) && account.Platform != PlatformGrok {
		s.finalizeCodexOAuthHeaders(ctx, c, account, upstreamReq.Header, codexFingerprintIDsFromContext(c), promptCacheKey)
		logger.L().Debug("openai messages: upstream identity restored",
			zap.Int64("account_id", account.ID),
			zap.String("upstream_model", upstreamModel),
			zap.Bool("compat_identity_restored", true),
		)
	}
	s.finalizeCodexAttemptHTTPWire(c, upstreamReq, nil)

	// 7. Send request
	proxyURL, err := s.resolveOpenAICompatibleProxyURL(ctx, account)
	if err != nil {
		return nil, err
	}
	// Grok may reject encrypted reasoning replayed under a different OAuth
	// account/cache identity. Match forwardGrokResponses: one strip+retry before
	// treating the 400 as a hard failure / failover trigger.
	var resp *http.Response
	phaseCtx := requestCtx
	var phaseGuard *openAIFirstOutputHeaderGuard
	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			if account.Platform != PlatformGrok {
				break
			}
			upstreamCtxRetry, releaseRetry := detachUpstreamContext(ctx)
			upstreamReq, err = buildGrokResponsesRequest(upstreamCtxRetry, c, account, responsesBody, token, grokCacheIdentity, s.cfg, s.settingService)
			releaseRetry()
			if err != nil {
				return nil, fmt.Errorf("build grok retry request: %w", err)
			}
		}
		if preparing {
			if err := preparationCtx.Err(); err != nil && preparationCtx.Value(openAIRecoveryPreparationKey{}) == true {
				return nil, err
			}
			releasePreparation()
			ctx, preparing = requestCtx, false
		}
		if clientStream && account.Platform == PlatformOpenAI {
			effort := ""
			if effective := extractOpenAIReasoningEffortFromBody(responsesBody, upstreamModel, billingModel, originalModel); effective != nil {
				effort = *effective
			}
			phaseCtx, phaseGuard, err = s.beginOpenAIHTTPOutputPhase(openAIPhaseParent(ctx, upstreamReq.Context()), c, account, effort)
			if err != nil {
				return nil, openAIOutputPhaseFailure(c, err, nil)
			}
			if phaseGuard != nil {
				defer phaseGuard.close()
				upstreamReq = upstreamReq.WithContext(phaseCtx)
			}
		}
		if reserveErr := ReserveOpenAIUpstreamAttempt(c, account.ID); reserveErr != nil {
			return nil, reserveErr
		}
		resp, err = s.doOpenAIUpstream(upstreamReq, proxyURL, account)
		if phaseGuard != nil && phaseGuard.failure() != nil {
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
			return nil, openAIOutputPhaseFailure(c, phaseGuard.failure(), nil)
		}
		if err != nil {
			transportErr := s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
			RecordOpenAIRetryFailure(c, 0, transportErr)
			return nil, transportErr
		}
		if account.Platform != PlatformGrok || attempt > 0 || resp.StatusCode != http.StatusBadRequest {
			break
		}
		respBody := s.readUpstreamErrorBody(resp)
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
		// Prefer explicit decrypt errors; also strip once on any 400 when the
		// outbound body still carries reasoning.encrypted_content (account
		// switch often returns opaque "Upstream error: 400").
		shouldStrip := isGrokInvalidEncryptedContentResponse(resp.StatusCode, respBody) ||
			requestHasGrokEncryptedReasoning(responsesBody)
		if !shouldStrip {
			resp.Body = io.NopCloser(bytes.NewReader(respBody))
			break
		}
		retryBody, changed, trimErr := trimGrokInvalidEncryptedContentRetryBody(responsesBody)
		if trimErr != nil {
			return nil, fmt.Errorf("prepare Grok invalid encrypted_content retry: %w", trimErr)
		}
		if !changed {
			resp.Body = io.NopCloser(bytes.NewReader(respBody))
			break
		}
		responsesBody = retryBody
		logger.L().Info("openai messages: retrying after stripping invalid Grok encrypted_content",
			zap.Int64("account_id", account.ID),
			zap.Bool("cache_identity_present", strings.TrimSpace(grokCacheIdentity) != ""),
			zap.String("upstream_error_preview", truncateOpenAIWSLogValue(string(respBody), 240)),
		)
	}
	defer func() { _ = resp.Body.Close() }()

	// 8. Handle error response with failover
	if resp.StatusCode >= 400 {
		respBody, upstreamMsg := s.readOpenAIUpstreamError(resp)
		_ = resp.Body.Close()
		if phaseGuard != nil {
			phaseGuard.close()
		}
		if !agentIdentityTaskRecoveryWasTried(ctx) && s.isAgentIdentityAccount(ctx, account) && isAgentIdentityTaskInvalidHTTPResponse(resp.StatusCode, respBody) {
			expectedTaskID := account.GetCredential("task_id")
			if err := s.recoverAgentIdentityTask(ctx, account, expectedTaskID); err != nil {
				return nil, fmt.Errorf("agent identity task recovery failed: %w", err)
			}
			return s.ForwardAsAnthropic(markAgentIdentityTaskRecoveryTried(ctx), c, account, body, promptCacheKey, defaultMappedModel)
		}
		if previousResponseID != "" && (isOpenAICompatPreviousResponseNotFound(resp.StatusCode, upstreamMsg, respBody) || isOpenAICompatPreviousResponseUnsupported(resp.StatusCode, upstreamMsg, respBody)) {
			if isOpenAICompatPreviousResponseUnsupported(resp.StatusCode, upstreamMsg, respBody) {
				s.disableOpenAICompatSessionContinuation(ctx, c, account, promptCacheKey)
			} else {
				s.deleteOpenAICompatSessionResponseID(ctx, c, account, promptCacheKey)
			}
			logger.L().Info("openai messages: previous_response_id unavailable, retrying without continuation",
				zap.Int64("account_id", account.ID),
				zap.String("previous_response_id", openAIWSStateIDDigest(previousResponseID)),
				zap.String("upstream_model", upstreamModel),
			)
			return s.ForwardAsAnthropic(ctx, c, account, body, promptCacheKey, defaultMappedModel)
		}
		// Grok account-switched history often fails decrypt; strip encrypted
		// reasoning once at the client-body level so failover accounts can accept
		// the multi-turn tool continuation instead of cascading 400s.
		if account.Platform == PlatformGrok &&
			isGrokInvalidEncryptedContentResponse(resp.StatusCode, respBody) &&
			!grokEncryptedContentStripRetried(ctx) {
			if strippedBody, ok := stripAnthropicThinkingSignatures(body); ok {
				logger.L().Info("openai messages: stripping thinking signatures for Grok failover retry",
					zap.Int64("account_id", account.ID),
				)
				return s.ForwardAsAnthropic(markGrokEncryptedContentStripRetried(ctx), c, account, strippedBody, promptCacheKey, defaultMappedModel)
			}
		}
		if foErr := s.failoverOpenAIUpstreamHTTPError(ctx, c, account, resp, respBody, upstreamMsg, upstreamModel); foErr != nil {
			RecordOpenAIRetryFailure(c, resp.StatusCode, foErr)
			return nil, foErr
		}
		// Non-failover error: return Anthropic-formatted error to client
		return s.handleAnthropicErrorResponse(resp, c, account, billingModel)
	}
	if account.Platform == PlatformGrok && account.Type == AccountTypeOAuth && !account.IsShadow() {
		s.updateGrokUsageFromResponse(withGrokTeamRateLimitModel(ctx, upstreamModel), account, resp.Header, resp.StatusCode)
	}

	if account.UsesOpenAICodexProtocol() && promptCacheKey != "" {
		if turnState := strings.TrimSpace(resp.Header.Get("x-codex-turn-state")); turnState != "" {
			s.bindOpenAICompatSessionTurnState(ctx, c, account, promptCacheKey, turnState)
		}
	}

	// 9. Handle normal response
	// Upstream is always streaming; choose response format based on client preference.
	var result *OpenAIForwardResult
	var handleErr error
	if clientStream {
		streamCtx := withOpenAIStreamProxyURL(phaseCtx, proxyURL)
		effort := ""
		if effective := extractOpenAIReasoningEffortFromBody(responsesBody, upstreamModel, billingModel, originalModel); effective != nil {
			effort = *effective
		}
		result, handleErr = s.handleAnthropicStreamingResponseWithReasoning(streamCtx, resp, c, account, originalModel, billingModel, upstreamModel, startTime, effort)
	} else {
		// Client wants JSON: buffer the streaming response and assemble a JSON reply.
		result, handleErr = s.handleAnthropicBufferedStreamingResponse(resp, c, account, originalModel, billingModel, upstreamModel, startTime)
	}

	if phaseGuard != nil && phaseGuard.timeoutFailure() != nil {
		return result, openAIOutputPhaseFailure(c, phaseGuard.timeoutFailure(), resp.Header)
	}

	// cyber_policy：标记已设、error 已按 Anthropic 格式发给客户端。丢弃 result、返回哨兵，
	// 使 handler 落入 tokens=0 免费用量行（对齐 /v1/responses），不计费、不 failover。
	if GetOpsCyberPolicy(c) != nil {
		if handleErr == nil {
			handleErr = errOpenAICyberPolicyForwarded
		}
		return nil, handleErr
	}

	// Propagate ServiceTier and ReasoningEffort to result for billing
	if handleErr == nil && result != nil {
		if compatContinuationEnabled && promptCacheKey != "" && result.ResponseID != "" {
			s.bindOpenAICompatSessionResponseID(ctx, c, account, promptCacheKey, result.ResponseID)
		}
		if promptCacheKey != "" && anthropicDigestChain != "" {
			s.bindOpenAICompatAnthropicDigestPromptCacheKey(account, apiKeyID, anthropicDigestChain, promptCacheKey, anthropicMatchedDigestChain)
		}
		// 计费 tier 优先采用上游回显值；上游未回显时回退到最终出站 body（经过
		// fast policy filter/force 之后）里的 tier。
		if tier := resolvedOpenAIUpstreamServiceTier(c, extractOpenAIServiceTierFromBody(responsesBody)); tier != nil {
			result.ServiceTier = tier
		}
		if responsesReq.Reasoning != nil && responsesReq.Reasoning.Effort != "" {
			re := responsesReq.Reasoning.Effort
			result.ReasoningEffort = &re
		}
	}

	// Extract and save Codex usage snapshot from response headers (for OAuth accounts).
	// 排除 spark 影子:其 codex_* 仅由 QueryUsage(/wham/usage bengalfox)更新(外审第7轮 P1)。
	if handleErr == nil && account.Type == AccountTypeOAuth && !account.IsShadow() && account.Platform != PlatformGrok {
		if snapshot := ParseCodexRateLimitHeaders(resp.Header); snapshot != nil {
			s.updateCodexUsageSnapshot(ctx, account.ID, snapshot)
		}
	} else if handleErr == nil && account.IsShadow() && account.ParentAccountID != nil {
		notifyOpenAIAutoReset(*account.ParentAccountID)
	}

	return result, handleErr
}

func ensureCodexOAuthInstructionsField(reqBody map[string]any) {
	if reqBody == nil {
		return
	}
	if value, ok := reqBody["instructions"]; !ok || value == nil {
		reqBody["instructions"] = ""
		return
	}
	if _, ok := reqBody["instructions"].(string); !ok {
		reqBody["instructions"] = ""
	}
}

// handleAnthropicErrorResponse reads an upstream error and returns it in
// Anthropic error format.
func (s *OpenAIGatewayService) handleAnthropicErrorResponse(
	resp *http.Response,
	c *gin.Context,
	account *Account,
	requestedModel ...string,
) (*OpenAIForwardResult, error) {
	return s.handleCompatErrorResponse(resp, c, account, writeAnthropicError, requestedModel...)
}

// handleAnthropicBufferedStreamingResponse reads all Responses SSE events from
// the upstream streaming response, finds the terminal event (response.completed
// / response.incomplete / response.failed), converts the complete response to
// Anthropic Messages JSON format, and writes it to the client.
// This is used when the client requested stream=false but the upstream is always
// streaming.
func (s *OpenAIGatewayService) handleAnthropicBufferedStreamingResponse(
	resp *http.Response,
	c *gin.Context,
	account *Account,
	originalModel string,
	billingModel string,
	upstreamModel string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")

	finalResponse, usage, acc, err := s.readOpenAICompatBufferedTerminal(resp, c, "openai messages buffered", requestID)
	if err != nil {
		var readErr *openAICompatBufferedReadError
		if errors.As(err, &readErr) && readErr != nil {
			return nil, readErr.cause
		}
		return nil, err
	}

	if finalResponse == nil {
		writeAnthropicError(c, http.StatusBadGateway, "api_error", "Upstream stream ended without a terminal response event")
		return nil, fmt.Errorf("upstream stream ended without terminal event")
	}
	observer := upstreamResponseModelObserverFromContext(c)
	if observer == nil {
		observer = beginUpstreamResponseModelObservation(c)
	}
	observer.Observe(finalResponse.Model, true)
	observer.ObserveServiceTier(finalResponse.ServiceTier, true)

	if strings.TrimSpace(finalResponse.Status) == "failed" {
		payload, _ := json.Marshal(gin.H{"type": "response.failed", "response": finalResponse})
		if hit, code, msg := detectOpenAICyberPolicy(payload); hit {
			MarkOpsCyberPolicy(c, CyberPolicyMark{
				Code:           code,
				Message:        msg,
				Body:           truncateString(string(payload), 4096),
				UpstreamStatus: http.StatusOK,
				UpstreamInTok:  usage.InputTokens,
				UpstreamOutTok: usage.OutputTokens,
			})
			clientMsg := cyberPolicyClientMessage(account, msg)
			writeAnthropicError(c, http.StatusBadRequest, "invalid_request_error", clientMsg)
			return nil, fmt.Errorf("openai cyber_policy: %s", msg)
		}
		message := openAICompatFailedResponseMessage(finalResponse)
		if openAIStreamFailedEventShouldFailover(payload, message) {
			return nil, s.newOpenAIStreamFailoverErrorWithModel(c, account, false, requestID, payload, message, upstreamModel, resp.Header)
		}
		message = s.recordOpenAIStreamUpstreamError(c, account, false, requestID, "http_error", payload, message)
		// 统一走语义状态推断 + body 归一化（与 /v1/responses 路径一致），
		// 使按错误码配置的透传规则可命中。
		if status, errType, errMsg, matched := applyOpenAIStreamFailedErrorPassthroughRule(
			c, account.Platform, payload, message,
		); matched {
			if errMsg == "" {
				errMsg = message
			}
			MarkResponseCommitted(c)
			writeAnthropicError(c, status, errType, errMsg)
			return nil, fmt.Errorf("upstream response failed (passthrough): %s", errMsg)
		}
		writeAnthropicError(c, http.StatusBadGateway, "api_error", message)
		return nil, fmt.Errorf("upstream response failed: %s", message)
	}
	if strings.TrimSpace(finalResponse.Status) == "completed" {
		logOpenAISuccessMissingUsage(c.Request.Context(), c, account, resp, &usage, "response.completed", false)
		s.recordOpenAIHTTP2StreamSuccess(resp, "response.completed")
	}

	// When the terminal event has an empty output array, reconstruct from
	// accumulated delta events so the client receives the full content.
	acc.SupplementResponseOutput(finalResponse)

	anthropicResp := apicompat.ResponsesToAnthropic(finalResponse, originalModel)

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.JSON(http.StatusOK, anthropicResp)

	result := &OpenAIForwardResult{
		RequestID:                     requestID,
		UpstreamHeaders:               resp.Header,
		ResponseID:                    finalResponse.ID,
		Usage:                         usage,
		Model:                         originalModel,
		BillingModel:                  billingModel,
		UpstreamModel:                 upstreamModel,
		UpstreamResponseModel:         observedUpstreamResponseModel(c),
		UpstreamResponseModelConflict: observedUpstreamResponseModelConflict(c),
		UpstreamResponseServiceTier:   observedUpstreamResponseServiceTier(c),
		Stream:                        false,
		Duration:                      time.Since(startTime),
	}
	// Grok /v1/messages uses Responses upstream; count native search for surcharge.
	if account != nil && account.IsGrok() && finalResponse != nil {
		if body, err := json.Marshal(finalResponse); err == nil {
			if n := countGrokNativeSearchCallsFromJSONBytes(body); n > 0 {
				result.SearchCount = n
			}
		}
	}
	return result, nil
}

func isOpenAICompatResponsesTerminalEvent(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "response.completed", "response.done", "response.incomplete", "response.failed", "response.cancelled", "response.canceled", "error":
		return true
	default:
		return false
	}
}

func (s *OpenAIGatewayService) recordOpenAIMessagesStreamUpstreamError(c *gin.Context, account *Account, upstreamRequestID, kind, message string) {
	if c == nil {
		return
	}
	message = sanitizeUpstreamErrorMessage(message)
	setOpsUpstreamError(c, http.StatusBadGateway, message, "")
	event := OpsUpstreamErrorEvent{
		Platform:           PlatformOpenAI,
		UpstreamStatusCode: http.StatusBadGateway,
		UpstreamRequestID:  strings.TrimSpace(upstreamRequestID),
		Kind:               kind,
		Message:            message,
	}
	if account != nil {
		event.Platform = account.Platform
		event.AccountID = account.ID
		event.AccountName = account.Name
	}
	appendOpsUpstreamError(c, event)
}

func isOpenAICompatDoneSentinelLine(line string) bool {
	payload, ok := extractOpenAISSEDataLine(line)
	return ok && strings.TrimSpace(payload) == "[DONE]"
}

// openAICompatBufferedReadError 只标记错误发生在上游响应体读取阶段；
// 具体端点自行决定是否允许重放请求，避免共享读取器扩大重试范围。
type openAICompatBufferedReadError struct {
	cause error
}

func (e *openAICompatBufferedReadError) Error() string { return e.cause.Error() }
func (e *openAICompatBufferedReadError) Unwrap() error { return e.cause }

func openAICompatTerminalResponse(event *apicompat.ResponsesStreamEvent, payload []byte) *apicompat.ResponsesResponse {
	if event == nil {
		return nil
	}
	if event.Response != nil {
		return event.Response
	}
	switch strings.TrimSpace(event.Type) {
	case "response.failed", "error":
		message := extractOpenAISSEErrorMessage(payload)
		if message == "" {
			message = "Upstream response failed"
		}
		return &apicompat.ResponsesResponse{
			Status: "failed",
			Error:  &apicompat.ResponsesError{Code: event.Code, Message: message},
		}
	default:
		return nil
	}
}

func (s *OpenAIGatewayService) readOpenAICompatBufferedTerminal(
	resp *http.Response,
	c *gin.Context,
	logPrefix string,
	requestID string,
) (*apicompat.ResponsesResponse, OpenAIUsage, *apicompat.BufferedResponseAccumulator, error) {
	acc := apicompat.NewBufferedResponseAccumulator()
	var usage OpenAIUsage
	if resp == nil || resp.Body == nil {
		return nil, usage, acc, errors.New("upstream response body is nil")
	}

	scanner := s.newUpstreamSSEScanner(resp.Body)

	streamInterval := time.Duration(0)
	if s.cfg != nil && s.cfg.Gateway.StreamDataIntervalTimeout > 0 {
		streamInterval = time.Duration(s.cfg.Gateway.StreamDataIntervalTimeout) * time.Second
	}
	var timeoutCh <-chan time.Time
	var timeoutTimer *time.Timer
	resetTimeout := func() {
		if streamInterval <= 0 {
			return
		}
		if timeoutTimer == nil {
			timeoutTimer = time.NewTimer(streamInterval)
			timeoutCh = timeoutTimer.C
			return
		}
		if !timeoutTimer.Stop() {
			select {
			case <-timeoutTimer.C:
			default:
			}
		}
		timeoutTimer.Reset(streamInterval)
	}
	stopTimeout := func() {
		if timeoutTimer == nil {
			return
		}
		if !timeoutTimer.Stop() {
			select {
			case <-timeoutTimer.C:
			default:
			}
		}
	}
	resetTimeout()
	defer stopTimeout()

	type scanEvent struct {
		line string
		err  error
	}
	events := make(chan scanEvent, 16)
	done := make(chan struct{})
	go func() {
		defer close(events)
		for scanner.Scan() {
			select {
			case events <- scanEvent{line: scanner.Text()}:
			case <-done:
				return
			}
		}
		if err := scanner.Err(); err != nil {
			select {
			case events <- scanEvent{err: err}:
			case <-done:
			}
		}
	}()
	defer close(done)

	var parser openAICompatSSEFrameParser
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				if frame, ok := parser.Finish(); ok {
					payload := openAICompatPayloadWithEventType(frame.Data, frame.EventType)
					payload = string(restoreCodexToolNamesFromContext(c, []byte(payload)))
					var event apicompat.ResponsesStreamEvent
					if err := json.Unmarshal([]byte(payload), &event); err == nil {
						s.parseSSEUsageBytesWithType([]byte(payload), event.Type, &usage)
						acc.ProcessEvent(&event)
						if response := openAICompatTerminalResponse(&event, []byte(payload)); isOpenAICompatResponsesTerminalEvent(event.Type) && response != nil {
							if event.Usage != nil {
								usage = copyOpenAIUsageFromResponsesUsage(event.Usage)
								if response.Usage == nil {
									response.Usage = event.Usage
								}
							}
							if response.Usage != nil {
								usage = copyOpenAIUsageFromResponsesUsage(response.Usage)
							}
							return response, usage, acc, nil
						}
					}
				}
				return nil, usage, acc, nil
			}
			resetTimeout()
			if ev.err != nil {
				if !errors.Is(ev.err, context.Canceled) && !errors.Is(ev.err, context.DeadlineExceeded) {
					logger.L().Warn(logPrefix+": read error",
						zap.Error(ev.err),
						zap.String("request_id", requestID),
					)
				}
				return nil, usage, acc, &openAICompatBufferedReadError{cause: ev.err}
			}

			if isOpenAICompatDoneSentinelLine(ev.line) {
				return nil, usage, acc, nil
			}
			frame, ok := parser.AddLine(ev.line)
			if !ok {
				continue
			}
			payload := openAICompatPayloadWithEventType(frame.Data, frame.EventType)
			payload = string(restoreCodexToolNamesFromContext(c, []byte(payload)))

			var event apicompat.ResponsesStreamEvent
			if err := json.Unmarshal([]byte(payload), &event); err != nil {
				logger.L().Warn(logPrefix+": failed to parse event",
					zap.Error(err),
					zap.String("request_id", requestID),
				)
				continue
			}
			s.parseSSEUsageBytesWithType([]byte(payload), event.Type, &usage)

			acc.ProcessEvent(&event)

			if response := openAICompatTerminalResponse(&event, []byte(payload)); isOpenAICompatResponsesTerminalEvent(event.Type) && response != nil {
				if event.Usage != nil {
					usage = copyOpenAIUsageFromResponsesUsage(event.Usage)
					if response.Usage == nil {
						response.Usage = event.Usage
					}
				}
				if response.Usage != nil {
					usage = copyOpenAIUsageFromResponsesUsage(response.Usage)
				}
				return response, usage, acc, nil
			}

		case <-timeoutCh:
			_ = resp.Body.Close()
			logger.L().Warn(logPrefix+": data interval timeout",
				zap.String("request_id", requestID),
				zap.Duration("interval", streamInterval),
			)
			return nil, usage, acc, ErrOpenAIStreamIntervalTimeout
		}
	}
}

// handleAnthropicStreamingResponse reads Responses SSE events from upstream,
// converts each to Anthropic SSE events, and writes them to the client.
// When StreamKeepaliveInterval is configured, it uses a goroutine + channel
// pattern to send Anthropic ping events during periods of upstream silence,
// preventing proxy/client timeout disconnections.
func (s *OpenAIGatewayService) handleAnthropicStreamingResponse(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
	originalModel string,
	billingModel string,
	upstreamModel string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	return s.handleAnthropicStreamingResponseWithReasoning(ctx, resp, c, account, originalModel, billingModel, upstreamModel, startTime, "")
}

func (s *OpenAIGatewayService) handleAnthropicStreamingResponseWithReasoning(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
	originalModel, billingModel, upstreamModel string,
	startTime time.Time,
	reasoningEffort string,
) (*OpenAIForwardResult, error) {
	if ctx == nil {
		ctx = context.Background()
		if c != nil && c.Request != nil {
			ctx = c.Request.Context()
		}
	}
	noteOpenAIAttemptProtoFromResponse(c, resp)
	requestID := resp.Header.Get("x-request-id")
	attemptWriterSizeBefore := OpenAICompactKeepaliveAdjustedWrittenSize(c)
	downstreamKeepaliveBytes := 0
	stageFirstOutput := account != nil && (account.Platform == PlatformOpenAI || account.IsOpenAIOAuthLike())
	var attemptResponseHeaders http.Header
	if stageFirstOutput {
		if s.responseHeaderFilter != nil {
			attemptResponseHeaders = responseheaders.FilterHeaders(resp.Header, s.responseHeaderFilter)
		} else if rid := strings.TrimSpace(requestID); rid != "" {
			attemptResponseHeaders = http.Header{"X-Request-Id": []string{rid}}
		}
		stageOpenAICodexTurnState(&attemptResponseHeaders, resp.Header)
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	if !stageFirstOutput {
		if s.responseHeaderFilter != nil {
			responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
		}
		if requestID != "" {
			c.Header("x-request-id", requestID)
		}
		s.relayOpenAICodexTurnState(c, account, resp.Header)
	}
	headersWritten := false
	applyAttemptResponseHeaders := func() {
		if !stageFirstOutput || len(attemptResponseHeaders) == 0 || c.Writer.Written() {
			return
		}
		c.Writer.Header().Del(http.CanonicalHeaderKey(openAICodexTurnStateHeader))
		for key, values := range attemptResponseHeaders {
			c.Writer.Header().Del(key)
			for _, value := range values {
				c.Writer.Header().Add(key, value)
			}
		}
		s.noteStagedOpenAICodexTurnStateCommitted(c, account, attemptResponseHeaders)
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
	}
	writeStreamHeaders := func() {
		if headersWritten {
			return
		}
		headersWritten = true
		applyAttemptResponseHeaders()
		if !c.Writer.Written() {
			c.Writer.WriteHeader(http.StatusOK)
		}
		MarkOpenAIAttemptTransportCommitted(c)
	}
	writeStableTransportHeaders := func() {
		if headersWritten || c.Writer.Written() {
			return
		}
		headersWritten = true
		c.Writer.WriteHeader(http.StatusOK)
		MarkOpenAIAttemptTransportCommitted(c)
	}
	var firstOutputStage *openAIFirstOutputStage
	if stageFirstOutput {
		firstOutputStage = newDefaultOpenAIFirstOutputStage()
		defer func() {
			if err := firstOutputStage.Close(); err != nil && account != nil {
				logger.L().Warn("openai messages stream: first-output stage cleanup failed",
					zap.Int64("account_id", account.ID),
					zap.Error(err),
				)
			}
		}()
	}
	phaseGuard := openAIPhaseWatchdogFromContext(ctx)
	firstOutputTimeout := time.Duration(0)
	if stageFirstOutput {
		firstOutputTimeout = s.openAIFirstOutputTimeout(reasoningEffort)
	}
	ttftMode := s.openAITTFTMode(ctx)
	stopFirstOutputTimer := func() {}

	state := apicompat.NewResponsesEventToAnthropicState()
	state.Model = originalModel
	var usage OpenAIUsage
	responseID := ""
	var firstTokenMs *int
	firstFrameSeen := false
	firstSemanticSeen := false
	clientDisconnected := false
	clientOutputStarted := false
	var streamFailoverErr error
	var streamNonFailoverErr error
	terminalEventType := ""
	searchCount := 0
	streamSearchSeen := make(map[string]struct{})
	countSearch := account != nil && account.IsGrok()

	scanner := s.newUpstreamSSEScanner(resp.Body)

	streamInterval := time.Duration(0)
	if s.cfg != nil && s.cfg.Gateway.StreamDataIntervalTimeout > 0 {
		streamInterval = time.Duration(s.cfg.Gateway.StreamDataIntervalTimeout) * time.Second
	}
	var intervalTicker *time.Ticker
	if streamInterval > 0 {
		intervalTicker = time.NewTicker(streamInterval)
		defer intervalTicker.Stop()
	}
	var intervalCh <-chan time.Time
	if intervalTicker != nil {
		intervalCh = intervalTicker.C
	}
	observer := upstreamResponseModelObserverFromContext(c)
	if observer == nil {
		observer = beginUpstreamResponseModelObservation(c)
	}

	// resultWithUsage builds the final result snapshot.
	resultWithUsage := func() *OpenAIForwardResult {
		out := &OpenAIForwardResult{
			RequestID:                     requestID,
			UpstreamHeaders:               resp.Header,
			ResponseID:                    responseID,
			Usage:                         usage,
			Model:                         originalModel,
			BillingModel:                  billingModel,
			UpstreamModel:                 upstreamModel,
			UpstreamResponseModel:         observedUpstreamResponseModel(c),
			UpstreamResponseModelConflict: observedUpstreamResponseModelConflict(c),
			UpstreamResponseServiceTier:   observedUpstreamResponseServiceTier(c),
			Stream:                        true,
			Duration:                      time.Since(startTime),
			FirstTokenMs:                  firstTokenMs,
			ClientDisconnect:              clientDisconnected,
		}
		if searchCount > 0 {
			out.SearchCount = searchCount
		}
		return out
	}

	// processDataLine handles a single "data: ..." SSE line from upstream.
	newPreOutputFailoverError := func(payload []byte, message string, cause string) *UpstreamFailoverError {
		failoverErr := s.newOpenAIStreamFailoverErrorWithModel(c, account, false, requestID, payload, message, upstreamModel, resp.Header)
		if attemptWriterSizeBefore >= 0 || downstreamKeepaliveBytes > 0 || openAIStreamKeepaliveBytes(c) > 0 {
			failoverErr.SafeToFailoverAfterWrite = true
		}
		return annotateOpenAIPreOutputFailover(c, failoverErr, cause, OpenAIRetryDecisionFailoverOtherAccount)
	}
	commitFirstOutputStage := func() error {
		if firstOutputStage == nil || firstOutputStage.closed || firstOutputStage.Buffered() == 0 {
			return nil
		}
		if responseID != "" {
			if err := s.bindPersistentOpenAIResponse(ctx, c, account, responseID); err != nil {
				denyOpenAIMessagesReplay(c)
				return localOpenAIOutputFailure(fmt.Errorf("persist Messages response ownership: %w", err))
			}
			if affinity, enabled := openAIAffinityFromGin(c); enabled && affinity.Writable && s.openAIAffinityEnabled() && account.IsOpenAIOAuth() {
				// A persisted ownership decision must survive a later write failure.
				denyOpenAIMessagesReplay(c)
			}
		}
		if phaseGuard != nil {
			if err := phaseGuard.tryCommit(); err != nil {
				return openAIOutputPhaseFailure(c, err, resp.Header)
			}
		}
		MarkOpenAISemanticOutputStarted(c)
		applyAttemptResponseHeaders()
		writeStreamHeaders()
		if err := firstOutputStage.CommitTo(c.Writer); err != nil {
			return err
		}
		c.Writer.Flush()
		return nil
	}
	writeClientSSE := func(sse string) error {
		// Keep every converted event in the stage until an upstream event with
		// confirmed semantic output makes the whole staged preamble publishable.
		if firstOutputStage != nil && !firstOutputStage.closed && !clientOutputStarted {
			if _, err := firstOutputStage.WriteString(sse); err != nil {
				return err
			}
			return nil
		}
		writeStreamHeaders()
		_, err := fmt.Fprint(c.Writer, sse)
		return err
	}
	finishScanErr := func(scanErr error) (*OpenAIForwardResult, error) {
		if phaseGuard != nil && phaseGuard.timeoutFailure() != nil {
			return resultWithUsage(), openAIOutputPhaseFailure(c, phaseGuard.timeoutFailure(), resp.Header)
		}
		if scanErr == nil {
			return resultWithUsage(), nil
		}
		if !errors.Is(scanErr, context.Canceled) && !errors.Is(scanErr, context.DeadlineExceeded) {
			logger.L().Warn("openai messages stream: read error",
				zap.Error(scanErr),
				zap.String("request_id", requestID),
			)
		}
		if errors.Is(scanErr, context.Canceled) || errors.Is(scanErr, context.DeadlineExceeded) {
			return resultWithUsage(), fmt.Errorf("stream usage incomplete: %w", scanErr)
		}
		if !openAIStreamClientOutputStarted(c, clientOutputStarted, attemptWriterSizeBefore, downstreamKeepaliveBytes) {
			msg := "OpenAI messages stream disconnected before completion"
			if errText := strings.TrimSpace(scanErr.Error()); errText != "" {
				msg += ": " + errText
			}
			s.recordOpenAIHTTP2StreamFailure(ctx, resp, scanErr)
			return resultWithUsage(), withOpenAIUnderlyingError(newPreOutputFailoverError(nil, msg, classifyOpenAIStreamScanCause(scanErr)), scanErr)
		}
		if clientDisconnected {
			return resultWithUsage(), fmt.Errorf("stream usage incomplete after disconnect: %w", scanErr)
		}
		s.recordOpenAIHTTP2StreamFailure(ctx, resp, scanErr)
		return resultWithUsage(), fmt.Errorf("stream usage incomplete: %w", scanErr)
	}

	processDataLine := func(payload string) bool {
		payload = string(restoreCodexToolNamesFromContext(c, []byte(payload)))
		if !firstFrameSeen {
			firstFrameSeen = true
			MarkOpenAIAttemptTTFTPhase(c, "frame", int(time.Since(startTime).Milliseconds()))
		}
		if countSearch {
			searchCount += countGrokNativeSearchCallsInSSEDataDedup([]byte(payload), streamSearchSeen)
		}

		var event apicompat.ResponsesStreamEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			if stageFirstOutput {
				denyOpenAIMessagesReplay(c)
			}
			logger.L().Warn("openai messages stream: failed to parse event",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
			return false
		}
		eventTypeEarly := strings.TrimSpace(event.Type)
		if stageFirstOutput && !openAIMessagesEventAllowsUncommittedReplay(payload, eventTypeEarly) {
			denyOpenAIMessagesReplay(c)
		}
		eventHasSemanticOutput := openAIStreamDataStartsVisibleOutput(payload, eventTypeEarly)
		if eventTypeEarly == "response.output_item.added" || eventTypeEarly == "response.content_part.added" || eventTypeEarly == "response.reasoning_summary_part.added" {
			eventHasSemanticOutput = openAIStreamAddedEventStartsClientOutput([]byte(payload), eventTypeEarly)
		}
		if !firstSemanticSeen && eventHasSemanticOutput {
			firstSemanticSeen = true
			MarkOpenAIAttemptTTFTPhase(c, "semantic", int(time.Since(startTime).Milliseconds()))
		}
		observer.ObserveOpenAI([]byte(payload), event.Type)
		s.parseSSEUsageBytesWithType([]byte(payload), event.Type, &usage)

		eventType := strings.TrimSpace(event.Type)
		if responseID == "" && event.Response != nil {
			if id := strings.TrimSpace(event.Response.ID); id != "" {
				responseID = id
			}
		}
		isBareErrorEvent := eventType == "error"
		isTerminalEvent := isOpenAICompatResponsesTerminalEvent(eventType) || isBareErrorEvent
		if isTerminalEvent {
			terminalEventType = eventType
			if event.Response != nil {
				if id := strings.TrimSpace(event.Response.ID); id != "" {
					responseID = id
				}
				if event.Response.Usage != nil {
					usage = copyOpenAIUsageFromResponsesUsage(event.Response.Usage)
				}
			}
			if event.Usage != nil {
				usage = copyOpenAIUsageFromResponsesUsage(event.Usage)
			}
			// cyber_policy 致命不可重试：标记供 handler 事后记录；以 Anthropic SSE error 事件
			// 回写让客户端感知并停止重试（F4），丢弃后续转换输出。
			if eventType == "response.failed" || isBareErrorEvent {
				payloadBytes := []byte(payload)
				if hit, code, msg := detectOpenAICyberPolicy(payloadBytes); hit {
					MarkOpsCyberPolicy(c, CyberPolicyMark{
						Code:           code,
						Message:        msg,
						Body:           truncateString(payload, 4096),
						UpstreamStatus: http.StatusOK,
						UpstreamInTok:  usage.InputTokens,
						UpstreamOutTok: usage.OutputTokens,
					})
					if !clientDisconnected {
						writeStreamHeaders()
						clientMsg := cyberPolicyClientMessage(account, msg)
						if _, err := fmt.Fprint(c.Writer, buildAnthropicStreamErrorSSE("invalid_request_error", clientMsg)); err == nil {
							c.Writer.Flush()
						}
						clientDisconnected = true
					}
					return true
				}
				message := extractOpenAISSEErrorMessage(payloadBytes)
				// Once Anthropic output has started, switching accounts would splice
				// two model streams together. Surface a proper Anthropic error event
				// instead of returning a failover error that the handler cannot retry.
				shouldFailover := openAIStreamFailedEventShouldFailover(payloadBytes, message)
				if isBareErrorEvent {
					shouldFailover = openAIStreamErrorEventShouldFailover(payloadBytes, message)
				}
				if !clientOutputStarted && shouldFailover {
					streamFailoverErr = s.newOpenAIStreamFailoverErrorWithModel(c, account, false, requestID, payloadBytes, message, upstreamModel, resp.Header)
					return true
				}
				message = s.recordOpenAIStreamUpstreamError(c, account, false, requestID, "http_error", payloadBytes, message)
				errStatus, errType, errMsg := http.StatusBadGateway, "api_error", message
				// 统一走语义状态推断 + body 归一化（与 /v1/responses 路径一致），
				// 使按错误码配置的透传规则可命中。
				if status, et, em, matched := applyOpenAIStreamFailedErrorPassthroughRule(
					c, account.Platform, payloadBytes, message,
				); matched {
					if em == "" {
						em = errMsg
					}
					errStatus, errType, errMsg = status, et, em
					MarkResponseCommitted(c)
				}
				if !clientDisconnected {
					if !headersWritten && !c.Writer.Written() {
						writeAnthropicError(c, errStatus, errType, errMsg)
						clientOutputStarted = true
					} else {
						writeStreamHeaders()
						if _, err := fmt.Fprint(c.Writer, buildAnthropicStreamErrorSSE(errType, errMsg)); err == nil {
							c.Writer.Flush()
						}
					}
				}
				streamNonFailoverErr = fmt.Errorf("upstream response failed: %s", errMsg)
				return true
			}
		}

		// Convert to Anthropic events. Structural events may be emitted by the
		// converter for a semantic upstream event, so commit only after the
		// complete converted batch has been staged.
		// eventHasSemanticOutput was classified before conversion so structural
		// Anthropic events cannot commit the stage by themselves.
		events := apicompat.ResponsesEventToAnthropicEvents(&event, state)
		if !clientDisconnected {
			for _, evt := range events {
				sse, err := apicompat.ResponsesAnthropicEventToSSE(evt)
				if err != nil {
					logger.L().Warn("openai messages stream: failed to marshal event",
						zap.Error(err),
						zap.String("request_id", requestID),
					)
					continue
				}
				if err := writeClientSSE(sse); err != nil {
					if firstOutputStage != nil && !firstOutputStage.closed && (errors.Is(err, errOpenAIFirstOutputStageLimit) || strings.Contains(err.Error(), "first-output")) {
						denyOpenAIMessagesReplay(c)
						streamNonFailoverErr = localOpenAIOutputFailure(err)
						return true
					}
					clientDisconnected = true
					logger.L().Info("openai messages stream: client disconnected, continuing to drain upstream for billing",
						zap.String("request_id", requestID),
					)
					break
				}
			}
		}
		if eventHasSemanticOutput && len(events) > 0 && !clientDisconnected {
			if !clientOutputStarted {
				if err := commitFirstOutputStage(); err != nil {
					if errors.Is(err, errOpenAILocalOutputFailure) {
						streamNonFailoverErr = err
						return true
					}
					var phaseFailure *UpstreamFailoverError
					if errors.As(err, &phaseFailure) {
						streamFailoverErr = err
						return true
					}
					clientDisconnected = true
				} else {
					clientOutputStarted = true
					stopFirstOutputTimer()
				}
			}
			if !clientDisconnected {
				MarkOpenAISemanticOutputStarted(c)
				MarkOpenAIAttemptTTFTPhase(c, "visible", int(time.Since(startTime).Milliseconds()))
				if firstTokenMs == nil {
					ms := int(time.Since(startTime).Milliseconds())
					if openAIStreamDataStartsTTFT(payload, eventType, false, ttftMode) || ttftMode == OpenAITTFTModeVisible || firstSemanticSeen {
						firstTokenMs = &ms
					}
				}
			}
		}
		if len(events) > 0 && !clientDisconnected && clientOutputStarted {
			c.Writer.Flush()
		}
		return isTerminalEvent
	}

	// finalizeStream sends any remaining Anthropic events and returns the result.
	finalizeStream := func() (*OpenAIForwardResult, error) {
		if streamFailoverErr != nil {
			return resultWithUsage(), streamFailoverErr
		}
		if streamNonFailoverErr != nil {
			return resultWithUsage(), streamNonFailoverErr
		}
		if finalEvents := apicompat.FinalizeResponsesAnthropicStream(state); len(finalEvents) > 0 && !clientDisconnected {
			for _, evt := range finalEvents {
				sse, err := apicompat.ResponsesAnthropicEventToSSE(evt)
				if err != nil {
					continue
				}
				if err := writeClientSSE(sse); err != nil {
					clientDisconnected = true
					logger.L().Info("openai messages stream: client disconnected during final flush",
						zap.String("request_id", requestID),
					)
					break
				}
			}
			if !clientDisconnected {
				c.Writer.Flush()
			}
		}
		if !clientOutputStarted && !clientDisconnected && terminalEventType == "response.completed" {
			if err := commitFirstOutputStage(); err != nil {
				if errors.Is(err, errOpenAILocalOutputFailure) {
					return resultWithUsage(), err
				}
				var phaseFailure *UpstreamFailoverError
				if errors.As(err, &phaseFailure) {
					return resultWithUsage(), err
				}
				clientDisconnected = true
			} else {
				clientOutputStarted = true
				stopFirstOutputTimer()
			}
		}
		logOpenAISuccessMissingUsage(c.Request.Context(), c, account, resp, &usage, terminalEventType, clientDisconnected)
		s.recordOpenAIHTTP2StreamSuccess(resp, terminalEventType)
		return resultWithUsage(), nil
	}

	// handleScanErr logs scanner errors if meaningful.
	missingTerminalErr := func() (*OpenAIForwardResult, error) {
		result := resultWithUsage()
		if clientDisconnected {
			return result, fmt.Errorf("stream usage incomplete: missing terminal event")
		}
		message := "OpenAI messages stream ended before a terminal event"
		streamErr := ErrOpenAIStreamMissingTerminal
		if !openAIStreamClientOutputStarted(c, clientOutputStarted, attemptWriterSizeBefore, downstreamKeepaliveBytes) {
			s.recordOpenAIHTTP2StreamFailure(ctx, resp, streamErr)
			return result, withOpenAIUnderlyingError(newPreOutputFailoverError(nil, message, OpenAIFailureCauseMissingTerminal), streamErr)
		}
		s.recordOpenAIHTTP2StreamFailure(ctx, resp, streamErr)
		s.recordOpenAIMessagesStreamUpstreamError(c, account, requestID, "stream_missing_terminal", message)
		return result, fmt.Errorf("stream usage incomplete: missing terminal event")
	}
	processFrame := func(frame openAICompatSSEFrame) bool {
		payload := openAICompatPayloadWithEventType(frame.Data, frame.EventType)
		return processDataLine(payload)
	}

	// ── Determine keepalive interval ──
	keepaliveInterval := time.Duration(0)
	if s.cfg != nil && s.cfg.Gateway.StreamKeepaliveInterval > 0 {
		keepaliveInterval = time.Duration(s.cfg.Gateway.StreamKeepaliveInterval) * time.Second
	}

	// ── No keepalive: fast synchronous path (no goroutine overhead) ──
	if streamInterval <= 0 && keepaliveInterval <= 0 && firstOutputTimeout <= 0 && phaseGuard == nil {
		var parser openAICompatSSEFrameParser
		for scanner.Scan() {
			line := scanner.Text()
			if isOpenAICompatDoneSentinelLine(line) {
				return missingTerminalErr()
			}
			frame, ok := parser.AddLine(line)
			if !ok {
				continue
			}
			if processFrame(frame) {
				return finalizeStream()
			}
		}
		if err := scanner.Err(); err != nil {
			return finishScanErr(err)
		}
		if frame, ok := parser.Finish(); ok {
			if strings.TrimSpace(frame.Data) == "[DONE]" {
				return missingTerminalErr()
			}
			if processFrame(frame) {
				return finalizeStream()
			}
		}
		return missingTerminalErr()
	}

	// ── With keepalive: goroutine + channel + select ──
	type scanEvent struct {
		line string
		err  error
	}
	events := make(chan scanEvent, 16)
	done := make(chan struct{})
	var lastReadAt int64
	atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
	sendEvent := func(ev scanEvent) bool {
		select {
		case events <- ev:
			return true
		case <-done:
			return false
		}
	}
	go func() {
		defer close(events)
		for scanner.Scan() {
			atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
			if !sendEvent(scanEvent{line: scanner.Text()}) {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			_ = sendEvent(scanEvent{err: err})
		}
	}()
	defer close(done)

	var keepaliveTicker *time.Ticker
	if keepaliveInterval > 0 {
		keepaliveTicker = time.NewTicker(keepaliveInterval)
		defer keepaliveTicker.Stop()
	}
	var keepaliveCh <-chan time.Time
	if keepaliveTicker != nil {
		keepaliveCh = keepaliveTicker.C
	}
	var firstOutputTimer *time.Timer
	var firstOutputCh <-chan time.Time
	if phaseGuard != nil {
		firstOutputCh = phaseGuard.done
	} else if firstOutputTimeout > 0 {
		remaining := time.Until(startTime.Add(firstOutputTimeout))
		if remaining <= 0 {
			remaining = time.Nanosecond
		}
		firstOutputTimer = time.NewTimer(remaining)
		firstOutputCh = firstOutputTimer.C
		defer firstOutputTimer.Stop()
	}
	stopFirstOutputTimer = func() {
		if firstOutputTimer == nil {
			return
		}
		if !firstOutputTimer.Stop() {
			select {
			case <-firstOutputTimer.C:
			default:
			}
		}
		firstOutputTimer = nil
		firstOutputCh = nil
	}
	lastDataAt := time.Now()
	var parser openAICompatSSEFrameParser

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				// Upstream closed
				if frame, ok := parser.Finish(); ok {
					if strings.TrimSpace(frame.Data) == "[DONE]" {
						return missingTerminalErr()
					}
					if processFrame(frame) {
						return finalizeStream()
					}
				}
				return missingTerminalErr()
			}
			if ev.err != nil {
				return finishScanErr(ev.err)
			}
			lastDataAt = time.Now()
			line := ev.line
			if isOpenAICompatDoneSentinelLine(line) {
				return missingTerminalErr()
			}
			frame, ok := parser.AddLine(line)
			if !ok {
				continue
			}
			if processFrame(frame) {
				return finalizeStream()
			}

		case <-intervalCh:
			lastRead := time.Unix(0, atomic.LoadInt64(&lastReadAt))
			if time.Since(lastRead) < streamInterval {
				continue
			}
			if clientDisconnected {
				return resultWithUsage(), fmt.Errorf("stream usage incomplete after timeout")
			}
			logger.L().Warn("openai messages stream: data interval timeout",
				zap.String("request_id", requestID),
				zap.String("model", originalModel),
				zap.Duration("interval", streamInterval),
			)
			streamErr := ErrOpenAIStreamIntervalTimeout
			if !openAIStreamClientOutputStarted(c, clientOutputStarted, attemptWriterSizeBefore, downstreamKeepaliveBytes) {
				s.recordOpenAIHTTP2StreamFailure(ctx, resp, streamErr)
				return resultWithUsage(), withOpenAIUnderlyingError(newPreOutputFailoverError(nil, streamErr.Error(), OpenAIFailureCauseIntervalTimeout), streamErr)
			}
			s.recordOpenAIHTTP2StreamFailure(ctx, resp, streamErr)
			return resultWithUsage(), streamErr

		case <-keepaliveCh:
			if clientDisconnected {
				continue
			}
			if time.Since(lastDataAt) < keepaliveInterval {
				continue
			}
			// Local Anthropic ping is transport-visible but non-semantic.
			// Do not close the pre-output recovery window (F03).
			writeStableTransportHeaders()
			n, err := fmt.Fprint(c.Writer, "event: ping\ndata: {\"type\":\"ping\"}\n\n")
			if err != nil {
				logger.L().Info("openai messages stream: client disconnected during keepalive",
					zap.String("request_id", requestID),
				)
				clientDisconnected = true
				continue
			}
			downstreamKeepaliveBytes += n
			recordOpenAIStreamKeepaliveBytes(c, n)
			MarkOpenAIAttemptHeartbeat(c)
			c.Writer.Flush()

		case <-firstOutputCh:
			if phaseGuard != nil {
				_ = resp.Body.Close()
				return resultWithUsage(), openAIOutputPhaseFailure(c, phaseGuard.timeoutFailure(), resp.Header)
			}
			if clientOutputStarted || clientDisconnected {
				continue
			}
			stopFirstOutputTimer()
			if firstOutputStage != nil {
				_ = firstOutputStage.Close()
			}
			err := s.newOpenAIFirstOutputTimeoutError(ctx, c, account, startTime, originalModel, reasoningEffort, firstOutputTimeout, "semantic_output", resp.Header)
			return resultWithUsage(), annotateOpenAIPreOutputFailover(c, err, OpenAIFailureCauseFirstOutputTimeout, OpenAIRetryDecisionFailoverOtherAccount)
		}
	}
}

// writeAnthropicError writes an error response in Anthropic Messages API format.
func writeAnthropicError(c *gin.Context, statusCode int, errType, message string) {
	c.JSON(statusCode, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
}

// buildAnthropicStreamErrorSSE builds one Anthropic SSE `error` event so a
// streaming response can terminate with a visible error (e.g. upstream
// cyber_policy) and programmatic clients stop retrying.
// Marshal 失败的兜底仅保留固定提示。
func buildAnthropicStreamErrorSSE(errType, message string) string {
	payload, err := json.Marshal(gin.H{
		"type": "error",
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
	if err != nil {
		return "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"" + errType + "\",\"message\":\"upstream error\"}}\n\n"
	}
	return "event: error\ndata: " + string(payload) + "\n\n"
}

func copyOpenAIUsageFromResponsesUsage(usage *apicompat.ResponsesUsage) OpenAIUsage {
	if usage == nil {
		return OpenAIUsage{}
	}
	result := OpenAIUsage{
		InputTokens:              usage.InputTokens,
		OutputTokens:             usage.OutputTokens,
		CacheCreationInputTokens: usage.CacheCreationInputTokens,
	}
	if usage.InputTokensDetails != nil {
		result.CacheReadInputTokens = usage.InputTokensDetails.CachedTokens
	}
	return result
}

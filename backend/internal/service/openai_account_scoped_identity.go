package service

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/cespare/xxhash/v2"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const (
	openAIAccountScopedIdentityExtraKey       = "openai_account_scoped_identity_enabled"
	openAIAccountIdentityMissingDeviceLogRate = 5 * time.Minute
	openAICodexInstallationIDHeader           = "x-codex-installation-id"
	openAICodexWindowIDHeader                 = "x-codex-window-id"
)

var openAIAccountIdentityMissingDeviceLogAt sync.Map // key: int64(accountID), value: *atomic.Int64

func (s *OpenAIGatewayService) isOpenAIAccountScopedIdentityEnabled(account *Account) bool {
	if account == nil || !account.IsOpenAIOAuth() || account.IsOpenAIPersonalAccessToken() || account.IsOpenAIAgentIdentity() {
		return false
	}
	// Fingerprint convergence already provides the account-level identity.
	// Layering API-key-scoped rewriting on top would split its stable window and
	// installation IDs again, so the two mechanisms are mutually exclusive.
	if account.GetCodexFingerprintMode() != codexFingerprintOff {
		return false
	}
	if override := openAIAccountScopedIdentityOverride(account); override != nil {
		return *override
	}
	return s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIAccountScopedIdentity.Enabled
}

func openAIAccountScopedIdentityOverride(account *Account) *bool {
	if account == nil || account.Extra == nil {
		return nil
	}
	if enabled, ok := account.Extra[openAIAccountScopedIdentityExtraKey].(bool); ok {
		return &enabled
	}
	openAIExtra, _ := account.Extra[PlatformOpenAI].(map[string]any)
	if enabled, ok := openAIExtra[openAIAccountScopedIdentityExtraKey].(bool); ok {
		return &enabled
	}
	return nil
}

func deriveOpenAIAccountScopedSessionID(accountID, apiKeyID int64, rawSessionID string) string {
	rawSessionID = strings.TrimSpace(rawSessionID)
	if rawSessionID == "" {
		return ""
	}
	return hashOpenAIAccountIdentity("openai-session-v1", accountID, apiKeyID, rawSessionID)
}

func deriveOpenAIAccountScopedWindowID(accountID, apiKeyID int64, sessionID, rawWindowID string) string {
	rawWindowID = strings.TrimSpace(rawWindowID)
	if rawWindowID == "" {
		return ""
	}
	seed := hashOpenAIAccountIdentity(
		"openai-window-v1",
		accountID,
		apiKeyID,
		strings.TrimSpace(sessionID),
		rawWindowID,
	)
	return generateSessionUUID(seed)
}

func hashOpenAIAccountIdentity(domain string, accountID, apiKeyID int64, values ...string) string {
	h := xxhash.New()
	writeOpenAIIdentityHashString(h, domain)
	var fixed [8]byte
	binary.BigEndian.PutUint64(fixed[:], uint64(accountID))
	_, _ = h.Write(fixed[:])
	binary.BigEndian.PutUint64(fixed[:], uint64(apiKeyID))
	_, _ = h.Write(fixed[:])
	for _, value := range values {
		writeOpenAIIdentityHashString(h, value)
	}
	return fmt.Sprintf("%016x", h.Sum64())
}

func writeOpenAIIdentityHashString(h *xxhash.Digest, value string) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = h.Write(length[:])
	_, _ = h.WriteString(value)
}

func (s *OpenAIGatewayService) openAIOutboundSessionID(account *Account, apiKeyID int64, rawSessionID string) string {
	if s.isOpenAIAccountScopedIdentityEnabled(account) {
		return deriveOpenAIAccountScopedSessionID(account.ID, apiKeyID, rawSessionID)
	}
	return isolateOpenAISessionID(apiKeyID, rawSessionID)
}

func (s *OpenAIGatewayService) applyOpenAIAccountScopedHeaders(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	headers http.Header,
	sessionID string,
) {
	if !s.isOpenAIAccountScopedIdentityEnabled(account) || headers == nil {
		return
	}

	deviceID := strings.TrimSpace(account.GetOpenAIDeviceID())
	headers.Del(openAICodexInstallationIDHeader)
	if deviceID != "" {
		headers.Set(openAICodexInstallationIDHeader, deviceID)
	} else {
		s.logOpenAIAccountIdentityMissingDevice(ctx, account)
	}

	rawWindowID := strings.TrimSpace(headers.Get(openAICodexWindowIDHeader))
	headers.Del(openAICodexWindowIDHeader)
	if rawWindowID != "" {
		headers.Set(openAICodexWindowIDHeader, deriveOpenAIAccountScopedWindowID(
			account.ID,
			getAPIKeyIDFromContext(c),
			sessionID,
			rawWindowID,
		))
	}
}

func (s *OpenAIGatewayService) applyOpenAIAccountScopedBody(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	sessionID string,
) ([]byte, error) {
	if !s.isOpenAIAccountScopedIdentityEnabled(account) {
		return body, nil
	}

	var reqBody map[string]any
	if err := json.Unmarshal(body, &reqBody); err != nil {
		return nil, fmt.Errorf("decode OpenAI account-scoped identity body: %w", err)
	}
	if !s.applyOpenAIAccountScopedClientMetadata(ctx, c, account, reqBody, sessionID) {
		return body, nil
	}
	updated, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("encode OpenAI account-scoped identity body: %w", err)
	}
	return updated, nil
}

func (s *OpenAIGatewayService) applyOpenAIAccountScopedClientMetadata(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	reqBody map[string]any,
	sessionID string,
) bool {
	if !s.isOpenAIAccountScopedIdentityEnabled(account) || reqBody == nil {
		return false
	}

	metadata, exists := reqBody["client_metadata"].(map[string]any)
	if !exists {
		if stringMetadata, ok := reqBody["client_metadata"].(map[string]string); ok {
			metadata = make(map[string]any, len(stringMetadata)+1)
			for key, value := range stringMetadata {
				metadata[key] = value
			}
		} else {
			metadata = make(map[string]any)
		}
	}

	changed := false
	deviceID := strings.TrimSpace(account.GetOpenAIDeviceID())
	if deviceID == "" {
		if _, present := metadata[openAICodexInstallationIDHeader]; present {
			delete(metadata, openAICodexInstallationIDHeader)
			changed = true
		}
		s.logOpenAIAccountIdentityMissingDevice(ctx, account)
	} else if current, _ := metadata[openAICodexInstallationIDHeader].(string); current != deviceID {
		metadata[openAICodexInstallationIDHeader] = deviceID
		changed = true
	}

	if rawWindowID, ok := metadata[openAICodexWindowIDHeader].(string); ok && strings.TrimSpace(rawWindowID) != "" {
		windowID := deriveOpenAIAccountScopedWindowID(
			account.ID,
			getAPIKeyIDFromContext(c),
			sessionID,
			rawWindowID,
		)
		if windowID != rawWindowID {
			metadata[openAICodexWindowIDHeader] = windowID
			changed = true
		}
	}

	if changed || exists || len(metadata) > 0 {
		reqBody["client_metadata"] = metadata
	}
	return changed
}

func (s *OpenAIGatewayService) logOpenAIAccountIdentityMissingDevice(ctx context.Context, account *Account) {
	if s == nil || account == nil {
		return
	}
	value, _ := openAIAccountIdentityMissingDeviceLogAt.LoadOrStore(account.ID, &atomic.Int64{})
	lastLogAt, _ := value.(*atomic.Int64)
	if lastLogAt == nil {
		return
	}
	now := time.Now().UnixNano()
	last := lastLogAt.Load()
	if now-last < int64(openAIAccountIdentityMissingDeviceLogRate) || !lastLogAt.CompareAndSwap(last, now) {
		return
	}
	logger.FromContext(ctx).Debug(
		"OpenAI account-scoped identity omitted installation ID because the selected account has no device ID",
		zap.String("component", "service.openai_gateway"),
		zap.Int64("account_id", account.ID),
	)
}

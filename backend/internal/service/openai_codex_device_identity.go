package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const openaiDeviceIDExtraKey = "openai_device_id"

// canonicalCodexInstallationID accepts official Codex installation IDs only:
// a canonical RFC 4122 UUIDv4. Official desktop/CLI persist one such value per
// install and send it as x-codex-installation-id.
func canonicalCodexInstallationID(raw string) string {
	trimmed := strings.TrimSpace(raw)
	parsed, err := uuid.Parse(trimmed)
	if err != nil || parsed == uuid.Nil || parsed.Version() != 4 || parsed.Variant() != uuid.RFC4122 {
		return ""
	}
	return parsed.String()
}

func installationIDFromTurnMetadata(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return ""
	}
	value, _ := metadata["installation_id"].(string)
	return canonicalCodexInstallationID(value)
}

func extractInboundCodexInstallationID(h http.Header) string {
	if h == nil {
		return ""
	}
	headerID := canonicalCodexInstallationID(h.Get("x-codex-installation-id"))
	metaID := installationIDFromTurnMetadata(h.Get("x-codex-turn-metadata"))
	switch {
	case headerID != "" && metaID != "" && headerID != metaID:
		return ""
	case headerID != "":
		return headerID
	default:
		return metaID
	}
}

// adoptOfficialCodexInstallationID keeps one durable machine identity per
// account. If the account has not stored openai_device_id yet, an official
// Codex client may donate its real UUIDv4 installation ID for this snapshot.
func adoptOfficialCodexInstallationID(account *Account, headers http.Header, snapshot *CodexIdentitySnapshot) {
	if snapshot == nil || account == nil || headers == nil {
		return
	}
	if strings.TrimSpace(account.GetOpenAIDeviceID()) != "" {
		return
	}
	if !openai.IsCodexOfficialClientByHeaders(headers.Get("User-Agent"), headers.Get("originator")) {
		return
	}
	inbound := extractInboundCodexInstallationID(headers)
	if inbound == "" {
		return
	}
	snapshot.installationID = inbound
	snapshot.learnedOfficialInstallation = true
}

func (s *OpenAIGatewayService) persistLearnedCodexDeviceID(ctx context.Context, account *Account, snapshot *CodexIdentitySnapshot) {
	if s == nil || s.accountRepo == nil || account == nil || snapshot == nil || !snapshot.learnedOfficialInstallation {
		return
	}
	if strings.TrimSpace(account.GetOpenAIDeviceID()) != "" {
		return
	}
	deviceID := canonicalCodexInstallationID(snapshot.installationID)
	if deviceID == "" {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.accountRepo.UpdateExtra(ctx, account.ID, map[string]any{openaiDeviceIDExtraKey: deviceID}); err != nil {
		logger.FromContext(ctx).Warn(
			"failed to persist official Codex installation ID",
			zap.String("component", "service.openai_gateway"),
			zap.Int64("account_id", account.ID),
			zap.Error(err),
		)
		return
	}
	if account.Extra == nil {
		account.Extra = make(map[string]any, 1)
	}
	account.Extra[openaiDeviceIDExtraKey] = deviceID
}

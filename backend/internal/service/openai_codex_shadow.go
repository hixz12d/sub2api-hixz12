package service

import (
	"bytes"
	"fmt"
	"github.com/gin-gonic/gin"
	"log/slog"
	"strconv"
	"strings"
	"sync/atomic"
)

const codexShadowComparisonContextKey = "codex_relay_shadow_comparison"

type CodexShadowComparison struct {
	Compared             bool
	IdentityMatch        bool
	ApplicationMatch     bool
	BodyMatch            bool
	TransportTupleMatch  bool
	DifferenceCategories []string
	ErrorCategory        string
}

type CodexShadowMetricsSnapshot struct {
	Compared   int64
	Mismatched int64
	Errors     int64
}

var codexShadowComparedTotal atomic.Int64
var codexShadowMismatchedTotal atomic.Int64
var codexShadowErrorsTotal atomic.Int64

func SnapshotCodexShadowMetrics() CodexShadowMetricsSnapshot {
	return CodexShadowMetricsSnapshot{
		Compared:   codexShadowComparedTotal.Load(),
		Mismatched: codexShadowMismatchedTotal.Load(),
		Errors:     codexShadowErrorsTotal.Load(),
	}
}

func CodexShadowComparisonFromContext(c interface {
	Get(string) (any, bool)
}) (CodexShadowComparison, bool) {
	if c == nil {
		return CodexShadowComparison{}, false
	}
	value, ok := c.Get(codexShadowComparisonContextKey)
	comparison, valid := value.(CodexShadowComparison)
	return comparison, ok && valid
}

// compareCodexRelayShadowForRequest is separate from the generic metrics holder
// so the shadow path can use Gin request context without exposing identifiers.
func (s *OpenAIGatewayService) compareCodexRelayShadowForRequest(account *Account, c *gin.Context, legacy *CodexIdentitySnapshot) {
	if account == nil || c == nil || c.Request == nil || account.Extra == nil {
		return
	}
	enabled, _ := account.Extra[CodexRelayShadowEnabledExtraKey].(bool)
	if !enabled {
		return
	}
	comparison := CodexShadowComparison{Compared: true}
	defer func() {
		c.Set(codexShadowComparisonContextKey, comparison)
		codexShadowComparedTotal.Add(1)
		if comparison.ErrorCategory != "" {
			codexShadowErrorsTotal.Add(1)
		}
		if len(comparison.DifferenceCategories) > 0 {
			codexShadowMismatchedTotal.Add(1)
		}
		if comparison.ErrorCategory != "" || len(comparison.DifferenceCategories) > 0 {
			slog.Warn("codex_relay_shadow_mismatch",
				"account_id", account.ID,
				"error_category", comparison.ErrorCategory,
				"difference_categories", comparison.DifferenceCategories,
			)
		}
	}()
	if s == nil || s.cfg == nil || strings.TrimSpace(s.cfg.Gateway.OpenAIAffinity.Secret) == "" {
		comparison.ErrorCategory = "missing_secret"
		return
	}
	plan, ok := CodexRequestPlanFromContext(c.Request.Context())
	if !ok {
		comparison.ErrorCategory = "missing_plan"
		return
	}
	profileID := strings.TrimSpace(account.GetExtraString(CodexClientProfileExtraKey))
	if profileID == "" {
		profileID = CodexProfileCLI
	}
	mode, err := resolveCodexFingerprintModeForFinalizer(account)
	if err != nil {
		comparison.ErrorCategory = "invalid_identity_mode"
		return
	}
	deriver, err := NewCodexIdentityDeriver(s.cfg.Gateway.OpenAIAffinity.Secret)
	if err != nil {
		comparison.ErrorCategory = "invalid_secret"
		return
	}
	credentialVersion := deriver.DigestHex("codex/credential-version/v2", account.GetOpenAIAccessToken())
	proxyIdentity := "direct"
	if account.ProxyID != nil {
		proxyIdentity = fmt.Sprintf("proxy:%d", *account.ProxyID)
	}
	candidate, err := FinalizeCodexAttempt(plan, CodexAttemptInput{
		AccountID:              account.ID,
		AccountVersion:         account.UpdatedAt.UTC().Format("20060102T150405.000000000Z"),
		CredentialVersion:      credentialVersion,
		ProxyIdentity:          proxyIdentity,
		ProfileID:              profileID,
		FingerprintMode:        string(mode),
		AttemptNumber:          0,
		TransportConfigVersion: "tls:" + strconv.FormatInt(account.GetTLSFingerprintProfileID(), 10),
	}, s.cfg.Gateway.OpenAIAffinity.Secret)
	if err != nil {
		comparison.ErrorCategory = "v2_finalize_failed"
		return
	}
	v2Identity := candidate.Identity()
	comparison.IdentityMatch = codexShadowIdentityEqual(legacy, v2Identity)
	legacyApp := resolveCodexOutboundIdentity(account.GetOpenAIUserAgent())
	profile := candidate.Profile()
	comparison.ApplicationMatch = legacyApp.userAgent == profile.App.UserAgent && legacyApp.originator == profile.App.Originator && legacyApp.version == profile.App.Version
	legacyBody, legacyBodyErr := applyCodexFingerprintToRawBody(plan.body, legacy)
	comparison.BodyMatch = legacyBodyErr == nil && bytes.Equal(legacyBody, candidate.FinalHTTPBody())
	comparison.TransportTupleMatch = profile.Transport.TLSProfileID == "chrome_auto" && profile.Transport.HTTP2ProfileID == "chrome-h2-v1"
	if !comparison.IdentityMatch {
		comparison.DifferenceCategories = append(comparison.DifferenceCategories, "identity")
	}
	if !comparison.ApplicationMatch {
		comparison.DifferenceCategories = append(comparison.DifferenceCategories, "application")
	}
	if !comparison.BodyMatch {
		comparison.DifferenceCategories = append(comparison.DifferenceCategories, "body")
	}
	if !comparison.TransportTupleMatch {
		comparison.DifferenceCategories = append(comparison.DifferenceCategories, "transport")
	}
}

func codexShadowIdentityEqual(left, right *CodexIdentitySnapshot) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.mode == right.mode &&
		left.installationID == right.installationID &&
		left.sessionID == right.sessionID &&
		left.threadID == right.threadID &&
		left.windowID == right.windowID
}

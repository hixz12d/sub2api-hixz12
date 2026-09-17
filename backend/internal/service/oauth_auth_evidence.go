package service

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type oauthHTTPBearerKey struct{}
type oauthUnauthorizedRecorder interface {
	RecordOAuthUnauthorized(context.Context, *Account, string, bool, time.Time) error
}

// Only a request actually sent to the fixed official HTTPS host supplies proof.
// An account snapshot alone cannot identify a cached/in-flight HTTP credential.
func withOAuthResponseCredential(ctx context.Context, response *http.Response) context.Context {
	if response == nil || response.Request == nil || response.Request.URL == nil {
		return ctx
	}
	request := response.Request
	if request.URL.Scheme != "https" || request.URL.Host != "chatgpt.com" {
		return ctx
	}
	auth := request.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") || strings.TrimSpace(auth[7:]) == "" {
		return ctx
	}
	return context.WithValue(ctx, oauthHTTPBearerKey{}, auth[7:])
}

func (s *RateLimitService) recordVersionedOAuth401(ctx context.Context, account *Account, permanent bool) bool {
	token, _ := ctx.Value(oauthHTTPBearerKey{}).(string)
	recorder, supported := s.accountRepo.(oauthUnauthorizedRecorder)
	if token == "" || !supported || account == nil || !account.IsOpenAI() || !account.IsOAuth() || account.IsShadow() {
		return false
	}
	// A cached token differing from the request snapshot has no safe version;
	// don't let that response disable the current credential.
	if token != account.GetCredential("access_token") {
		return true
	}
	minutes := 10
	if s.cfg != nil && s.cfg.RateLimit.OAuth401CooldownMinutes > 0 {
		minutes = s.cfg.RateLimit.OAuth401CooldownMinutes
	}
	until := time.Now().Add(time.Duration(minutes) * time.Minute)
	// Legacy imports may have no _token_version. The repository still compares
	// the complete credential snapshot before persisting their 401 state; only
	// versioned credentials receive automatic-recovery evidence.
	// Failure leaves no recoverable marker and must never fall back to a blind
	// overwrite. The request still fails over; the shared outbox handles state.
	if err := recorder.RecordOAuthUnauthorized(ctx, account, token, permanent, until); err != nil {
		slog.Warn("oauth_auth_evidence_write_failed", "account_id", account.ID)
	}
	if s.tokenCacheInvalidator != nil {
		_ = s.tokenCacheInvalidator.InvalidateToken(ctx, account)
	}
	return true
}

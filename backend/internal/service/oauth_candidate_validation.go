package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const OAuthValidationScope = "codex_identity_usage_catalog"

var ErrOAuthValidationFailed = infraerrors.BadRequest("OAUTH_VALIDATION_FAILED", "candidate identity or Codex access could not be verified; no credentials were written")
var ErrOAuthAuthUnattributed = infraerrors.Conflict("AUTH_ERROR_UNATTRIBUTED", "authentication error has no matching credential-version evidence; no credentials were written")

type oauthCandidateValidator interface {
	Validate(context.Context, *Account, map[string]any) error
}

type oauthHTTPValidator struct {
	factory PrivacyClientFactory
	egress  OpenAIEgressResolver
}

// OAuthAccessTokenHash binds evidence to the actual HTTP bearer, never an ID token.
func OAuthAccessTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (v *oauthHTTPValidator) Validate(ctx context.Context, account *Account, candidate map[string]any) error {
	if v.factory == nil || v.egress == nil {
		return ErrOAuthValidationFailed
	}
	route, err := v.egress.Resolve(ctx, account)
	if err != nil {
		return ErrOAuthValidationFailed
	}
	wrapped, err := v.factory(route.ProxyURL)
	if err != nil || wrapped == nil {
		return ErrOAuthValidationFailed
	}
	// Use the existing proxy/TLS transport, with no automatic retries, redirects,
	// token-provider lookup, refresh, response cache or raw-body logging.
	client := &http.Client{Transport: wrapped.GetClient().Transport, Timeout: 20 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	return validateOAuthCandidateHTTP(ctx, client, account, candidate)
}

func validateOAuthCandidateHTTP(ctx context.Context, client *http.Client, account *Account, candidate map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	token := syncIdentityString(candidate, "access_token")
	official := syncIdentityString(account.Credentials, "chatgpt_account_id")
	email := syncIdentityString(account.Credentials, "email")
	if email == "" {
		email = syncIdentityString(account.Extra, "email")
	}
	if token == "" || official == "" || email == "" {
		return ErrOAuthValidationFailed
	}
	// A workspace, when specified, must agree with the account identity upstream.
	for _, key := range []string{"workspace_id", "organization_uuid", "organization_id"} {
		if value := syncIdentityString(account.Credentials, key); value != "" && value != official {
			return ErrOAuthValidationFailed
		}
	}
	if value := syncIdentityString(account.Extra, "workspace_id"); value != "" && value != official {
		return ErrOAuthValidationFailed
	}
	read := func(path string, out any) error {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://chatgpt.com/backend-api/"+path, nil)
		if err != nil {
			return ErrOAuthValidationFailed
		}
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("ChatGPT-Account-Id", official)
		request.Header.Set("Accept", "application/json")
		request.Header.Set("OpenAI-Beta", "codex-1")
		request.Header.Set("Originator", "codex_cli_rs")
		response, err := client.Do(request)
		if err != nil {
			return ErrOAuthValidationFailed
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusOK {
			return ErrOAuthValidationFailed
		}
		body, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
		if err != nil || len(body) > 1<<20 || json.Unmarshal(body, out) != nil {
			return ErrOAuthValidationFailed
		}
		return nil
	}
	var usage struct {
		Email     string `json:"email"`
		AccountID string `json:"account_id"`
		UserID    string `json:"user_id"`
		RateLimit *struct {
			Allowed      *bool `json:"allowed"`
			LimitReached *bool `json:"limit_reached"`
		} `json:"rate_limit"`
	}
	if err := read("wham/usage", &usage); err != nil {
		return err
	}
	if !strings.EqualFold(usage.Email, email) || usage.AccountID != official || usage.UserID == "" || usage.RateLimit == nil || usage.RateLimit.Allowed == nil || usage.RateLimit.LimitReached == nil {
		return ErrOAuthValidationFailed
	}
	if expectedUser := syncIdentityString(account.Credentials, "chatgpt_user_id"); expectedUser != "" && usage.UserID != expectedUser {
		return ErrOAuthValidationFailed
	}
	if !*usage.RateLimit.Allowed || *usage.RateLimit.LimitReached {
		return ErrOAuthValidationFailed
	}
	var manifest struct {
		Models []struct {
			Slug string `json:"slug"`
		} `json:"models"`
	}
	if err := read("codex/models?client_version=0.101.0", &manifest); err != nil {
		return err
	}
	for _, model := range manifest.Models {
		if strings.TrimSpace(model.Slug) != "" {
			return nil
		}
	}
	return ErrOAuthValidationFailed
}

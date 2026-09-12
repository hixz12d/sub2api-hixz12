package service

import (
	"context"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// ReadOAuthSyncAccessToken is a conditional secret read for the integration.
// It never refreshes a token or returns refresh/ID/session tokens.
func (s *OAuthSyncService) ReadOAuthSyncAccessToken(ctx context.Context, id int64, instance string, version int64, updatedAt string) (map[string]any, error) {
	current, err := s.InstanceID(ctx)
	if err != nil {
		return nil, ErrIdempotencyStoreUnavail
	}
	if instance == "" || instance != current {
		return nil, infraerrors.Conflict("SYNC_INSTANCE_MISMATCH", "integration instance changed")
	}
	stamp, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil || version <= 0 {
		return nil, ErrOAuthSyncInvalid
	}
	account, err := s.accounts.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, ErrAccountNotFound
	}
	if !account.IsOpenAI() || !account.IsOAuth() || account.IsShadow() {
		return nil, ErrOAuthSyncInvalid
	}
	if version != account.GetCredentialAsInt64("_token_version") || !stamp.Equal(account.UpdatedAt) {
		return nil, ErrOAuthSyncConflict
	}
	access, client := account.GetCredential("access_token"), account.GetCredential("client_id")
	if strings.TrimSpace(access) == "" || strings.TrimSpace(client) == "" || strings.TrimSpace(account.GetCredential("refresh_token")) == "" {
		return nil, ErrOAuthSyncInvalid
	}
	return map[string]any{"schema_version": 1, "instance_id": current, "remote_account_id": id,
		"credential_version": version, "account_updated_at": account.UpdatedAt,
		"access_token": access, "client_id": client, "refresh_configured": true}, nil
}

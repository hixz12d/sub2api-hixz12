package service

import (
	"context"
	"time"
)

type OpenAIRefreshHandoffRequest struct {
	Action             string         `json:"action"`
	OperationID        string         `json:"operation_id"`
	ExpectedInstanceID string         `json:"expected_instance_id"`
	ExpectedVersion    int64          `json:"expected_credential_version"`
	ExpectedUpdatedAt  string         `json:"expected_updated_at"`
	ExpectedIdentity   map[string]any `json:"expected_identity"`
}
type OpenAIRefreshHandoffRepository interface {
	OpenAIRefreshHandoff(context.Context, *Account, string, *OpenAIRefreshHandoffRequest) (map[string]any, error)
}

func (s *OAuthSyncService) RefreshHandoffSupported() bool {
	_, ok := s.accounts.(OpenAIRefreshHandoffRepository)
	return ok
}
func (s *OAuthSyncService) RefreshHandoff(ctx context.Context, scope string, id int64, req *OpenAIRefreshHandoffRequest) (map[string]any, error) {
	repo, ok := s.accounts.(OpenAIRefreshHandoffRepository)
	if !ok {
		return nil, ErrOpenAIRefreshFenced
	}
	instance, err := s.InstanceID(ctx)
	if err != nil {
		return nil, err
	}
	if instance != req.ExpectedInstanceID {
		return nil, ErrOAuthSyncConflict
	}
	key, err := NormalizeIdempotencyKey(req.OperationID)
	if err != nil || key == "" || key != req.OperationID {
		return nil, ErrOAuthSyncConflict
	}
	if req.ExpectedVersion <= 0 {
		return nil, ErrOAuthSyncConflict
	}
	if _, err = time.Parse(time.RFC3339Nano, req.ExpectedUpdatedAt); err != nil {
		return nil, ErrOAuthSyncConflict
	}
	if req.Action != "prepare" && req.Action != "read" && req.Action != "ack" {
		return nil, ErrOAuthSyncConflict
	}
	account, err := s.accounts.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if account == nil || !account.IsOpenAIOAuth() || account.IsCredentialShadow() || account.IsOpenAIPersonalAccessToken() {
		return nil, ErrOAuthSyncConflict
	}
	if err = identityMatches(account, req.ExpectedIdentity); err != nil {
		return nil, err
	}
	result, err := repo.OpenAIRefreshHandoff(ctx, account, scope, req)
	if err != nil {
		return nil, err
	}
	result["instance_id"] = instance
	result["remote_account_id"] = id
	result["operation_id"] = req.OperationID
	return result, nil
}

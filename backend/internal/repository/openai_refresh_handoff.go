package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"time"
)

func handoffIdentity(a *service.Account) map[string]any {
	m := map[string]any{}
	for _, k := range []string{"email", "chatgpt_account_id", "chatgpt_user_id", "workspace_id", "organization_id", "organization_uuid", "client_id"} {
		m[k] = a.GetCredential(k)
	}
	for _, k := range []string{"email", "workspace_id"} {
		m["extra_"+k] = a.Extra[k]
	}
	return m
}
func (r *accountRepository) IsOpenAIRefreshDelegated(ctx context.Context, id int64) (bool, error) {
	rows, err := r.sql.QueryContext(ctx, "SELECT EXISTS(SELECT 1 FROM openai_refresh_delegated_accounts WHERE account_id=$1)", id)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	var found bool
	if rows.Next() {
		err = rows.Scan(&found)
	}
	if err == nil {
		err = rows.Err()
	}
	return found, err
}

func (r *accountRepository) OpenAIRefreshHandoff(ctx context.Context, before *service.Account, scope string, req *service.OpenAIRefreshHandoffRequest) (map[string]any, error) {
	tx, err := r.refreshTx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var grant string
	err = tx.QueryRowContext(ctx, "SELECT grant_id::text FROM openai_refresh_handoffs WHERE operation_id=$1", req.OperationID).Scan(&grant)
	if errors.Is(err, sql.ErrNoRows) {
		if req.Action != "prepare" || before.GetCredential("refresh_token") == "" {
			return nil, service.ErrOpenAIRefreshFenced
		}
		grant, err = ensureOpenAIRefreshGrant(ctx, tx, service.OAuthAccessTokenHash(before.GetCredential("refresh_token")))
	}
	if err != nil {
		return nil, err
	}
	var owner string
	var epoch int64
	var active sql.NullString
	var uncertain bool
	err = tx.QueryRowContext(ctx, "SELECT owner,epoch,attempt::text,uncertain FROM openai_refresh_grants WHERE id=$1 FOR UPDATE", grant).Scan(&owner, &epoch, &active, &uncertain)
	if err != nil {
		return nil, err
	}
	var state, storedScope string
	var accountID, version int64
	var stamp time.Time
	var released sql.NullInt64
	err = tx.QueryRowContext(ctx, "SELECT state,scope,account_id,expected_version,expected_updated_at,released_version FROM openai_refresh_handoffs WHERE operation_id=$1 FOR UPDATE", req.OperationID).Scan(&state, &storedScope, &accountID, &version, &stamp, &released)
	expectedStamp, _ := time.Parse(time.RFC3339Nano, req.ExpectedUpdatedAt)
	newOperation := errors.Is(err, sql.ErrNoRows)
	if errors.Is(err, sql.ErrNoRows) {
		if owner != "sub2api" || req.Action != "prepare" {
			return nil, service.ErrOpenAIRefreshFenced
		}
		if before.GetCredentialAsInt64("_token_version") != req.ExpectedVersion || !before.UpdatedAt.Equal(expectedStamp) {
			return nil, service.ErrOAuthSyncConflict
		}
		identity, _ := json.Marshal(handoffIdentity(before))
		_, err = tx.ExecContext(ctx, `INSERT INTO openai_refresh_handoffs(operation_id,scope,account_id,grant_id,expected_version,expected_updated_at,identity,epoch) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb,$8)`, req.OperationID, scope, before.ID, grant, req.ExpectedVersion, expectedStamp, string(identity), epoch)
		if err != nil {
			return nil, err
		}
		_, err = tx.ExecContext(ctx, "UPDATE openai_refresh_grants SET owner='draining' WHERE id=$1", grant)
		if err != nil {
			return nil, err
		}
		state, owner = "draining", "draining"
	} else if err != nil {
		return nil, err
	} else if storedScope != scope || accountID != before.ID || version != req.ExpectedVersion || !stamp.Equal(expectedStamp) {
		return nil, service.ErrOAuthSyncConflict
	}
	// Recheck durable identity under the account lock. Credential rotation may finish
	// while draining, but identity changes never authorize a different handoff.
	var raw, extra []byte
	var currentStamp time.Time
	err = tx.QueryRowContext(ctx, `SELECT credentials,extra,updated_at FROM accounts WHERE id=$1 AND deleted_at IS NULL AND platform='openai' AND type='oauth' FOR UPDATE`, before.ID).Scan(&raw, &extra, &currentStamp)
	if err != nil {
		return nil, err
	}
	current := &service.Account{ID: before.ID}
	if json.Unmarshal(raw, &current.Credentials) != nil {
		return nil, service.ErrOAuthSyncConflict
	}
	_ = json.Unmarshal(extra, &current.Extra)
	if newOperation && (current.GetCredentialAsInt64("_token_version") != req.ExpectedVersion || !currentStamp.Equal(expectedStamp)) {
		return nil, service.ErrOAuthSyncConflict
	}
	identity, _ := json.Marshal(handoffIdentity(current))
	var same bool
	if err = tx.QueryRowContext(ctx, "SELECT identity=$2::jsonb FROM openai_refresh_handoffs WHERE operation_id=$1", req.OperationID, string(identity)).Scan(&same); err != nil {
		return nil, err
	}
	if !same {
		return nil, service.ErrOAuthSyncConflict
	}
	if state == "draining" && !active.Valid && !uncertain {
		token := current.GetCredential("refresh_token")
		var linked string
		var consumed bool
		if token == "" || current.GetCredential("access_token") == "" || current.GetCredential("client_id") == "" {
			return nil, service.ErrOpenAIRefreshUncertain
		}
		err = tx.QueryRowContext(ctx, "SELECT grant_id::text,consumed FROM openai_refresh_tokens WHERE token_hash=$1", service.OAuthAccessTokenHash(token)).Scan(&linked, &consumed)
		if err != nil || linked != grant || consumed {
			return nil, service.ErrOpenAIRefreshUncertain
		}
		escrow := map[string]any{}
		for _, k := range []string{"access_token", "refresh_token", "client_id", "expires_at"} {
			if v, ok := current.Credentials[k]; ok {
				escrow[k] = v
			}
		}
		encoded, err := json.Marshal(escrow)
		if err != nil {
			return nil, err
		}
		_, err = tx.ExecContext(ctx, `WITH changed AS (UPDATE accounts SET credentials=jsonb_set(credentials-'refresh_token'-'id_token'-'session_token','{_token_version}',
   to_jsonb(GREATEST((EXTRACT(EPOCH FROM clock_timestamp())*1000)::bigint,COALESCE((credentials->>'_token_version')::bigint,0)+1))),updated_at=clock_timestamp()
   WHERE deleted_at IS NULL AND platform='openai' AND type='oauth' AND
   encode(sha256(convert_to(COALESCE(credentials->>'refresh_token',''),'UTF8')),'hex') IN (SELECT token_hash FROM openai_refresh_tokens WHERE grant_id=$1)
   RETURNING id), delegated AS (INSERT INTO openai_refresh_delegated_accounts(account_id,grant_id) SELECT id,$1 FROM changed ON CONFLICT DO NOTHING RETURNING account_id)
   INSERT INTO scheduler_outbox(event_type,account_id,payload) SELECT $2,id,NULL FROM changed`, grant, service.SchedulerOutboxEventAccountChanged)
		if err != nil {
			return nil, err
		}
		if err = tx.QueryRowContext(ctx, "SELECT (credentials->>'_token_version')::bigint FROM accounts WHERE id=$1", before.ID).Scan(&released); err != nil {
			return nil, err
		}
		epoch++
		_, err = tx.ExecContext(ctx, "UPDATE openai_refresh_grants SET owner='team',epoch=$2 WHERE id=$1", grant, epoch)
		if err != nil {
			return nil, err
		}
		_, err = tx.ExecContext(ctx, "UPDATE openai_refresh_handoffs SET state='ready',epoch=$2,escrow=$3::jsonb,released_version=$4 WHERE operation_id=$1", req.OperationID, epoch, string(encoded), released.Int64)
		if err != nil {
			return nil, err
		}
		state = "ready"
	}
	result := map[string]any{"state": state, "epoch": epoch, "credential_version": released.Int64, "uncertain": uncertain}
	if req.Action == "read" {
		if state != "ready" {
			return nil, service.ErrOpenAIRefreshFenced
		}
		var encoded []byte
		if err = tx.QueryRowContext(ctx, "SELECT escrow FROM openai_refresh_handoffs WHERE operation_id=$1", req.OperationID).Scan(&encoded); err != nil {
			return nil, err
		}
		var credentials map[string]any
		if json.Unmarshal(encoded, &credentials) != nil || credentials == nil {
			return nil, service.ErrOpenAIRefreshUncertain
		}
		result["credentials"] = credentials
	}
	if req.Action == "ack" {
		if state != "ready" && state != "acknowledged" {
			return nil, service.ErrOpenAIRefreshFenced
		}
		_, err = tx.ExecContext(ctx, "UPDATE openai_refresh_handoffs SET escrow=NULL,state='acknowledged' WHERE operation_id=$1", req.OperationID)
		if err != nil {
			return nil, err
		}
		result["state"] = "acknowledged"
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

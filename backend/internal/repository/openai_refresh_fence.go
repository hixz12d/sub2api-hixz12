package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"time"
)

func (r *accountRepository) refreshTx(ctx context.Context) (*sql.Tx, error) {
	db, ok := r.sql.(*sql.DB)
	if !ok {
		return nil, service.ErrOpenAIRefreshFenced
	}
	return db.BeginTx(ctx, nil)
}
func ensureOpenAIRefreshGrant(ctx context.Context, tx *sql.Tx, hash string) (string, error) {
	var id string
	err := tx.QueryRowContext(ctx, "SELECT grant_id::text FROM openai_refresh_tokens WHERE token_hash=$1", hash).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	var created string
	if err = tx.QueryRowContext(ctx, "INSERT INTO openai_refresh_grants DEFAULT VALUES RETURNING id::text").Scan(&created); err != nil {
		return "", err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO openai_refresh_tokens(token_hash,grant_id) VALUES($1,$2) ON CONFLICT DO NOTHING", hash, created)
	if err != nil {
		return "", err
	}
	err = tx.QueryRowContext(ctx, "SELECT grant_id::text FROM openai_refresh_tokens WHERE token_hash=$1", hash).Scan(&id)
	if err != nil {
		return "", err
	}
	if id != created {
		_, err = tx.ExecContext(ctx, "DELETE FROM openai_refresh_grants WHERE id=$1", created)
	}
	return id, err
}
func (r *accountRepository) BeginOpenAIRefresh(ctx context.Context, token string) (*service.OpenAIRefreshTicket, error) {
	if token == "" {
		return nil, service.ErrOpenAIRefreshFenced
	}
	tx, err := r.refreshTx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	hash := service.OAuthAccessTokenHash(token)
	id, err := ensureOpenAIRefreshGrant(ctx, tx, hash)
	if err != nil {
		return nil, err
	}
	ticket := &service.OpenAIRefreshTicket{GrantID: id, TokenHash: hash}
	err = tx.QueryRowContext(ctx, `UPDATE openai_refresh_grants SET attempt=gen_random_uuid(),attempt_started_at=clock_timestamp()
 WHERE id=$1 AND owner='sub2api' AND attempt IS NULL AND NOT uncertain
 AND EXISTS(SELECT 1 FROM openai_refresh_tokens WHERE token_hash=$2 AND NOT consumed)
 RETURNING epoch,attempt::text`, id, hash).Scan(&ticket.Epoch, &ticket.Attempt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrOpenAIRefreshFenced
	}
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return ticket, nil
}
func (r *accountRepository) MarkOpenAIRefreshUncertain(ctx context.Context, t *service.OpenAIRefreshTicket) error {
	_, err := r.sql.ExecContext(ctx, "UPDATE openai_refresh_grants SET uncertain=TRUE WHERE id=$1 AND epoch=$2 AND attempt=$3", t.GrantID, t.Epoch, t.Attempt)
	return err
}
func (r *accountRepository) FinishOpenAIRefresh(ctx context.Context, t *service.OpenAIRefreshTicket, before *service.Account, credentials map[string]any, nextRT string) (bool, error) {
	tx, err := r.refreshTx(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	var id string
	err = tx.QueryRowContext(ctx, `SELECT id::text FROM openai_refresh_grants WHERE id=$1 AND epoch=$2 AND attempt=$3 AND owner IN ('sub2api','draining') AND NOT uncertain FOR UPDATE`, t.GrantID, t.Epoch, t.Attempt).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, service.ErrOpenAIRefreshFenced
	}
	if err != nil {
		return false, err
	}
	nextHash := service.OAuthAccessTokenHash(nextRT)
	// A provider may preserve its RT; that remains reusable only after this attempt commits.
	if nextRT == "" {
		nextHash = t.TokenHash
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO openai_refresh_tokens(token_hash,grant_id) VALUES($1,$2) ON CONFLICT DO NOTHING", nextHash, id)
	if err != nil {
		return false, err
	}
	var linked string
	if err = tx.QueryRowContext(ctx, "SELECT grant_id::text FROM openai_refresh_tokens WHERE token_hash=$1", nextHash).Scan(&linked); err != nil {
		return false, err
	}
	if linked != id {
		return false, service.ErrOpenAIRefreshUncertain
	}
	applied := true
	if before != nil {
		oldJSON, marshalErr := json.Marshal(normalizeJSONMap(before.Credentials))
		if marshalErr != nil {
			return false, marshalErr
		}
		version := time.Now().UnixMilli()
		if prior := before.GetCredentialAsInt64("_token_version"); version <= prior {
			version = prior + 1
		}
		credentials["_token_version"] = version
		newJSON, marshalErr := json.Marshal(normalizeJSONMap(credentials))
		if marshalErr != nil {
			return false, marshalErr
		}
		result, updateErr := tx.ExecContext(ctx, `UPDATE accounts SET credentials=$1::jsonb,updated_at=GREATEST(clock_timestamp(),updated_at+INTERVAL '1 microsecond')
 WHERE id=$2 AND deleted_at IS NULL AND platform='openai' AND type='oauth' AND credentials=$3::jsonb`, string(newJSON), before.ID, string(oldJSON))
		if updateErr != nil {
			return false, updateErr
		}
		count, countErr := result.RowsAffected()
		if countErr != nil {
			return false, countErr
		}
		applied = count == 1
		if applied {
			_, err = tx.ExecContext(ctx, "INSERT INTO scheduler_outbox(event_type,account_id,payload) VALUES($1,$2,NULL)", service.SchedulerOutboxEventAccountChanged, before.ID)
			if err != nil {
				return false, err
			}
		}
	}
	_, err = tx.ExecContext(ctx, "UPDATE openai_refresh_tokens SET consumed=TRUE WHERE token_hash=$1 AND token_hash<>$2", t.TokenHash, nextHash)
	if err != nil {
		return false, err
	}
	// A CAS loser must not leave the old RT reusable, including nonrotating providers.
	if !applied {
		_, err = tx.ExecContext(ctx, "UPDATE openai_refresh_tokens SET consumed=TRUE WHERE token_hash=$1", t.TokenHash)
		if err != nil {
			return false, err
		}
	}
	_, err = tx.ExecContext(ctx, "UPDATE openai_refresh_grants SET attempt=NULL,attempt_started_at=NULL WHERE id=$1", id)
	if err != nil {
		return false, err
	}
	return applied, tx.Commit()
}

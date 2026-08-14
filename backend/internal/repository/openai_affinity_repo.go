package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type openAIAffinityRepository struct{ db *sql.DB }

func NewOpenAIAffinityRepository(db *sql.DB) service.OpenAIAffinityRepository {
	return &openAIAffinityRepository{db: db}
}

const openAISessionBindingColumns = `id, owner_scope_hash, provider, namespace_hash, primary_hash,
account_id, strength, source, stateful, capability, created_at, updated_at, last_hit_at, expires_at, version`
const openAIResponseBindingColumns = `id, owner_scope_hash, provider, response_key_hash, account_id,
session_binding_id, capability, created_at, last_hit_at, expires_at, version`

func scanOpenAISessionBinding(row interface{ Scan(...any) error }) (*service.OpenAISessionBinding, error) {
	var binding service.OpenAISessionBinding
	var strength int16
	if err := row.Scan(&binding.ID, &binding.OwnerScopeHash, &binding.Provider, &binding.NamespaceHash,
		&binding.PrimaryHash, &binding.AccountID, &strength, &binding.Source, &binding.Stateful,
		&binding.Capability, &binding.CreatedAt, &binding.UpdatedAt, &binding.LastHitAt,
		&binding.ExpiresAt, &binding.Version); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrOpenAIAffinityNotFound
		}
		return nil, err
	}
	binding.Strength = service.AffinityStrength(strength)
	return &binding, nil
}

func scanOpenAIResponseBinding(row interface{ Scan(...any) error }) (*service.OpenAIResponseBinding, error) {
	var binding service.OpenAIResponseBinding
	var sessionID sql.NullInt64
	if err := row.Scan(&binding.ID, &binding.OwnerScopeHash, &binding.Provider, &binding.ResponseKeyHash,
		&binding.AccountID, &sessionID, &binding.Capability, &binding.CreatedAt, &binding.LastHitAt,
		&binding.ExpiresAt, &binding.Version); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrOpenAIAffinityNotFound
		}
		return nil, err
	}
	if sessionID.Valid {
		binding.SessionBindingID = &sessionID.Int64
	}
	return &binding, nil
}

func (r *openAIAffinityRepository) ResolveResponse(ctx context.Context, ownerScopeHash, provider, responseKeyHash string, now time.Time, refreshTTL, refreshMinInterval time.Duration) (*service.OpenAIResponseBinding, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("openai affinity repository unavailable")
	}
	binding, err := scanOpenAIResponseBinding(r.db.QueryRowContext(ctx, `SELECT `+openAIResponseBindingColumns+`
FROM gateway_response_bindings
WHERE owner_scope_hash=$1 AND provider=$2 AND response_key_hash=$3 AND expires_at>$4`, ownerScopeHash, provider, responseKeyHash, now))
	if err != nil {
		return nil, err
	}
	if refreshTTL > 0 && refreshMinInterval > 0 && now.Sub(binding.LastHitAt) >= refreshMinInterval {
		_, _ = r.db.ExecContext(ctx, `UPDATE gateway_response_bindings
SET last_hit_at=$2, expires_at=GREATEST(expires_at,$3), version=version+1
WHERE id=$1 AND last_hit_at<=$4`, binding.ID, now, now.Add(refreshTTL), now.Add(-refreshMinInterval))
	}
	return binding, nil
}

func (r *openAIAffinityRepository) ResolveSession(ctx context.Context, ownerScopeHash, provider, namespaceHash, primaryHash string, aliases []string, now time.Time, refreshTTL, refreshMinInterval time.Duration) (*service.OpenAISessionBinding, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("openai affinity repository unavailable")
	}
	var binding *service.OpenAISessionBinding
	var err error
	if strings.TrimSpace(primaryHash) != "" {
		binding, err = scanOpenAISessionBinding(r.db.QueryRowContext(ctx, `SELECT `+openAISessionBindingColumns+`
FROM gateway_session_bindings
WHERE owner_scope_hash=$1 AND provider=$2 AND namespace_hash=$3 AND primary_hash=$4 AND expires_at>$5`, ownerScopeHash, provider, namespaceHash, primaryHash, now))
		if err == nil {
			goto refresh
		}
		if !errors.Is(err, service.ErrOpenAIAffinityNotFound) {
			return nil, err
		}
	}
	if len(aliases) == 0 {
		return nil, service.ErrOpenAIAffinityNotFound
	}
	binding, err = scanOpenAISessionBinding(r.db.QueryRowContext(ctx, `SELECT
b.id,b.owner_scope_hash,b.provider,b.namespace_hash,b.primary_hash,b.account_id,
b.strength,b.source,b.stateful,b.capability,b.created_at,b.updated_at,b.last_hit_at,b.expires_at,b.version
FROM gateway_session_binding_aliases a
JOIN gateway_session_bindings b ON b.id=a.binding_id
WHERE a.owner_scope_hash=$1 AND a.provider=$2 AND a.namespace_hash=$3
  AND a.alias_hash=ANY($4) AND b.expires_at>$5
ORDER BY b.strength DESC, b.updated_at DESC LIMIT 1`, ownerScopeHash, provider, namespaceHash, pq.Array(aliases), now))
	if err != nil {
		return nil, err
	}

refresh:
	if refreshTTL > 0 && refreshMinInterval > 0 && now.Sub(binding.LastHitAt) >= refreshMinInterval {
		_, _ = r.db.ExecContext(ctx, `UPDATE gateway_session_bindings
SET last_hit_at=$2, updated_at=$2, expires_at=GREATEST(expires_at,$3), version=version+1
WHERE id=$1 AND last_hit_at<=$4`, binding.ID, now, now.Add(refreshTTL), now.Add(-refreshMinInterval))
	}
	return binding, nil
}

func (r *openAIAffinityRepository) CreateOrGetSession(ctx context.Context, identity service.SessionIdentity, accountID int64, expiresAt time.Time) (*service.OpenAISessionBinding, bool, error) {
	if r == nil || r.db == nil {
		return nil, false, errors.New("openai affinity repository unavailable")
	}
	if identity.PrimaryHash == "" || accountID <= 0 {
		return nil, false, service.ErrOpenAIAffinityNotFound
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, `INSERT INTO gateway_session_bindings
(owner_scope_hash,provider,namespace_hash,primary_hash,account_id,strength,source,stateful,capability,created_at,updated_at,last_hit_at,expires_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$10,$10,$11)
ON CONFLICT (owner_scope_hash,provider,namespace_hash,primary_hash) DO NOTHING`,
		identity.OwnerScopeHash, identity.Provider, identity.NamespaceHash, identity.PrimaryHash,
		accountID, int16(identity.Strength), identity.Source, identity.Stateful, identity.Capability, now, expiresAt)
	if err != nil {
		return nil, false, err
	}
	rows, _ := result.RowsAffected()
	binding, err := scanOpenAISessionBinding(tx.QueryRowContext(ctx, `SELECT `+openAISessionBindingColumns+`
FROM gateway_session_bindings WHERE owner_scope_hash=$1 AND provider=$2 AND namespace_hash=$3 AND primary_hash=$4 FOR UPDATE`,
		identity.OwnerScopeHash, identity.Provider, identity.NamespaceHash, identity.PrimaryHash))
	if err != nil {
		return nil, false, err
	}
	created := rows == 1
	if !binding.ExpiresAt.After(now) {
		_, err = tx.ExecContext(ctx, `UPDATE gateway_session_bindings SET account_id=$2,strength=$3,source=$4,stateful=$5,capability=$6,
updated_at=$7,last_hit_at=$7,expires_at=$8,version=version+1 WHERE id=$1`, binding.ID, accountID, int16(identity.Strength),
			identity.Source, identity.Stateful, identity.Capability, now, expiresAt)
		if err != nil {
			return nil, false, err
		}
		binding.AccountID, binding.Strength, binding.Source, binding.Stateful = accountID, identity.Strength, identity.Source, identity.Stateful
		binding.Capability, binding.UpdatedAt, binding.LastHitAt, binding.ExpiresAt = identity.Capability, now, now, expiresAt
		binding.Version++
		created = true
	}
	for _, alias := range identity.Aliases {
		if alias == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO gateway_session_binding_aliases
(binding_id,owner_scope_hash,provider,namespace_hash,alias_hash,source)
VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (owner_scope_hash,provider,namespace_hash,alias_hash) DO NOTHING`, binding.ID,
			identity.OwnerScopeHash, identity.Provider, identity.NamespaceHash, alias, identity.Source); err != nil {
			return nil, false, err
		}
		var aliasBindingID int64
		var aliasExpiresAt time.Time
		if err := tx.QueryRowContext(ctx, `SELECT a.binding_id,b.expires_at FROM gateway_session_binding_aliases a
JOIN gateway_session_bindings b ON b.id=a.binding_id
WHERE a.owner_scope_hash=$1 AND a.provider=$2 AND a.namespace_hash=$3 AND a.alias_hash=$4`,
			identity.OwnerScopeHash, identity.Provider, identity.NamespaceHash, alias).Scan(&aliasBindingID, &aliasExpiresAt); err != nil {
			return nil, false, err
		}
		if aliasBindingID != binding.ID {
			if !aliasExpiresAt.After(now) {
				if _, err := tx.ExecContext(ctx, `UPDATE gateway_session_binding_aliases SET binding_id=$2,source=$3
WHERE owner_scope_hash=$4 AND provider=$5 AND namespace_hash=$6 AND alias_hash=$7 AND binding_id=$1`,
					aliasBindingID, binding.ID, identity.Source, identity.OwnerScopeHash, identity.Provider, identity.NamespaceHash, alias); err != nil {
					return nil, false, err
				}
				continue
			}
			_ = tx.Rollback()
			winner, winnerErr := scanOpenAISessionBinding(r.db.QueryRowContext(ctx, `SELECT `+openAISessionBindingColumns+` FROM gateway_session_bindings WHERE id=$1`, aliasBindingID))
			if winnerErr != nil {
				return nil, false, fmt.Errorf("%w: resolve alias winner: %v", service.ErrOpenAIAffinityConflict, winnerErr)
			}
			return winner, false, nil
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return binding, created, nil
}

func (r *openAIAffinityRepository) BindResponseAndUpgrade(ctx context.Context, identity service.SessionIdentity, responseKeyHash string, accountID int64, responseExpiresAt, strongSessionExpiresAt time.Time) (*service.OpenAIResponseBinding, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("openai affinity repository unavailable")
	}
	if responseKeyHash == "" || accountID <= 0 {
		return nil, service.ErrOpenAIAffinityNotFound
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC()
	var sessionID sql.NullInt64
	if identity.PrimaryHash != "" {
		if err := tx.QueryRowContext(ctx, `SELECT id FROM gateway_session_bindings
WHERE owner_scope_hash=$1 AND provider=$2 AND namespace_hash=$3 AND primary_hash=$4 FOR UPDATE`,
			identity.OwnerScopeHash, identity.Provider, identity.NamespaceHash, identity.PrimaryHash).Scan(&sessionID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	if !sessionID.Valid && len(identity.Aliases) > 0 {
		if err := tx.QueryRowContext(ctx, `SELECT b.id
FROM gateway_session_binding_aliases a
JOIN gateway_session_bindings b ON b.id=a.binding_id
WHERE a.owner_scope_hash=$1 AND a.provider=$2 AND a.namespace_hash=$3 AND a.alias_hash=ANY($4)
ORDER BY b.strength DESC, b.updated_at DESC LIMIT 1 FOR UPDATE OF b`,
			identity.OwnerScopeHash, identity.Provider, identity.NamespaceHash, pq.Array(identity.Aliases)).Scan(&sessionID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	if sessionID.Valid {
		result, err := tx.ExecContext(ctx, `UPDATE gateway_session_bindings
SET strength=GREATEST(strength,$2), stateful=TRUE, updated_at=$3, last_hit_at=$3,
    expires_at=GREATEST(expires_at,$4), version=version+1
WHERE id=$1 AND account_id=$5`, sessionID.Int64, int16(service.AffinityStrong), now, strongSessionExpiresAt, accountID)
		if err != nil {
			return nil, err
		}
		if affected, _ := result.RowsAffected(); affected != 1 {
			return nil, fmt.Errorf("%w: session belongs to another account", service.ErrOpenAIAffinityConflict)
		}
	}
	var existingAccount int64
	var existingExpiresAt time.Time
	err = tx.QueryRowContext(ctx, `SELECT account_id,expires_at FROM gateway_response_bindings
WHERE owner_scope_hash=$1 AND provider=$2 AND response_key_hash=$3 FOR UPDATE`, identity.OwnerScopeHash, identity.Provider, responseKeyHash).Scan(&existingAccount, &existingExpiresAt)
	if err == nil && existingAccount != accountID && existingExpiresAt.After(now) {
		return nil, fmt.Errorf("%w: response belongs to account %d", service.ErrOpenAIAffinityConflict, existingAccount)
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO gateway_response_bindings
(owner_scope_hash,provider,response_key_hash,account_id,session_binding_id,capability,created_at,last_hit_at,expires_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$7,$8)
ON CONFLICT (owner_scope_hash,provider,response_key_hash) DO UPDATE SET
account_id=CASE WHEN gateway_response_bindings.expires_at<=$7 THEN EXCLUDED.account_id ELSE gateway_response_bindings.account_id END,
session_binding_id=CASE WHEN gateway_response_bindings.expires_at<=$7 THEN EXCLUDED.session_binding_id ELSE COALESCE(gateway_response_bindings.session_binding_id,EXCLUDED.session_binding_id) END,
capability=CASE WHEN gateway_response_bindings.expires_at<=$7 THEN EXCLUDED.capability ELSE gateway_response_bindings.capability END,
last_hit_at=EXCLUDED.last_hit_at,expires_at=EXCLUDED.expires_at,version=gateway_response_bindings.version+1
WHERE gateway_response_bindings.account_id=EXCLUDED.account_id OR gateway_response_bindings.expires_at<=$7`, identity.OwnerScopeHash, identity.Provider,
		responseKeyHash, accountID, sessionID, identity.Capability, now, responseExpiresAt)
	if err != nil {
		return nil, err
	}
	binding, err := scanOpenAIResponseBinding(tx.QueryRowContext(ctx, `SELECT `+openAIResponseBindingColumns+`
FROM gateway_response_bindings WHERE owner_scope_hash=$1 AND provider=$2 AND response_key_hash=$3`, identity.OwnerScopeHash, identity.Provider, responseKeyHash))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return binding, nil
}

func (r *openAIAffinityRepository) ListMigrationCandidates(ctx context.Context, fromAccountID int64, includeExpired bool, limit int, now time.Time) ([]service.OpenAIAffinityMigrationCandidate, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id,account_id,strength,version,expires_at
FROM gateway_session_bindings WHERE account_id=$1 AND ($2 OR expires_at>$3)
ORDER BY id LIMIT $4`, fromAccountID, includeExpired, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []service.OpenAIAffinityMigrationCandidate
	for rows.Next() {
		var item service.OpenAIAffinityMigrationCandidate
		var strength int16
		if err := rows.Scan(&item.BindingID, &item.AccountID, &strength, &item.Version, &item.ExpiresAt); err != nil {
			return nil, err
		}
		item.Strength = service.AffinityStrength(strength)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *openAIAffinityRepository) MigrateBindingCAS(ctx context.Context, bindingID, fromAccountID, toAccountID, expectedVersion int64, reason string, now time.Time) error {
	if strings.TrimSpace(reason) == "" || fromAccountID <= 0 || toAccountID <= 0 || fromAccountID == toAccountID {
		return errors.New("explicit migration reason and distinct accounts are required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `UPDATE gateway_session_bindings SET account_id=$1,updated_at=$2,version=version+1
WHERE id=$3 AND account_id=$4 AND version=$5`, toAccountID, now, bindingID, fromAccountID, expectedVersion)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return service.ErrOpenAIAffinityCAS
	}
	if _, err := tx.ExecContext(ctx, `UPDATE gateway_response_bindings SET account_id=$1,last_hit_at=$2,version=version+1
WHERE session_binding_id=$3 AND account_id=$4`, toAccountID, now, bindingID, fromAccountID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO gateway_affinity_migration_audit
(binding_id,from_account_id,to_account_id,expected_version,resulting_version,reason,created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7)`, bindingID, fromAccountID, toAccountID, expectedVersion, expectedVersion+1, strings.TrimSpace(reason), now); err != nil {
		return err
	}
	return tx.Commit()
}

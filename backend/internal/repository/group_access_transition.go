package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

var _ service.GroupAccessTransitionRepository = (*groupRepository)(nil)

func (r *groupRepository) UpdatePreservingUserAccess(ctx context.Context, groupIn *service.Group, expectedUpdatedAt time.Time) error {
	if groupIn == nil || !groupIn.IsExclusive || groupIn.IsSubscriptionType() || expectedUpdatedAt.IsZero() {
		return service.ErrGroupAccessTransitionConflict
	}
	txCtx, txClient, tx, err := beginRepositoryTx(ctx, r.client)
	if err != nil {
		return err
	}
	if tx != nil {
		defer func() { _ = tx.Rollback() }()
	}

	// Reserve the public → exclusive transition with a CAS and row lock. A
	// retry or concurrent edit must not grant access to a later cohort of users.
	result, err := txClient.ExecContext(txCtx, `UPDATE groups SET is_exclusive = TRUE
		WHERE id = $1 AND deleted_at IS NULL AND is_exclusive = FALSE
		AND subscription_type = 'standard' AND updated_at = $2`, groupIn.ID, expectedUpdatedAt)
	if err != nil {
		return fmt.Errorf("reserve group access transition: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return service.ErrGroupAccessTransitionConflict
	}

	// Users with public-group restrictions only keep their existing explicit
	// grants. All other non-deleted users registered before this transaction
	// retain access, including users who have not created an API key yet.
	// ON CONFLICT preserves existing grants and their per-user RPM overrides.
	_, err = txClient.ExecContext(txCtx, `INSERT INTO user_allowed_groups (user_id, group_id, created_at)
		SELECT id, $1, transaction_timestamp() FROM users
		WHERE deleted_at IS NULL AND restrict_public_groups = FALSE
		AND created_at <= transaction_timestamp()
		ON CONFLICT (user_id, group_id) DO NOTHING`, groupIn.ID)
	if err != nil {
		return fmt.Errorf("preserve existing group users: %w", err)
	}
	// Reuse the normal update and scheduler outbox inside this transaction.
	if err := r.Update(txCtx, groupIn); err != nil {
		return err
	}
	if tx != nil {
		return tx.Commit()
	}
	return nil
}

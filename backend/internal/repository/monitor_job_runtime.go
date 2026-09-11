package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// CancelDisabledNext releases one queued job when its global feature gate has
// closed. In-flight work is cancelled by its heartbeat or lease-expiry recovery.
func (r *MonitorJobRepository) CancelDisabledNext(ctx context.Context, probe, platformCapability, privateCapability bool) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	var id string
	err = tx.QueryRowContext(ctx, `SELECT id FROM monitor_jobs WHERE state='queued' AND
 ((kind='availability' AND NOT $1::boolean) OR
 (kind='capability' AND source='platform_group' AND NOT $2::boolean) OR
 (kind='capability' AND source IN ('external_api','site_api_key') AND NOT $3::boolean))
 ORDER BY created_at,id LIMIT 1 FOR UPDATE SKIP LOCKED`, probe, platformCapability, privateCapability).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := terminateMonitorJobTx(ctx, tx, id, service.MonitorJobCancelled, "feature_disabled"); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

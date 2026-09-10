package repository

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
)

var (
	ErrMonitorBudgetInput     = errors.New("invalid monitor budget request")
	ErrMonitorBudgetExhausted = errors.New("monitor request budget exhausted")
	ErrMonitorBudgetConflict  = errors.New("monitor budget configuration or dispatch conflict")
	ErrMonitorBudgetLease     = errors.New("monitor job is not eligible for budget consumption")
)

// MonitorBudgetLimit is trusted deployment/policy configuration, never user input.
// Existing daily bucket limits cannot be increased by a reservation request.
type MonitorBudgetLimit = service.MonitorBudgetLimit

// MonitorBudgetRepository accounts for request counts, not supplier charges.
// It does not send requests or replace authorization, transport fencing, or the
// attempt ledger. Callers must never send after a failed/ambiguous Consume.
type MonitorBudgetRepository struct{ db *sql.DB }

func NewMonitorBudgetRepository(db *sql.DB) *MonitorBudgetRepository {
	return &MonitorBudgetRepository{db: db}
}

type monitorBudgetJob struct {
	state, kind          string
	owner, group         sql.NullInt64
	reserved, dispatched int64
}

type monitorBudgetRow struct {
	id, scope                    string
	scopeID                      int64
	day                          time.Time
	reserved, consumed, released int64
	settled                      sql.NullTime
}

func normalizeMonitorBudgetLimits(input []MonitorBudgetLimit) ([]MonitorBudgetLimit, error) {
	if len(input) < 2 || len(input) > 4 {
		return nil, ErrMonitorBudgetInput
	}
	limits := append([]MonitorBudgetLimit(nil), input...)
	sort.Slice(limits, func(i, j int) bool {
		if limits[i].Scope != limits[j].Scope {
			return limits[i].Scope < limits[j].Scope
		}
		return limits[i].ScopeID < limits[j].ScopeID
	})
	global := false
	for i, limit := range limits {
		if limit.RequestLimit < 0 || limit.RequestLimit > 1000000000 {
			return nil, ErrMonitorBudgetInput
		}
		switch limit.Scope {
		case "global":
			if limit.ScopeID != 0 {
				return nil, ErrMonitorBudgetInput
			}
			global = true
		case "user", "group", "probe_group", "credential":
			if limit.ScopeID <= 0 {
				return nil, ErrMonitorBudgetInput
			}
		default:
			return nil, ErrMonitorBudgetInput
		}
		if i > 0 && limits[i-1].Scope == limit.Scope {
			return nil, ErrMonitorBudgetInput
		}
	}
	if !global {
		return nil, ErrMonitorBudgetInput
	}
	return limits, nil
}

func lockMonitorBudgetJob(ctx context.Context, tx *sql.Tx, id string) (monitorBudgetJob, error) {
	var job monitorBudgetJob
	err := tx.QueryRowContext(ctx, `SELECT j.state,j.kind,j.owner_user_id,p.group_id,j.outbound_reserved,j.outbound_dispatched
 FROM monitor_jobs j LEFT JOIN channel_monitor_group_policies p ON p.id=j.policy_id
 WHERE j.id=$1 FOR UPDATE OF j`, id).Scan(&job.state, &job.kind, &job.owner, &job.group, &job.reserved, &job.dispatched)
	return job, err
}

func validateMonitorBudgetScopes(job monitorBudgetJob, limits []MonitorBudgetLimit) error {
	user, group := false, false
	for _, limit := range limits {
		switch limit.Scope {
		case "user":
			if !job.owner.Valid || job.owner.Int64 != limit.ScopeID {
				return ErrMonitorBudgetInput
			}
			user = true
		case "group", "probe_group":
			if (limit.Scope == "probe_group") != (job.kind == "availability") {
				return ErrMonitorBudgetInput
			}
			if !job.group.Valid || job.group.Int64 != limit.ScopeID {
				return ErrMonitorBudgetInput
			}
			group = true
		}
	}
	if job.owner.Valid != user || job.group.Valid != group {
		return ErrMonitorBudgetInput
	}
	return nil
}

func readMonitorBudgetRows(ctx context.Context, tx *sql.Tx, jobID string) ([]monitorBudgetRow, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id,scope,scope_id,utc_day,reserved,consumed,released,settled_at
 FROM monitor_budget_reservations WHERE job_id=$1 ORDER BY scope,scope_id,utc_day FOR UPDATE`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []monitorBudgetRow
	for rows.Next() {
		var row monitorBudgetRow
		if err := rows.Scan(&row.id, &row.scope, &row.scopeID, &row.day, &row.reserved, &row.consumed, &row.released, &row.settled); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func releaseMonitorBudgetRow(ctx context.Context, tx *sql.Tx, row monitorBudgetRow) error {
	if row.settled.Valid {
		return nil
	}
	remaining := row.reserved - row.consumed - row.released
	result, err := tx.ExecContext(ctx, `UPDATE monitor_budget_buckets SET reserved=reserved-$4,updated_at=clock_timestamp()
 WHERE scope=$1 AND scope_id=$2 AND utc_day=$3 AND reserved >= $4`, row.scope, row.scopeID, row.day, remaining)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil {
		return err
	} else if n != 1 {
		return ErrMonitorBudgetConflict
	}
	_, err = tx.ExecContext(ctx, `UPDATE monitor_budget_reservations SET released=reserved-consumed,settled_at=clock_timestamp() WHERE id=$1`, row.id)
	return err
}

// syncMonitorBudgetDay moves only unconsumed reservations. A failed new-day
// reservation rolls back releases too. All callers lock jobs first, then scopes
// in the same order; database time is checked again after waiting on bucket locks.
func syncMonitorBudgetDay(ctx context.Context, tx *sql.Tx, jobID string, job monitorBudgetJob, limits []MonitorBudgetLimit, existing []monitorBudgetRow) (time.Time, error) {
	var day time.Time
	if err := tx.QueryRowContext(ctx, `SELECT (clock_timestamp() AT TIME ZONE 'UTC')::date`).Scan(&day); err != nil {
		return day, err
	}
	if len(existing) > 0 {
		seen := make(map[string]int64)
		for _, row := range existing {
			seen[row.scope] = row.scopeID
		}
		if len(seen) != len(limits) {
			return day, ErrMonitorBudgetConflict
		}
		for _, limit := range limits {
			if id, ok := seen[limit.Scope]; !ok || id != limit.ScopeID {
				return day, ErrMonitorBudgetConflict
			}
		}
	}
	remaining := job.reserved - job.dispatched
	for _, limit := range limits {
		var current *monitorBudgetRow
		for i := range existing {
			row := &existing[i]
			if row.scope != limit.Scope {
				continue
			}
			if row.day.Equal(day) {
				current = row
				continue
			}
			if err := releaseMonitorBudgetRow(ctx, tx, *row); err != nil {
				return day, err
			}
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO monitor_budget_buckets(scope,scope_id,utc_day,request_limit)
 VALUES($1,$2,$3,$4) ON CONFLICT(scope,scope_id,utc_day) DO NOTHING`, limit.Scope, limit.ScopeID, day, limit.RequestLimit)
		if err != nil {
			return day, err
		}
		var requestLimit int64
		if err := tx.QueryRowContext(ctx, `SELECT request_limit FROM monitor_budget_buckets
 WHERE scope=$1 AND scope_id=$2 AND utc_day=$3 FOR UPDATE`, limit.Scope, limit.ScopeID, day).Scan(&requestLimit); err != nil {
			return day, err
		}
		if requestLimit != limit.RequestLimit {
			return day, ErrMonitorBudgetConflict
		}
		if current != nil {
			if current.settled.Valid || current.reserved-current.consumed-current.released != remaining {
				return day, ErrMonitorBudgetConflict
			}
			continue
		}
		result, err := tx.ExecContext(ctx, `UPDATE monitor_budget_buckets SET reserved=reserved+$4,updated_at=clock_timestamp()
 WHERE scope=$1 AND scope_id=$2 AND utc_day=$3 AND $4 <= request_limit-consumed-reserved`, limit.Scope, limit.ScopeID, day, remaining)
		if err != nil {
			return day, err
		}
		if n, err := result.RowsAffected(); err != nil {
			return day, err
		} else if n != 1 {
			return day, ErrMonitorBudgetExhausted
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO monitor_budget_reservations(id,job_id,scope,scope_id,utc_day,reserved)
 VALUES($1,$2,$3,$4,$5,$6)`, uuid.NewString(), jobID, limit.Scope, limit.ScopeID, day, remaining)
		if err != nil {
			return day, err
		}
	}
	return day, nil
}

// Reserve atomically reserves the full job ceiling across all required scopes.
// It is idempotent for the same queued job/configuration. This does not enqueue
// a job: the scheduler must not claim jobs until reservations exist.
func (r *MonitorBudgetRepository) Reserve(ctx context.Context, jobID string, input []MonitorBudgetLimit) error {
	if !validDetectorResourceID(jobID) {
		return ErrMonitorBudgetInput
	}
	limits, err := normalizeMonitorBudgetLimits(input)
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := reserveMonitorBudgetTx(ctx, tx, jobID, limits); err != nil {
		return err
	}
	return tx.Commit()
}

func reserveMonitorBudgetTx(ctx context.Context, tx *sql.Tx, jobID string, limits []MonitorBudgetLimit) error {
	job, err := lockMonitorBudgetJob(ctx, tx, jobID)
	if err != nil {
		return err
	}
	if job.state != "queued" || job.dispatched != 0 {
		return ErrMonitorBudgetLease
	}
	if err := validateMonitorBudgetScopes(job, limits); err != nil {
		return err
	}
	existing, err := readMonitorBudgetRows(ctx, tx, jobID)
	if err != nil {
		return err
	}
	day, err := syncMonitorBudgetDay(ctx, tx, jobID, job, limits, existing)
	if err != nil {
		return err
	}
	var eligible bool
	err = tx.QueryRowContext(ctx, `SELECT cancel_requested_at IS NULL AND queue_deadline_at > clock_timestamp()
 AND deadline_at > clock_timestamp() AND $2::date=(clock_timestamp() AT TIME ZONE 'UTC')::date
 FROM monitor_jobs WHERE id=$1`, jobID, day).Scan(&eligible)
	if err != nil {
		return err
	}
	if !eligible {
		return ErrMonitorBudgetLease
	}
	return nil
}

// Consume grants exactly one accounting permit using the expected dispatch count
// as a CAS. Duplicate/uncertain calls never return a fresh permit for that count.
// Persisting an attempt and physical dispatch still belong to the fenced bridge.
func (r *MonitorBudgetRepository) Consume(ctx context.Context, jobID, leaseOwner string, generation, expectedDispatched int64, input []MonitorBudgetLimit) error {
	if !validDetectorResourceID(jobID) || leaseOwner == "" || len(leaseOwner) > 200 || generation <= 0 || expectedDispatched < 0 {
		return ErrMonitorBudgetInput
	}
	limits, err := normalizeMonitorBudgetLimits(input)
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := consumeMonitorBudgetTx(ctx, tx, jobID, leaseOwner, generation, expectedDispatched, limits); err != nil {
		return err
	}
	return tx.Commit()
}

func consumeMonitorBudgetTx(ctx context.Context, tx *sql.Tx, jobID, leaseOwner string, generation, expectedDispatched int64, limits []MonitorBudgetLimit) error {
	job, err := lockMonitorBudgetJob(ctx, tx, jobID)
	if err != nil {
		return err
	}
	if job.state != "running" {
		return ErrMonitorBudgetLease
	}
	if job.dispatched != expectedDispatched {
		return ErrMonitorBudgetConflict
	}
	if job.dispatched >= job.reserved {
		return ErrMonitorBudgetExhausted
	}
	if job.kind == "capability" {
		if err := lockMonitorJobBenchmarksTx(ctx, tx, jobID); err != nil {
			return err
		}
	}
	if err := validateMonitorBudgetScopes(job, limits); err != nil {
		return err
	}
	existing, err := readMonitorBudgetRows(ctx, tx, jobID)
	if err != nil {
		return err
	}
	if len(existing) == 0 {
		return ErrMonitorBudgetConflict
	}
	day, err := syncMonitorBudgetDay(ctx, tx, jobID, job, limits, existing)
	if err != nil {
		return err
	}
	for _, limit := range limits {
		result, err := tx.ExecContext(ctx, `UPDATE monitor_budget_reservations SET consumed=consumed+1
 WHERE job_id=$1 AND scope=$2 AND scope_id=$3 AND utc_day=$4 AND settled_at IS NULL AND consumed+released<reserved`, jobID, limit.Scope, limit.ScopeID, day)
		if err != nil {
			return err
		}
		if n, err := result.RowsAffected(); err != nil {
			return err
		} else if n != 1 {
			return ErrMonitorBudgetConflict
		}
		result, err = tx.ExecContext(ctx, `UPDATE monitor_budget_buckets SET reserved=reserved-1,consumed=consumed+1,updated_at=clock_timestamp()
 WHERE scope=$1 AND scope_id=$2 AND utc_day=$3 AND reserved>0`, limit.Scope, limit.ScopeID, day)
		if err != nil {
			return err
		}
		if n, err := result.RowsAffected(); err != nil {
			return err
		} else if n != 1 {
			return ErrMonitorBudgetConflict
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE monitor_jobs j SET outbound_dispatched=outbound_dispatched+1
 WHERE id=$1 AND state='running' AND lease_owner=$2 AND lease_generation=$3 AND outbound_dispatched=$4
 AND cancel_requested_at IS NULL AND lease_expires_at > clock_timestamp() AND deadline_at > clock_timestamp()
 AND $5::date=(clock_timestamp() AT TIME ZONE 'UTC')::date AND `+monitorJobPolicyValid, jobID, leaseOwner, generation, expectedDispatched, day)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil {
		return err
	} else if n != 1 {
		return ErrMonitorBudgetLease
	}
	return nil
}

// Settle is safe for concurrent completion/cancellation and recovery sweeps.
// Running jobs cannot release permits; dispatched/uncertain requests stay charged.
func (r *MonitorBudgetRepository) Settle(ctx context.Context, jobID string) error {
	if !validDetectorResourceID(jobID) {
		return ErrMonitorBudgetInput
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := settleMonitorBudgetTx(ctx, tx, jobID); err != nil {
		return err
	}
	return tx.Commit()
}

func settleMonitorBudgetTx(ctx context.Context, tx *sql.Tx, jobID string) error {
	job, err := lockMonitorBudgetJob(ctx, tx, jobID)
	if err != nil {
		return err
	}
	switch job.state {
	case "completed", "failed", "cancelled", "interrupted", "skipped":
	default:
		return ErrMonitorBudgetLease
	}
	rows, err := readMonitorBudgetRows(ctx, tx, jobID)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err := releaseMonitorBudgetRow(ctx, tx, row); err != nil {
			return err
		}
	}
	return nil
}

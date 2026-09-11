package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

var ErrMonitorJobLease = errors.New("monitor job lease is no longer valid")

// This is a persistence boundary, not execution admission. The caller must also
// enforce runtime flags, owner/Key permissions, engine readiness and transport
// fencing. No constructor starts a worker or enables an execution feature.
type MonitorJobRepository struct{ db *sql.DB }

func NewMonitorJobRepository(db *sql.DB) *MonitorJobRepository { return &MonitorJobRepository{db: db} }

type MonitorJobLease = service.MonitorJobLease

var _ service.MonitorJobStore = (*MonitorJobRepository)(nil)

const monitorJobPolicyValid = `(j.source <> 'platform_group' OR EXISTS (
 SELECT 1 FROM channel_monitor_group_policies p WHERE p.id=j.policy_id AND p.enabled AND p.deleted_at IS NULL
 AND p.evaluation_revision=j.evaluation_revision
 AND CASE WHEN j.kind='availability' THEN p.probe_config @> '{"enabled":true}'::jsonb
 AND CASE WHEN j.config_snapshot ? 'probe_config' AND j.config_snapshot ? 'targets' THEN
 j.config_snapshot->'probe_config'=p.probe_config AND
 (SELECT jsonb_agg(t.value->'target'->'request_model' ORDER BY t.ordinality) FROM jsonb_array_elements(j.config_snapshot->'targets') WITH ORDINALITY t(value,ordinality))=
 jsonb_build_array(p.primary_model) || CASE WHEN p.probe_config @> '{"include_extra_models":true}'::jsonb THEN p.extra_models ELSE '[]'::jsonb END
 ELSE j.config_snapshot->>'policy_version'=p.version::text END
 ELSE p.capability_config @> '{"enabled":true}'::jsonb END))`

func validMonitorJobLease(lease MonitorJobLease) bool {
	return validDetectorResourceID(lease.JobID) && lease.InstanceID != "" && len(lease.InstanceID) <= 200 && lease.Generation > 0
}

// Claim separates availability/capability pools and pins external credentials
// to their in-memory owner. SKIP LOCKED prevents competing workers sharing a job.
func (r *MonitorJobRepository) Claim(ctx context.Context, kind service.MonitorJobKind, instance string) (*MonitorJobLease, error) {
	return r.ClaimEligible(ctx, kind, instance, true, true)
}

func (r *MonitorJobRepository) ClaimEligible(ctx context.Context, kind service.MonitorJobKind, instance string, allowPlatform, allowPrivate bool) (*MonitorJobLease, error) {
	return r.claimSources(ctx, kind, instance, allowPlatform, allowPrivate, false)
}

func (r *MonitorJobRepository) ClaimManagedCapability(ctx context.Context, instance string, platform, private bool) (*MonitorJobLease, error) {
	return r.claimSources(ctx, service.MonitorJobCapability, instance, platform, private, true)
}

func (r *MonitorJobRepository) ClaimSiteCapability(ctx context.Context, instance string) (*MonitorJobLease, error) {
	return r.claimSources(ctx, service.MonitorJobCapability, instance, false, true, true)
}

func (r *MonitorJobRepository) claimSources(ctx context.Context, kind service.MonitorJobKind, instance string, allowPlatform, allowPrivate, siteOnly bool) (*MonitorJobLease, error) {
	if instance == "" || len(instance) > 200 || (kind != service.MonitorJobAvailability && kind != service.MonitorJobCapability) {
		return nil, ErrMonitorBudgetInput
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var lease MonitorJobLease
	err = tx.QueryRowContext(ctx, `WITH candidate AS (
 SELECT j.id FROM monitor_jobs j WHERE j.kind=$1 AND j.state='queued' AND j.cancel_requested_at IS NULL
 AND j.queue_deadline_at>clock_timestamp() AND j.deadline_at>clock_timestamp()
 AND ((j.source='platform_group' AND $3::boolean) OR (j.source IN ('external_api','site_api_key') AND $4::boolean))
 AND (NOT $5::boolean OR j.source IN ('site_api_key','platform_group'))
 AND (j.source<>'external_api' OR j.secret_owner_instance_id=$2) AND `+monitorJobPolicyValid+` AND `+monitorJobBenchmarksValid+` AND `+monitorQueuedBenchmarkChannelsCurrent+`
 AND EXISTS(SELECT 1 FROM monitor_budget_reservations r WHERE r.job_id=j.id AND r.scope='global'
 AND r.scope_id=0 AND r.settled_at IS NULL AND r.reserved-r.consumed-r.released=j.outbound_reserved-j.outbound_dispatched)
 AND (j.owner_user_id IS NULL OR EXISTS(SELECT 1 FROM monitor_budget_reservations r WHERE r.job_id=j.id
 AND r.scope='user' AND r.scope_id=j.owner_user_id AND r.settled_at IS NULL AND r.reserved-r.consumed-r.released=j.outbound_reserved-j.outbound_dispatched))
 AND (j.policy_id IS NULL OR EXISTS(SELECT 1 FROM monitor_budget_reservations r JOIN channel_monitor_group_policies p ON p.group_id=r.scope_id
 WHERE p.id=j.policy_id AND r.job_id=j.id AND r.scope=CASE WHEN j.kind='availability' THEN 'probe_group' ELSE 'group' END
 AND r.settled_at IS NULL AND r.reserved-r.consumed-r.released=j.outbound_reserved-j.outbound_dispatched))
 ORDER BY j.created_at,j.id LIMIT 1 FOR UPDATE OF j SKIP LOCKED)
 UPDATE monitor_jobs j SET state='running',started_at=COALESCE(j.started_at,clock_timestamp()),
 lease_owner=$2,lease_generation=j.lease_generation+1,lease_expires_at=LEAST(j.deadline_at,clock_timestamp()+interval '90 seconds')
 FROM candidate c WHERE j.id=c.id RETURNING j.id,j.lease_owner,j.lease_generation,j.lease_expires_at,j.kind,j.source`, kind, instance, allowPlatform, allowPrivate, siteOnly).Scan(
		&lease.JobID, &lease.InstanceID, &lease.Generation, &lease.ExpiresAt, &lease.Kind, &lease.Source)
	if err != nil {
		return nil, err
	}
	if lease.Kind == service.MonitorJobCapability {
		if err := lockMonitorJobBenchmarksTx(ctx, tx, lease.JobID); err != nil {
			return nil, err
		}
		if err := lockMonitorJobChannelsTx(ctx, tx, lease.JobID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &lease, nil
}

// Renew cannot revive an expired lease, reset cancellation, or extend a deadline.
func (r *MonitorJobRepository) Renew(ctx context.Context, lease MonitorJobLease) (time.Time, error) {
	if !validMonitorJobLease(lease) {
		return time.Time{}, ErrMonitorBudgetInput
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return time.Time{}, err
	}
	defer func() { _ = tx.Rollback() }()
	job, err := lockMonitorBudgetJob(ctx, tx, lease.JobID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return time.Time{}, ErrMonitorJobLease
		}
		return time.Time{}, err
	}
	if job.kind == "capability" {
		if err := lockMonitorJobBenchmarksTx(ctx, tx, lease.JobID); err != nil {
			if errors.Is(err, ErrBenchmarkRelease) {
				return time.Time{}, ErrMonitorJobLease
			}
			return time.Time{}, err
		}
	}
	var expires time.Time
	err = tx.QueryRowContext(ctx, `UPDATE monitor_jobs j SET lease_expires_at=LEAST(deadline_at,clock_timestamp()+interval '90 seconds')
 WHERE j.id=$1 AND j.state='running' AND j.lease_owner=$2 AND j.lease_generation=$3
 AND j.lease_expires_at>clock_timestamp() AND j.deadline_at>clock_timestamp() AND j.cancel_requested_at IS NULL
 AND `+monitorJobPolicyValid+` AND `+monitorJobBenchmarksValid+` RETURNING j.lease_expires_at`, lease.JobID, lease.InstanceID, lease.Generation).Scan(&expires)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, ErrMonitorJobLease
	}
	if err != nil {
		return time.Time{}, err
	}
	if err := tx.Commit(); err != nil {
		return time.Time{}, err
	}
	return expires, nil
}

// CancelForOwner never reveals whether a different owner's job exists.
// A running request is not assumed stopped: its lease stays until acknowledgement
// or recovery, while every new Consume/Renew rejects the cancellation marker.
func (r *MonitorJobRepository) CancelForOwner(ctx context.Context, jobID string, ownerID int64) error {
	if ownerID <= 0 || !validDetectorResourceID(jobID) {
		return sql.ErrNoRows
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var state service.MonitorJobState
	err = tx.QueryRowContext(ctx, `SELECT state FROM monitor_jobs WHERE id=$1 AND owner_user_id=$2
 AND kind='capability' AND source IN ('external_api','site_api_key') FOR UPDATE`, jobID, ownerID).Scan(&state)
	if err != nil {
		return err
	}
	if state.Terminal() {
		return tx.Commit()
	}
	if state == service.MonitorJobQueued {
		if err := terminateMonitorJobTx(ctx, tx, jobID, service.MonitorJobCancelled, "cancelled"); err != nil {
			return err
		}
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE monitor_jobs SET state='cancelling',cancel_requested_at=COALESCE(cancel_requested_at,clock_timestamp()) WHERE id=$1`, jobID)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Finish checks the fence under the row lock. Completion requires persisted
// terminal evidence; a network task ending is not by itself a successful report.
func (r *MonitorJobRepository) Finish(ctx context.Context, lease MonitorJobLease, state service.MonitorJobState) error {
	if !validMonitorJobLease(lease) || (state != service.MonitorJobCompleted && state != service.MonitorJobFailed && state != service.MonitorJobCancelled) {
		return ErrMonitorBudgetInput
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var cancelling bool
	var kind string
	var validUntil time.Time
	err = tx.QueryRowContext(ctx, `SELECT j.cancel_requested_at IS NOT NULL,LEAST(j.lease_expires_at,j.deadline_at),j.kind FROM monitor_jobs j
 WHERE j.id=$1 AND j.state IN ('running','cancelling') AND j.lease_owner=$2 AND j.lease_generation=$3
 AND j.lease_expires_at>clock_timestamp() AND j.deadline_at>clock_timestamp() FOR UPDATE`, lease.JobID, lease.InstanceID, lease.Generation).Scan(&cancelling, &validUntil, &kind)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrMonitorJobLease
	}
	if err != nil {
		return err
	}
	if cancelling && state != service.MonitorJobCancelled {
		return ErrMonitorJobLease
	}
	if state == service.MonitorJobCompleted {
		if kind == "capability" {
			if err := lockMonitorJobBenchmarksTx(ctx, tx, lease.JobID); err != nil {
				if errors.Is(err, ErrBenchmarkRelease) {
					return ErrMonitorJobLease
				}
				return err
			}
		}
		var complete bool
		err = tx.QueryRowContext(ctx, `SELECT `+monitorJobPolicyValid+` AND `+monitorJobBenchmarksValid+`
 AND j.outbound_completed=j.outbound_dispatched AND j.outbound_dispatched>0
 AND NOT EXISTS(SELECT 1 FROM llm_detector_attempts a JOIN llm_detector_executions e ON e.id=a.execution_id
 WHERE e.job_id=j.id AND a.dispatch_state IN ('reserved','dispatching'))
 AND CASE WHEN j.kind='capability' THEN
 EXISTS(SELECT 1 FROM llm_detector_executions e WHERE e.job_id=j.id)
 AND NOT EXISTS(SELECT 1 FROM llm_detector_executions e WHERE e.job_id=j.id AND
 (e.state NOT IN ('completed','failed','cancelled','interrupted','skipped') OR
 (e.state='completed' AND NOT EXISTS(SELECT 1 FROM llm_detector_reports r WHERE r.execution_id=e.id AND r.deleted_at IS NULL))))
 ELSE EXISTS(SELECT 1 FROM channel_monitor_probe_results p WHERE p.job_id=j.id) END
 FROM monitor_jobs j WHERE j.id=$1`, lease.JobID).Scan(&complete)
		if err != nil {
			return err
		}
		if !complete {
			return ErrMonitorJobLease
		}
	}
	code := "execution_failed"
	if state == service.MonitorJobCancelled {
		code = "cancelled"
	}
	if state == service.MonitorJobCompleted {
		code = ""
	}
	if err := terminateMonitorJobTx(ctx, tx, lease.JobID, state, code); err != nil {
		return err
	}
	// Bucket locks may have consumed the remaining lease while settling.
	var eligible bool
	err = tx.QueryRowContext(ctx, `SELECT $1::timestamptz>clock_timestamp()`, validUntil).Scan(&eligible)
	if err != nil {
		return err
	}
	if !eligible {
		return ErrMonitorJobLease
	}
	return tx.Commit()
}

// RecoverNext never transparently resends an uncertain attempt or resurrects an
// external secret after restart. One job per transaction bounds lock duration.
func (r *MonitorJobRepository) RecoverNext(ctx context.Context) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	var id, code string
	var state service.MonitorJobState
	err = tx.QueryRowContext(ctx, `SELECT j.id,j.state,CASE
 WHEN j.cancel_requested_at IS NOT NULL THEN 'cancelled'
 WHEN NOT `+monitorJobPolicyValid+` THEN 'configuration_changed'
 WHEN NOT `+monitorJobBenchmarksValid+` THEN 'benchmark_unavailable'
 WHEN NOT `+monitorQueuedBenchmarkChannelsCurrent+` THEN 'benchmark_channel_changed'
 WHEN j.state='queued' THEN 'queue_expired'
 WHEN j.deadline_at<=clock_timestamp() THEN 'deadline_exceeded'
 ELSE 'lease_expired' END
 FROM monitor_jobs j WHERE j.state IN ('queued','running','cancelling') AND
 ((j.state='queued' AND (j.queue_deadline_at<=clock_timestamp() OR j.cancel_requested_at IS NOT NULL))
 OR (j.state IN ('running','cancelling') AND j.lease_expires_at<=clock_timestamp())
 OR j.deadline_at<=clock_timestamp() OR NOT `+monitorJobPolicyValid+` OR NOT `+monitorJobBenchmarksValid+` OR NOT `+monitorQueuedBenchmarkChannelsCurrent+`)
 ORDER BY j.created_at,j.id LIMIT 1 FOR UPDATE OF j SKIP LOCKED`).Scan(&id, &state, &code)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	terminal := service.MonitorJobInterrupted
	if state == service.MonitorJobQueued {
		terminal = service.MonitorJobSkipped
	}
	if code == "cancelled" {
		terminal = service.MonitorJobCancelled
	}
	if err := terminateMonitorJobTx(ctx, tx, id, terminal, code); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// Caller holds the job lock. Attempts, child states, fence invalidation and
// budget release commit together, including in recovery after a process crash.
func terminateMonitorJobTx(ctx context.Context, tx *sql.Tx, jobID string, state service.MonitorJobState, code string) error {
	if err := interruptMonitorProbeDispatchesTx(ctx, tx, jobID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE llm_detector_attempts a SET dispatch_state=CASE WHEN a.dispatch_state='dispatching'
 THEN 'uncertain' ELSE 'cancelled_before_dispatch' END,completed_at=clock_timestamp()
 FROM llm_detector_executions e WHERE a.execution_id=e.id AND e.job_id=$1 AND a.dispatch_state IN ('reserved','dispatching')`, jobID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE llm_detector_executions SET state=CASE WHEN $2='cancelled' THEN 'cancelled' ELSE 'interrupted' END,
 finished_at=clock_timestamp(),failure_code=NULLIF($3,'') WHERE job_id=$1 AND state IN ('queued','running','cancelling')`, jobID, string(state), code)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE monitor_jobs SET state=$2,finished_at=clock_timestamp(),failure_code=NULLIF($3,''),
 cancel_requested_at=CASE WHEN $2='cancelled' THEN COALESCE(cancel_requested_at,clock_timestamp()) ELSE cancel_requested_at END,
 lease_owner=NULL,lease_expires_at=NULL,lease_generation=lease_generation+1 WHERE id=$1`, jobID, state, code)
	if err != nil {
		return err
	}
	if err := finishMonitorProbeScheduleTx(ctx, tx, jobID); err != nil {
		return err
	}
	if err := finishPlatformCapabilityScheduleTx(ctx, tx, jobID); err != nil {
		return err
	}
	return settleMonitorBudgetTx(ctx, tx, jobID)
}

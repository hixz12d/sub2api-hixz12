package repository

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
)

var _ service.MonitorProbeStore = (*MonitorJobRepository)(nil)

const monitorProbeJobQuery = `SELECT j.id,j.policy_id,p.group_id,j.config_snapshot,j.deadline_at
 FROM monitor_jobs j JOIN channel_monitor_group_policies p ON p.id=j.policy_id
 WHERE j.id=$1 AND j.kind='availability' AND j.source='platform_group' AND j.state='running'
 AND j.lease_owner=$2 AND j.lease_generation=$3 AND j.lease_expires_at>clock_timestamp()
 AND j.deadline_at>clock_timestamp() AND j.cancel_requested_at IS NULL AND ` + monitorJobPolicyValid

func scanMonitorProbeJob(row monitorPolicyScanner) (*service.MonitorProbeJob, error) {
	var job service.MonitorProbeJob
	var raw []byte
	if err := row.Scan(&job.ID, &job.PolicyID, &job.GroupID, &raw, &job.Deadline); err != nil {
		return nil, err
	}
	if len(raw) > 262144 || json.Unmarshal(raw, &job.Snapshot) != nil || job.Snapshot.ProbeConfig == nil ||
		!job.Snapshot.ProbeConfig.Enabled || job.Snapshot.ProbeConfig.Validate() != nil || len(job.Snapshot.Targets) < 1 || len(job.Snapshot.Targets) > 20 {
		return nil, ErrMonitorBudgetInput
	}
	return &job, nil
}

func (r *MonitorJobRepository) GetProbeJob(ctx context.Context, lease MonitorJobLease) (*service.MonitorProbeJob, error) {
	if !validMonitorJobLease(lease) {
		return nil, ErrMonitorBudgetInput
	}
	return scanMonitorProbeJob(r.db.QueryRowContext(ctx, monitorProbeJobQuery, lease.JobID, lease.InstanceID, lease.Generation))
}

func (r *MonitorJobRepository) BeginProbeDispatch(ctx context.Context, input service.MonitorProbeDispatch, globalLimit int64) (string, error) {
	lease := input.Lease
	if !validMonitorJobLease(lease) || input.AccountID <= 0 || input.TargetIndex < 0 || input.SampleIndex < 0 || !validMonitorConfigurationHash(input.RequestSHA256) || globalLimit < 1 || globalLimit > 1000000000 {
		return "", ErrMonitorBudgetInput
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	budget, err := lockMonitorBudgetJob(ctx, tx, lease.JobID)
	if err != nil {
		return "", err
	}
	job, err := scanMonitorProbeJob(tx.QueryRowContext(ctx, monitorProbeJobQuery, lease.JobID, lease.InstanceID, lease.Generation))
	if err != nil {
		return "", err
	}
	cfg := job.Snapshot.ProbeConfig
	if input.TargetIndex >= len(job.Snapshot.Targets) || input.SampleIndex >= cfg.SampleSize {
		return "", ErrMonitorBudgetInput
	}
	target := job.Snapshot.Targets[input.TargetIndex]
	if target.GroupID == nil || *target.GroupID != job.GroupID || target.Target.RequestModel != input.RequestModel {
		return "", ErrMonitorBudgetInput
	}
	if cfg.SelectionMode == service.MonitorSelectionFixed && cfg.FixedAccountIDs[input.SampleIndex] != input.AccountID {
		return "", ErrMonitorBudgetInput
	}
	var permitted bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_groups ag JOIN accounts a ON a.id=ag.account_id JOIN groups g ON g.id=ag.group_id
 WHERE ag.group_id=$1 AND ag.account_id=$2 AND a.status='active' AND a.schedulable AND a.deleted_at IS NULL AND g.status='active' AND g.deleted_at IS NULL)
 AND NOT EXISTS(SELECT 1 FROM monitor_probe_dispatches WHERE job_id=$3 AND
 ((target_index=$4 AND sample_index=$5) OR (sample_index=$5 AND account_id<>$2)))`, job.GroupID, input.AccountID, job.ID, input.TargetIndex, input.SampleIndex).Scan(&permitted)
	if err != nil {
		return "", err
	}
	if !permitted {
		return "", service.ErrMonitorOutboundDenied
	}
	limits := []MonitorBudgetLimit{{Scope: "global", RequestLimit: globalLimit}, {Scope: "probe_group", ScopeID: job.GroupID, RequestLimit: int64(cfg.DailyRequestLimit)}}
	if err := consumeMonitorBudgetTx(ctx, tx, job.ID, lease.InstanceID, lease.Generation, budget.dispatched, limits); err != nil {
		return "", err
	}
	id := uuid.NewString()
	_, err = tx.ExecContext(ctx, `INSERT INTO monitor_probe_dispatches(id,job_id,target_index,sample_index,account_id,request_model,request_sha256,lease_generation,dispatch_ordinal)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, job.ID, input.TargetIndex, input.SampleIndex, input.AccountID, input.RequestModel, input.RequestSHA256, lease.Generation, budget.dispatched)
	if err != nil {
		return "", err
	}
	if _, err := scanMonitorProbeJob(tx.QueryRowContext(ctx, monitorProbeJobQuery, lease.JobID, lease.InstanceID, lease.Generation)); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return id, nil
}

func validMonitorProbeOutcome(outcome service.MonitorProbeOutcome) bool {
	if !validDetectorResourceID(outcome.DispatchID) || (outcome.TTFTMs != nil && *outcome.TTFTMs < 0) {
		return false
	}
	switch outcome.TransportState {
	case "passed", "failed", "incomplete", "unsupported":
	default:
		return false
	}
	switch outcome.ChallengeState {
	case "passed", "failed", "not_evaluated":
	default:
		return false
	}
	if outcome.ChallengeState == "passed" && outcome.TransportState != "passed" {
		return false
	}
	switch outcome.ErrorCategory {
	case "", "http_error", "transport_error", "invalid_response", "challenge_mismatch", "response_too_large", "credential_changed", "target_unavailable", "unsupported", "cancelled":
	default:
		return false
	}
	return true
}

func (r *MonitorJobRepository) CompleteProbeDispatch(ctx context.Context, lease MonitorJobLease, outcome service.MonitorProbeOutcome) error {
	if !validMonitorJobLease(lease) || !validMonitorProbeOutcome(outcome) {
		return ErrMonitorBudgetInput
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := lockMonitorBudgetJob(ctx, tx, lease.JobID); err != nil {
		return err
	}
	job, err := scanMonitorProbeJob(tx.QueryRowContext(ctx, monitorProbeJobQuery, lease.JobID, lease.InstanceID, lease.Generation))
	if err != nil {
		return err
	}
	var accountID int64
	var model, state string
	err = tx.QueryRowContext(ctx, `SELECT account_id,request_model,state FROM monitor_probe_dispatches
 WHERE id=$1 AND job_id=$2 AND lease_generation=$3 FOR UPDATE`, outcome.DispatchID, job.ID, lease.Generation).Scan(&accountID, &model, &state)
	if err != nil {
		return err
	}
	if state == "completed" {
		var same bool
		err = tx.QueryRowContext(ctx, `SELECT transport_state=$2 AND challenge_state=$3 AND error_category IS NOT DISTINCT FROM NULLIF($4,'')
 AND ttft_ms IS NOT DISTINCT FROM $5::bigint FROM channel_monitor_probe_results WHERE id=$1`, outcome.DispatchID, outcome.TransportState, outcome.ChallengeState, outcome.ErrorCategory, outcome.TTFTMs).Scan(&same)
		if err != nil {
			return err
		}
		if !same {
			return ErrMonitorBudgetConflict
		}
		return tx.Commit()
	}
	if state != "dispatching" {
		return service.ErrMonitorOutboundDenied
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO channel_monitor_probe_results(id,job_id,policy_id,group_id,request_model,account_id,checked_at,transport_state,challenge_state,ttft_ms,error_category)
 VALUES($1,$2,$3,$4,$5,$6,clock_timestamp(),$7,$8,$9,NULLIF($10,''))`, outcome.DispatchID, job.ID, job.PolicyID, job.GroupID, model, accountID, outcome.TransportState, outcome.ChallengeState, outcome.TTFTMs, outcome.ErrorCategory)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE monitor_probe_dispatches SET state='completed',completed_at=clock_timestamp() WHERE id=$1`, outcome.DispatchID)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE monitor_jobs j SET outbound_completed=outbound_completed+1 WHERE j.id=$1
 AND j.outbound_completed<j.outbound_dispatched AND j.state='running' AND j.cancel_requested_at IS NULL
 AND j.lease_owner=$2 AND j.lease_generation=$3 AND j.lease_expires_at>clock_timestamp() AND j.deadline_at>clock_timestamp()
 AND `+monitorJobPolicyValid, job.ID, lease.InstanceID, lease.Generation)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrMonitorJobLease
	}
	return tx.Commit()
}

// This is independent of the capability attempt table and never creates fake
// benchmark executions merely to record a short availability check.
func interruptMonitorProbeDispatchesTx(ctx context.Context, tx *sql.Tx, jobID string) error {
	_, err := tx.ExecContext(ctx, `UPDATE monitor_probe_dispatches SET state='uncertain',completed_at=clock_timestamp() WHERE job_id=$1 AND state='dispatching'`, jobID)
	return err
}

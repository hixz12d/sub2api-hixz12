package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
)

// ListDueProbePolicyIDs is bounded discovery only. EnqueueProbe rechecks the
// policy under its lock, so concurrent schedulers can safely use the same list.
func (r *MonitorJobRepository) ListDueProbePolicyIDs(ctx context.Context, limit int) ([]int64, error) {
	if limit < 1 || limit > 100 {
		return nil, ErrMonitorBudgetInput
	}
	rows, err := r.db.QueryContext(ctx, `SELECT p.id FROM channel_monitor_group_policies p
 WHERE p.enabled AND p.deleted_at IS NULL AND p.probe_config @> '{"enabled":true}'::jsonb
 AND p.next_probe_at<=clock_timestamp() AND p.active_probe_job_id IS NULL
 AND NOT EXISTS(SELECT 1 FROM monitor_jobs j WHERE j.policy_id=p.id AND j.kind='availability' AND j.state IN ('queued','running','cancelling'))
 ORDER BY p.next_probe_at,p.id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// EnqueueProbe freezes a single occurrence and reserves every possible request
// before publishing the queued job. No account/network selection occurs inside
// the transaction. The executor may not exceed the frozen sampling ceiling.
func (r *MonitorJobRepository) EnqueueProbe(ctx context.Context, policyID, globalDailyLimit int64) (string, error) {
	if policyID <= 0 || globalDailyLimit < 1 || globalDailyLimit > 1000000000 {
		return "", ErrMonitorBudgetInput
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	policy, err := scanMonitorPolicy(tx.QueryRowContext(ctx, `SELECT `+monitorPolicyColumns+` FROM channel_monitor_group_policies p
 WHERE p.id=$1 AND p.enabled AND p.deleted_at IS NULL AND p.probe_config @> '{"enabled":true}'::jsonb
 AND p.next_probe_at<=clock_timestamp() AND p.active_probe_job_id IS NULL
 AND NOT EXISTS(SELECT 1 FROM monitor_jobs j WHERE j.policy_id=p.id AND j.kind='availability' AND j.state IN ('queued','running','cancelling'))
 FOR UPDATE OF p SKIP LOCKED`, policyID))
	if errors.Is(err, service.ErrMonitorPolicyNotFound) {
		return "", sql.ErrNoRows
	}
	if err != nil {
		return "", err
	}
	if err := policy.Normalize(); err != nil {
		return "", ErrMonitorBudgetInput
	}
	models := []string{policy.PrimaryModel}
	if policy.ProbeConfig.IncludeExtraModels {
		models = append(models, policy.ExtraModels...)
	}
	targets := make([]service.DetectorTargetSpec, 0, len(models))
	for _, model := range models {
		targets = append(targets, service.DetectorTargetSpec{Source: service.MonitorSourcePlatformGroup, GroupID: &policy.GroupID, Target: service.DetectorTarget{RequestModel: model}})
	}
	snapshot, err := json.Marshal(service.MonitorJobSnapshot{PolicyVersion: policy.Version, Targets: targets, ProbeConfig: &policy.ProbeConfig})
	if err != nil {
		return "", err
	}
	ceiling := int64(len(models) * policy.ProbeConfig.SampleSize)
	// Requests execute serially within this job. Refuse schedules that cannot fit
	// the parent deadline rather than silently reducing sampling or timeout.
	if ceiling*int64(policy.ProbeConfig.TimeoutSeconds)+300 > 7200 {
		return "", ErrMonitorBudgetInput
	}
	occurrence := fmt.Sprintf("probe:%d:%d:%d", policy.ID, policy.Version, policy.NextProbeAt.UTC().UnixMicro())
	keySum := sha256.Sum256([]byte(occurrence))
	payloadSum := sha256.Sum256(snapshot)
	id := uuid.NewString()
	_, err = tx.ExecContext(ctx, `INSERT INTO monitor_jobs(id,kind,source,policy_id,evaluation_revision,idempotency_scope,idempotency_key_hash,payload_hash,
 schedule_occurrence_id,config_snapshot,deadline_at,queue_deadline_at,base_requests_planned,outbound_reserved)
 VALUES($1,'availability','platform_group',$2,$3,$4,$5,$6,$7,$8,transaction_timestamp()+interval '2 hours',transaction_timestamp()+interval '5 minutes',$9,$9)`,
		id, policy.ID, policy.EvaluationRevision, fmt.Sprintf("policy:%d:availability", policy.ID), hex.EncodeToString(keySum[:]), hex.EncodeToString(payloadSum[:]), occurrence, snapshot, ceiling)
	if err != nil {
		return "", err
	}
	limits := []MonitorBudgetLimit{{Scope: "global", RequestLimit: globalDailyLimit}, {Scope: "probe_group", ScopeID: policy.GroupID, RequestLimit: int64(policy.ProbeConfig.DailyRequestLimit)}}
	// Helpers acquire budget scopes in canonical order; global precedes group.
	if err := reserveMonitorBudgetTx(ctx, tx, id, limits); err != nil {
		return "", err
	}
	_, err = tx.ExecContext(ctx, `UPDATE channel_monitor_group_policies SET active_probe_job_id=$2,next_probe_at=NULL WHERE id=$1`, policy.ID, id)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return id, nil
}

// Called in the same terminal transaction as budget settlement. Only the job
// still referenced by the policy may advance its schedule; stale workers cannot
// erase a newer job. Current config controls the next run, not the old snapshot.
func finishMonitorProbeScheduleTx(ctx context.Context, tx *sql.Tx, jobID string) error {
	var policyID int64
	var enabled bool
	var deleted sql.NullTime
	var raw []byte
	err := tx.QueryRowContext(ctx, `SELECT p.id,p.enabled,p.deleted_at,p.probe_config FROM channel_monitor_group_policies p
 WHERE p.active_probe_job_id=$1 FOR UPDATE`, jobID).Scan(&policyID, &enabled, &deleted, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	cfg := service.DefaultGroupProbeConfig()
	var next any
	if enabled && !deleted.Valid && service.DecodeMonitorConfig(raw, &cfg) == nil && cfg.Enabled && cfg.Validate() == nil {
		delay := cfg.IntervalSeconds
		if cfg.JitterSeconds > 0 {
			delay += rand.IntN(2*cfg.JitterSeconds+1) - cfg.JitterSeconds
		}
		var finished time.Time
		if err := tx.QueryRowContext(ctx, `SELECT finished_at FROM monitor_jobs WHERE id=$1`, jobID).Scan(&finished); err != nil {
			return err
		}
		next = finished.Add(time.Duration(delay) * time.Second)
	}
	_, err = tx.ExecContext(ctx, `UPDATE channel_monitor_group_policies SET active_probe_job_id=NULL,next_probe_at=$2 WHERE id=$1 AND active_probe_job_id=$3`, policyID, next, jobID)
	return err
}

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

func (r *MonitorJobRepository) DueCapabilityPolicies(ctx context.Context, limit int, groups []int64) ([]service.ChannelMonitorGroupPolicy, error) {
	if limit < 1 || limit > 100 {
		return nil, ErrMonitorBudgetInput
	}
	rows, err := r.db.QueryContext(ctx, `SELECT `+monitorPolicyColumns+` FROM channel_monitor_group_policies WHERE group_id=ANY($2) AND enabled AND deleted_at IS NULL AND capability_config @> '{"enabled":true}' AND next_capability_at<=clock_timestamp() AND active_capability_job_id IS NULL ORDER BY next_capability_at,id LIMIT $1`, limit, pq.Array(groups))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := []service.ChannelMonitorGroupPolicy{}
	for rows.Next() {
		p, err := scanMonitorPolicy(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *p)
	}
	return items, rows.Err()
}
func (r *MonitorJobRepository) DeferCapabilityPolicy(ctx context.Context, id, version int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE channel_monitor_group_policies SET next_capability_at=clock_timestamp()+interval '30 minutes' WHERE id=$1 AND version=$2 AND active_capability_job_id IS NULL`, id, version)
	return err
}
func (r *MonitorJobRepository) EnqueuePlatformCapability(ctx context.Context, p service.ChannelMonitorGroupPolicy, snapshot service.MonitorJobSnapshot, total int64, limits []service.MonitorBudgetLimit) (string, error) {
	if p.NextCapabilityAt == nil || p.ID <= 0 || p.Version <= 0 || total < 1 || total > 7200 || len(snapshot.Targets) < 1 || len(snapshot.Targets) > 24 || len(snapshot.Targets) != len(snapshot.Benchmarks) || snapshot.CapabilityConfig == nil {
		return "", ErrMonitorBudgetInput
	}
	if err := p.Normalize(); err != nil {
		return "", err
	}
	raw, err := json.Marshal(snapshot)
	if err != nil || len(raw) > 262144 {
		return "", ErrMonitorBudgetInput
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	for _, manifest := range snapshot.Benchmarks {
		if err = lockApprovedBenchmarkTx(ctx, tx, manifest); err != nil {
			return "", err
		}
		if err = lockCurrentBenchmarkChannelTx(ctx, tx, manifest); err != nil {
			return "", err
		}
	}
	var version int64
	err = tx.QueryRowContext(ctx, `SELECT version FROM channel_monitor_group_policies WHERE id=$1 AND version=$2 AND evaluation_revision=$3 AND enabled AND deleted_at IS NULL AND capability_config @> '{"enabled":true}' AND active_capability_job_id IS NULL AND next_capability_at<=clock_timestamp() AND NOT EXISTS(SELECT 1 FROM monitor_jobs WHERE policy_id=$1 AND kind='capability' AND state IN('queued','running','cancelling')) FOR UPDATE`, p.ID, p.Version, p.EvaluationRevision).Scan(&version)
	if err != nil {
		return "", err
	}
	snapshot.RetestOf = ""
	if p.CapabilityConfig.RetestOnMismatch && p.CapabilityConfig.RetestLimitPerExecution > 0 {
		err = tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT CASE WHEN j.state='completed' AND j.evaluation_revision=$2 AND COALESCE(j.config_snapshot->>'retest_of','')='' AND EXISTS(SELECT 1 FROM llm_detector_executions e WHERE e.job_id=j.id AND e.verdict='mismatch') THEN j.id::text ELSE '' END FROM monitor_jobs j WHERE j.policy_id=$1 AND j.kind='capability' ORDER BY j.created_at DESC,j.id DESC LIMIT 1),'')`, p.ID, p.EvaluationRevision).Scan(&snapshot.RetestOf)
		if err != nil {
			return "", err
		}
	}
	raw, err = json.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	id := uuid.NewString()
	occurrence := fmt.Sprintf("capability:%d:%d:%s", p.ID, p.Version, p.NextCapabilityAt.UTC().Format("2006-01-02T15:04:05.999999999Z"))
	_, err = tx.ExecContext(ctx, `INSERT INTO monitor_jobs(id,kind,source,policy_id,evaluation_revision,idempotency_scope,idempotency_key_hash,payload_hash,schedule_occurrence_id,config_snapshot,deadline_at,queue_deadline_at,base_requests_planned,outbound_reserved) VALUES($1,'capability','platform_group',$2,$3,$4,$5,$6,$7,$8,clock_timestamp()+make_interval(secs=>$9),clock_timestamp()+interval '5 minutes',$10,$10)`, id, p.ID, p.EvaluationRevision, fmt.Sprintf("policy:%d:capability", p.ID), benchmarkDigest([]byte(occurrence)), benchmarkDigest(raw), occurrence, raw, p.CapabilityConfig.ExecutionTimeoutSeconds+300, total)
	if err != nil {
		return "", err
	}
	if err = reserveMonitorBudgetTx(ctx, tx, id, limits); err != nil {
		return "", err
	}
	_, err = tx.ExecContext(ctx, `UPDATE channel_monitor_group_policies SET active_capability_job_id=$2,next_capability_at=NULL WHERE id=$1`, p.ID, id)
	if err != nil {
		return "", err
	}
	return id, tx.Commit()
}
func finishPlatformCapabilityScheduleTx(ctx context.Context, tx *sql.Tx, job string) error {
	_, err := tx.ExecContext(ctx, `UPDATE channel_monitor_group_policies p SET active_capability_job_id=NULL,next_capability_at=CASE WHEN p.enabled AND p.deleted_at IS NULL AND p.capability_config @> '{"enabled":true}' THEN clock_timestamp()+make_interval(secs=>CASE WHEN j.state='completed' AND j.evaluation_revision=p.evaluation_revision AND p.capability_config @> '{"retest_on_mismatch":true}' AND (p.capability_config->>'retest_limit_per_execution')::integer>0 AND COALESCE(j.config_snapshot->>'retest_of','')='' AND EXISTS(SELECT 1 FROM llm_detector_executions e WHERE e.job_id=j.id AND e.verdict='mismatch') THEN (p.capability_config->>'retest_delay_seconds')::integer ELSE (p.capability_config->>'interval_seconds')::integer + floor(random()*(2*COALESCE((p.capability_config->>'jitter_seconds')::integer,0)+1))::integer-COALESCE((p.capability_config->>'jitter_seconds')::integer,0) END) ELSE NULL END FROM monitor_jobs j WHERE j.id=$1 AND j.kind='capability' AND j.source='platform_group' AND p.id=j.policy_id AND p.active_capability_job_id=j.id`, job)
	return err
}

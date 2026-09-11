package repository

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *ChannelMonitorGroupRepository) ListPolicies(ctx context.Context, limit, offset int) ([]service.ChannelMonitorGroupPolicy, error) {
	if limit < 1 || limit > 100 || offset < 0 || offset > 10000 {
		return nil, service.ErrMonitorPolicyConflict
	}
	rows, err := r.db.QueryContext(ctx, `SELECT `+monitorPolicyColumns+` FROM channel_monitor_group_policies WHERE deleted_at IS NULL ORDER BY id LIMIT $1 OFFSET $2`, limit, offset)
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

// Policy-only mutation preserves the existing job->policy lock order. Guards
// invalidate changed execution snapshots; recovery fences and settles old jobs.
func (r *ChannelMonitorGroupRepository) SavePolicy(ctx context.Context, p service.ChannelMonitorGroupPolicy, expected, actor int64) (*service.ChannelMonitorGroupPolicy, error) {
	if actor <= 0 || expected < 0 || (p.ID > 0 && expected == 0) || (p.ID == 0 && expected != 0) {
		return nil, service.ErrMonitorPolicyConflict
	}
	if err := p.Normalize(); err != nil {
		return nil, err
	}
	revision, err := p.CapabilityRevision()
	if err != nil {
		return nil, err
	}
	p.EvaluationRevision = revision
	extra, err := json.Marshal(p.ExtraModels)
	if err != nil {
		return nil, err
	}
	probe, err := json.Marshal(p.ProbeConfig)
	if err != nil {
		return nil, err
	}
	capability, err := json.Marshal(p.CapabilityConfig)
	if err != nil {
		return nil, err
	}
	if p.ID == 0 {
		return scanMonitorPolicy(r.db.QueryRowContext(ctx, `INSERT INTO channel_monitor_group_policies(group_id,display_name,primary_model,extra_models,enabled,probe_config,capability_config,evaluation_revision,created_by,updated_by,next_probe_at,next_capability_at) SELECT g.id,$2,$3,$4,$5,$6,$7,$8,$9,$9,CASE WHEN $5 AND $10 THEN clock_timestamp() END,CASE WHEN $5 AND $11 THEN clock_timestamp() END FROM groups g WHERE g.id=$1 AND g.deleted_at IS NULL AND g.status='active' RETURNING `+monitorPolicyColumns, p.GroupID, p.DisplayName, p.PrimaryModel, extra, p.Enabled, probe, capability, p.EvaluationRevision, actor, p.ProbeConfig.Enabled, p.CapabilityConfig.Enabled))
	}
	result, err := scanMonitorPolicy(r.db.QueryRowContext(ctx, `UPDATE channel_monitor_group_policies p SET display_name=$2,primary_model=$3,extra_models=$4,enabled=$5,probe_config=$6,capability_config=$7,evaluation_revision=$8,updated_by=$9,updated_at=clock_timestamp(),version=version+1,
 next_probe_at=CASE WHEN p.enabled=$5 AND p.probe_config=$6::jsonb AND p.primary_model=$3::varchar AND p.extra_models=$4::jsonb THEN p.next_probe_at WHEN NOT $5 OR NOT $12 THEN NULL WHEN active_probe_job_id IS NULL THEN clock_timestamp() ELSE NULL END,
 next_capability_at=CASE WHEN p.enabled=$5 AND p.capability_config=$7::jsonb THEN p.next_capability_at WHEN NOT $5 OR NOT $13 THEN NULL WHEN active_capability_job_id IS NULL THEN clock_timestamp() ELSE NULL END
 WHERE p.id=$1 AND p.version=$10 AND p.group_id=$11 AND p.deleted_at IS NULL AND EXISTS(SELECT 1 FROM groups g WHERE g.id=p.group_id AND g.deleted_at IS NULL AND g.status='active') RETURNING `+monitorPolicyColumns, p.ID, p.DisplayName, p.PrimaryModel, extra, p.Enabled, probe, capability, p.EvaluationRevision, actor, expected, p.GroupID, p.ProbeConfig.Enabled, p.CapabilityConfig.Enabled))
	if errors.Is(err, service.ErrMonitorPolicyNotFound) {
		return nil, service.ErrMonitorPolicyConflict
	}
	return result, err
}
func (r *ChannelMonitorGroupRepository) DeletePolicy(ctx context.Context, id, version, actor int64) error {
	if id <= 0 || version <= 0 || actor <= 0 {
		return service.ErrMonitorPolicyConflict
	}
	result, err := r.db.ExecContext(ctx, `UPDATE channel_monitor_group_policies SET enabled=false,deleted_at=clock_timestamp(),updated_at=clock_timestamp(),updated_by=$3,version=version+1,next_probe_at=NULL,next_capability_at=NULL WHERE id=$1 AND version=$2 AND deleted_at IS NULL`, id, version, actor)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return service.ErrMonitorPolicyConflict
	}
	return nil
}

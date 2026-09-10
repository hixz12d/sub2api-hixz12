package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

// SQL-owned like ChannelMonitorV2Repository. Construction has no background work.
type ChannelMonitorGroupRepository struct{ db *sql.DB }

func NewChannelMonitorGroupRepository(db *sql.DB) *ChannelMonitorGroupRepository {
	return &ChannelMonitorGroupRepository{db: db}
}

const monitorPolicyColumns = `id, group_id, display_name, primary_model, extra_models,
 enabled, probe_config, capability_config, version, evaluation_revision,
 next_probe_at, next_capability_at, active_probe_job_id, active_capability_job_id,
 created_by, updated_by, created_at, updated_at, deleted_at`

type monitorPolicyScanner interface{ Scan(...any) error }

func scanMonitorPolicy(row monitorPolicyScanner) (*service.ChannelMonitorGroupPolicy, error) {
	var p service.ChannelMonitorGroupPolicy
	var extra, probe, capability []byte
	err := row.Scan(&p.ID, &p.GroupID, &p.DisplayName, &p.PrimaryModel, &extra,
		&p.Enabled, &probe, &capability, &p.Version, &p.EvaluationRevision,
		&p.NextProbeAt, &p.NextCapabilityAt, &p.ActiveProbeJobID, &p.ActiveCapabilityJobID,
		&p.CreatedBy, &p.UpdatedBy, &p.CreatedAt, &p.UpdatedAt, &p.DeletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrMonitorPolicyNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read monitor policy: %w", err)
	}
	if err = json.Unmarshal(extra, &p.ExtraModels); err != nil {
		return nil, fmt.Errorf("decode monitor models: %w", err)
	}
	p.ProbeConfig = service.DefaultGroupProbeConfig()
	p.CapabilityConfig = service.DefaultGroupCapabilityConfig()
	if err = service.DecodeMonitorConfig(probe, &p.ProbeConfig); err != nil {
		return nil, fmt.Errorf("decode probe config: %w", err)
	}
	if err = service.DecodeMonitorConfig(capability, &p.CapabilityConfig); err != nil {
		return nil, fmt.Errorf("decode capability config: %w", err)
	}
	return &p, nil
}

// For internal/admin callers only; public reads require a group-scoped query.
func (r *ChannelMonitorGroupRepository) GetPolicy(ctx context.Context, id int64) (*service.ChannelMonitorGroupPolicy, error) {
	return scanMonitorPolicy(r.db.QueryRowContext(ctx, `SELECT `+monitorPolicyColumns+`
 FROM channel_monitor_group_policies WHERE id=$1 AND deleted_at IS NULL`, id))
}

func encodeInactiveMonitorPolicy(p service.ChannelMonitorGroupPolicy) (service.ChannelMonitorGroupPolicy, []byte, []byte, []byte, error) {
	// Activation needs P3 admission and P5 transactional scheduling/cancellation.
	// Until then even internal callers cannot persist an enabled paid probe.
	if p.ProbeConfig.Enabled || p.CapabilityConfig.Enabled {
		return p, nil, nil, nil, errors.New("monitor execution is not available in this phase")
	}
	if err := p.Normalize(); err != nil {
		return p, nil, nil, nil, err
	}
	revision, err := p.CapabilityRevision()
	if err != nil {
		return p, nil, nil, nil, err
	}
	p.EvaluationRevision = revision
	extra, err := json.Marshal(p.ExtraModels)
	if err != nil {
		return p, nil, nil, nil, err
	}
	probe, err := json.Marshal(p.ProbeConfig)
	if err != nil {
		return p, nil, nil, nil, err
	}
	capability, err := json.Marshal(p.CapabilityConfig)
	return p, extra, probe, capability, err
}

func (r *ChannelMonitorGroupRepository) CreateInactivePolicy(ctx context.Context, policy service.ChannelMonitorGroupPolicy, actorID int64) (*service.ChannelMonitorGroupPolicy, error) {
	if actorID <= 0 {
		return nil, errors.New("monitor policy actor is required")
	}
	p, extra, probe, capability, err := encodeInactiveMonitorPolicy(policy)
	if err != nil {
		return nil, err
	}
	created, err := scanMonitorPolicy(r.db.QueryRowContext(ctx, `INSERT INTO channel_monitor_group_policies
 (group_id,display_name,primary_model,extra_models,enabled,probe_config,capability_config,evaluation_revision,created_by,updated_by)
 SELECT g.id,$2,$3,$4,$5,$6,$7,$8,$9,$9 FROM groups g
 WHERE g.id=$1 AND g.deleted_at IS NULL AND g.status='active'
 RETURNING `+monitorPolicyColumns, p.GroupID, p.DisplayName, p.PrimaryModel,
		string(extra), p.Enabled, string(probe), string(capability), p.EvaluationRevision, actorID))
	var pgErr *pq.Error
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.Constraint == "channel_monitor_group_policies_live_group_uq" {
		return nil, service.ErrMonitorPolicyExists
	}
	return created, err
}

func (r *ChannelMonitorGroupRepository) UpdateInactivePolicy(ctx context.Context, policy service.ChannelMonitorGroupPolicy, expectedVersion, actorID int64) (*service.ChannelMonitorGroupPolicy, error) {
	if actorID <= 0 || policy.ID <= 0 || expectedVersion <= 0 {
		return nil, errors.New("invalid monitor policy update identity")
	}
	p, extra, probe, capability, err := encodeInactiveMonitorPolicy(policy)
	if err != nil {
		return nil, err
	}
	updated, err := scanMonitorPolicy(r.db.QueryRowContext(ctx, `UPDATE channel_monitor_group_policies p
 SET display_name=$2,primary_model=$3,extra_models=$4,enabled=$5,probe_config=$6,
 capability_config=$7,evaluation_revision=$8,updated_by=$9,updated_at=NOW(),version=version+1
 WHERE p.id=$1 AND p.version=$10 AND p.group_id=$11 AND p.deleted_at IS NULL
 AND EXISTS (SELECT 1 FROM groups g WHERE g.id=p.group_id AND g.deleted_at IS NULL AND g.status='active')
 AND p.next_probe_at IS NULL AND p.next_capability_at IS NULL
 AND p.active_probe_job_id IS NULL AND p.active_capability_job_id IS NULL
 AND COALESCE(p.probe_config->>'enabled','false')='false'
 AND COALESCE(p.capability_config->>'enabled','false')='false'
 AND NOT EXISTS (SELECT 1 FROM monitor_jobs j WHERE j.policy_id=p.id AND j.state IN ('queued','running','cancelling'))
 RETURNING `+monitorPolicyColumns, p.ID, p.DisplayName, p.PrimaryModel, string(extra),
		p.Enabled, string(probe), string(capability), p.EvaluationRevision, actorID, expectedVersion, p.GroupID))
	if errors.Is(err, service.ErrMonitorPolicyNotFound) {
		return nil, service.ErrMonitorPolicyConflict
	}
	return updated, err
}

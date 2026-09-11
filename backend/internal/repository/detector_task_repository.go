package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
)

func (r *LLMDetectorRepository) ReportsForOwner(ctx context.Context, jobID string, owner int64) ([]service.DetectorReportEvidence, error) {
	if _, err := r.GetJobForOwner(ctx, jobID, owner); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT r.evidence FROM llm_detector_reports r JOIN llm_detector_executions e ON e.id=r.execution_id JOIN monitor_jobs j ON j.id=e.job_id WHERE j.id=$1 AND j.owner_user_id=$2 AND j.source='site_api_key' AND r.deleted_at IS NULL ORDER BY e.target_index,r.version DESC LIMIT 24`, jobID, owner)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := []service.DetectorReportEvidence{}
	for rows.Next() {
		var raw []byte
		var evidence service.DetectorReportEvidence
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		if len(raw) > 1048576 || json.Unmarshal(raw, &evidence) != nil {
			return nil, ErrMonitorBudgetInput
		}
		items = append(items, evidence)
	}
	return items, rows.Err()
}

func (r *LLMDetectorRepository) SaveDetectorPlan(ctx context.Context, p *service.DetectorPlan) error {
	if p == nil || p.OwnerUserID == nil || *p.OwnerUserID <= 0 || p.Source != service.MonitorSourceSiteAPIKey || p.TargetSpec.Source != p.Source || p.TargetSpec.SiteAPIKeyID == nil || p.PlannedBaseRequests < 1 || p.PlannedBaseRequests > 300 || p.MaximumOutboundRequests != p.PlannedBaseRequests || p.RetryBudgetRequests != 0 || !validMonitorConfigurationHash(p.ConfigurationHash) {
		return ErrMonitorBudgetInput
	}
	target, err := json.Marshal(p.TargetSpec)
	if err != nil {
		return err
	}
	manifest, err := json.Marshal(p.Benchmark)
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = lockApprovedBenchmarkTx(ctx, tx, p.Benchmark); err != nil {
		return err
	}
	if err = lockCurrentBenchmarkChannelTx(ctx, tx, p.Benchmark); err != nil {
		return err
	}
	// Serialize admission per owner and bound queued preview accumulation.
	var owner int64
	if err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, *p.OwnerUserID).Scan(&owner); err != nil {
		return err
	}
	var count int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM llm_detector_plans WHERE owner_user_id=$1 AND expires_at>clock_timestamp()`, owner).Scan(&count); err != nil {
		return err
	}
	if count >= 20 {
		return ErrMonitorBudgetInput
	}
	p.ID = uuid.NewString()
	err = tx.QueryRowContext(ctx, `INSERT INTO llm_detector_plans(id,owner_user_id,source,normalized_target_spec,benchmark_manifest,configuration_hash,planned_base_requests,retry_budget_requests,maximum_outbound_requests,estimate_status,expires_at) VALUES($1,$2,'site_api_key',$3,$4,$5,$6,0,$6,'unknown',clock_timestamp()+interval '5 minutes') RETURNING created_at,expires_at`, p.ID, owner, target, manifest, p.ConfigurationHash, p.PlannedBaseRequests).Scan(&p.CreatedAt, &p.ExpiresAt)
	if err != nil {
		return err
	}
	if p.ExpiresAt.Before(time.Now()) {
		return ErrMonitorBudgetInput
	}
	return tx.Commit()
}

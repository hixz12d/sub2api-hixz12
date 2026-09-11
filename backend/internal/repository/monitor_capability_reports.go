package repository

import (
	"context"
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *ChannelMonitorGroupRepository) PolicyReports(ctx context.Context, policy int64, limit, offset int) ([]service.PlatformCapabilityReport, error) {
	if policy <= 0 || limit < 1 || limit > 100 || offset < 0 || offset > 10000 {
		return nil, service.ErrMonitorPolicyConflict
	}
	rows, err := r.db.QueryContext(ctx, `SELECT j.id,e.account_id,e.request_model,r.evidence,r.created_at FROM llm_detector_reports r JOIN llm_detector_executions e ON e.id=r.execution_id JOIN monitor_jobs j ON j.id=e.job_id WHERE j.policy_id=$1 AND j.source='platform_group' AND j.kind='capability' AND r.deleted_at IS NULL ORDER BY r.created_at DESC,r.id DESC LIMIT $2 OFFSET $3`, policy, limit, offset)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := []service.PlatformCapabilityReport{}
	for rows.Next() {
		var item service.PlatformCapabilityReport
		var raw []byte
		if err = rows.Scan(&item.JobID, &item.AccountID, &item.RequestModel, &raw, &item.CreatedAt); err != nil {
			return nil, err
		}
		if len(raw) > 1048576 || json.Unmarshal(raw, &item.Evidence) != nil {
			return nil, ErrMonitorBudgetInput
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

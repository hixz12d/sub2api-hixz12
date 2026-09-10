package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
)

// Read-only foundation. Job creation/claiming will be introduced with atomic
// plan consumption and budget reservation, never as independent loose inserts.
type LLMDetectorRepository struct{ db *sql.DB }

func NewLLMDetectorRepository(db *sql.DB) *LLMDetectorRepository {
	return &LLMDetectorRepository{db: db}
}

// Malformed resource IDs have the same not-found result as inaccessible IDs.
func validDetectorResourceID(id string) bool {
	return len(id) == 36 && uuid.Validate(id) == nil
}

func (r *LLMDetectorRepository) GetPlanForOwner(ctx context.Context, id string, ownerID int64) (*service.DetectorPlan, error) {
	if ownerID <= 0 || !validDetectorResourceID(id) {
		return nil, sql.ErrNoRows
	}
	var plan service.DetectorPlan
	var target, benchmark []byte
	err := r.db.QueryRowContext(ctx, `SELECT p.id,p.owner_user_id,p.source,p.policy_id,p.evaluation_revision,
 p.normalized_target_spec,p.benchmark_manifest,p.configuration_hash,p.planned_base_requests,
 p.retry_budget_requests,p.maximum_outbound_requests,p.estimated_tokens,p.estimated_cost,
 p.estimate_currency,p.estimate_status,p.expires_at,j.id,p.created_at
 FROM llm_detector_plans p LEFT JOIN monitor_jobs j ON j.plan_id=p.id
 WHERE p.id=$1 AND p.owner_user_id=$2 AND p.source IN ('external_api','site_api_key')`, id, ownerID).Scan(
		&plan.ID, &plan.OwnerUserID, &plan.Source, &plan.PolicyID, &plan.EvaluationRevision,
		&target, &benchmark, &plan.ConfigurationHash, &plan.PlannedBaseRequests,
		&plan.RetryBudgetRequests, &plan.MaximumOutboundRequests, &plan.EstimatedTokens, &plan.EstimatedCost,
		&plan.EstimateCurrency, &plan.EstimateStatus, &plan.ExpiresAt, &plan.ConsumedJobID, &plan.CreatedAt)
	if err != nil {
		return nil, err
	}
	if err = service.DecodeMonitorConfig(target, &plan.TargetSpec); err != nil {
		return nil, fmt.Errorf("decode detector target: %w", err)
	}
	if err = service.DecodeMonitorConfig(benchmark, &plan.Benchmark); err != nil {
		return nil, fmt.Errorf("decode benchmark manifest: %w", err)
	}
	return &plan, nil
}

func (r *LLMDetectorRepository) GetJobForOwner(ctx context.Context, id string, ownerID int64) (*service.MonitorJob, error) {
	if ownerID <= 0 || !validDetectorResourceID(id) {
		return nil, sql.ErrNoRows
	}
	var job service.MonitorJob
	var snapshot []byte
	err := r.db.QueryRowContext(ctx, `SELECT id,kind,source,policy_id,evaluation_revision,plan_id,owner_user_id,
 idempotency_scope,idempotency_key_hash,payload_hash,schedule_occurrence_id,state,config_snapshot,
 created_at,started_at,finished_at,deadline_at,queue_deadline_at,lease_owner,lease_expires_at,
 lease_generation,secret_owner_instance_id,cancel_requested_at,failure_code,
 base_requests_planned,outbound_reserved,outbound_dispatched,outbound_completed
 FROM monitor_jobs WHERE id=$1 AND owner_user_id=$2 AND source IN ('external_api','site_api_key') AND kind='capability'`, id, ownerID).Scan(
		&job.ID, &job.Kind, &job.Source, &job.PolicyID, &job.EvaluationRevision, &job.PlanID, &job.OwnerUserID,
		&job.IdempotencyScope, &job.IdempotencyKeyHash, &job.PayloadHash, &job.ScheduleOccurrenceID, &job.State, &snapshot,
		&job.CreatedAt, &job.StartedAt, &job.FinishedAt, &job.DeadlineAt, &job.QueueDeadlineAt,
		&job.LeaseOwner, &job.LeaseExpiresAt, &job.LeaseGeneration, &job.SecretOwnerInstanceID,
		&job.CancelRequestedAt, &job.FailureCode, &job.BaseRequestsPlanned,
		&job.OutboundReserved, &job.OutboundDispatched, &job.OutboundCompleted)
	if err != nil {
		return nil, err
	}
	if len(snapshot) > 262144 {
		return nil, errors.New("detector snapshot exceeds size limit")
	}
	if err = json.Unmarshal(snapshot, &job.ConfigSnapshot); err != nil {
		return nil, fmt.Errorf("decode detector snapshot: %w", err)
	}
	return &job, nil
}

func (r *LLMDetectorRepository) GetReportForOwner(ctx context.Context, jobID, reportID string, ownerID int64) (*service.DetectorReport, error) {
	if ownerID <= 0 || !validDetectorResourceID(jobID) || !validDetectorResourceID(reportID) {
		return nil, sql.ErrNoRows
	}
	var report service.DetectorReport
	var evidence []byte
	err := r.db.QueryRowContext(ctx, `SELECT r.id,r.execution_id,r.version,r.evidence,r.created_at
 FROM llm_detector_reports r
 JOIN llm_detector_executions e ON e.id=r.execution_id
 JOIN monitor_jobs j ON j.id=e.job_id
 WHERE r.id=$1 AND j.id=$2 AND j.owner_user_id=$3 AND r.deleted_at IS NULL
 AND j.source IN ('external_api','site_api_key') AND j.kind='capability'`, reportID, jobID, ownerID).Scan(
		&report.ID, &report.ExecutionID, &report.Version, &evidence, &report.CreatedAt)
	if err != nil {
		return nil, err
	}
	if len(evidence) > 1048576 {
		return nil, errors.New("detector report exceeds size limit")
	}
	if err = json.Unmarshal(evidence, &report.Evidence); err != nil {
		return nil, fmt.Errorf("decode detector report: %w", err)
	}
	return &report, nil
}

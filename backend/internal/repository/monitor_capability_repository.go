package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
)

var _ service.CapabilityStore = (*MonitorJobRepository)(nil)

const capabilityJobQuery = `SELECT j.id,j.owner_user_id,j.config_snapshot,j.deadline_at FROM monitor_jobs j
 WHERE j.id=$1 AND j.kind='capability' AND j.state='running' AND j.lease_owner=$2 AND j.lease_generation=$3
 AND j.lease_expires_at>clock_timestamp() AND j.deadline_at>clock_timestamp() AND j.cancel_requested_at IS NULL
 AND ` + monitorJobPolicyValid + ` AND ` + monitorJobBenchmarksValid

func scanCapabilityJob(row monitorPolicyScanner) (*service.CapabilityJob, error) {
	var j service.CapabilityJob
	var raw []byte
	if err := row.Scan(&j.ID, &j.OwnerID, &raw, &j.Deadline); err != nil {
		return nil, err
	}
	if len(raw) > 262144 || service.DecodeMonitorConfig(raw, &j.Snapshot) != nil || len(j.Snapshot.Targets) < 1 || len(j.Snapshot.Targets) > 24 || len(j.Snapshot.Benchmarks) != len(j.Snapshot.Targets) {
		return nil, ErrMonitorBudgetInput
	}
	return &j, nil
}
func (r *MonitorJobRepository) GetCapabilityJob(ctx context.Context, lease MonitorJobLease) (*service.CapabilityJob, error) {
	if !validMonitorJobLease(lease) {
		return nil, ErrMonitorBudgetInput
	}
	return scanCapabilityJob(r.db.QueryRowContext(ctx, capabilityJobQuery, lease.JobID, lease.InstanceID, lease.Generation))
}
func lockCapabilityJob(ctx context.Context, tx *sql.Tx, lease MonitorJobLease) (*service.CapabilityJob, error) {
	if !validMonitorJobLease(lease) {
		return nil, ErrMonitorBudgetInput
	}
	if _, err := lockMonitorBudgetJob(ctx, tx, lease.JobID); err != nil {
		return nil, err
	}
	if err := lockMonitorJobBenchmarksTx(ctx, tx, lease.JobID); err != nil {
		return nil, err
	}
	return scanCapabilityJob(tx.QueryRowContext(ctx, capabilityJobQuery, lease.JobID, lease.InstanceID, lease.Generation))
}
func (r *MonitorJobRepository) BeginCapabilityExecution(ctx context.Context, input service.CapabilityExecutionInput) (string, error) {
	if input.TargetIndex < 0 || input.TargetIndex >= 24 || input.Plan.PlannedSamples < 1 || input.Plan.PlannedSamples > 300 {
		return "", ErrMonitorBudgetInput
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	job, err := lockCapabilityJob(ctx, tx, input.Lease)
	if err != nil {
		return "", err
	}
	if input.TargetIndex >= len(job.Snapshot.Targets) {
		return "", ErrMonitorBudgetInput
	}
	target, b := job.Snapshot.Targets[input.TargetIndex], job.Snapshot.Benchmarks[input.TargetIndex]
	if input.Plan.ContractHash != b.RequestContractHash || input.Plan.BenchmarkSHA256 != b.SHA256 {
		return "", ErrBenchmarkRelease
	}
	if target.Source != input.Lease.Source {
		return "", ErrMonitorBudgetInput
	}
	if target.Source == service.MonitorSourcePlatformGroup {
		if input.AccountID == nil || *input.AccountID <= 0 || target.AccountID == nil || *target.AccountID != *input.AccountID || input.CredentialRevision == "" || target.GroupID == nil || job.Snapshot.CapabilityConfig == nil {
			return "", ErrMonitorBudgetInput
		}
	} else if input.AccountID != nil {
		return "", ErrMonitorBudgetInput
	}
	total := 0
	seen := map[string]bool{}
	for _, cell := range input.Plan.Cells {
		if cell.ID == "" || len(cell.ID) > 200 || seen[cell.ID] || cell.Count < 1 || cell.Count > 300 || len(cell.Payload) > 65536 || benchmarkDigest(cell.Payload) != cell.SHA256 {
			return "", ErrMonitorBudgetInput
		}
		seen[cell.ID] = true
		total += cell.Count
	}
	if total != input.Plan.PlannedSamples {
		return "", ErrMonitorBudgetInput
	}
	snapshot, err := json.Marshal(input.Plan)
	if err != nil || len(snapshot) > 1048576 {
		return "", ErrMonitorBudgetInput
	}
	var selection any
	if job.Snapshot.CapabilityConfig != nil {
		selection = job.Snapshot.CapabilityConfig.SelectionMode
	}
	id := uuid.NewString()
	_, err = tx.ExecContext(ctx, `INSERT INTO llm_detector_executions(id,job_id,source,target_index,group_id,account_id,credential_revision,site_api_key_id,
 request_model,claimed_model,benchmark_id,benchmark_version,benchmark_sha256,engine_commit,engine_version,scoring_version,sample_policy_version,
 request_contract_hash,effective_contract_hash,contract_status,tier,base_request_count,retry_budget,selection_mode,selection_snapshot,state,started_at,planned_samples)
 VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,NULL,'unknown',$19,$20,0,$21,$22,'running',clock_timestamp(),$20)`, id, job.ID, target.Source, input.TargetIndex, target.GroupID, input.AccountID, input.CredentialRevision, target.SiteAPIKeyID, target.Target.RequestModel, target.Target.ClaimedModel, b.ID, b.Version, b.SHA256, b.EngineCommit, b.EngineVersion, b.ScoringVersion, b.SamplePolicyVersion, b.RequestContractHash, target.Tier, input.Plan.PlannedSamples, selection, snapshot)
	if err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	return id, nil
}
func (r *MonitorJobRepository) BeginCapabilityDispatch(ctx context.Context, input service.CapabilityDispatch) (string, error) {
	if !validDetectorResourceID(input.ExecutionID) || input.SampleIndex < 0 || !validMonitorConfigurationHash(input.RequestSHA256) {
		return "", ErrMonitorBudgetInput
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	job, err := lockCapabilityJob(ctx, tx, input.Lease)
	if err != nil {
		return "", err
	}
	var raw []byte
	var accountID *int64
	err = tx.QueryRowContext(ctx, `SELECT selection_snapshot,account_id FROM llm_detector_executions WHERE id=$1 AND job_id=$2 AND state='running' FOR UPDATE`, input.ExecutionID, job.ID).Scan(&raw, &accountID)
	if err != nil {
		return "", err
	}
	var plan service.CapabilityPlan
	if len(raw) > 1048576 || json.Unmarshal(raw, &plan) != nil {
		return "", ErrMonitorBudgetInput
	}
	matched := false
	for _, cell := range plan.Cells {
		if cell.ID == input.CellID && input.SampleIndex < cell.Count && cell.SHA256 == input.RequestSHA256 {
			matched = true
		}
	}
	if !matched {
		return "", ErrMonitorBudgetInput
	}
	var dispatched int64
	if err = tx.QueryRowContext(ctx, `SELECT outbound_dispatched FROM monitor_jobs WHERE id=$1`, job.ID).Scan(&dispatched); err != nil {
		return "", err
	}
	if err = consumeMonitorBudgetTx(ctx, tx, job.ID, input.Lease.InstanceID, input.Lease.Generation, dispatched, input.Limits); err != nil {
		return "", err
	}
	id, outbound := uuid.NewString(), uuid.NewString()
	if err = chargeMonitorSpendTx(ctx, tx, input.CampaignID, job.ID, outbound, input.UpperBoundMicros); err != nil {
		return "", err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO llm_detector_attempts(id,execution_id,cell_id,sample_index,attempt_index,outbound_request_id,lease_generation,dispatch_state,started_at,effective_account_id,contract_status)
 VALUES($1,$2,$3,$4,0,$5,$6,'dispatching',clock_timestamp(),$7,'unknown')`, id, input.ExecutionID, input.CellID, input.SampleIndex, outbound, input.Lease.Generation, accountID)
	if err != nil {
		return "", err
	}
	if _, err = scanCapabilityJob(tx.QueryRowContext(ctx, capabilityJobQuery, input.Lease.JobID, input.Lease.InstanceID, input.Lease.Generation)); err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	return id, nil
}
func (r *MonitorJobRepository) CompleteCapabilityDispatch(ctx context.Context, lease MonitorJobLease, out service.CapabilityOutcome) error {
	if !validDetectorResourceID(out.AttemptID) || out.ElapsedMs < 0 || (out.Answer != nil && (len(*out.Answer) > 8192 || !utf8.ValidString(*out.Answer))) || (out.HTTPStatus != nil && (*out.HTTPStatus < 100 || *out.HTTPStatus > 599)) {
		return ErrMonitorBudgetInput
	}
	switch out.ErrorCategory {
	case "", "transport_error", "http_error", "invalid_response", "response_too_large", "credential_echo":
	default:
		return ErrMonitorBudgetInput
	}
	if (out.Answer == nil) == (out.ErrorCategory == "") {
		return ErrMonitorBudgetInput
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = lockCapabilityJob(ctx, tx, lease); err != nil {
		return err
	}
	usage, err := json.Marshal(out.Usage)
	if err != nil {
		return ErrMonitorBudgetInput
	}
	var usageValue any
	if out.Usage != nil {
		usageValue = usage
	}
	result, err := tx.ExecContext(ctx, `UPDATE llm_detector_attempts a SET dispatch_state='completed',completed_at=clock_timestamp(),http_status=$4,error_category=NULLIF($5,''),normalized_answer=$6,usage_summary=$7,elapsed_ms=$8
 FROM llm_detector_executions e WHERE a.id=$1 AND a.execution_id=e.id AND e.job_id=$2 AND e.state='running' AND a.lease_generation=$3 AND a.dispatch_state='dispatching'`, out.AttemptID, lease.JobID, lease.Generation, out.HTTPStatus, out.ErrorCategory, out.Answer, usageValue, out.ElapsedMs)
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
	_, err = tx.ExecContext(ctx, `UPDATE monitor_jobs SET outbound_completed=outbound_completed+1 WHERE id=$1`, lease.JobID)
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (r *MonitorJobRepository) SaveCapabilityReport(ctx context.Context, lease MonitorJobLease, executionID string, evidence service.DetectorReportEvidence) error {
	if !validDetectorResourceID(executionID) {
		return ErrMonitorBudgetInput
	}
	switch evidence.ContractStatus {
	case service.DetectorContractExact, service.DetectorContractMutated, service.DetectorContractUnknown, service.DetectorContractUnsupported:
	default:
		return ErrMonitorBudgetInput
	}
	if evidence.ContractStatus != service.DetectorContractExact && evidence.Verdict != service.DetectorNotEvaluated {
		return ErrMonitorBudgetInput
	}
	switch evidence.Verdict {
	case service.DetectorMatch, service.DetectorMismatch, service.DetectorInsufficient, service.DetectorNotEvaluated:
	default:
		return ErrMonitorBudgetInput
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	job, err := lockCapabilityJob(ctx, tx, lease)
	if err != nil {
		return err
	}
	var targetIndex int
	var planned int64
	err = tx.QueryRowContext(ctx, `SELECT target_index,planned_samples FROM llm_detector_executions WHERE id=$1 AND job_id=$2 AND state='running' FOR UPDATE`, executionID, job.ID).Scan(&targetIndex, &planned)
	if err != nil {
		return err
	}
	var completed, valid int64
	err = tx.QueryRowContext(ctx, `SELECT count(*),count(normalized_answer) FROM llm_detector_attempts WHERE execution_id=$1 AND dispatch_state='completed'`, executionID).Scan(&completed, &valid)
	if err != nil {
		return err
	}
	if completed != planned || targetIndex < 0 || targetIndex >= len(job.Snapshot.Benchmarks) {
		return ErrMonitorBudgetInput
	}
	evidence.Benchmark = job.Snapshot.Benchmarks[targetIndex]
	raw, err := json.Marshal(evidence)
	if err != nil || len(raw) > 1048576 {
		return ErrMonitorBudgetInput
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO llm_detector_reports(id,execution_id,version,evidence) VALUES($1,$2,1,$3)`, uuid.NewString(), executionID, raw)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE llm_detector_executions SET state='completed',finished_at=clock_timestamp(),verdict=$2,valid_samples=$3,contract_status=$4 WHERE id=$1`, executionID, evidence.Verdict, valid, evidence.ContractStatus)
	if err != nil {
		return err
	}
	return tx.Commit()
}

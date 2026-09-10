//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestCapabilityDispatchTransactionPostgres(t *testing.T) {
	ctx := context.Background()
	db := monitorBudgetTestDB(t)
	repo := NewMonitorJobRepository(db)
	_, err := db.Exec(`INSERT INTO api_keys(id) VALUES(1)`)
	require.NoError(t, err)
	planID := monitorJobTestPlan(t, db, 1)
	_, err = db.Exec(`UPDATE llm_detector_plans SET normalized_target_spec='{"source":"site_api_key","site_api_key_id":1,"tier":"low","target":{"request_model":"gpt-test","claimed_model":"gpt-test"}}' WHERE id=$1`, planID)
	require.NoError(t, err)
	limits := monitorBudgetTestLimits(1, 100, 100)
	jobID, err := repo.CreateFromPlan(ctx, CreateMonitorJobInput{PlanID: planID, OwnerID: 1, IdempotencyKey: "capability-dispatch", ConfigurationHash: strings.Repeat("a", 64), Limits: limits})
	require.NoError(t, err)
	lease, err := repo.Claim(ctx, service.MonitorJobCapability, "worker")
	require.NoError(t, err)
	job, err := repo.GetCapabilityJob(ctx, *lease)
	require.NoError(t, err)
	require.Equal(t, jobID, job.ID)
	payload := []byte(`{"model":"gpt-test","stream":true}`)
	b := job.Snapshot.Benchmarks[0]
	plan := service.CapabilityPlan{ContractHash: b.RequestContractHash, BenchmarkSHA256: b.SHA256, Mode: "gpt", PlannedSamples: 3, Cells: []service.CapabilityCell{{ID: "cell", Count: 3, Payload: payload, SHA256: benchmarkDigest(payload)}}}
	execution, err := repo.BeginCapabilityExecution(ctx, service.CapabilityExecutionInput{Lease: *lease, Plan: plan})
	require.NoError(t, err)
	campaign := uuid.NewString()
	_, err = db.Exec(`INSERT INTO monitor_spend_campaigns(id,request_limit,usd_limit_micros,enabled) VALUES($1,3,30,true)`, campaign)
	require.NoError(t, err)
	input := service.CapabilityDispatch{Lease: *lease, ExecutionID: execution, CellID: "cell", RequestSHA256: benchmarkDigest(payload), CampaignID: campaign, UpperBoundMicros: 10, Limits: limits}
	first, err := repo.BeginCapabilityDispatch(ctx, input)
	require.NoError(t, err)
	_, err = repo.BeginCapabilityDispatch(ctx, input)
	require.Error(t, err)
	var charges int
	require.NoError(t, db.QueryRow(`SELECT requests_charged FROM monitor_spend_campaigns WHERE id=$1`, campaign).Scan(&charges))
	require.Equal(t, 1, charges)
	answer := "3"
	status := 200
	outcome := service.CapabilityOutcome{AttemptID: first, HTTPStatus: &status, Answer: &answer, ElapsedMs: 1}
	require.NoError(t, repo.CompleteCapabilityDispatch(ctx, *lease, outcome))
	require.Error(t, repo.CompleteCapabilityDispatch(ctx, *lease, outcome))
	evidence := service.DetectorReportEvidence{Verdict: service.DetectorInsufficient, ContractStatus: service.DetectorContractExact}
	require.Error(t, repo.SaveCapabilityReport(ctx, *lease, execution, evidence))
	input.SampleIndex = 1
	input.UpperBoundMicros = 21
	_, err = repo.BeginCapabilityDispatch(ctx, input)
	require.ErrorIs(t, err, ErrMonitorSpendDenied)
	require.NoError(t, db.QueryRow(`SELECT outbound_dispatched FROM monitor_jobs WHERE id=$1`, jobID).Scan(&charges))
	require.Equal(t, 1, charges)
	input.UpperBoundMicros = 10
	for i := 1; i < 3; i++ {
		input.SampleIndex = i
		id, err := repo.BeginCapabilityDispatch(ctx, input)
		require.NoError(t, err)
		require.NoError(t, repo.CompleteCapabilityDispatch(ctx, *lease, service.CapabilityOutcome{AttemptID: id, ErrorCategory: "transport_error", ElapsedMs: 1}))
	}
	require.NoError(t, repo.SaveCapabilityReport(ctx, *lease, execution, evidence))
	require.NoError(t, repo.Finish(ctx, *lease, service.MonitorJobCompleted))
	var raw []byte
	require.NoError(t, db.QueryRow(`SELECT evidence FROM llm_detector_reports WHERE execution_id=$1`, execution).Scan(&raw))
	var stored service.DetectorReportEvidence
	require.NoError(t, json.Unmarshal(raw, &stored))
	require.Equal(t, b.ReleaseID, stored.Benchmark.ReleaseID)
	require.Equal(t, service.DetectorInsufficient, stored.Verdict)
	require.NoError(t, db.QueryRow(`SELECT usd_charged_micros FROM monitor_spend_campaigns WHERE id=$1`, campaign).Scan(&charges))
	require.Equal(t, 30, charges)
	monitorBudgetTestBucket(t, db, "global", 0, 0, 3)
}

//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type capabilityIntegrationEngine struct{ plan *service.CapabilityPlan }

func (e capabilityIntegrationEngine) Plan(context.Context, *service.BenchmarkRelease, service.DetectorTargetSpec) (*service.CapabilityPlan, error) {
	return e.plan, nil
}
func (e capabilityIntegrationEngine) Decode(_ context.Context, _ *service.BenchmarkRelease, raw []byte) (*service.CapabilityDecoded, error) {
	var result service.CapabilityDecoded
	err := json.Unmarshal(raw, &result)
	return &result, err
}
func (e capabilityIntegrationEngine) Score(_ context.Context, _ *service.BenchmarkRelease, _ service.DetectorTargetSpec, _ string, status service.DetectorContractStatus, samples []service.CapabilitySample) (*service.DetectorReportEvidence, error) {
	return &service.DetectorReportEvidence{Verdict: service.DetectorNotEvaluated, ContractStatus: status}, nil
}

type capabilityIntegrationTarget struct{ binding *service.CapabilityBinding }

func (t capabilityIntegrationTarget) Resolve(context.Context, *service.CapabilityJob, int) (*service.CapabilityBinding, error) {
	return t.binding, nil
}
func (t capabilityIntegrationTarget) Check(context.Context, *service.CapabilityJob, int, *service.CapabilityBinding) error {
	return nil
}

type capabilityIntegrationHTTP struct {
	service.HTTPUpstream
	client *http.Client
}

func (h capabilityIntegrationHTTP) Do(req *http.Request, _ string, account int64, _ int) (*http.Response, error) {
	return httpClientWithMonitorGuard(h.client, req, account).Do(req)
}

type capabilityIntegrationSlots struct {
	service.MonitorProbeConcurrency
}

func TestCapabilityPhysicalHTTPPostgres(t *testing.T) {
	ctx := context.Background()
	db := monitorBudgetTestDB(t)
	jobs := NewMonitorJobRepository(db)
	_, err := db.Exec(`INSERT INTO api_keys(id) VALUES(1)`)
	require.NoError(t, err)
	id := monitorJobTestPlan(t, db, 1)
	_, err = db.Exec(`UPDATE llm_detector_plans SET normalized_target_spec='{"source":"site_api_key","site_api_key_id":1,"tier":"low","target":{"request_model":"gpt-test","claimed_model":"gpt-test"}}' WHERE id=$1`, id)
	require.NoError(t, err)
	limits := monitorBudgetTestLimits(1, 100, 100)
	jobID, err := jobs.CreateFromPlan(ctx, CreateMonitorJobInput{PlanID: id, OwnerID: 1, IdempotencyKey: "physical-http", ConfigurationHash: strings.Repeat("a", 64), Limits: limits})
	require.NoError(t, err)
	lease, err := jobs.Claim(ctx, service.MonitorJobCapability, "worker")
	require.NoError(t, err)
	job, err := jobs.GetCapabilityJob(ctx, *lease)
	require.NoError(t, err)
	b := job.Snapshot.Benchmarks[0]
	campaign := uuid.NewString()
	_, err = db.Exec(`INSERT INTO monitor_spend_campaigns(id,request_limit,usd_limit_micros,enabled) VALUES($1,3,30,true)`, campaign)
	require.NoError(t, err)
	var received atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer fixture-key" {
			http.Error(w, "denied", 403)
			return
		}
		received.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"answer":"3","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer server.Close()
	payload := []byte(`{"model":"gpt-test","stream":true}`)
	plan := &service.CapabilityPlan{Mode: "gpt", ContractHash: b.RequestContractHash, BenchmarkSHA256: b.SHA256, PlannedSamples: 3, Cells: []service.CapabilityCell{{ID: "cell", Count: 3, Payload: payload, SHA256: benchmarkDigest(payload)}}}
	binding := &service.CapabilityBinding{Endpoint: server.URL, Headers: http.Header{"Authorization": []string{"Bearer fixture-key"}}, Secrets: []string{"fixture-key"}, CampaignID: campaign, Limits: limits, UpperBound: func([]byte) (int64, error) { return 10, nil }, ContractStatus: service.DetectorContractUnknown}
	executor := &service.CapabilityExecutor{Store: jobs, Engine: capabilityIntegrationEngine{plan}, Releases: NewBenchmarkRegistryRepository(db), Targets: capabilityIntegrationTarget{binding}, Upstream: capabilityIntegrationHTTP{client: server.Client()}, Slots: capabilityIntegrationSlots{}}
	require.NoError(t, executor.Execute(ctx, *lease))
	require.EqualValues(t, 3, received.Load())
	require.NoError(t, jobs.Finish(ctx, *lease, service.MonitorJobCompleted))
	var state, verdict string
	var dispatched, completed int
	require.NoError(t, db.QueryRow(`SELECT j.state,j.outbound_dispatched,j.outbound_completed,e.verdict FROM monitor_jobs j JOIN llm_detector_executions e ON e.job_id=j.id WHERE j.id=$1`, jobID).Scan(&state, &dispatched, &completed, &verdict))
	require.Equal(t, "completed", state)
	require.Equal(t, 3, dispatched)
	require.Equal(t, 3, completed)
	require.Equal(t, "not_evaluated", verdict)
	var micros int
	require.NoError(t, db.QueryRow(`SELECT usd_charged_micros FROM monitor_spend_campaigns WHERE id=$1`, campaign).Scan(&micros))
	require.Equal(t, 30, micros)
}

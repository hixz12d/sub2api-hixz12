//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type capabilityFixture struct {
	CapabilityEngine
	CapabilityStore
	CapabilityTargetResolver
	CapabilityReleases
	HTTPUpstream
	MonitorProbeConcurrency
	sends, permits, completed, reports int
	denied                             bool
	noGuard                            bool
	binding                            *CapabilityBinding
	job                                *CapabilityJob
	plan                               *CapabilityPlan
	release                            *BenchmarkRelease
}

func (f *capabilityFixture) GetCapabilityJob(context.Context, MonitorJobLease) (*CapabilityJob, error) {
	return f.job, nil
}
func (f *capabilityFixture) Get(context.Context, string) (*BenchmarkRelease, error) {
	return f.release, nil
}
func (f *capabilityFixture) Resolve(context.Context, *CapabilityJob, int) (*CapabilityBinding, error) {
	return f.binding, nil
}
func (f *capabilityFixture) Check(context.Context, *CapabilityJob, int, *CapabilityBinding) error {
	if f.denied {
		return ErrMonitorOutboundDenied
	}
	return nil
}
func (f *capabilityFixture) Plan(context.Context, *BenchmarkRelease, DetectorTargetSpec) (*CapabilityPlan, error) {
	return f.plan, nil
}
func (f *capabilityFixture) Decode(context.Context, *BenchmarkRelease, []byte) (*CapabilityDecoded, error) {
	return &CapabilityDecoded{Answer: "3"}, nil
}
func (f *capabilityFixture) Score(_ context.Context, _ *BenchmarkRelease, _ DetectorTargetSpec, _ string, status DetectorContractStatus, samples []CapabilitySample) (*DetectorReportEvidence, error) {
	verdict := DetectorInsufficient
	if status != DetectorContractExact {
		verdict = DetectorNotEvaluated
	}
	return &DetectorReportEvidence{Verdict: verdict, ContractStatus: status}, nil
}
func (f *capabilityFixture) BeginCapabilityExecution(context.Context, CapabilityExecutionInput) (string, error) {
	return "execution", nil
}
func (f *capabilityFixture) BeginCapabilityDispatch(context.Context, CapabilityDispatch) (string, error) {
	f.permits++
	return "attempt", nil
}
func (f *capabilityFixture) CompleteCapabilityDispatch(context.Context, MonitorJobLease, CapabilityOutcome) error {
	f.completed++
	return nil
}
func (f *capabilityFixture) SaveCapabilityReport(context.Context, MonitorJobLease, string, DetectorReportEvidence) error {
	f.reports++
	return nil
}
func (f *capabilityFixture) Do(req *http.Request, _ string, account int64, _ int) (*http.Response, error) {
	if !f.noGuard {
		g := MonitorOutboundGuardFromContext(req.Context())
		if err := g.Authorize(req.Context(), account, req); err != nil {
			return nil, err
		}
		if err := g.Authorize(req.Context(), account, req); err == nil {
			panic("second send authorized")
		}
	}
	f.sends++
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("fixture"))}, nil
}
func TestCapabilityExecutorGuard(t *testing.T) {
	for _, denied := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "revoked"}[denied], func(t *testing.T) {
			f := &capabilityFixture{denied: denied}
			f.release = &BenchmarkRelease{State: "approved", SHA256: "hash", EngineLockSHA256: "lock"}
			f.job = &CapabilityJob{Deadline: time.Now().Add(time.Minute), Snapshot: MonitorJobSnapshot{Targets: []DetectorTargetSpec{{}}, Benchmarks: []DetectorBenchmarkManifest{{SHA256: "hash", EngineLockSHA256: "lock", RequestContractHash: "contract"}}}}
			f.plan = &CapabilityPlan{ContractHash: "contract", Cells: []CapabilityCell{{ID: "cell", Count: 2, Payload: []byte(`{"model":"test"}`)}}}
			f.binding = &CapabilityBinding{Endpoint: "https://example.test/v1/responses", Headers: http.Header{"Authorization": []string{"Bearer fixture-secret"}}, Secrets: []string{"fixture-secret"}, UpperBound: func([]byte) (int64, error) { return 10, nil }}
			executor := &CapabilityExecutor{Store: f, Engine: f, Releases: f, Targets: f, Upstream: f, Slots: f}
			err := executor.Execute(context.Background(), MonitorJobLease{Kind: MonitorJobCapability})
			if denied {
				require.Error(t, err)
				require.Zero(t, f.sends)
				require.Zero(t, f.permits)
			} else {
				require.NoError(t, err)
				require.Equal(t, 2, f.sends)
				require.Equal(t, 2, f.permits)
				require.Equal(t, 2, f.completed)
				require.Equal(t, 1, f.reports)
			}
		})
	}
}
func TestCapabilityPriceCeiling(t *testing.T) {
	price := CapabilityPriceCeiling{InputTokenLimit: 1000, InputMicrosPerToken: 3, OutputMicrosPerToken: 15, FixedMicros: 7}
	bound, err := price.UpperBound([]byte(`{"max_output_tokens":256}`))
	require.NoError(t, err)
	require.EqualValues(t, 6847, bound)
	for _, raw := range []string{`{}`, `{"max_tokens":-1}`, `{"max_tokens":1.5}`, `{"max_tokens":1,"max_output_tokens":2}`, `{"max_tokens":null}`, `null`} {
		_, err = price.UpperBound([]byte(raw))
		require.Error(t, err)
	}
	price.InputMicrosPerToken = math.MaxInt64
	_, err = price.UpperBound([]byte(`{"max_tokens":1}`))
	require.Error(t, err)
}
func TestCapabilityBoundedOutputCopy(t *testing.T) {
	out := &benchmarkBoundedOutput{limit: 1024}
	_, err := io.Copy(out, io.LimitReader(bytes.NewReader(make([]byte, 1025)), 1025))
	require.ErrorIs(t, err, ErrBenchmarkValidator)
	require.LessOrEqual(t, out.Len(), 1024)
}

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sync"
	"time"
)

// Resolver implementations must recheck owner, Key, group and account admission.
// An external credential stays in memory on the instance that owns the job.
type CapabilityTargetResolver interface {
	Resolve(context.Context, *CapabilityJob, int) (*CapabilityBinding, error)
	Check(context.Context, *CapabilityJob, int, *CapabilityBinding) error
}
type CapabilityBinding struct {
	ContractStatus     DetectorContractStatus
	Mode               string
	AccountID          *int64
	CredentialRevision string
	Endpoint           string
	Headers            http.Header
	ProxyURL           string
	Concurrency        int
	Secrets            []string
	CampaignID         string
	Limits             []MonitorBudgetLimit
	// UpperBound is server-owned pricing, including all input and maximum output.
	UpperBound func([]byte) (int64, error)
}
type CapabilityReleases interface {
	Get(context.Context, string) (*BenchmarkRelease, error)
}
type CapabilityExecutor struct {
	Scheduler *PlatformCapabilityScheduler
	Store     CapabilityStore
	Engine    CapabilityEngine
	Releases  CapabilityReleases
	Targets   CapabilityTargetResolver
	Upstream  HTTPUpstream
	Slots     MonitorProbeConcurrency
}

func (e *CapabilityExecutor) Ready() bool {
	return e != nil && e.Store != nil && e.Engine != nil && e.Releases != nil && e.Targets != nil && e.Upstream != nil && e.Slots != nil
}
func (e *CapabilityExecutor) Execute(ctx context.Context, lease MonitorJobLease) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !e.Ready() || lease.Kind != MonitorJobCapability {
		return ErrMonitorOutboundDenied
	}
	job, err := e.Store.GetCapabilityJob(ctx, lease)
	if err != nil {
		return err
	}
	if job == nil {
		return ErrMonitorOutboundDenied
	}
	ctx, cancel := context.WithDeadline(ctx, job.Deadline)
	defer cancel()
	ctx, err = WithRequestOrigin(ctx, capabilityOrigin(lease.Source))
	if err != nil {
		return err
	}
	for index, target := range job.Snapshot.Targets {
		if index >= len(job.Snapshot.Benchmarks) {
			return ErrMonitorOutboundDenied
		}
		manifest := job.Snapshot.Benchmarks[index]
		release, err := e.Releases.Get(ctx, manifest.ReleaseID)
		if err != nil {
			return err
		}
		if release == nil || release.State != "approved" || release.SHA256 != manifest.SHA256 || release.EngineLockSHA256 != manifest.EngineLockSHA256 {
			return ErrMonitorOutboundDenied
		}
		binding, err := e.Targets.Resolve(ctx, job, index)
		if err != nil {
			return err
		}
		if binding == nil || binding.UpperBound == nil || binding.Endpoint == "" || len(binding.Secrets) == 0 {
			return ErrMonitorOutboundDenied
		}
		if binding.ContractStatus == "" {
			binding.ContractStatus = DetectorContractUnknown
		}
		plan, err := e.Engine.Plan(ctx, release, target)
		if err != nil {
			return err
		}
		if plan == nil || plan.ContractHash != manifest.RequestContractHash || (binding.Mode != "" && plan.Mode != binding.Mode) {
			return ErrMonitorOutboundDenied
		}
		execution, err := e.Store.BeginCapabilityExecution(ctx, CapabilityExecutionInput{Lease: lease, TargetIndex: index, AccountID: binding.AccountID, CredentialRevision: binding.CredentialRevision, Plan: *plan})
		if err != nil {
			return err
		}
		samples := []CapabilitySample{}
		for _, cell := range plan.Cells {
			for sample := 0; sample < cell.Count; sample++ {
				answer, err := e.sample(ctx, lease, job, index, binding, release, execution, cell, sample)
				if err != nil {
					return err
				}
				if answer != nil {
					samples = append(samples, CapabilitySample{CellID: cell.ID, SampleIndex: sample, Answer: *answer})
				}
			}
		}
		evidence, err := e.Engine.Score(ctx, release, target, plan.ContractHash, binding.ContractStatus, samples)
		if err != nil {
			return err
		}
		if evidence == nil || evidence.ContractStatus != binding.ContractStatus || (binding.ContractStatus != DetectorContractExact && evidence.Verdict != DetectorNotEvaluated) {
			return ErrBenchmarkValidator
		}
		if err = e.Store.SaveCapabilityReport(ctx, lease, execution, *evidence); err != nil {
			return err
		}
	}
	return nil
}
func (e *CapabilityExecutor) sample(ctx context.Context, lease MonitorJobLease, job *CapabilityJob, index int, b *CapabilityBinding, release *BenchmarkRelease, execution string, cell CapabilityCell, sample int) (*string, error) {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	if err := e.Targets.Check(ctx, job, index, b); err != nil {
		return nil, err
	}
	account := int64(0)
	if b.AccountID != nil {
		account = *b.AccountID
		slot, err := e.Slots.AcquireAccountSlot(ctx, account, b.Concurrency)
		if err != nil || slot == nil || !slot.Acquired || slot.ReleaseFunc == nil {
			return nil, ErrMonitorOutboundDenied
		}
		defer slot.ReleaseFunc()
	}
	bound, err := b.UpperBound(cell.Payload)
	if err != nil || bound <= 0 {
		return nil, ErrMonitorOutboundDenied
	}
	g := &capabilityGuard{executor: e, job: job, index: index, binding: b, payload: cell.Payload, dispatch: CapabilityDispatch{Lease: lease, ExecutionID: execution, CellID: cell.ID, SampleIndex: sample, RequestSHA256: cell.SHA256, CampaignID: b.CampaignID, UpperBoundMicros: bound, Limits: b.Limits}}
	req, err := http.NewRequestWithContext(WithMonitorOutboundGuard(ctx, g), http.MethodPost, b.Endpoint, bytes.NewReader(cell.Payload))
	if err != nil {
		return nil, ErrMonitorOutboundDenied
	}
	req.Header = b.Headers.Clone()
	started := time.Now()
	response, sendErr := e.Upstream.Do(req, b.ProxyURL, account, b.Concurrency)
	g.mu.Lock()
	attempt := g.attempt
	g.mu.Unlock()
	if attempt == "" {
		if response != nil && response.Body != nil {
			response.Body.Close()
		}
		return nil, ErrMonitorOutboundDenied
	}
	outcome := CapabilityOutcome{AttemptID: attempt, ErrorCategory: "transport_error"}
	if response != nil && response.Body != nil {
		status := response.StatusCode
		outcome.HTTPStatus = &status
		raw, readErr := io.ReadAll(io.LimitReader(response.Body, 1024*1024+1))
		response.Body.Close()
		switch {
		case sendErr != nil || readErr != nil:
		case len(raw) > 1024*1024:
			outcome.ErrorCategory = "response_too_large"
		case status < 200 || status >= 300:
			outcome.ErrorCategory = "http_error"
		default:
			echoed := false
			for _, secret := range b.Secrets {
				if secret != "" && bytes.Contains(raw, []byte(secret)) {
					echoed = true
				}
			}
			if echoed {
				outcome.ErrorCategory = "credential_echo"
			} else {
				decoded, decodeErr := e.Engine.Decode(ctx, release, raw)
				if decodeErr != nil || decoded == nil {
					outcome.ErrorCategory = "invalid_response"
				} else {
					for _, secret := range b.Secrets {
						if secret != "" && bytes.Contains([]byte(decoded.Answer), []byte(secret)) {
							echoed = true
						}
					}
					if echoed {
						outcome.ErrorCategory = "credential_echo"
					} else {
						outcome.Answer = &decoded.Answer
						outcome.Usage = &decoded.Usage
						outcome.ErrorCategory = ""
					}
				}
			}
		}
	}
	outcome.ElapsedMs = time.Since(started).Milliseconds()
	if err = e.Store.CompleteCapabilityDispatch(ctx, lease, outcome); err != nil {
		return nil, err
	}
	return outcome.Answer, nil
}

func capabilityOrigin(source MonitorJobSource) RequestOrigin {
	if source == MonitorSourcePlatformGroup {
		return RequestOriginCapabilityDetector
	}
	return RequestOriginUserDetector
}

type capabilityGuard struct {
	attempted bool
	executor  *CapabilityExecutor
	job       *CapabilityJob
	index     int
	binding   *CapabilityBinding
	payload   []byte
	dispatch  CapabilityDispatch
	mu        sync.Mutex
	attempt   string
}

func (g *capabilityGuard) Authorize(ctx context.Context, account int64, req *http.Request) error {
	if ctx.Err() != nil || req == nil || req.URL == nil {
		return ErrMonitorOutboundDenied
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	expected := int64(0)
	if g.binding.AccountID != nil {
		expected = *g.binding.AccountID
	}
	if g.attempted || account != expected || req.Method != http.MethodPost || req.URL.String() != g.binding.Endpoint || req.GetBody == nil || RequestOriginFromContext(ctx) != capabilityOrigin(g.dispatch.Lease.Source) {
		return ErrMonitorOutboundDenied
	}
	for key, values := range g.binding.Headers {
		actual := req.Header.Values(key)
		if len(actual) != len(values) {
			return ErrMonitorOutboundDenied
		}
		for i := range values {
			if actual[i] != values[i] {
				return ErrMonitorOutboundDenied
			}
		}
	}
	body, err := req.GetBody()
	if err != nil {
		return ErrMonitorOutboundDenied
	}
	raw, err := io.ReadAll(io.LimitReader(body, 65537))
	body.Close()
	if err != nil || !bytes.Equal(raw, g.payload) {
		return ErrMonitorOutboundDenied
	}
	if err = g.executor.Targets.Check(ctx, g.job, g.index, g.binding); err != nil {
		return ErrMonitorOutboundDenied
	}
	// A commit response can be lost. Never retry this permit in a transport fallback.
	g.attempted = true
	g.attempt, err = g.executor.Store.BeginCapabilityDispatch(ctx, g.dispatch)
	return err
}

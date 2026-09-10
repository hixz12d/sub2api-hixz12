package service

import (
	"context"
	"encoding/json"
	"time"
)

type DetectorJobInput struct {
	PlanID                string
	OwnerID               int64
	IdempotencyKey        string
	ConfigurationHash     string
	SecretOwnerInstanceID string
	Limits                []MonitorBudgetLimit
}
type DetectorTaskStore interface {
	ReportsForOwner(context.Context, string, int64) ([]DetectorReportEvidence, error)
	SaveDetectorPlan(context.Context, *DetectorPlan) error
	GetPlanForOwner(context.Context, string, int64) (*DetectorPlan, error)
	GetJobForOwner(context.Context, string, int64) (*MonitorJob, error)
}
type DetectorTaskJobs interface {
	CreateFromPlan(context.Context, DetectorJobInput) (string, error)
	CancelForOwner(context.Context, string, int64) error
}
type DetectorTaskService struct {
	Store    DetectorTaskStore
	Jobs     DetectorTaskJobs
	Executor *CapabilityExecutor
	Registry BenchmarkRegistryStore
}

func NewDetectorTaskService(store DetectorTaskStore, jobs DetectorTaskJobs, executor *CapabilityExecutor, registry BenchmarkRegistryStore) *DetectorTaskService {
	return &DetectorTaskService{store, jobs, executor, registry}
}
func (s *DetectorTaskService) Plan(ctx context.Context, owner, keyID int64, channel, model, claimed, tier string) (*DetectorPlan, error) {
	if s == nil || s.Store == nil || s.Jobs == nil || !s.Executor.Ready() || s.Registry == nil || owner <= 0 || keyID <= 0 || !validMonitorIdentifier(model, 200) || !validMonitorIdentifier(claimed, 200) || (tier != "low" && tier != "medium" && tier != "high") {
		return nil, ErrMonitorOutboundDenied
	}
	channels, err := s.Registry.Channels(ctx)
	if err != nil {
		return nil, err
	}
	var selected *BenchmarkChannel
	for i := range channels {
		if channels[i].Name == channel && channels[i].State == "approved" {
			selected = &channels[i]
			break
		}
	}
	if selected == nil {
		return nil, ErrBenchmarkRelease
	}
	release, err := s.Registry.Get(ctx, selected.ReleaseID)
	if err != nil {
		return nil, err
	}
	if release == nil || release.State != "approved" {
		return nil, ErrBenchmarkRelease
	}
	var metadata struct {
		Commit  string `json:"engine_commit"`
		Version string `json:"engine_version"`
		Scoring string `json:"scoring_version"`
		Policy  string `json:"sample_policy_version"`
	}
	if json.Unmarshal(release.EngineLock, &metadata) != nil {
		return nil, ErrBenchmarkRelease
	}
	manifest := DetectorBenchmarkManifest{Channel: channel, ChannelRevision: selected.Revision, ReleaseID: release.ID, EngineLockSHA256: release.EngineLockSHA256, ID: release.BenchmarkID, Version: release.Version, SHA256: release.SHA256, EngineCommit: metadata.Commit, EngineVersion: metadata.Version, ScoringVersion: metadata.Scoring, SamplePolicyVersion: metadata.Policy}
	target := DetectorTargetSpec{Source: MonitorSourceSiteAPIKey, SiteAPIKeyID: &keyID, Tier: tier, Target: DetectorTarget{RequestModel: model, ClaimedModel: claimed}}
	job := &CapabilityJob{OwnerID: &owner, Snapshot: MonitorJobSnapshot{Targets: []DetectorTargetSpec{target}, Benchmarks: []DetectorBenchmarkManifest{manifest}}}
	binding, err := s.Executor.Targets.Resolve(ctx, job, 0)
	if err != nil {
		return nil, err
	}
	plan, err := s.Executor.Engine.Plan(ctx, release, target)
	if err != nil {
		return nil, err
	}
	if plan == nil || binding == nil || plan.Mode != binding.Mode {
		return nil, ErrMonitorOutboundDenied
	}
	manifest.RequestContractHash = plan.ContractHash
	raw, err := json.Marshal(struct {
		Target    DetectorTargetSpec
		Benchmark DetectorBenchmarkManifest
	}{target, manifest})
	if err != nil {
		return nil, err
	}
	result := &DetectorPlan{OwnerUserID: &owner, Source: MonitorSourceSiteAPIKey, TargetSpec: target, Benchmark: manifest, ConfigurationHash: benchmarkSHA256(raw), PlannedBaseRequests: int64(plan.PlannedSamples), MaximumOutboundRequests: int64(plan.PlannedSamples), EstimateStatus: "unknown", ExpiresAt: time.Now().Add(5 * time.Minute)}
	if err = s.Executor.Targets.Check(ctx, job, 0, binding); err != nil {
		return nil, err
	}
	if err = s.Store.SaveDetectorPlan(ctx, result); err != nil {
		return nil, err
	}
	return result, nil
}
func (s *DetectorTaskService) Create(ctx context.Context, owner int64, id, key, hash string) (string, error) {
	if s == nil || !s.Executor.Ready() {
		return "", ErrMonitorOutboundDenied
	}
	plan, err := s.Store.GetPlanForOwner(ctx, id, owner)
	if err != nil {
		return "", err
	}
	if plan == nil || plan.Source != MonitorSourceSiteAPIKey || plan.ConfigurationHash != hash || (!plan.ExpiresAt.After(time.Now()) && plan.ConsumedJobID == nil) {
		return "", ErrMonitorOutboundDenied
	}
	job := &CapabilityJob{OwnerID: &owner, Snapshot: MonitorJobSnapshot{Targets: []DetectorTargetSpec{plan.TargetSpec}, Benchmarks: []DetectorBenchmarkManifest{plan.Benchmark}}}
	binding, err := s.Executor.Targets.Resolve(ctx, job, 0)
	if err != nil {
		return "", err
	}
	return s.Jobs.CreateFromPlan(ctx, DetectorJobInput{PlanID: id, OwnerID: owner, IdempotencyKey: key, ConfigurationHash: hash, Limits: binding.Limits})
}

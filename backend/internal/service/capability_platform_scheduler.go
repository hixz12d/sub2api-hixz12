package service

import (
	"context"
	"encoding/json"
	"math/rand/v2"
	"slices"
)

type PlatformCapabilityQueue interface {
	DueCapabilityPolicies(context.Context, int, []int64) ([]ChannelMonitorGroupPolicy, error)
	EnqueuePlatformCapability(context.Context, ChannelMonitorGroupPolicy, MonitorJobSnapshot, int64, []MonitorBudgetLimit) (string, error)
	DeferCapabilityPolicy(context.Context, int64, int64) error
}
type PlatformCapabilityScheduler struct {
	Queue    PlatformCapabilityQueue
	Accounts MonitorProbeAccounts
	Resolver *PlatformCapabilityResolver
	Engine   CapabilityEngine
	Registry BenchmarkRegistryStore
}

func (s *PlatformCapabilityScheduler) Schedule(ctx context.Context) error {
	if s == nil || s.Queue == nil || s.Resolver == nil || s.Engine == nil || s.Registry == nil {
		return ErrMonitorOutboundDenied
	}
	if !s.Resolver.Flags.GetMonitorFeatureFlags(ctx).ScheduledDetectionAllowed(true) {
		return nil
	}
	policies, err := s.Queue.DueCapabilityPolicies(ctx, 1, s.Resolver.AllowedGroupIDs)
	if err != nil {
		return err
	}
	for _, policy := range policies {
		if !slices.Contains(s.Resolver.AllowedGroupIDs, policy.GroupID) {
			continue
		}
		if err = s.enqueue(ctx, policy); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if deferErr := s.Queue.DeferCapabilityPolicy(ctx, policy.ID, policy.Version); deferErr != nil {
				return deferErr
			}
			return err
		}
	}
	return nil
}
func (s *PlatformCapabilityScheduler) enqueue(ctx context.Context, p ChannelMonitorGroupPolicy) error {
	if err := p.Normalize(); err != nil {
		return err
	}
	candidates, err := s.Accounts.ListSchedulableByGroupID(ctx, p.GroupID)
	if err != nil {
		return err
	}
	group, err := s.Resolver.Groups.GetByID(ctx, p.GroupID)
	if err != nil {
		return err
	}
	targets := make([]DetectorTargetSpec, 0, len(p.CapabilityConfig.Targets))
	for _, target := range p.CapabilityConfig.Targets {
		targets = append(targets, DetectorTargetSpec{Source: MonitorSourcePlatformGroup, GroupID: &p.GroupID, Target: target, Tier: p.CapabilityConfig.Tier})
	}
	eligible := []int64{}
	for i := range candidates {
		a := &candidates[i]
		if monitorProbeAccountAllowed(group, a, targets) && !shouldForwardOpenAIResponsesViaRawChatCompletions(a) {
			allowed := true
			for _, target := range targets {
				if a.GetMappedModel(target.Target.RequestModel) != target.Target.RequestModel {
					allowed = false
				}
			}
			if allowed {
				eligible = append(eligible, a.ID)
			}
		}
	}
	selected := append([]int64(nil), p.CapabilityConfig.FixedAccountIDs...)
	if p.CapabilityConfig.SelectionMode == MonitorSelectionRandom {
		rand.Shuffle(len(eligible), func(i, j int) { eligible[i], eligible[j] = eligible[j], eligible[i] })
		if len(eligible) < p.CapabilityConfig.SampleSize {
			return ErrMonitorProbeUnsupported
		}
		selected = eligible[:p.CapabilityConfig.SampleSize]
	}
	if len(selected) != p.CapabilityConfig.SampleSize {
		return ErrMonitorProbeUnsupported
	}
	for _, id := range selected {
		if !slices.Contains(eligible, id) {
			return ErrMonitorProbeUnsupported
		}
	}
	releases, err := s.Registry.List(ctx, 100, 0)
	if err != nil {
		return err
	}
	channels, err := s.Registry.Channels(ctx)
	if err != nil {
		return err
	}
	snapshot := MonitorJobSnapshot{PolicyVersion: p.Version, CapabilityConfig: &p.CapabilityConfig, Targets: []DetectorTargetSpec{}, Benchmarks: []DetectorBenchmarkManifest{}}
	var total int64
	for _, target := range targets {
		releaseID := ""
		for _, release := range releases {
			if release.State == "approved" && release.BenchmarkID == target.Target.BenchmarkID && release.Version == target.Target.BenchmarkVersion && release.SHA256 == target.Target.BenchmarkSHA256 {
				releaseID = release.ID
				break
			}
		}
		var channelRevision int64
		if target.Target.BenchmarkChannel != "" {
			releaseID = ""
			for _, channel := range channels {
				if channel.Name == target.Target.BenchmarkChannel && channel.State == "approved" {
					releaseID = channel.ReleaseID
					channelRevision = channel.Revision
					break
				}
			}
		}
		if releaseID == "" {
			return ErrBenchmarkRelease
		}
		release, err := s.Registry.Get(ctx, releaseID)
		if err != nil {
			return err
		}
		if release == nil || release.State != "approved" {
			return ErrBenchmarkRelease
		}
		target.Target.BenchmarkID = release.BenchmarkID
		target.Target.BenchmarkVersion = release.Version
		target.Target.BenchmarkSHA256 = release.SHA256
		plan, err := s.Engine.Plan(ctx, release, target)
		if err != nil {
			return err
		}
		if plan == nil || plan.Mode != "gpt" || plan.PlannedSamples < 1 || plan.PlannedSamples > 300 {
			return ErrMonitorOutboundDenied
		}
		var metadata struct {
			Commit  string `json:"engine_commit"`
			Version string `json:"engine_version"`
			Scoring string `json:"scoring_version"`
			Policy  string `json:"sample_policy_version"`
		}
		if json.Unmarshal(release.EngineLock, &metadata) != nil {
			return ErrBenchmarkRelease
		}
		manifest := DetectorBenchmarkManifest{Channel: target.Target.BenchmarkChannel, ChannelRevision: channelRevision, ReleaseID: release.ID, EngineLockSHA256: release.EngineLockSHA256, ID: release.BenchmarkID, Version: release.Version, SHA256: release.SHA256, EngineCommit: metadata.Commit, EngineVersion: metadata.Version, ScoringVersion: metadata.Scoring, SamplePolicyVersion: metadata.Policy, RequestContractHash: plan.ContractHash}
		for _, id := range selected {
			frozen := target
			frozen.AccountID = &id
			snapshot.Targets = append(snapshot.Targets, frozen)
			snapshot.Benchmarks = append(snapshot.Benchmarks, manifest)
			total += int64(plan.PlannedSamples)
		}
	}
	job := &CapabilityJob{Snapshot: snapshot}
	var limits []MonitorBudgetLimit
	for index := range snapshot.Targets {
		binding, err := s.Resolver.Resolve(ctx, job, index)
		if err != nil {
			return err
		}
		limits = binding.Limits
	}
	if total < 1 || len(snapshot.Targets) > 24 {
		return ErrMonitorOutboundDenied
	}
	_, err = s.Queue.EnqueuePlatformCapability(ctx, p, snapshot, total, limits)
	return err
}

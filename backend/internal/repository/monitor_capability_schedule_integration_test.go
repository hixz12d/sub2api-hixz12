//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPlatformCapabilityPolicyLifecycle(t *testing.T) {
	db := monitorBudgetTestDB(t)
	ctx := context.Background()
	_, err := db.Exec(`ALTER TABLE groups ADD deleted_at timestamptz; ALTER TABLE groups ADD status text DEFAULT 'active'`)
	require.NoError(t, err)
	raw := monitorTestApprovedManifest(t, db, 1)
	var manifest service.DetectorBenchmarkManifest
	require.NoError(t, json.Unmarshal([]byte(raw), &manifest))
	config := service.DefaultGroupCapabilityConfig()
	config.Enabled = true
	registry := NewBenchmarkRegistryRepository(db)
	channelRevision, err := registry.Activate(ctx, "auto", manifest.ReleaseID, 0, 1)
	require.NoError(t, err)
	manifest.Channel = "auto"
	manifest.ChannelRevision = channelRevision
	config.Targets = []service.DetectorTarget{{BenchmarkChannel: "auto", RequestModel: "model", ClaimedModel: "model", BenchmarkID: manifest.ID, BenchmarkVersion: manifest.Version, BenchmarkSHA256: manifest.SHA256}}
	policies := NewChannelMonitorGroupRepository(db)
	p, err := policies.SavePolicy(ctx, service.ChannelMonitorGroupPolicy{GroupID: 1, DisplayName: "Original", PrimaryModel: "model", Enabled: true, ProbeConfig: service.DefaultGroupProbeConfig(), CapabilityConfig: config}, 0, 1)
	require.NoError(t, err)
	queue := NewMonitorJobRepository(db)
	due, err := queue.DueCapabilityPolicies(ctx, 1, []int64{1})
	require.NoError(t, err)
	require.Len(t, due, 1)
	account := int64(1)
	snapshot := service.MonitorJobSnapshot{PolicyVersion: p.Version, CapabilityConfig: &p.CapabilityConfig, Targets: []service.DetectorTargetSpec{{Source: service.MonitorSourcePlatformGroup, GroupID: &p.GroupID, AccountID: &account, Tier: "low", Target: config.Targets[0]}}, Benchmarks: []service.DetectorBenchmarkManifest{manifest}}
	limits := []MonitorBudgetLimit{{Scope: "global", RequestLimit: 10}, {Scope: "group", ScopeID: 1, RequestLimit: 10}}
	id, err := queue.EnqueuePlatformCapability(ctx, *p, snapshot, 3, limits)
	require.NoError(t, err)
	_, err = queue.EnqueuePlatformCapability(ctx, *p, snapshot, 3, limits)
	require.Error(t, err)
	lease, err := queue.ClaimManagedCapability(ctx, "worker", true, false)
	require.NoError(t, err)
	require.Equal(t, id, lease.JobID)
	_, err = registry.Activate(ctx, "auto", manifest.ReleaseID, channelRevision, 1)
	require.NoError(t, err)
	_, err = queue.Renew(ctx, *lease)
	require.NoError(t, err)
	p, err = policies.GetPolicy(ctx, p.ID)
	require.NoError(t, err)
	require.EqualValues(t, 2, p.Version)
	p.DisplayName = "Renamed"
	renamed, err := policies.SavePolicy(ctx, *p, p.Version, 1)
	require.NoError(t, err)
	_, err = queue.Renew(ctx, *lease)
	require.NoError(t, err)
	renamed.CapabilityConfig.Tier = "high"
	changed, err := policies.SavePolicy(ctx, *renamed, renamed.Version, 1)
	require.NoError(t, err)
	_, err = queue.Renew(ctx, *lease)
	require.Error(t, err)
	recovered, err := queue.RecoverNext(ctx)
	require.NoError(t, err)
	require.True(t, recovered)
	monitorBudgetTestBucket(t, db, "global", 0, 0, 0)
	var active *string
	require.NoError(t, db.QueryRow(`SELECT active_capability_job_id FROM channel_monitor_group_policies WHERE id=$1`, p.ID).Scan(&active))
	require.Nil(t, active)
	require.NoError(t, policies.DeletePolicy(ctx, changed.ID, changed.Version, 1))
	_, err = policies.GetPolicy(ctx, changed.ID)
	require.ErrorIs(t, err, service.ErrMonitorPolicyNotFound)
}

//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestDetectorPlanCreationPostgres(t *testing.T) {
	db := monitorBudgetTestDB(t)
	ctx := context.Background()
	owner := int64(1)
	key := int64(1)
	raw := monitorTestApprovedManifest(t, db, owner)
	var manifest service.DetectorBenchmarkManifest
	require.NoError(t, json.Unmarshal([]byte(raw), &manifest))
	repo := NewLLMDetectorRepository(db)
	p := &service.DetectorPlan{OwnerUserID: &owner, Source: service.MonitorSourceSiteAPIKey, TargetSpec: service.DetectorTargetSpec{Source: service.MonitorSourceSiteAPIKey, SiteAPIKeyID: &key}, Benchmark: manifest, ConfigurationHash: strings.Repeat("a", 64), PlannedBaseRequests: 3, MaximumOutboundRequests: 3}
	require.NoError(t, repo.SaveDetectorPlan(ctx, p))
	stored, err := repo.GetPlanForOwner(ctx, p.ID, owner)
	require.NoError(t, err)
	require.EqualValues(t, 3, stored.MaximumOutboundRequests)
	_, err = repo.GetPlanForOwner(ctx, p.ID, 2)
	require.Error(t, err)
	jobs := NewMonitorJobRepository(db)
	input := CreateMonitorJobInput{PlanID: p.ID, OwnerID: owner, ConfigurationHash: p.ConfigurationHash, IdempotencyKey: "task-entry", Limits: monitorBudgetTestLimits(1, 100, 100)}
	id, err := jobs.CreateFromPlan(ctx, input)
	require.NoError(t, err)
	same, err := jobs.CreateFromPlan(ctx, input)
	require.NoError(t, err)
	require.Equal(t, id, same)
	require.NoError(t, jobs.CancelForOwner(ctx, id, owner))
	job, err := repo.GetJobForOwner(ctx, id, owner)
	require.NoError(t, err)
	require.Equal(t, service.MonitorJobCancelled, job.State)
}

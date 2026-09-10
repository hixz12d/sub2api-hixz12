//go:build integration && meow

package repository

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type benchmarkTestAdminUsers struct{ service.UserRepository }

func (benchmarkTestAdminUsers) GetByID(_ context.Context, id int64) (*service.User, error) {
	role := service.RoleUser
	if id == 1 {
		role = service.RoleAdmin
	}
	return &service.User{ID: id, Role: role, Status: service.StatusActive}, nil
}

func TestBenchmarkRealAdapterApprovalPostgres(t *testing.T) {
	ctx := context.Background()
	db := monitorBudgetTestDB(t)
	root, err := filepath.Abs("../../..")
	require.NoError(t, err)
	python := os.Getenv("MEOW_TEST_PYTHON")
	engine := os.Getenv("MEOW_ENGINE_ROOT")
	require.NotEmpty(t, python, "MEOW_TEST_PYTHON must name the isolated Python interpreter")
	require.NotEmpty(t, engine, "MEOW_ENGINE_ROOT required; this test cannot silently skip")
	adapter := filepath.Join(root, "workers", "capability", "meow_adapter.py")
	raw, err := os.ReadFile(adapter)
	require.NoError(t, err)
	cfg := &config.Config{LLMDetectorEngineAllowed: true, LLMDetectorPython: python, LLMDetectorAdapter: adapter, LLMDetectorEngineRoot: engine, LLMDetectorAdapterSHA256: benchmarkDigest(raw)}
	store := NewBenchmarkRegistryRepository(db)
	svc := service.ProvideBenchmarkService(cfg, store, benchmarkTestAdminUsers{})
	lock, err := os.ReadFile(filepath.Join(root, "workers", "capability", "meow.lock.json"))
	require.NoError(t, err)
	var metadata struct {
		Benchmarks []struct {
			ID, Version, Mode, Path, SHA256 string
			ContentSHA256                   string `json:"content_sha256"`
		}
	}
	require.NoError(t, json.Unmarshal(lock, &metadata))
	for _, b := range metadata.Benchmarks {
		t.Run(b.ID, func(t *testing.T) {
			payload, err := os.ReadFile(filepath.Join(engine, filepath.FromSlash(b.Path)))
			require.NoError(t, err)
			input := BenchmarkPackageInput{ID: b.ID, Version: b.Version, Mode: b.Mode, SHA256: b.SHA256, ContentSHA256: b.ContentSHA256, Payload: payload}
			_, err = svc.Stage(ctx, 2, input)
			require.ErrorIs(t, err, service.ErrBenchmarkAdmin)
			id, err := svc.Stage(ctx, 1, input)
			require.NoError(t, err)
			_, err = svc.Activate(ctx, 1, b.ID, id, 0)
			require.ErrorIs(t, err, ErrBenchmarkRelease)
			require.NoError(t, svc.Approve(ctx, 1, id))
			release, err := store.Get(ctx, id)
			require.NoError(t, err)
			require.Equal(t, "approved", release.State)
			require.Contains(t, string(release.Receipt), cfg.LLMDetectorAdapterSHA256)
			var packageModels struct {
				Models []struct {
					ID string `json:"id"`
				} `json:"models"`
			}
			require.NoError(t, json.Unmarshal(payload, &packageModels))
			claimed := ""
			for _, model := range packageModels.Models {
				if model.ID != "reference-only:other" {
					claimed = model.ID
					break
				}
			}
			require.NotEmpty(t, claimed)
			target := service.DetectorTargetSpec{Tier: "low", Target: service.DetectorTarget{RequestModel: claimed, ClaimedModel: claimed}}
			engineBridge := service.NewPinnedCapabilityEngine(cfg)
			plan, err := engineBridge.Plan(ctx, release, target)
			require.NoError(t, err)
			require.Greater(t, plan.PlannedSamples, 0)
			report, err := engineBridge.Score(ctx, release, target, plan.ContractHash, service.DetectorContractExact, nil)
			require.NoError(t, err)
			require.Equal(t, service.DetectorInsufficient, report.Verdict)
			_, err = engineBridge.Score(ctx, release, target, "invalid", service.DetectorContractExact, nil)
			require.Error(t, err)
			revision, err := svc.Activate(ctx, 1, b.ID, id, 0)
			require.NoError(t, err)
			require.EqualValues(t, 1, revision)
			_, err = svc.Activate(ctx, 1, b.ID, id, 0)
			require.ErrorIs(t, err, ErrBenchmarkRelease)
			require.NoError(t, svc.Withdraw(ctx, 1, id, "offline acceptance"))
			require.ErrorIs(t, svc.Approve(ctx, 1, id), ErrBenchmarkRelease)
		})
	}
	items, channels, err := svc.List(ctx, 1, 50, 0)
	require.NoError(t, err)
	require.Len(t, items, len(metadata.Benchmarks))
	require.Len(t, channels, len(metadata.Benchmarks))
	for _, c := range channels {
		require.Equal(t, "withdrawn", c.State)
	}
}

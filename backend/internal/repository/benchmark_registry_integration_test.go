//go:build integration

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestBenchmarkRegistryPostgres(t *testing.T) {
	ctx := context.Background()
	db, client := testMonitorDatabase(t)
	actor := mustCreateUser(t, client, &service.User{Email: "registry-" + uuid.NewString() + "@example.com"})
	r := NewBenchmarkRegistryRepository(db)
	input := BenchmarkPackageInput{ID: "fixture-" + uuid.NewString(), Version: "1", Mode: "gpt", Payload: []byte(`{"fixture":true}`), ContentSHA256: strings.Repeat("a", 64), EngineLock: json.RawMessage(`{"schema_version":1,"fixture":true}`)}
	input.SHA256 = benchmarkDigest(input.Payload)
	id, err := r.Stage(ctx, input, actor.ID)
	require.NoError(t, err)
	again, err := r.Stage(ctx, input, actor.ID)
	require.NoError(t, err)
	require.Equal(t, id, again)
	release, err := r.Get(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "candidate", release.State)
	require.Equal(t, []byte(input.EngineLock), []byte(release.EngineLock))
	channel := "stable-" + uuid.NewString()
	_, err = r.Activate(ctx, channel, id, 0, actor.ID)
	require.ErrorIs(t, err, ErrBenchmarkRelease)
	changed := input
	changed.Payload = []byte(`{"fixture":false}`)
	changed.SHA256 = benchmarkDigest(changed.Payload)
	_, err = r.Stage(ctx, changed, actor.ID)
	require.ErrorIs(t, err, ErrBenchmarkRelease)
	receipt := func(p BenchmarkPackageInput) json.RawMessage {
		raw, e := json.Marshal(map[string]any{"benchmark_sha256": p.SHA256, "engine_lock_sha256": benchmarkDigest(p.EngineLock), "adapter_sha256": strings.Repeat("b", 64), "calibration_valid": true})
		require.NoError(t, e)
		return raw
	}
	require.NoError(t, r.ApproveValidated(ctx, id, input.SHA256, benchmarkDigest(input.EngineLock), receipt(input), actor.ID))
	next := input
	next.Version = "2"
	nextID, err := r.Stage(ctx, next, actor.ID)
	require.NoError(t, err)
	require.NoError(t, r.ApproveValidated(ctx, nextID, next.SHA256, benchmarkDigest(next.EngineLock), receipt(next), actor.ID))
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, candidate := range []string{id, nextID} {
		wg.Add(1)
		go func(candidate string) {
			defer wg.Done()
			_, err := r.Activate(ctx, channel, candidate, 0, actor.ID)
			errs <- err
		}(candidate)
	}
	wg.Wait()
	close(errs)
	success, conflict := 0, 0
	for err := range errs {
		if err == nil {
			success++
		} else {
			require.ErrorIs(t, err, ErrBenchmarkRelease)
			conflict++
		}
	}
	require.Equal(t, 1, success)
	require.Equal(t, 1, conflict)
	frozen, revision, err := r.Freeze(ctx, channel)
	require.NoError(t, err)
	require.EqualValues(t, 1, revision)
	rev, err := r.Activate(ctx, channel, nextID, revision, actor.ID)
	require.NoError(t, err)
	require.EqualValues(t, 2, rev)
	old, err := r.Get(ctx, frozen)
	require.NoError(t, err)
	require.Equal(t, "approved", old.State)
	// Audit FK failure must roll back the channel update as well.
	_, err = r.Activate(ctx, channel, id, 2, 9223372036854775807)
	require.Error(t, err)
	_, revision, err = r.Freeze(ctx, channel)
	require.NoError(t, err)
	require.EqualValues(t, 2, revision)
	require.NoError(t, r.Withdraw(ctx, nextID, "invalid calibration", actor.ID))
	require.NoError(t, r.Withdraw(ctx, nextID, "invalid calibration", actor.ID))
	_, _, err = r.Freeze(ctx, channel)
	require.ErrorIs(t, err, sql.ErrNoRows)
	require.ErrorIs(t, r.ApproveValidated(ctx, nextID, next.SHA256, benchmarkDigest(next.EngineLock), receipt(next), actor.ID), ErrBenchmarkRelease)
	_, err = r.Activate(ctx, channel, id, 0, actor.ID)
	require.ErrorIs(t, err, ErrBenchmarkRelease)
	rev, err = r.Activate(ctx, channel, id, 2, actor.ID)
	require.NoError(t, err)
	require.EqualValues(t, 3, rev)
	withdrawn, err := r.Get(ctx, nextID)
	require.NoError(t, err)
	require.Equal(t, "withdrawn", withdrawn.State)
	require.NotEmpty(t, withdrawn.Receipt)
	require.Equal(t, input.Payload, withdrawn.Payload)
	var audits int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM monitor_benchmark_audit WHERE release_id=$1 AND operation='withdraw'`, nextID).Scan(&audits))
	require.Equal(t, 1, audits)
}

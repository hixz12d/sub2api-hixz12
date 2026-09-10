//go:build unit

package service

import (
	"context"
	"database/sql"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type monitorRuntimeTestFlags struct{ value atomic.Value }

func (f *monitorRuntimeTestFlags) GetMonitorFeatureFlags(context.Context) MonitorFeatureFlags {
	return f.value.Load().(MonitorFeatureFlags)
}

type monitorRuntimeTestExecutor struct {
	execute func(context.Context, MonitorJobLease) error
}

func (e monitorRuntimeTestExecutor) Ready() bool { return true }
func (e monitorRuntimeTestExecutor) Execute(ctx context.Context, l MonitorJobLease) error {
	return e.execute(ctx, l)
}

type monitorRuntimeTestStore struct {
	claim             func(context.Context, MonitorJobKind, string, bool, bool) (*MonitorJobLease, error)
	renew             func(context.Context, MonitorJobLease) (time.Time, error)
	finished          chan MonitorJobState
	enqueued, claimed atomic.Int64
}

func (s *monitorRuntimeTestStore) ListDueProbePolicyIDs(context.Context, int) ([]int64, error) {
	return []int64{1}, nil
}
func (s *monitorRuntimeTestStore) EnqueueProbe(context.Context, int64, int64) (string, error) {
	s.enqueued.Add(1)
	return "job", nil
}
func (s *monitorRuntimeTestStore) ClaimEligible(ctx context.Context, k MonitorJobKind, i string, p, u bool) (*MonitorJobLease, error) {
	s.claimed.Add(1)
	if s.claim != nil {
		return s.claim(ctx, k, i, p, u)
	}
	return nil, sql.ErrNoRows
}
func (s *monitorRuntimeTestStore) Renew(ctx context.Context, l MonitorJobLease) (time.Time, error) {
	if s.renew != nil {
		return s.renew(ctx, l)
	}
	return time.Now().Add(time.Minute), nil
}
func (s *monitorRuntimeTestStore) Finish(_ context.Context, _ MonitorJobLease, state MonitorJobState) error {
	if s.finished != nil {
		s.finished <- state
	}
	return nil
}
func (s *monitorRuntimeTestStore) RecoverNext(context.Context) (bool, error) { return false, nil }
func (s *monitorRuntimeTestStore) CancelDisabledNext(context.Context, bool, bool, bool) (bool, error) {
	return false, nil
}

func monitorRuntimeTestSetup(t *testing.T, store *monitorRuntimeTestStore, probe, capability MonitorJobExecutor) (*MonitorJobRuntime, *monitorRuntimeTestFlags) {
	t.Helper()
	flags := &monitorRuntimeTestFlags{}
	flags.value.Store(MonitorFeatureFlags{})
	options := DefaultMonitorJobRuntimeOptions("fixture")
	options.ProbeWorkers = 1
	options.CapabilityWorkers = 1
	options.PollInterval = 5 * time.Millisecond
	options.HeartbeatInterval = 5 * time.Millisecond
	runtime, err := NewMonitorJobRuntime(store, flags, probe, capability, options)
	require.NoError(t, err)
	t.Cleanup(runtime.Stop)
	return runtime, flags
}

func TestMonitorJobRuntimeClosedAndMissingExecutor(t *testing.T) {
	store := &monitorRuntimeTestStore{}
	runtime, flags := monitorRuntimeTestSetup(t, store, nil, nil)
	runtime.scheduleOnce()
	require.False(t, runtime.runOne(MonitorJobAvailability))
	require.Zero(t, store.claimed.Load())
	require.Zero(t, store.enqueued.Load())
	flags.value.Store(MonitorFeatureFlags{ChannelMonitorEnabled: true, ChannelMonitorV2: true, GroupProbeEnabled: true, DetectorEnabled: true, EngineAllowed: true, UserTestingEnabled: true, ScheduledEnabled: true})
	runtime.scheduleOnce()
	require.False(t, runtime.runOne(MonitorJobAvailability))
	require.False(t, runtime.runOne(MonitorJobCapability))
	require.Zero(t, store.claimed.Load())
	require.Zero(t, store.enqueued.Load())
	runtime.Stop()
	runtime.Start()
	require.Zero(t, store.claimed.Load())
}

func TestMonitorJobRuntimeSourceAdmission(t *testing.T) {
	store := &monitorRuntimeTestStore{}
	executor := monitorRuntimeTestExecutor{execute: func(context.Context, MonitorJobLease) error { return nil }}
	runtime, flags := monitorRuntimeTestSetup(t, store, executor, executor)
	flags.value.Store(MonitorFeatureFlags{DetectorEnabled: true, EngineAllowed: true, UserTestingEnabled: true})
	store.claim = func(_ context.Context, kind MonitorJobKind, _ string, platform, private bool) (*MonitorJobLease, error) {
		require.Equal(t, MonitorJobCapability, kind)
		require.False(t, platform)
		require.True(t, private)
		return nil, sql.ErrNoRows
	}
	runtime.runOne(MonitorJobCapability)
	flags.value.Store(MonitorFeatureFlags{ChannelMonitorEnabled: true, ChannelMonitorV2: true, DetectorEnabled: true, EngineAllowed: true, ScheduledEnabled: true})
	store.claim = func(_ context.Context, _ MonitorJobKind, _ string, platform, private bool) (*MonitorJobLease, error) {
		require.True(t, platform)
		require.False(t, private)
		return nil, sql.ErrNoRows
	}
	runtime.runOne(MonitorJobCapability)
}

func TestMonitorJobRuntimeCancellation(t *testing.T) {
	for _, reason := range []string{"lease_lost", "feature_disabled", "shutdown"} {
		t.Run(reason, func(t *testing.T) {
			entered := make(chan struct{})
			store := &monitorRuntimeTestStore{finished: make(chan MonitorJobState, 2)}
			var claimed atomic.Bool
			store.claim = func(_ context.Context, kind MonitorJobKind, instance string, _, _ bool) (*MonitorJobLease, error) {
				if kind != MonitorJobAvailability || !claimed.CompareAndSwap(false, true) {
					return nil, sql.ErrNoRows
				}
				return &MonitorJobLease{JobID: "fixture", InstanceID: instance, Generation: 1, Kind: kind, Source: MonitorSourcePlatformGroup}, nil
			}
			if reason == "lease_lost" {
				store.renew = func(context.Context, MonitorJobLease) (time.Time, error) {
					return time.Time{}, errors.New("lost lease")
				}
			}
			executor := monitorRuntimeTestExecutor{execute: func(ctx context.Context, _ MonitorJobLease) error { close(entered); <-ctx.Done(); return ctx.Err() }}
			runtime, flags := monitorRuntimeTestSetup(t, store, executor, nil)
			flags.value.Store(MonitorFeatureFlags{ChannelMonitorEnabled: true, ChannelMonitorV2: true, GroupProbeEnabled: true})
			runtime.Start()
			runtime.Start()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("executor did not start")
			}
			if reason == "feature_disabled" {
				flags.value.Store(MonitorFeatureFlags{})
			}
			if reason == "shutdown" {
				runtime.Stop()
			}
			select {
			case state := <-store.finished:
				require.Equal(t, MonitorJobCancelled, state)
			case <-time.After(time.Second):
				t.Fatal("executor was not cancelled")
			}
			runtime.Stop()
		})
	}
}

func TestMonitorJobRuntimeIndependentPools(t *testing.T) {
	started := make(chan MonitorJobKind, 2)
	store := &monitorRuntimeTestStore{finished: make(chan MonitorJobState, 2)}
	var probeClaimed, capabilityClaimed atomic.Bool
	store.claim = func(_ context.Context, kind MonitorJobKind, instance string, _, _ bool) (*MonitorJobLease, error) {
		marker := &probeClaimed
		if kind == MonitorJobCapability {
			marker = &capabilityClaimed
		}
		if !marker.CompareAndSwap(false, true) {
			return nil, sql.ErrNoRows
		}
		return &MonitorJobLease{JobID: string(kind), InstanceID: instance, Generation: 1, Kind: kind, Source: MonitorSourcePlatformGroup}, nil
	}
	executor := monitorRuntimeTestExecutor{execute: func(ctx context.Context, lease MonitorJobLease) error {
		started <- lease.Kind
		<-ctx.Done()
		return ctx.Err()
	}}
	runtime, flags := monitorRuntimeTestSetup(t, store, executor, executor)
	flags.value.Store(MonitorFeatureFlags{ChannelMonitorEnabled: true, ChannelMonitorV2: true, GroupProbeEnabled: true, DetectorEnabled: true, EngineAllowed: true, ScheduledEnabled: true})
	runtime.Start()
	seen := map[MonitorJobKind]bool{}
	for i := 0; i < 2; i++ {
		select {
		case kind := <-started:
			seen[kind] = true
		case <-time.After(time.Second):
			t.Fatal("one pool blocked the other")
		}
	}
	require.True(t, seen[MonitorJobAvailability])
	require.True(t, seen[MonitorJobCapability])
	runtime.Stop()
}

func TestMonitorJobRuntimeExecutorPanic(t *testing.T) {
	store := &monitorRuntimeTestStore{finished: make(chan MonitorJobState, 1)}
	runtime, flags := monitorRuntimeTestSetup(t, store, nil, nil)
	flags.value.Store(MonitorFeatureFlags{ChannelMonitorEnabled: true, ChannelMonitorV2: true, GroupProbeEnabled: true})
	executor := monitorRuntimeTestExecutor{execute: func(context.Context, MonitorJobLease) error { panic("sensitive fixture must not appear in logs") }}
	runtime.execute(MonitorJobLease{Kind: MonitorJobAvailability, Source: MonitorSourcePlatformGroup}, executor)
	require.Equal(t, MonitorJobFailed, <-store.finished)
}

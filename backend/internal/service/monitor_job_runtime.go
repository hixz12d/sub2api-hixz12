package service

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"sync"
	"time"
)

type MonitorRuntimeFlags interface {
	GetMonitorFeatureFlags(context.Context) MonitorFeatureFlags
}

type MonitorJobRuntimeOptions struct {
	InstanceID        string
	ProbeWorkers      int
	CapabilityWorkers int
	PollInterval      time.Duration
	HeartbeatInterval time.Duration
	DatabaseTimeout   time.Duration
	GlobalDailyLimit  int64
}

func DefaultMonitorJobRuntimeOptions(instance string) MonitorJobRuntimeOptions {
	return MonitorJobRuntimeOptions{InstanceID: instance, ProbeWorkers: 4, CapabilityWorkers: 2, PollInterval: 5 * time.Second,
		HeartbeatInterval: 15 * time.Second, DatabaseTimeout: 5 * time.Second, GlobalDailyLimit: 10000}
}

// MonitorJobRuntime runs bounded, independent pools over the persistent queue.
// Constructors do not start work; missing flags or executor readiness fail closed.
type MonitorJobRuntime struct {
	store             MonitorJobStore
	flags             MonitorRuntimeFlags
	probe, capability MonitorJobExecutor
	options           MonitorJobRuntimeOptions
	ctx               context.Context
	cancel            context.CancelFunc
	mu                sync.Mutex
	started, stopped  bool
	wg                sync.WaitGroup
}

func NewMonitorJobRuntime(store MonitorJobStore, flags MonitorRuntimeFlags, probe, capability MonitorJobExecutor, options MonitorJobRuntimeOptions) (*MonitorJobRuntime, error) {
	if store == nil || flags == nil || options.InstanceID == "" || len(options.InstanceID) > 200 || options.ProbeWorkers < 1 || options.ProbeWorkers > 8 ||
		options.CapabilityWorkers < 1 || options.CapabilityWorkers > 2 || options.PollInterval <= 0 || options.HeartbeatInterval <= 0 || options.HeartbeatInterval > 30*time.Second ||
		options.DatabaseTimeout <= 0 || options.DatabaseTimeout > 15*time.Second || options.GlobalDailyLimit < 1 || options.GlobalDailyLimit > 1000000000 {
		return nil, errors.New("invalid monitor runtime configuration")
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &MonitorJobRuntime{store: store, flags: flags, probe: probe, capability: capability, options: options, ctx: ctx, cancel: cancel}, nil
}

func (r *MonitorJobRuntime) Start() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started || r.stopped {
		return
	}
	r.started = true
	r.wg.Add(2 + r.options.ProbeWorkers + r.options.CapabilityWorkers)
	go r.scheduleLoop()
	go r.platformScheduleLoop()
	for i := 0; i < r.options.ProbeWorkers; i++ {
		go r.workerLoop(MonitorJobAvailability)
	}
	for i := 0; i < r.options.CapabilityWorkers; i++ {
		go r.workerLoop(MonitorJobCapability)
	}
}

func (r *MonitorJobRuntime) Stop() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.stopped = true
	r.cancel()
	r.mu.Unlock()
	r.wg.Wait()
}

func (r *MonitorJobRuntime) readFlags(ctx context.Context) MonitorFeatureFlags {
	readCtx, cancel := context.WithTimeout(ctx, r.options.DatabaseTimeout)
	defer cancel()
	return r.flags.GetMonitorFeatureFlags(readCtx)
}

func (r *MonitorJobRuntime) wait() bool {
	timer := time.NewTimer(r.options.PollInterval)
	defer timer.Stop()
	select {
	case <-r.ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (r *MonitorJobRuntime) scheduleLoop() {
	defer r.wg.Done()
	for r.ctx.Err() == nil {
		r.scheduleOnce()
		if !r.wait() {
			return
		}
	}
}

func (r *MonitorJobRuntime) scheduleOnce() {
	flags := r.readFlags(r.ctx)
	for i := 0; i < 100 && r.ctx.Err() == nil; i++ {
		ctx, cancel := context.WithTimeout(r.ctx, r.options.DatabaseTimeout)
		changed, err := r.store.CancelDisabledNext(ctx, flags.GroupProbeAllowed(), flags.ScheduledDetectionAllowed(true), flags.UserTestingAllowed(true))
		cancel()
		if err != nil {
			r.warn("cancel_disabled_failed", "")
			break
		}
		if !changed {
			break
		}
	}
	for i := 0; i < 100 && r.ctx.Err() == nil; i++ {
		ctx, cancel := context.WithTimeout(r.ctx, r.options.DatabaseTimeout)
		changed, err := r.store.RecoverNext(ctx)
		cancel()
		if err != nil {
			r.warn("recovery_failed", "")
			break
		}
		if !changed {
			break
		}
	}
	if !flags.GroupProbeAllowed() || r.probe == nil || !r.probe.Ready() || r.ctx.Err() != nil {
		return
	}
	ctx, cancel := context.WithTimeout(r.ctx, r.options.DatabaseTimeout)
	ids, err := r.store.ListDueProbePolicyIDs(ctx, 100)
	cancel()
	if err != nil {
		r.warn("schedule_list_failed", "")
		return
	}
	for _, id := range ids {
		if r.ctx.Err() != nil || !r.readFlags(r.ctx).GroupProbeAllowed() || !r.probe.Ready() {
			return
		}
		ctx, cancel := context.WithTimeout(r.ctx, r.options.DatabaseTimeout)
		_, err := r.store.EnqueueProbe(ctx, id, r.options.GlobalDailyLimit)
		cancel()
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			r.warn("schedule_enqueue_failed", "")
		}
	}
}

func (r *MonitorJobRuntime) workerLoop(kind MonitorJobKind) {
	defer r.wg.Done()
	for r.ctx.Err() == nil {
		if !r.runOne(kind) && !r.wait() {
			return
		}
	}
}

func (r *MonitorJobRuntime) admission(flags MonitorFeatureFlags, kind MonitorJobKind) (MonitorJobExecutor, bool, bool) {
	if kind == MonitorJobAvailability {
		ready := r.probe != nil && r.probe.Ready()
		return r.probe, ready && flags.GroupProbeAllowed(), false
	}
	ready := r.capability != nil && r.capability.Ready()
	platform := flags.ScheduledDetectionAllowed(ready)
	if restricted, ok := r.capability.(interface{ AllowsPlatformJobs() bool }); ok {
		platform = platform && restricted.AllowsPlatformJobs()
	}
	return r.capability, platform, flags.UserTestingAllowed(ready)
}

func (r *MonitorJobRuntime) runOne(kind MonitorJobKind) bool {
	executor, platform, private := r.admission(r.readFlags(r.ctx), kind)
	if !platform && !private {
		return false
	}
	ctx, cancel := context.WithTimeout(r.ctx, r.options.DatabaseTimeout)
	var lease *MonitorJobLease
	var err error
	if managed, ok := executor.(interface{ ManagedCapabilitiesOnly() bool }); ok && managed.ManagedCapabilitiesOnly() {
		store, supported := r.store.(interface {
			ClaimManagedCapability(context.Context, string, bool, bool) (*MonitorJobLease, error)
		})
		if !supported {
			cancel()
			return false
		}
		lease, err = store.ClaimManagedCapability(ctx, r.options.InstanceID, platform, private)
	} else if restricted, ok := executor.(interface{ SiteJobsOnly() bool }); ok && restricted.SiteJobsOnly() {
		store, supported := r.store.(interface {
			ClaimSiteCapability(context.Context, string) (*MonitorJobLease, error)
		})
		if !supported {
			cancel()
			return false
		}
		lease, err = store.ClaimSiteCapability(ctx, r.options.InstanceID)
	} else {
		lease, err = r.store.ClaimEligible(ctx, kind, r.options.InstanceID, platform, private)
	}
	cancel()
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			r.warn("claim_failed", "")
		}
		return false
	}
	if lease == nil {
		return false
	}
	r.execute(*lease, executor)
	return true
}

func (r *MonitorJobRuntime) execute(lease MonitorJobLease, executor MonitorJobExecutor) {
	ctx, cancel := context.WithTimeout(r.ctx, 2*time.Hour)
	defer cancel()
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(r.options.HeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, platform, private := r.admission(r.readFlags(ctx), lease.Kind)
				allowed := private
				if lease.Source == MonitorSourcePlatformGroup {
					allowed = platform
				}
				if !allowed {
					cancel()
					return
				}
				renewCtx, renewCancel := context.WithTimeout(ctx, r.options.DatabaseTimeout)
				_, err := r.store.Renew(renewCtx, lease)
				renewCancel()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()
	err := runMonitorExecutor(ctx, executor, lease)
	cancelled := ctx.Err() != nil
	cancel()
	<-heartbeatDone
	state := MonitorJobCompleted
	if err != nil {
		state = MonitorJobFailed
	}
	if cancelled {
		state = MonitorJobCancelled
	}
	// Shutdown cancels transport, but terminal settlement gets its own short window.
	finishCtx, finishCancel := context.WithTimeout(context.Background(), r.options.DatabaseTimeout)
	defer finishCancel()
	if err := r.store.Finish(finishCtx, lease, state); err != nil {
		r.warn("finish_failed", lease.JobID)
	}
}

func runMonitorExecutor(ctx context.Context, executor MonitorJobExecutor, lease MonitorJobLease) (err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("monitor executor panic")
		}
	}()
	if err := ctx.Err(); err != nil {
		return err
	}
	return executor.Execute(ctx, lease)
}

func (r *MonitorJobRuntime) warn(code, jobID string) {
	if r.ctx.Err() == nil {
		slog.Warn("monitor runtime operation failed", "code", code, "job_id", jobID)
	}
}

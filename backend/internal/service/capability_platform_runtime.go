package service

import (
	"context"
	"time"
)

// Offline planning cannot block the independent cancellation/recovery loop.
func (r *MonitorJobRuntime) platformScheduleLoop() {
	defer r.wg.Done()
	scheduler, ok := r.capability.(interface{ Schedule(context.Context) error })
	if !ok {
		return
	}
	for r.ctx.Err() == nil {
		if _, platform, _ := r.admission(r.readFlags(r.ctx), MonitorJobCapability); platform {
			ctx, cancel := context.WithTimeout(r.ctx, 90*time.Second)
			if err := scheduler.Schedule(ctx); err != nil {
				r.warn("capability_schedule_failed", "")
			}
			cancel()
		}
		if !r.wait() {
			return
		}
	}
}
func (e *CapabilityExecutor) ManagedCapabilitiesOnly() bool { return true }

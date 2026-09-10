package service

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/google/uuid"
)

func ProvideMonitorProbeExecutor(cfg *config.Config, store MonitorProbeStore, accounts AccountRepository, groups GroupRepository, upstream HTTPUpstream, settings *SettingService, slots *ConcurrencyService) *MonitorProbeExecutor {
	if cfg == nil || !cfg.ChannelMonitorProbeWorkerAllowed {
		return nil
	}
	return NewMonitorProbeExecutor(store, accounts, groups, upstream, settings, slots, 10000)
}

func ProvideMonitorJobRuntime(store MonitorJobStore, settings *SettingService, probe *MonitorProbeExecutor, capability *CapabilityExecutor) (*MonitorJobRuntime, error) {
	options := DefaultMonitorJobRuntimeOptions(uuid.NewString())
	// Missing explicit deployment admission leaves capability execution disabled.
	runtime, err := NewMonitorJobRuntime(store, settings, probe, capability, options)
	if err != nil {
		return nil, err
	}
	runtime.Start()
	return runtime, nil
}

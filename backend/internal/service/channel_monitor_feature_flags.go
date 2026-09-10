package service

// Global feature gates, not authorization. Callers must additionally validate
// policy state, ownership, target admission, budgets and executor health.
type MonitorFeatureFlags struct {
	ChannelMonitorEnabled bool
	ChannelMonitorV2      bool
	GroupViewEnabled      bool
	GroupProbeEnabled     bool
	ShowOutputTPS         bool
	DetectorEnabled       bool
	UserTestingEnabled    bool
	ScheduledEnabled      bool
	EngineAllowed         bool
}

func (f MonitorFeatureFlags) GroupViewAllowed() bool {
	return f.ChannelMonitorEnabled && f.ChannelMonitorV2 && f.GroupViewEnabled
}

func (f MonitorFeatureFlags) GroupProbeAllowed() bool {
	return f.ChannelMonitorEnabled && f.ChannelMonitorV2 && f.GroupProbeEnabled
}

func (f MonitorFeatureFlags) ScheduledDetectionAllowed(executorReady bool) bool {
	return f.ChannelMonitorEnabled && f.ChannelMonitorV2 && f.DetectorEnabled &&
		f.ScheduledEnabled && f.EngineAllowed && executorReady
}

func (f MonitorFeatureFlags) UserTestingAllowed(executorReady bool) bool {
	return f.DetectorEnabled && f.UserTestingEnabled && f.EngineAllowed && executorReady
}

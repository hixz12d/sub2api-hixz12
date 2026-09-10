package service

import "testing"

func TestMonitorFeatureFlagsDefaultClosed(t *testing.T) {
	f := MonitorFeatureFlags{}
	if f.GroupViewAllowed() || f.GroupProbeAllowed() || f.ScheduledDetectionAllowed(true) || f.UserTestingAllowed(true) || f.ShowOutputTPS {
		t.Fatal("zero-value feature flags must be closed")
	}
}

func TestMonitorFeatureFlagsIndependentGates(t *testing.T) {
	f := MonitorFeatureFlags{DetectorEnabled: true, UserTestingEnabled: true, EngineAllowed: true}
	if !f.UserTestingAllowed(true) {
		t.Fatal("private testing must not require monitor page")
	}
	if f.UserTestingAllowed(false) {
		t.Fatal("missing executor must block execution")
	}
	if f.ScheduledDetectionAllowed(true) {
		t.Fatal("schedule must require explicit opt-in")
	}
	f.ChannelMonitorEnabled, f.ChannelMonitorV2, f.ScheduledEnabled = true, true, true
	if !f.ScheduledDetectionAllowed(true) || f.GroupProbeAllowed() {
		t.Fatal("capability must be independent of ordinary probe")
	}
	f.GroupViewEnabled, f.GroupProbeEnabled = true, true
	if !f.GroupViewAllowed() || !f.GroupProbeAllowed() {
		t.Fatal("explicit gates not enabled")
	}
	f.ChannelMonitorV2 = false
	if f.GroupViewAllowed() || f.GroupProbeAllowed() || f.ScheduledDetectionAllowed(true) {
		t.Fatal("new monitor features leaked into V1")
	}
	f.EngineAllowed = false
	if f.UserTestingAllowed(true) {
		t.Fatal("engine admission is required")
	}
}

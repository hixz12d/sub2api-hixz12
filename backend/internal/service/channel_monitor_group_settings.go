package service

import "context"

// Read separately from legacy runtime defaults: a settings failure must never
// inherit V1's historical fail-open behavior for a new paid operation.
func (s *SettingService) GetMonitorFeatureFlags(ctx context.Context) MonitorFeatureFlags {
	if s == nil || s.settingRepo == nil {
		return MonitorFeatureFlags{}
	}
	values, err := s.settingRepo.GetMultiple(ctx, []string{
		SettingKeyChannelMonitorEnabled, SettingKeyChannelMonitorMode,
		SettingKeyChannelMonitorGroupViewEnabled, SettingKeyChannelMonitorGroupProbeEnabled,
		SettingKeyChannelMonitorShowOutputTPS, SettingKeyLLMDetectorEnabled,
		SettingKeyLLMDetectorUserTestingEnabled, SettingKeyLLMDetectorScheduledEnabled,
	})
	if err != nil {
		return MonitorFeatureFlags{}
	}
	return MonitorFeatureFlags{
		ChannelMonitorEnabled: values[SettingKeyChannelMonitorEnabled] == "true",
		ChannelMonitorV2:      values[SettingKeyChannelMonitorMode] == ChannelMonitorModeV2,
		GroupViewEnabled:      values[SettingKeyChannelMonitorGroupViewEnabled] == "true",
		GroupProbeEnabled:     values[SettingKeyChannelMonitorGroupProbeEnabled] == "true",
		ShowOutputTPS:         values[SettingKeyChannelMonitorShowOutputTPS] == "true",
		DetectorEnabled:       values[SettingKeyLLMDetectorEnabled] == "true",
		UserTestingEnabled:    values[SettingKeyLLMDetectorUserTestingEnabled] == "true",
		ScheduledEnabled:      values[SettingKeyLLMDetectorScheduledEnabled] == "true",
		EngineAllowed:         s.cfg != nil && s.cfg.LLMDetectorEngineAllowed,
	}
}

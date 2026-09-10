//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func TestMonitorGroupSettingsFailClosedAndPublicProjection(t *testing.T) {
	ctx := context.Background()
	for _, value := range []string{"", "false", "TRUE", "1", "yes", "true"} {
		repo := &settingPublicRepoStub{values: map[string]string{
			SettingKeyChannelMonitorGroupViewEnabled:  value,
			SettingKeyChannelMonitorGroupProbeEnabled: value,
			SettingKeyChannelMonitorShowOutputTPS:     value,
			SettingKeyLLMDetectorEnabled:              value,
			SettingKeyLLMDetectorUserTestingEnabled:   value,
			SettingKeyLLMDetectorScheduledEnabled:     value,
		}}
		svc := NewSettingService(repo, &config.Config{})
		flags := svc.GetMonitorFeatureFlags(ctx)
		want := value == "true"
		if flags.GroupViewEnabled != want || flags.GroupProbeEnabled != want || flags.ShowOutputTPS != want || flags.DetectorEnabled != want || flags.UserTestingEnabled != want || flags.ScheduledEnabled != want || flags.EngineAllowed {
			t.Fatalf("incorrect opt-in parse %q: %+v", value, flags)
		}
		public, err := svc.GetPublicSettings(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if public.ChannelMonitorGroupViewEnabled != want || public.ChannelMonitorShowOutputTPS != want || public.LLMDetectorEnabled != want || public.LLMDetectorUserTestingEnabled != want {
			t.Fatalf("public flag drift for %q", value)
		}
		payload, err := svc.GetPublicSettingsForInjection(ctx)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		if fields[SettingKeyChannelMonitorGroupViewEnabled] != want || fields[SettingKeyLLMDetectorEnabled] != want {
			t.Fatal("injection flag drift")
		}
		if _, exists := fields["llm_detector_engine_allowed"]; exists {
			t.Fatal("deployment admission leaked publicly")
		}
	}
	repo := &settingPublicRepoStub{err: errors.New("unavailable")}
	svc := NewSettingService(repo, &config.Config{LLMDetectorEngineAllowed: true})
	if svc.GetMonitorFeatureFlags(ctx) != (MonitorFeatureFlags{}) {
		t.Fatal("read failure did not fail closed")
	}
	if (*SettingService)(nil).GetMonitorFeatureFlags(ctx) != (MonitorFeatureFlags{}) {
		t.Fatal("nil service did not fail closed")
	}
}

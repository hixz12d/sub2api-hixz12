//go:build unit

package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMonitorSettingsRequestOptionalFlags(t *testing.T) {
	flags := []struct {
		key string
		get func(UpdateSettingsRequest) *bool
	}{
		{"channel_monitor_group_view_enabled", func(r UpdateSettingsRequest) *bool { return r.ChannelMonitorGroupViewEnabled }},
		{"channel_monitor_group_probe_enabled", func(r UpdateSettingsRequest) *bool { return r.ChannelMonitorGroupProbeEnabled }},
		{"channel_monitor_show_output_tps", func(r UpdateSettingsRequest) *bool { return r.ChannelMonitorShowOutputTPS }},
		{"llm_detector_enabled", func(r UpdateSettingsRequest) *bool { return r.LLMDetectorEnabled }},
		{"llm_detector_user_testing_enabled", func(r UpdateSettingsRequest) *bool { return r.LLMDetectorUserTestingEnabled }},
		{"llm_detector_scheduled_enabled", func(r UpdateSettingsRequest) *bool { return r.LLMDetectorScheduledEnabled }},
	}
	for _, flag := range flags {
		t.Run(flag.key, func(t *testing.T) {
			var omitted UpdateSettingsRequest
			if err := json.Unmarshal([]byte(`{}`), &omitted); err != nil {
				t.Fatal(err)
			}
			if flag.get(omitted) != nil {
				t.Fatal("omitted flag must remain distinguishable from false")
			}
			for _, value := range []string{"true", "false"} {
				var supplied UpdateSettingsRequest
				if err := json.Unmarshal([]byte(`{"`+flag.key+`":`+value+`}`), &supplied); err != nil {
					t.Fatal(err)
				}
				got := flag.get(supplied)
				if got == nil || *got != (value == "true") {
					t.Fatal("explicit flag value was lost")
				}
			}
		})
	}
}

func TestMonitorSettingsRejectDeploymentOverrideBeforeServiceAccess(t *testing.T) {
	for _, value := range []string{"true", "false", "null"} {
		t.Run(value, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", strings.NewReader(`{"llm_detector_engine_allowed":`+value+`}`))
			ctx.Request.Header.Set("Content-Type", "application/json")
			// Nil dependencies ensure rejection happens before any persistence access.
			(&SettingHandler{}).UpdateSettings(ctx)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("unexpected status %d", recorder.Code)
			}
			if !strings.Contains(recorder.Body.String(), "deployment-owned and read-only") {
				t.Fatal("missing read-only rejection")
			}
		})
	}
}

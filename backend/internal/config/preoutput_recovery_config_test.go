package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBlueprintV2PreoutputRecoveryConfigLoad(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		modeEnv     string
		elapsedEnv  string
		highEnv     string
		wantHigh    int
		wantMode    string
		wantElapsed int
		wantError   string
	}{
		{name: "defaults", wantMode: "legacy"},
		{name: "high_env", modeEnv: "bounded_preoutput", highEnv: "260", wantHigh: 260, wantMode: "bounded_preoutput"},
		{name: "high_yaml_env_override", yaml: "gateway:\n  openai_preoutput_recovery_mode: bounded_preoutput\n  openai_preoutput_recovery_high_effort_max_elapsed_seconds: 180\n", highEnv: "260", wantHigh: 260, wantMode: "bounded_preoutput"},
		{name: "invalid_high", modeEnv: "bounded_preoutput", highEnv: "3601", wantError: "gateway.openai_preoutput_recovery_high_effort_max_elapsed_seconds"},
		{name: "legacy_total_override", elapsedEnv: "110", wantError: "must be bounded_preoutput"},
		{name: "legacy_high_override", highEnv: "260", wantError: "must be bounded_preoutput"},
		{name: "env_only", modeEnv: "bounded_preoutput", elapsedEnv: "150", wantMode: "bounded_preoutput", wantElapsed: 150},
		{name: "yaml", yaml: "gateway:\n  openai_preoutput_recovery_mode: bounded_preoutput\n  openai_preoutput_recovery_max_elapsed_seconds: 120\n", wantMode: "bounded_preoutput", wantElapsed: 120},
		{name: "env_overrides_yaml", yaml: "gateway:\n  openai_preoutput_recovery_mode: legacy\n  openai_preoutput_recovery_max_elapsed_seconds: 0\n", modeEnv: "bounded_preoutput", elapsedEnv: "180", wantMode: "bounded_preoutput", wantElapsed: 180},
		{name: "old_config", yaml: "gateway:\n  openai_first_output_timeout_seconds: 45\n", wantMode: "legacy"},
		{name: "explicit_empty_mode", yaml: "gateway:\n  openai_preoutput_recovery_mode: \"\"\n", wantMode: ""},
		{name: "normalized_policy_input", modeEnv: " BOUNDED_PREOUTPUT ", wantMode: " BOUNDED_PREOUTPUT "},
		{name: "minimum", modeEnv: "bounded_preoutput", elapsedEnv: "30", wantMode: "bounded_preoutput", wantElapsed: 30},
		{name: "maximum", modeEnv: "bounded_preoutput", elapsedEnv: "3600", wantMode: "bounded_preoutput", wantElapsed: 3600},
		{name: "invalid_mode", modeEnv: "unlimited", wantError: "gateway.openai_preoutput_recovery_mode"},
		{name: "negative_elapsed", elapsedEnv: "-1", wantError: "gateway.openai_preoutput_recovery_max_elapsed_seconds"},
		{name: "below_minimum", elapsedEnv: "29", wantError: "gateway.openai_preoutput_recovery_max_elapsed_seconds"},
		{name: "above_maximum", elapsedEnv: "3601", wantError: "gateway.openai_preoutput_recovery_max_elapsed_seconds"},
		{name: "non_integer", elapsedEnv: "invalid", wantError: "unmarshal config error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetViperWithJWTSecret(t)
			t.Setenv("GATEWAY_OPENAI_PREOUTPUT_RECOVERY_MODE", tt.modeEnv)
			t.Setenv("GATEWAY_OPENAI_PREOUTPUT_RECOVERY_MAX_ELAPSED_SECONDS", tt.elapsedEnv)
			t.Setenv("GATEWAY_OPENAI_PREOUTPUT_RECOVERY_HIGH_EFFORT_MAX_ELAPSED_SECONDS", tt.highEnv)
			if tt.yaml != "" {
				path := filepath.Join(t.TempDir(), "config.yaml")
				require.NoError(t, os.WriteFile(path, []byte(tt.yaml), 0o600))
				t.Setenv("CONFIG_FILE", path)
			}
			cfg, err := Load()
			if tt.wantError != "" {
				require.ErrorContains(t, err, tt.wantError)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantMode, cfg.Gateway.OpenAIPreoutputRecoveryMode)
			require.Equal(t, tt.wantElapsed, cfg.Gateway.OpenAIPreoutputRecoveryMaxElapsedSeconds)
			require.Equal(t, tt.wantHigh, cfg.Gateway.OpenAIPreoutputRecoveryHighEffortMaxElapsedSeconds)
		})
	}
}

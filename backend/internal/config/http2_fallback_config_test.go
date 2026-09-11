package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadOpenAIHTTP2FallbackThreshold(t *testing.T) {
	for _, tc := range []struct {
		name      string
		yaml      string
		env       string
		want      int
		wantError bool
	}{
		{name: "default", want: 1},
		{name: "explicit_env", env: "2", want: 2},
		{name: "explicit_yaml", yaml: "gateway:\n  openai_http2:\n    fallback_error_threshold: 3\n", want: 3},
		{name: "env_overrides_yaml", yaml: "gateway:\n  openai_http2:\n    fallback_error_threshold: 3\n", env: "2", want: 2},
		{name: "zero_uses_transport_default", env: "0", want: 0},
		{name: "negative_rejected", env: "-1", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetViperWithJWTSecret(t)
			t.Setenv("GATEWAY_OPENAI_HTTP2_FALLBACK_ERROR_THRESHOLD", tc.env)
			if tc.yaml != "" {
				path := filepath.Join(t.TempDir(), "config.yaml")
				require.NoError(t, os.WriteFile(path, []byte(tc.yaml), 0o600))
				t.Setenv("CONFIG_FILE", path)
			}
			cfg, err := Load()
			if tc.wantError {
				require.ErrorContains(t, err, "gateway.openai_http2.fallback_error_threshold")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, cfg.Gateway.OpenAIHTTP2.FallbackErrorThreshold)
		})
	}
}

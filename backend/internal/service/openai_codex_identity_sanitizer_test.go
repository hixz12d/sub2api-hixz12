package service

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizeCodexClientMetadataFailsClosed(t *testing.T) {
	body := map[string]any{
		"client_metadata": map[string]any{
			"session_id":            "session",
			"cwd":                   "C:/secret/workspace",
			"workspace":             "private-repo",
			"git_branch":            "private-branch",
			"os":                    "Windows",
			"terminal":              "pwsh",
			"plugin":                "private-plugin",
			"mcp":                   "private-mcp",
			"trace_id":              "trace-secret",
			"x-codex-turn-metadata": `{"installation_id":"install","workspace":"private","thread_id":"thread"}`,
		},
	}

	require.True(t, sanitizeCodexClientMetadata(body))
	metadata := body["client_metadata"].(map[string]any)
	require.Equal(t, "session", metadata["session_id"])
	for _, key := range []string{"cwd", "workspace", "git_branch", "os", "terminal", "plugin", "mcp", "trace_id"} {
		_, exists := metadata[key]
		require.False(t, exists, "metadata key %q must not reach upstream", key)
	}
	var turn map[string]any
	require.NoError(t, json.Unmarshal([]byte(metadata["x-codex-turn-metadata"].(string)), &turn))
	require.Equal(t, "install", turn["installation_id"])
	require.Equal(t, "thread", turn["thread_id"])
	require.NotContains(t, turn, "workspace")
}

func TestSanitizeCodexOAuthHeadersRemovesClientTracingAndTimeout(t *testing.T) {
	headers := http.Header{
		"Cookie":              {"session=secret"},
		"Traceparent":         {"00-trace"},
		"Tracestate":          {"vendor=secret"},
		"X-Request-Timeout":   {"120000"},
		"X-Stainless-Timeout": {"120000"},
		"X-Forwarded-For":     {"203.0.113.10"},
		"X-Real-IP":           {"203.0.113.10"},
		"X-Codex-Turn-State":  {"opaque-turn-state"},
	}

	sanitizeCodexOAuthHeaders(headers)
	require.Empty(t, headers.Get("Cookie"))
	require.Empty(t, headers.Get("Traceparent"))
	require.Empty(t, headers.Get("Tracestate"))
	require.Empty(t, headers.Get("X-Request-Timeout"))
	require.Empty(t, headers.Get("X-Stainless-Timeout"))
	require.Empty(t, headers.Get("X-Forwarded-For"))
	require.Empty(t, headers.Get("X-Real-IP"))
	require.Equal(t, "opaque-turn-state", headers.Get("X-Codex-Turn-State"), "turn-state is protocol state, not tracing metadata")
}

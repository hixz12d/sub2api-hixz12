package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

func TestRunCompletesAndReportsCorrelation(t *testing.T) {
	const clientRequestID = "4f39347f-e5b4-4a08-92b8-59d0122c4dd7"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer sk-canary", r.Header.Get("Authorization"))
		require.NotEmpty(t, r.Header.Get("X-Request-ID"))

		w.Header().Set("X-Client-Request-ID", clientRequestID)
		conn, err := websocket.Accept(w, r, nil)
		require.NoError(t, err)
		defer conn.CloseNow()

		_, payload, err := conn.Read(r.Context())
		require.NoError(t, err)
		var request map[string]any
		require.NoError(t, json.Unmarshal(payload, &request))
		require.Equal(t, "response.create", request["type"])
		require.Equal(t, true, request["stream"])

		require.NoError(t, conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"response.created","response":{"id":"resp_canary"}}`)))
		require.NoError(t, conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"response.output_text.delta","delta":"pong"}`)))
		require.NoError(t, conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"response.completed","response":{"id":"resp_canary"}}`)))
	}))
	defer server.Close()

	keyFile := filepath.Join(t.TempDir(), "api-key")
	require.NoError(t, os.WriteFile(keyFile, []byte("sk-canary\n"), 0o600))

	var output bytes.Buffer
	err := run(context.Background(), options{
		URL:     "ws" + strings.TrimPrefix(server.URL, "http"),
		KeyFile: keyFile,
		Model:   "gpt-5.4",
		Prompt:  "Reply exactly pong",
		Timeout: 5 * time.Second,
	}, &output)
	require.NoError(t, err)

	var got result
	require.NoError(t, json.Unmarshal(output.Bytes(), &got))
	require.NotEmpty(t, got.RequestID)
	require.Equal(t, clientRequestID, got.ClientRequestID)
	require.Equal(t, "response.completed", got.TerminalEvent)
	require.Equal(t, "resp_canary", got.ResponseID)
	require.Equal(t, "pong", got.OutputText)
}

func TestReadAPIKeyRejectsBroadPermissions(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("Windows does not expose Unix permission bits consistently")
	}
	path := filepath.Join(t.TempDir(), "api-key")
	require.NoError(t, os.WriteFile(path, []byte("sk-canary\n"), 0o644))

	_, err := readAPIKey(path)
	require.ErrorContains(t, err, "permissions")
}

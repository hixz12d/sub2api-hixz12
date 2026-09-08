//go:build unit

package handler

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type messagesAcceptanceUsageRepo struct {
	service.UsageLogRepository
	mu   sync.Mutex
	logs []service.UsageLog
}

func (r *messagesAcceptanceUsageRepo) Create(_ context.Context, log *service.UsageLog) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logs = append(r.logs, *log)
	return true, nil
}
func (r *messagesAcceptanceUsageRepo) snapshot() []service.UsageLog {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]service.UsageLog(nil), r.logs...)
}

func TestMessagesAcceptanceHTTPPingThenFatalError(t *testing.T) {
	terminal := make(chan struct{})
	server, u, h := blueprintPhaseHandlerServer(t, "/v1/messages", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Request-Id", "private_before_ping")
		_, _ = io.WriteString(w, blueprintPhasePreamble)
		w.(http.Flusher).Flush()
		select {
		case <-terminal:
		case <-r.Context().Done():
			return
		}
		_, _ = io.WriteString(w, "data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"type\":\"invalid_request_error\",\"message\":\"Content policy violation\"}}}\n\n")
	}, 10)
	h.cfg.Gateway.StreamKeepaliveInterval = 1
	h.cfg.Gateway.OpenAIFirstOutputTimeoutSeconds = 0
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp := blueprintPhaseRequest(t, ctx, server, "/v1/messages")
	reader := bufio.NewReader(resp.Body)
	var seen strings.Builder
	for {
		line, err := reader.ReadString('\n')
		require.NoError(t, err)
		seen.WriteString(line)
		if strings.Contains(line, "event: ping") {
			break
		}
	}
	close(terminal)
	rest, err := io.ReadAll(reader)
	require.NoError(t, err)
	seen.Write(rest)
	require.Contains(t, seen.String(), "event: error")
	require.Contains(t, seen.String(), "Content policy violation")
	require.NotContains(t, seen.String(), "event: message_start")
	require.NotEqual(t, "private_before_ping", resp.Header.Get("X-Request-Id"))
	require.EqualValues(t, 1, u.calls.Load())
}

func TestMessagesAcceptanceHTTPFailoverUsageIsolation(t *testing.T) {
	var hits atomic.Int32
	usageRepo := &messagesAcceptanceUsageRepo{}
	server, u, h := blueprintPhaseHandlerServer(t, "/v1/messages", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if hits.Add(1) == 1 {
			w.Header().Set("X-Request-Id", "attempt_A_private_header")
			_, _ = io.WriteString(w, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"attempt_A_private_body\"}}\n\ndata: {\"type\":\"response.failed\",\"response\":{\"id\":\"attempt_A_private_body\",\"error\":{\"code\":\"server_error\",\"message\":\"upstream overloaded\"}}}\n\n")
			return
		}
		w.Header().Set("X-Request-Id", "attempt_B_header")
		_, _ = io.WriteString(w, blueprintPhasePreamble+blueprintPhaseText+blueprintPhaseCompleted)
	}, 10, usageRepo)
	h.maxAccountSwitches = 2
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp := blueprintPhaseRequest(t, ctx, server, "/v1/messages")
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	require.EqualValues(t, 2, u.calls.Load())
	require.Contains(t, string(body), "phase_visible_text")
	require.Contains(t, string(body), "event: message_stop")
	require.NotContains(t, string(body), "attempt_A")
	require.NotEqual(t, "attempt_A_private_header", resp.Header.Get("X-Request-Id"))
	require.Eventually(t, func() bool { return len(usageRepo.snapshot()) > 0 }, time.Second, 10*time.Millisecond)
	logs := usageRepo.snapshot()
	require.Len(t, logs, 1)
	require.EqualValues(t, 2, logs[0].AccountID)
	require.EqualValues(t, 1, logs[0].InputTokens)
	require.EqualValues(t, 1, logs[0].OutputTokens)
}

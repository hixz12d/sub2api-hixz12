package service

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBlueprintV2HeaderGuardStopAndCloseAreIdempotent(t *testing.T) {
	var releases atomic.Int32
	ctx, guard := newOpenAIFirstOutputHeaderGuard(context.Background(), func() { releases.Add(1) }, time.Now().Add(time.Hour))
	t.Cleanup(guard.close)
	done := make(chan struct{})
	go func() {
		defer close(done)
		guard.stopHeaderWait()
		guard.stopHeaderWait()
		guard.close()
		guard.close()
		guard.stopHeaderWait()
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("stop/close blocked after timer was stopped")
	}
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	require.EqualValues(t, 1, releases.Load())
}

func TestBlueprintV2HeaderGuardCancelsSlowHTTPHeaders(t *testing.T) {
	entered := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
	}))
	defer server.Close()
	parent, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx, guard := newOpenAIFirstOutputHeaderGuard(parent, func() {}, time.Now().Add(time.Second))
	defer guard.close()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	require.NoError(t, err)
	resp, err := server.Client().Do(req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.Error(t, err)
	select {
	case <-entered:
	default:
		t.Fatal("request never reached the slow-header fixture")
	}
	require.True(t, guard.stopHeaderWait())
	require.NoError(t, parent.Err(), "gateway timer must not cancel the caller")
}

func TestBlueprintV2HeaderGuardDisarmPreservesHTTPBody(t *testing.T) {
	finish := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: first\n\n")
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Errorf("flush SSE response: %v", err)
			return
		}
		select {
		case <-finish:
			_, _ = io.WriteString(w, "data: last\n\n")
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	parent, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	deadline := time.Now().Add(time.Second)
	ctx, guard := newOpenAIFirstOutputHeaderGuard(parent, func() {}, deadline)
	defer guard.close()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	require.NoError(t, err)
	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.False(t, guard.stopHeaderWait())
	require.False(t, guard.stopHeaderWait())
	reader := bufio.NewReader(resp.Body)
	line, err := reader.ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "data: first\n", line)
	timer := time.NewTimer(time.Until(deadline) + 20*time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		t.Fatal("disarmed header deadline canceled the body")
	}
	close(finish)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Contains(t, string(body), "data: last")
	require.NoError(t, ctx.Err())
}

func TestBlueprintV2HeaderGuardStopRacesExpiry(t *testing.T) {
	for i := 0; i < 100; i++ {
		ctx, guard := newOpenAIFirstOutputHeaderGuard(context.Background(), func() {}, time.Now())
		expired := guard.stopHeaderWait()
		if expired {
			require.ErrorIs(t, ctx.Err(), context.Canceled)
		} else {
			require.NoError(t, ctx.Err())
		}
		require.Equal(t, expired, guard.stopHeaderWait())
		guard.close()
		require.Equal(t, expired, guard.stopHeaderWait())
	}
}

//go:build unit

package repository

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type monitorHTTPTestGuard func(context.Context, int64, *http.Request) error

func (g monitorHTTPTestGuard) Authorize(ctx context.Context, id int64, req *http.Request) error {
	return g(ctx, id, req)
}

type monitorHTTPTestTransport func(*http.Request) (*http.Response, error)

func (f monitorHTTPTestTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestMonitorHTTPGuardActualAccountAndRedirect(t *testing.T) {
	var selected, unexpected atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/selected" {
			selected.Add(1)
			http.Redirect(w, r, "/unexpected", http.StatusTemporaryRedirect)
			return
		}
		unexpected.Add(1)
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()
	upstream := NewHTTPUpstream(nil)
	guard := monitorHTTPTestGuard(func(_ context.Context, id int64, req *http.Request) error {
		if id != 7 || req.URL.Path != "/selected" {
			return service.ErrMonitorOutboundDenied
		}
		return nil
	})
	ctx := service.WithMonitorOutboundGuard(context.Background(), guard)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/selected", strings.NewReader("fixture"))
	require.NoError(t, err)
	response, err := upstream.Do(request, "", 7, 1)
	require.NoError(t, err)
	require.Equal(t, http.StatusTemporaryRedirect, response.StatusCode)
	require.NoError(t, response.Body.Close())
	require.EqualValues(t, 1, selected.Load())
	require.Zero(t, unexpected.Load())
	request, err = http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/selected", strings.NewReader("fixture"))
	require.NoError(t, err)
	_, err = upstream.Do(request, "", 8, 1)
	require.ErrorIs(t, err, service.ErrMonitorOutboundDenied)
	require.EqualValues(t, 1, selected.Load())
	// A forged public header is not the trusted context marker, and the shared
	// cached client retains its ordinary redirect behavior for real traffic.
	request, err = http.NewRequest(http.MethodPost, server.URL+"/selected", strings.NewReader("fixture"))
	require.NoError(t, err)
	request.Header.Set("X-Monitor-Guard", "enabled")
	response, err = upstream.Do(request, "", 7, 1)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.EqualValues(t, 1, unexpected.Load())
	impl := upstream.(*httpUpstreamService)
	for _, entry := range impl.clients {
		require.Zero(t, atomic.LoadInt64(&entry.inFlight))
	}
}

func TestMonitorHTTPGuardFallbackRequiresAnotherPermit(t *testing.T) {
	var guards, sends atomic.Int64
	guard := monitorHTTPTestGuard(func(_ context.Context, id int64, _ *http.Request) error {
		if id != 7 || guards.Add(1) > 1 {
			return service.ErrMonitorOutboundDenied
		}
		return nil
	})
	req, err := http.NewRequestWithContext(service.WithMonitorOutboundGuard(context.Background(), guard), http.MethodPost, "https://"+grokCLIProxyHost+"/v1/chat/completions", strings.NewReader("fixture"))
	require.NoError(t, err)
	req.Header.Set("X-XAI-Token-Auth", "xai-grok-cli")
	req.Header.Set("Authorization", "Bearer fixture-not-a-key")
	base := &http.Client{Transport: monitorHTTPTestTransport(func(sent *http.Request) (*http.Response, error) {
		sends.Add(1)
		require.Nil(t, sent.GetBody, "hidden transport replay must be disabled")
		_ = sent.Body.Close()
		return &http.Response{StatusCode: http.StatusForbidden, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("Access denied")), Request: sent}, nil
	})}
	client := httpClientWithGrokAccessDeniedFallback(httpClientWithMonitorGuard(base, req, 7))
	response, err := client.Do(req)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.EqualValues(t, 2, guards.Load())
	require.EqualValues(t, 1, sends.Load())
	require.Equal(t, http.StatusForbidden, response.StatusCode)
	require.NotNil(t, req.GetBody, "do not mutate the caller request")
}

func TestMonitorHTTPGuardCancelledAndUnauditedTLS(t *testing.T) {
	var calls atomic.Int64
	guard := monitorHTTPTestGuard(func(context.Context, int64, *http.Request) error { calls.Add(1); return nil })
	ctx, cancel := context.WithCancel(service.WithMonitorOutboundGuard(context.Background(), guard))
	cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://fixture.invalid/v1/responses", strings.NewReader("fixture"))
	require.NoError(t, err)
	client := httpClientWithMonitorGuard(&http.Client{Transport: monitorHTTPTestTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("cancelled request reached transport")
		return nil, nil
	})}, req, 7)
	_, err = client.Do(req)
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, calls.Load())
	req, err = http.NewRequestWithContext(service.WithMonitorOutboundGuard(context.Background(), guard), http.MethodPost, "https://fixture.invalid/v1/responses", strings.NewReader("fixture"))
	require.NoError(t, err)
	_, err = NewHTTPUpstream(nil).DoWithTLS(req, "", 7, 1, &tlsfingerprint.Profile{})
	require.ErrorIs(t, err, service.ErrMonitorOutboundDenied)
	require.Zero(t, calls.Load())
}

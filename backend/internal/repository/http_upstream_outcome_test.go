package repository

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func newHTTP2OutcomeTestService() *httpUpstreamService {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIHTTP2 = config.GatewayOpenAIHTTP2Config{
		Enabled: true, AllowProxyFallbackToHTTP1: true,
		FallbackErrorThreshold: 2, FallbackWindowSeconds: 60, FallbackTTLSeconds: 600,
	}
	return NewHTTPUpstream(cfg).(*httpUpstreamService)
}

func newHTTP2OutcomeForTest(t *testing.T, svc *httpUpstreamService, proxy string, major int) *openAIHTTP2Outcome {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "https://upstream.example/v1/responses", nil)
	o := svc.newOpenAIHTTP2Outcome(req, &upstreamClientEntry{proxyKey: proxy, protocolMode: upstreamProtocolModeOpenAIH2}, service.HTTPUpstreamProfileOpenAI)
	o.protoMajor, o.statusCode = major, http.StatusOK
	return o
}

func TestHTTP2OutcomeExactlyOnceAndGenerationIsolation(t *testing.T) {
	svc := newHTTP2OutcomeTestService()
	proxy := "http://proxy.example:8080"
	staleSuccess := newHTTP2OutcomeForTest(t, svc, proxy, 2)
	staleFailure := newHTTP2OutcomeForTest(t, svc, proxy, 2)
	first := newHTTP2OutcomeForTest(t, svc, proxy, 2)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); first.report(io.ErrUnexpectedEOF) }()
	}
	wg.Wait()
	require.False(t, svc.isOpenAIHTTP2FallbackActive(proxy), "duplicate reports count as one attempt")
	newHTTP2OutcomeForTest(t, svc, proxy, 2).report(io.ErrUnexpectedEOF)
	require.True(t, svc.isOpenAIHTTP2FallbackActive(proxy))
	state := svc.getOrCreateOpenAIHTTP2FallbackState(proxy)
	until := state.fallbackUntil
	staleSuccess.report(nil)
	require.Equal(t, until, state.fallbackUntil)
	require.False(t, state.isFallbackActive(until.Add(time.Second)))
	newHTTP2OutcomeForTest(t, svc, proxy, 2).report(io.ErrUnexpectedEOF)
	staleFailure.report(io.ErrUnexpectedEOF)
	require.False(t, svc.isOpenAIHTTP2FallbackActive(proxy), "old epoch failure must not trigger a new quarantine")
	require.Equal(t, 1, state.errorCount)
}

func TestHTTP2OutcomeIgnoresH1CancellationTimeoutAndUnboundResponse(t *testing.T) {
	for _, tc := range []struct {
		name  string
		major int
		err   error
	}{
		{"h1", 1, io.ErrUnexpectedEOF},
		{"cancel", 2, context.Canceled},
		{"timeout", 2, context.DeadlineExceeded},
		{"clean_eof", 2, io.EOF},
		{"business_error", 2, errors.New("rate limit exceeded")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := newHTTP2OutcomeTestService()
			for i := 0; i < 3; i++ {
				newHTTP2OutcomeForTest(t, svc, "http://proxy.example:8080", tc.major).report(tc.err)
			}
			require.False(t, svc.isOpenAIHTTP2FallbackActive("http://proxy.example:8080"))
		})
	}
	svc := newHTTP2OutcomeTestService()
	for i := 0; i < 3; i++ {
		svc.RecordOpenAIHTTP2ResponseOutcome(&http.Response{ProtoMajor: 2, Request: httptest.NewRequest(http.MethodGet, "https://upstream.example", nil)}, io.ErrUnexpectedEOF)
	}
	require.False(t, svc.isOpenAIHTTP2FallbackActive("http://proxy.example:8080"))
}

func TestHTTP2OutcomeTerminalSuccessPreservesRecentFailures(t *testing.T) {
	svc := newHTTP2OutcomeTestService()
	proxy := "http://proxy.example:8080"
	newHTTP2OutcomeForTest(t, svc, proxy, 2).report(io.ErrUnexpectedEOF)
	success := newHTTP2OutcomeForTest(t, svc, proxy, 2)
	success.report(nil)
	success.report(io.ErrUnexpectedEOF)
	require.False(t, svc.isOpenAIHTTP2FallbackActive(proxy), "one response can report only one outcome")
	require.Equal(t, 1, svc.getOrCreateOpenAIHTTP2FallbackState(proxy).errorCount)
	newHTTP2OutcomeForTest(t, svc, proxy, 2).report(io.ErrUnexpectedEOF)
	require.True(t, svc.isOpenAIHTTP2FallbackActive(proxy), "interleaved success must not hide intermittent failures")
}

// Exercise the real Do -> CONNECT -> TLS/ALPN -> SSE read-error path. The proxy
// accepts only this test server, and every hijacked connection is closed/joined.
func TestHTTP2OutcomeRealDoFallsBackAfterTwoBrokenStreams(t *testing.T) {
	testHTTP2OutcomeRealDo(t, "http", false, 2)
}

func TestHTTP2OutcomeRealDoSOCKSFallbackPreservesProxy(t *testing.T) {
	for _, scheme := range []string{"socks5", "socks5h"} {
		t.Run(scheme, func(t *testing.T) { testHTTP2OutcomeRealDo(t, scheme, false, 2) })
	}
}

func testHTTP2OutcomeRealDo(t *testing.T, scheme string, useTLSFingerprint bool, fallbackThreshold int) {
	t.Helper()
	wantFailures := fallbackThreshold
	if wantFailures == 0 {
		wantFailures = 1
	}
	var h2Calls atomic.Int32
	target := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if r.ProtoMajor == 2 {
			h2Calls.Add(1)
			_, _ = io.WriteString(w, "data: {\"type\":\"response.created\"}\n\n")
			w.(http.Flusher).Flush()
			panic(http.ErrAbortHandler)
		}
		_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\"}\n\n")
	}))
	target.EnableHTTP2 = true
	target.StartTLS()
	defer target.Close()
	var mu sync.Mutex
	var connections []net.Conn
	var tunnels sync.WaitGroup
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || r.Host != target.Listener.Addr().String() {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		upstream, err := net.DialTimeout("tcp", target.Listener.Addr().String(), time.Second)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		client, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			_ = upstream.Close()
			return
		}
		mu.Lock()
		connections = append(connections, client, upstream)
		tunnels.Add(1)
		mu.Unlock()
		defer tunnels.Done()
		defer func() { _ = client.Close(); _ = upstream.Close() }()
		_, _ = rw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		if rw.Flush() != nil {
			return
		}
		done := make(chan struct{})
		go func() { _, _ = io.Copy(upstream, rw); _ = upstream.Close(); close(done) }()
		_, _ = io.Copy(client, upstream)
		_ = client.Close()
		<-done
	}))
	defer func() {
		proxy.Close()
		mu.Lock()
		for _, c := range connections {
			_ = c.Close()
		}
		mu.Unlock()
		tunnels.Wait()
	}()
	svc := newHTTP2OutcomeTestService()
	svc.cfg.Gateway.OpenAIHTTP2.FallbackErrorThreshold = fallbackThreshold
	budget := service.NewOpenAIRetryBudget(false)
	proxyURL := proxy.URL
	var socksCalls *atomic.Int64
	if scheme != "http" {
		proxyURL, socksCalls = startTestSOCKS5Proxy(t)
		proxyURL = strings.Replace(proxyURL, "socks5h://", scheme+"://", 1)
	}
	roots := x509.NewCertPool()
	roots.AddCert(target.Certificate())
	var profile *tlsfingerprint.Profile
	if useTLSFingerprint {
		profile = tlsfingerprint.BuiltinChromeAutoProfile()
		certFile := filepath.Join(t.TempDir(), "upstream-ca.pem")
		certificate := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: target.Certificate().Raw})
		require.NoError(t, os.WriteFile(certFile, certificate, 0600))
		t.Setenv("SSL_CERT_FILE", certFile)
		t.Setenv("SSL_CERT_DIR", t.TempDir())
	}
	proxyKey, _, err := normalizeProxyURL(proxyURL)
	require.NoError(t, err)
	for i := 0; i <= wantFailures; i++ {
		if fallbackThreshold == 0 {
			require.NoError(t, budget.Reserve(1), "default fallback must fit the existing two-attempt budget")
		}
		var entry *upstreamClientEntry
		if profile == nil {
			entry, err = svc.getClientEntry(proxyURL, 1, 1, service.HTTPUpstreamProfileOpenAI, false, false)
		} else {
			entry, err = svc.getClientEntryWithTLS(proxyURL, 1, 1, profile, service.HTTPUpstreamProfileOpenAI, false, false)
		}
		require.NoError(t, err)
		// uTLS uses the isolated process's temporary trust store; native TLS can
		// receive a pool directly, before first use of each cached transport.
		if profile == nil && (i == 0 || i == wantFailures) {
			tr := entry.client.Transport.(*http.Transport)
			if tr.TLSClientConfig == nil {
				tr.TLSClientConfig = target.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
				tr.TLSClientConfig.InsecureSkipVerify = false
			}
			tr.TLSClientConfig.RootCAs = roots
		}
		if tr, ok := entry.client.Transport.(interface{ CloseIdleConnections() }); ok {
			defer tr.CloseIdleConnections()
		}
		ctx, cancel := context.WithTimeout(service.WithHTTPUpstreamProfile(t.Context(), service.HTTPUpstreamProfileOpenAI), 10*time.Second)
		t.Cleanup(cancel)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.URL, nil)
		require.NoError(t, err)
		req.Header.Set("Accept", "text/event-stream")
		resp, err := svc.DoWithTLS(req, proxyURL, 1, 1, profile)
		require.NoError(t, err)
		_, readErr := io.ReadAll(resp.Body)
		if i < wantFailures {
			require.Equal(t, 2, resp.ProtoMajor)
			require.Error(t, readErr)
			svc.RecordOpenAIHTTP2ResponseOutcome(resp, readErr)
			svc.RecordOpenAIHTTP2ResponseOutcome(resp, readErr)
			require.Equal(t, i == wantFailures-1, svc.isOpenAIHTTP2FallbackActive(proxyKey))
		} else {
			require.Equal(t, 1, resp.ProtoMajor)
			require.NoError(t, readErr)
			svc.RecordOpenAIHTTP2ResponseOutcome(resp, nil)
			require.True(t, svc.isOpenAIHTTP2FallbackActive(proxyKey))
		}
		require.NoError(t, resp.Body.Close())
		cancel()
		require.Zero(t, atomic.LoadInt64(&entry.inFlight))
	}
	require.Equal(t, int32(wantFailures), h2Calls.Load())
	if fallbackThreshold == 0 {
		require.Equal(t, 2, budget.Snapshot().Attempts)
		require.ErrorIs(t, budget.Reserve(1), service.ErrOpenAIRetryBudgetExhausted)
	}
	if socksCalls != nil {
		require.GreaterOrEqual(t, socksCalls.Load(), int64(2), "fallback must open its H1 connection through the SOCKS proxy")
	}
}

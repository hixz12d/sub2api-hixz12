package repository

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestHTTP2OutcomeDefaultRecoversWithinRetryBudget(t *testing.T) {
	for _, scheme := range []string{"http", "socks5", "socks5h"} {
		t.Run(scheme, func(t *testing.T) { testHTTP2OutcomeRealDo(t, scheme, false, 0) })
	}
}

func TestHTTP2OutcomeSuccessExpiresOnlyStaleFailures(t *testing.T) {
	for _, stale := range []bool{false, true} {
		name := "recent"
		if stale {
			name = "expired"
		}
		t.Run(name, func(t *testing.T) {
			svc := newHTTP2OutcomeTestService()
			proxy := "http://proxy.example:8080"
			state := svc.getOrCreateOpenAIHTTP2FallbackState(proxy)
			started := time.Now()
			if stale {
				started = started.Add(-2 * time.Minute)
			}
			tripped, _ := state.recordFailure(started, 2, time.Minute, 10*time.Minute)
			require.False(t, tripped)

			newHTTP2OutcomeForTest(t, svc, proxy, 2).report(nil)
			if stale {
				require.Zero(t, state.errorCount)
				require.True(t, state.windowStart.IsZero())
			} else {
				require.Equal(t, 1, state.errorCount)
				require.Equal(t, started, state.windowStart)
			}
			newHTTP2OutcomeForTest(t, svc, proxy, 2).report(io.ErrUnexpectedEOF)
			require.Equal(t, !stale, svc.isOpenAIHTTP2FallbackActive(proxy))
		})
	}
}

func TestHTTP2OutcomeConcurrentSuccessDoesNotHideFailures(t *testing.T) {
	svc := newHTTP2OutcomeTestService()
	proxy := "socks5://proxy.example:1080"
	outcomes := make([]*openAIHTTP2Outcome, 32)
	for i := range outcomes {
		outcomes[i] = newHTTP2OutcomeForTest(t, svc, proxy, 2)
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i, outcome := range outcomes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if i < 2 {
				outcome.report(io.ErrUnexpectedEOF)
			} else {
				outcome.report(nil)
			}
		}()
	}
	close(start)
	wg.Wait()
	require.True(t, svc.isOpenAIHTTP2FallbackActive(proxy))
}

func TestHTTP2OutcomeDefaultFallbackIsScopedAndExpires(t *testing.T) {
	svc := newHTTP2OutcomeTestService()
	svc.cfg.Gateway.OpenAIHTTP2.FallbackErrorThreshold = 0
	require.Equal(t, 1, svc.resolveOpenAIHTTP2Settings().fallbackErrorThreshold)
	proxy := "http://proxy.example:8080"
	key, parsed, err := normalizeProxyURL(proxy)
	require.NoError(t, err)
	newHTTP2OutcomeForTest(t, svc, proxy, 2).report(io.ErrUnexpectedEOF)
	require.Equal(t, upstreamProtocolModeOpenAIH1Fallback, svc.resolveProtocolMode(service.HTTPUpstreamProfileOpenAI, key, parsed))
	require.Equal(t, upstreamProtocolModeDefault, svc.resolveProtocolMode(service.HTTPUpstreamProfileDefault, key, parsed))
	require.Equal(t, upstreamProtocolModeGrok, svc.resolveProtocolMode(service.HTTPUpstreamProfileGrok, key, parsed))
	require.Equal(t, upstreamProtocolModeOpenAIH2, svc.resolveProtocolMode(service.HTTPUpstreamProfileOpenAI, "direct", nil))
	otherKey, otherProxy, err := normalizeProxyURL("http://healthy.example:8080")
	require.NoError(t, err)
	require.Equal(t, upstreamProtocolModeOpenAIH2, svc.resolveProtocolMode(service.HTTPUpstreamProfileOpenAI, otherKey, otherProxy))

	state := svc.getOrCreateOpenAIHTTP2FallbackState(proxy)
	until := state.fallbackUntil
	newHTTP2OutcomeForTest(t, svc, proxy, 1).report(io.ErrUnexpectedEOF)
	require.Equal(t, until, state.fallbackUntil, "H1 errors must not prolong H2 quarantine")
	require.False(t, state.isFallbackActive(until.Add(time.Nanosecond)))
	require.Equal(t, upstreamProtocolModeOpenAIH2, svc.resolveProtocolMode(service.HTTPUpstreamProfileOpenAI, key, parsed))
}

func TestHTTP2OutcomeDefaultStillIgnoresNonTransportFailures(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"cancel", context.Canceled},
		{"deadline", context.DeadlineExceeded},
		{"header_timeout", errors.New("http2: timeout awaiting response headers")},
		{"business_error", errors.New("rate limit exceeded")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := newHTTP2OutcomeTestService()
			svc.cfg.Gateway.OpenAIHTTP2.FallbackErrorThreshold = 0
			proxy := "http://proxy.example:8080"
			newHTTP2OutcomeForTest(t, svc, proxy, 2).report(tc.err)
			require.False(t, svc.isOpenAIHTTP2FallbackActive(proxy))
			require.Zero(t, svc.getOrCreateOpenAIHTTP2FallbackState(proxy).errorCount)
		})
	}
}

func TestHTTP2OutcomeFallbackOptOutAndDirectRemainUnchanged(t *testing.T) {
	for _, tc := range []struct {
		name     string
		disabled bool
		optOut   bool
		proxy    string
	}{
		{name: "disabled", disabled: true, proxy: "http://proxy.example:8080"},
		{name: "opt_out", optOut: true, proxy: "http://proxy.example:8080"},
		{name: "direct", proxy: "direct"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := newHTTP2OutcomeTestService()
			svc.cfg.Gateway.OpenAIHTTP2.FallbackErrorThreshold = 0
			svc.cfg.Gateway.OpenAIHTTP2.Enabled = !tc.disabled
			svc.cfg.Gateway.OpenAIHTTP2.AllowProxyFallbackToHTTP1 = !tc.optOut
			req := httptest.NewRequest(http.MethodPost, "https://upstream.example/v1/responses", nil)
			outcome := svc.newOpenAIHTTP2Outcome(req, &upstreamClientEntry{
				proxyKey: tc.proxy, protocolMode: upstreamProtocolModeOpenAIH2,
			}, service.HTTPUpstreamProfileOpenAI)
			require.Nil(t, outcome)
			outcome.report(io.ErrUnexpectedEOF)
			require.False(t, svc.isOpenAIHTTP2FallbackActive(tc.proxy))
		})
	}
}

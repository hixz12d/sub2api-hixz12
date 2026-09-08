package repository

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestHTTP2OutcomeFingerprintFallbackALPN(t *testing.T) {
	for _, mode := range []string{upstreamProtocolModeOpenAIH2, upstreamProtocolModeOpenAIH1Fallback, upstreamProtocolModeOpenAIH1} {
		t.Run(mode, func(t *testing.T) {
			hello := make(chan []string, 1)
			target := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
			target.TLS = &tls.Config{GetConfigForClient: func(info *tls.ClientHelloInfo) (*tls.Config, error) {
				select {
				case hello <- append([]string(nil), info.SupportedProtos...):
				default:
				}
				return nil, errors.New("test stops after ClientHello")
			}}
			target.StartTLS()
			defer target.Close()
			profile := tlsfingerprint.BuiltinChromeAutoProfile()
			rt, err := buildUpstreamRoundTripperWithTLSFingerprint(http2KeepAliveTestPoolSettings(), nil, profile, mode)
			require.NoError(t, err)
			if closer, ok := rt.(interface{ CloseIdleConnections() }); ok {
				defer closer.CloseIdleConnections()
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.URL, nil)
			require.NoError(t, err)
			_, err = rt.RoundTrip(req)
			require.Error(t, err, "server deliberately stops the handshake")
			select {
			case protocols := <-hello:
				if mode == upstreamProtocolModeOpenAIH2 {
					require.Contains(t, protocols, "h2")
				} else {
					require.Equal(t, []string{"http/1.1"}, protocols)
				}
			case <-ctx.Done():
				t.Fatal("no ClientHello received")
			}
			require.Empty(t, profile.ALPNProtocols, "do not mutate the caller's shared profile")
		})
	}
}

func TestHTTP2OutcomeDoWithTLSBindsAttempt(t *testing.T) {
	svc := newHTTP2OutcomeTestService()
	profile := tlsfingerprint.BuiltinChromeAutoProfile()
	proxy := "socks5://proxy.example:1080"
	proxyKey, _, err := normalizeProxyURL(proxy)
	require.NoError(t, err)
	for i := 0; i < 2; i++ {
		entry, err := svc.getClientEntryWithTLS(proxy, 1, 1, profile, service.HTTPUpstreamProfileOpenAI, false, false)
		require.NoError(t, err)
		entry.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, ProtoMajor: 2, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
		})
		req, err := http.NewRequestWithContext(service.WithHTTPUpstreamProfile(t.Context(), service.HTTPUpstreamProfileOpenAI), http.MethodPost, "https://upstream.example/v1/responses", nil)
		require.NoError(t, err)
		resp, err := svc.DoWithTLS(req, proxy, 1, 1, profile)
		require.NoError(t, err)
		svc.RecordOpenAIHTTP2ResponseOutcome(resp, io.ErrUnexpectedEOF)
		require.NoError(t, resp.Body.Close())
		require.Equal(t, i == 1, svc.isOpenAIHTTP2FallbackActive(proxyKey))
	}
	entry, err := svc.getClientEntryWithTLS(proxy, 1, 1, profile, service.HTTPUpstreamProfileOpenAI, false, false)
	require.NoError(t, err)
	require.Equal(t, upstreamProtocolModeOpenAIH1Fallback, entry.protocolMode)
	require.IsType(t, &http.Transport{}, entry.client.Transport)
}

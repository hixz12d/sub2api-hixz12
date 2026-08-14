package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type openAIEgressProxyRepoStub struct {
	ProxyRepository
	proxy *Proxy
	err   error
}

func (r *openAIEgressProxyRepoStub) GetByID(context.Context, int64) (*Proxy, error) {
	return r.proxy, r.err
}

func strictOpenAIEgressConfig() *config.Config {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIEgress.Mode = string(OpenAIEgressProxyRequired)
	cfg.Gateway.OpenAIEgress.FailoverRoutePolicy = "same_route"
	return cfg
}

func TestOpenAIEgressResolverStrictRequiresProxy(t *testing.T) {
	resolver := newOpenAIEgressResolver(strictOpenAIEgressConfig(), nil)
	route, err := resolver.Resolve(context.Background(), &Account{Platform: PlatformOpenAI})
	require.ErrorIs(t, err, ErrOpenAIProxyRequired)
	require.Empty(t, route.ProxyURL)
	require.False(t, route.Direct)
}

func TestOpenAIEgressResolverOptionalReturnsExplicitDirectRoute(t *testing.T) {
	resolver := newOpenAIEgressResolver(&config.Config{}, nil)
	route, err := resolver.Resolve(context.Background(), &Account{Platform: PlatformOpenAI})
	require.NoError(t, err)
	require.True(t, route.Direct)
	require.Equal(t, "direct", route.RouteKey)
}

func TestOpenAIEgressResolverLoadsIPv6ProxyAndNormalizesSOCKSRemoteDNS(t *testing.T) {
	proxyID := int64(73)
	proxy := &Proxy{
		ID:        proxyID,
		Protocol:  "socks5",
		Host:      "2001:db8::73",
		Port:      1080,
		Username:  "ipv6-user",
		Password:  "secret-password",
		Status:    StatusActive,
		UpdatedAt: time.Unix(1700000000, 0).UTC(),
	}
	resolver := newOpenAIEgressResolver(strictOpenAIEgressConfig(), &openAIEgressProxyRepoStub{proxy: proxy})
	route, err := resolver.Resolve(context.Background(), &Account{Platform: PlatformOpenAI, ProxyID: &proxyID})
	require.NoError(t, err)
	require.False(t, route.Direct)
	require.Equal(t, proxyID, route.ProxyID)
	require.True(t, strings.HasPrefix(route.ProxyURL, "socks5h://"))
	require.Contains(t, route.ProxyURL, "@[2001:db8::73]:1080")
	require.NotContains(t, route.RouteKey, "secret-password")
	require.Len(t, strings.TrimPrefix(route.RouteKey, "proxy:"), 16)
}

func TestOpenAIEgressResolverLookupFailureNeverReturnsDirect(t *testing.T) {
	proxyID := int64(9)
	resolver := newOpenAIEgressResolver(strictOpenAIEgressConfig(), &openAIEgressProxyRepoStub{err: errors.New("database unavailable")})
	route, err := resolver.Resolve(context.Background(), &Account{Platform: PlatformOpenAI, ProxyID: &proxyID})
	require.ErrorIs(t, err, ErrOpenAIProxyUnavailable)
	require.False(t, route.Direct)
	require.Empty(t, route.ProxyURL)
	require.NotContains(t, err.Error(), "database unavailable")
}

func TestOpenAIEgressResolverRejectsExpiredDisabledAndDirectFallback(t *testing.T) {
	now := time.Now()
	base := Proxy{ID: 1, Protocol: "http", Host: "2001:db8::1", Port: 8080, Status: StatusActive, UpdatedAt: now}
	resolver := newOpenAIEgressResolver(strictOpenAIEgressConfig(), nil).(*openAIEgressResolver)

	disabled := base
	disabled.Status = StatusDisabled
	_, err := resolver.resolveProxy(&disabled)
	require.ErrorIs(t, err, ErrOpenAIProxyUnavailable)

	expired := base
	expired.ExpiresAt = &now
	_, err = resolver.resolveProxy(&expired)
	require.ErrorIs(t, err, ErrOpenAIProxyUnavailable)

	directFallback := base
	directFallback.FallbackMode = FallbackModeDirect
	_, err = resolver.resolveProxy(&directFallback)
	require.ErrorIs(t, err, ErrOpenAIProxyInvalid)
}

func TestOpenAIEgressRouteKeyChangesWithCredentialsAndVersion(t *testing.T) {
	base := &Proxy{ID: 1, Protocol: "https", Host: "2001:db8::2", Port: 443, Username: "u", Password: "a", Status: StatusActive, UpdatedAt: time.Unix(1, 0)}
	resolver := newOpenAIEgressResolver(strictOpenAIEgressConfig(), nil).(*openAIEgressResolver)
	first, err := resolver.resolveProxy(base)
	require.NoError(t, err)
	second, err := resolver.resolveProxy(base)
	require.NoError(t, err)
	require.Equal(t, first.RouteKey, second.RouteKey)

	changed := *base
	changed.Password = "b"
	third, err := resolver.resolveProxy(&changed)
	require.NoError(t, err)
	require.NotEqual(t, first.RouteKey, third.RouteKey)

	changed = *base
	changed.UpdatedAt = base.UpdatedAt.Add(time.Second)
	fourth, err := resolver.resolveProxy(&changed)
	require.NoError(t, err)
	require.NotEqual(t, first.RouteKey, fourth.RouteKey)
}

func TestOpenAIWSCompatibilityIncludesRouteKey(t *testing.T) {
	headers := mapHeader("x-codex-beta-features", "responses_websockets=2026-02-06")
	a := normalizeOpenAIWSHandshakeCompatibility(headers, "session", "proxy:aaaa")
	b := normalizeOpenAIWSHandshakeCompatibility(headers, "session", "proxy:bbbb")
	require.NotEqual(t, a, b)

	conn := &openAIWSConn{routeKey: "proxy:aaaa"}
	require.True(t, conn.matchesRouteKey("proxy:aaaa"))
	require.False(t, conn.matchesRouteKey("proxy:bbbb"))
	require.False(t, sameOpenAIWSPrewarmTarget(
		openAIWSAcquireRequest{WSURL: "wss://api.openai.com/v1/responses", RouteKey: "proxy:aaaa", Headers: headers},
		openAIWSAcquireRequest{WSURL: "wss://api.openai.com/v1/responses", RouteKey: "proxy:bbbb", Headers: headers},
	))
}

func mapHeader(key, value string) map[string][]string {
	return map[string][]string{key: {value}}
}

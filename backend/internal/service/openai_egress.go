package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyurl"
)

type OpenAIEgressMode string

const (
	OpenAIEgressOptional      OpenAIEgressMode = "optional"
	OpenAIEgressProxyRequired OpenAIEgressMode = "proxy_required"
)

var (
	ErrOpenAIProxyRequired    = errors.New("OpenAI egress proxy is required")
	ErrOpenAIProxyUnavailable = errors.New("OpenAI egress proxy is unavailable")
	ErrOpenAIProxyInvalid     = errors.New("OpenAI egress proxy is invalid")
	ErrOpenAIRouteMismatch    = errors.New("OpenAI egress route mismatch")
)

type OpenAIEgressRoute struct {
	ProxyURL string
	ProxyID  int64
	RouteKey string
	Direct   bool
}

type OpenAIEgressResolver interface {
	Resolve(ctx context.Context, account *Account) (OpenAIEgressRoute, error)
}

type openAIEgressResolver struct {
	mode        OpenAIEgressMode
	proxyRepo   ProxyRepository
	accountRepo AccountRepository
}

func newOpenAIEgressResolver(cfg *config.Config, proxyRepo ProxyRepository) OpenAIEgressResolver {
	return newOpenAIEgressResolverWithAccountRepo(cfg, proxyRepo, nil)
}

func newOpenAIEgressResolverWithAccountRepo(cfg *config.Config, proxyRepo ProxyRepository, accountRepo AccountRepository) OpenAIEgressResolver {
	mode := OpenAIEgressOptional
	if cfg != nil {
		mode = OpenAIEgressMode(strings.ToLower(strings.TrimSpace(cfg.Gateway.OpenAIEgress.Mode)))
	}
	if mode == "" {
		mode = OpenAIEgressOptional
	}
	return &openAIEgressResolver{mode: mode, proxyRepo: proxyRepo, accountRepo: accountRepo}
}

func (r *openAIEgressResolver) Resolve(ctx context.Context, account *Account) (OpenAIEgressRoute, error) {
	if account == nil {
		return OpenAIEgressRoute{}, fmt.Errorf("%w: account is missing", ErrOpenAIProxyUnavailable)
	}
	if account.Platform != PlatformOpenAI {
		return directOpenAIEgressRoute(), nil
	}
	if account.ProxyID == nil || *account.ProxyID <= 0 {
		if r.mode == OpenAIEgressProxyRequired {
			return OpenAIEgressRoute{}, ErrOpenAIProxyRequired
		}
		return directOpenAIEgressRoute(), nil
	}

	proxyID := *account.ProxyID
	proxy := account.Proxy
	if proxy == nil {
		switch {
		case r.proxyRepo != nil:
			loadedProxy, err := r.proxyRepo.GetByID(ctx, proxyID)
			if err != nil {
				return OpenAIEgressRoute{}, fmt.Errorf("%w: proxy lookup failed", ErrOpenAIProxyUnavailable)
			}
			proxy = loadedProxy
		case r.accountRepo != nil && account.ID > 0:
			loadedAccount, err := r.accountRepo.GetByID(ctx, account.ID)
			if err != nil || loadedAccount == nil {
				return OpenAIEgressRoute{}, fmt.Errorf("%w: account hydration failed", ErrOpenAIProxyUnavailable)
			}
			if loadedAccount.ProxyID == nil || *loadedAccount.ProxyID != proxyID || loadedAccount.Proxy == nil {
				return OpenAIEgressRoute{}, fmt.Errorf("%w: hydrated proxy relation is unavailable", ErrOpenAIProxyUnavailable)
			}
			proxy = loadedAccount.Proxy
		default:
			return OpenAIEgressRoute{}, fmt.Errorf("%w: proxy relation is not loaded", ErrOpenAIProxyUnavailable)
		}
	}
	if proxy == nil || proxy.ID != proxyID {
		return OpenAIEgressRoute{}, fmt.Errorf("%w: proxy relation does not match account", ErrOpenAIProxyUnavailable)
	}
	return r.resolveProxy(proxy)
}

func (r *openAIEgressResolver) resolveProxy(proxy *Proxy) (OpenAIEgressRoute, error) {
	if proxy == nil {
		return OpenAIEgressRoute{}, ErrOpenAIProxyUnavailable
	}
	if !proxy.IsActive() || proxy.IsExpired(time.Now()) {
		return OpenAIEgressRoute{}, ErrOpenAIProxyUnavailable
	}
	if r.mode == OpenAIEgressProxyRequired && strings.EqualFold(strings.TrimSpace(proxy.FallbackMode), FallbackModeDirect) {
		return OpenAIEgressRoute{}, fmt.Errorf("%w: direct fallback is forbidden", ErrOpenAIProxyInvalid)
	}
	if proxy.Port < 1 || proxy.Port > 65535 || strings.TrimSpace(proxy.Host) == "" {
		return OpenAIEgressRoute{}, ErrOpenAIProxyInvalid
	}

	normalized, parsed, err := proxyurl.Parse(proxy.URL())
	if err != nil || parsed == nil {
		return OpenAIEgressRoute{}, ErrOpenAIProxyInvalid
	}
	if parsed.Port() == "" {
		return OpenAIEgressRoute{}, ErrOpenAIProxyInvalid
	}
	if ip := net.ParseIP(parsed.Hostname()); ip != nil && strings.Contains(parsed.Hostname(), ":") {
		expected := net.JoinHostPort(parsed.Hostname(), strconv.Itoa(proxy.Port))
		if parsed.Host != expected {
			return OpenAIEgressRoute{}, ErrOpenAIProxyInvalid
		}
	}

	return OpenAIEgressRoute{
		ProxyURL: normalized,
		ProxyID:  proxy.ID,
		RouteKey: openAIEgressRouteKey(proxy.ID, proxy.UpdatedAt, normalized),
		Direct:   false,
	}, nil
}

func directOpenAIEgressRoute() OpenAIEgressRoute {
	return OpenAIEgressRoute{RouteKey: "direct", Direct: true}
}

func openAIEgressRouteKey(proxyID int64, updatedAt time.Time, normalizedProxyURL string) string {
	canonical := normalizedProxyURL
	if parsed, err := url.Parse(normalizedProxyURL); err == nil && parsed != nil {
		canonical = parsed.String()
	}
	sum := sha256.Sum256([]byte(strconv.FormatInt(proxyID, 10) + "\x00" + updatedAt.UTC().Format(time.RFC3339Nano) + "\x00" + canonical))
	return fmt.Sprintf("proxy:%x", sum[:8])
}

package tlsfingerprint

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/imroc/req/v3"
	reqhttp2 "github.com/imroc/req/v3/http2"
)

// Chrome HTTP/2 values match Chromium / official Codex Electron, not Go's
// default 1GiB connection window and 4MiB stream window. ClientHello is
// already HelloChrome_Auto; these settings close the H2 half of the mismatch
// that shows up as protocol errors or silent proxy resets.
const (
	ChromeHTTP2HeaderTableSize      uint32 = 65536
	ChromeHTTP2EnablePush           uint32 = 0
	ChromeHTTP2MaxConcurrentStreams uint32 = 1000
	ChromeHTTP2InitialWindowSize    uint32 = 6291456
	ChromeHTTP2MaxHeaderListSize    uint32 = 262144
	ChromeHTTP2ConnectionFlow       uint32 = 15663105
)

// ChromeHTTP2Options tunes pool and keepalive on the impersonating transport.
type ChromeHTTP2Options struct {
	MaxIdleConns          int
	IdleConnTimeout       time.Duration
	ReadIdleTimeout       time.Duration
	PingTimeout           time.Duration
	ResponseHeaderTimeout time.Duration
}

var chromeHTTP2Settings = []reqhttp2.Setting{
	{ID: reqhttp2.SettingHeaderTableSize, Val: ChromeHTTP2HeaderTableSize},
	{ID: reqhttp2.SettingEnablePush, Val: ChromeHTTP2EnablePush},
	{ID: reqhttp2.SettingMaxConcurrentStreams, Val: ChromeHTTP2MaxConcurrentStreams},
	{ID: reqhttp2.SettingInitialWindowSize, Val: ChromeHTTP2InitialWindowSize},
	{ID: reqhttp2.SettingMaxHeaderListSize, Val: ChromeHTTP2MaxHeaderListSize},
}

var chromeHTTP2PseudoHeaderOrder = []string{
	":method",
	":authority",
	":scheme",
	":path",
}

// chromeHTTP2HeaderOrder is a stable order for headers we actually send to
// OpenAI/Codex. It is not Chrome's HTML navigation list — injecting
// sec-ch-ua / accept:text/html would break the API path.
var chromeHTTP2HeaderOrder = []string{
	"host",
	"authorization",
	"content-type",
	"accept",
	"accept-encoding",
	"accept-language",
	"user-agent",
	"openai-beta",
	"openai-organization",
	"openai-project",
	"openai-intent",
	"originator",
	"session-id",
	"session_id",
	"thread-id",
	"thread_id",
	"conversation_id",
	"x-codex-installation-id",
	"x-codex-window-id",
	"x-codex-turn-metadata",
	"x-client-request-id",
	"x-session-id",
	"cookie",
}

type chromeHTTP2RoundTripper struct {
	inner *req.Transport
}

// NewChromeHTTP2RoundTripper builds an HTTP/2 transport that reuses an already
// completed utls handshake and emits Chrome SETTINGS / WINDOW_UPDATE / header
// order. Proxy CONNECT must happen inside dialTLS; this transport does not
// speak to the proxy itself.
func NewChromeHTTP2RoundTripper(dialTLS func(ctx context.Context, network, addr string) (net.Conn, error), opts ChromeHTTP2Options) http.RoundTripper {
	if dialTLS == nil {
		panic("tlsfingerprint: NewChromeHTTP2RoundTripper requires dialTLS")
	}
	t := req.T()
	t.DisableCompression = true
	t.DisableAutoDecode()
	t.SetProxy(func(*http.Request) (*url.URL, error) { return nil, nil })
	t.EnableForceHTTP2()
	t.SetDialTLS(dialTLS)
	t.SetHTTP2SettingsFrame(chromeHTTP2Settings...)
	t.SetHTTP2ConnectionFlow(ChromeHTTP2ConnectionFlow)
	t.SetHTTP2HeaderPriority(reqhttp2.PriorityParam{
		StreamDep: 0,
		Exclusive: true,
		Weight:    255,
	})
	if opts.MaxIdleConns > 0 {
		t.SetMaxIdleConns(opts.MaxIdleConns)
	}
	if opts.IdleConnTimeout > 0 {
		t.SetIdleConnTimeout(opts.IdleConnTimeout)
	}
	if opts.ReadIdleTimeout > 0 {
		t.SetHTTP2ReadIdleTimeout(opts.ReadIdleTimeout)
	}
	if opts.PingTimeout > 0 {
		t.SetHTTP2PingTimeout(opts.PingTimeout)
	}
	if opts.ResponseHeaderTimeout > 0 {
		t.SetResponseHeaderTimeout(opts.ResponseHeaderTimeout)
	}
	return &chromeHTTP2RoundTripper{inner: t}
}

func (t *chromeHTTP2RoundTripper) RoundTrip(httpReq *http.Request) (*http.Response, error) {
	if httpReq == nil {
		return t.inner.RoundTrip(httpReq)
	}
	httpReq = httpReq.Clone(httpReq.Context())
	if httpReq.Header == nil {
		httpReq.Header = make(http.Header)
	}
	httpReq.Header[req.HeaderOderKey] = append([]string(nil), chromeHTTP2HeaderOrder...)
	httpReq.Header[req.PseudoHeaderOderKey] = append([]string(nil), chromeHTTP2PseudoHeaderOrder...)
	return t.inner.RoundTrip(httpReq)
}

func (t *chromeHTTP2RoundTripper) CloseIdleConnections() {
	if t != nil && t.inner != nil {
		t.inner.CloseIdleConnections()
	}
}

// IsChromeHTTP2RoundTripper reports whether rt is the Chrome H2 impersonating transport.
func IsChromeHTTP2RoundTripper(rt http.RoundTripper) bool {
	_, ok := rt.(*chromeHTTP2RoundTripper)
	return ok
}

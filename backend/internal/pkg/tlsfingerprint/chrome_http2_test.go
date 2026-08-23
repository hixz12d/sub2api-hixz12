package tlsfingerprint

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"golang.org/x/net/http2"
)

type fakeH2TLSConn struct {
	net.Conn
}

func (c fakeH2TLSConn) ConnectionState() tls.ConnectionState {
	return tls.ConnectionState{
		HandshakeComplete:          true,
		Version:                    tls.VersionTLS13,
		NegotiatedProtocol:         "h2",
		NegotiatedProtocolIsMutual: true,
	}
}

func (c fakeH2TLSConn) Handshake() error { return nil }

func (c fakeH2TLSConn) HandshakeContext(context.Context) error { return nil }

func TestNewChromeHTTP2RoundTripperRequiresDialer(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic when dialTLS is nil")
		}
	}()
	_ = NewChromeHTTP2RoundTripper(nil, ChromeHTTP2Options{})
}

func TestChromeHTTP2ClientPrefaceSettingsAndWindowUpdate(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
	})

	type captured struct {
		settings []http2.Setting
		window   uint32
		err      error
	}
	done := make(chan captured, 1)
	go func() {
		var result captured
		defer func() { done <- result }()

		preface := make([]byte, len(http2.ClientPreface))
		if _, err := io.ReadFull(serverConn, preface); err != nil {
			result.err = err
			return
		}
		if string(preface) != http2.ClientPreface {
			result.err = io.ErrUnexpectedEOF
			return
		}
		fr := http2.NewFramer(io.Discard, serverConn)
		frame, err := fr.ReadFrame()
		if err != nil {
			result.err = err
			return
		}
		sf, ok := frame.(*http2.SettingsFrame)
		if !ok {
			result.err = io.ErrUnexpectedEOF
			return
		}
		_ = sf.ForeachSetting(func(s http2.Setting) error {
			result.settings = append(result.settings, s)
			return nil
		})
		frame, err = fr.ReadFrame()
		if err != nil {
			result.err = err
			return
		}
		wu, ok := frame.(*http2.WindowUpdateFrame)
		if !ok {
			result.err = io.ErrUnexpectedEOF
			return
		}
		result.window = wu.Increment
	}()

	rt := NewChromeHTTP2RoundTripper(func(context.Context, string, string) (net.Conn, error) {
		return fakeH2TLSConn{Conn: clientConn}, nil
	}, ChromeHTTP2Options{
		ReadIdleTimeout: time.Second,
		PingTimeout:     time.Second,
	})
	if !IsChromeHTTP2RoundTripper(rt) {
		t.Fatal("expected chrome HTTP/2 round tripper")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", nil)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _, _ = rt.RoundTrip(req) }()

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("capture failed: %v", got.err)
		}
		want := []http2.Setting{
			{ID: http2.SettingHeaderTableSize, Val: ChromeHTTP2HeaderTableSize},
			{ID: http2.SettingEnablePush, Val: ChromeHTTP2EnablePush},
			{ID: http2.SettingMaxConcurrentStreams, Val: ChromeHTTP2MaxConcurrentStreams},
			{ID: http2.SettingInitialWindowSize, Val: ChromeHTTP2InitialWindowSize},
			{ID: http2.SettingMaxHeaderListSize, Val: ChromeHTTP2MaxHeaderListSize},
		}
		if len(got.settings) != len(want) {
			t.Fatalf("settings count = %d, want %d (%v)", len(got.settings), len(want), got.settings)
		}
		for i := range want {
			if got.settings[i] != want[i] {
				t.Fatalf("setting[%d] = %v, want %v", i, got.settings[i], want[i])
			}
		}
		if got.window != ChromeHTTP2ConnectionFlow {
			t.Fatalf("WINDOW_UPDATE = %d, want %d", got.window, ChromeHTTP2ConnectionFlow)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for Chrome HTTP/2 preface")
	}
}

func TestChromeHTTP2HeaderOrderKeys(t *testing.T) {
	if chromeHTTP2PseudoHeaderOrder[0] != ":method" || chromeHTTP2PseudoHeaderOrder[1] != ":authority" {
		t.Fatalf("unexpected pseudo order %v", chromeHTTP2PseudoHeaderOrder)
	}
	if chromeHTTP2HeaderOrder[0] != "host" || chromeHTTP2HeaderOrder[1] != "authorization" {
		t.Fatalf("unexpected header order %v", chromeHTTP2HeaderOrder)
	}
}

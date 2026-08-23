package tlsfingerprint

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

func TestPrefixConnReadsLeftoverThenUnderlying(t *testing.T) {
	server, client := net.Pipe()
	t.Cleanup(func() {
		_ = server.Close()
		_ = client.Close()
	})
	wrapped := &prefixConn{Conn: client, r: io.MultiReader(bytes.NewReader([]byte("AB")), client)}
	go func() {
		_, _ = server.Write([]byte("CD"))
		_ = server.Close()
	}()

	got, err := io.ReadAll(wrapped)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "ABCD" {
		t.Fatalf("got %q, want ABCD", got)
	}
}

func TestHTTPProxyTunnelSendsCONNECTAndPreservesLeftover(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	type seen struct {
		host string
		auth string
	}
	gotCh := make(chan seen, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		br := bufio.NewReader(conn)
		req, err := http.ReadRequest(br)
		if err != nil {
			return
		}
		gotCh <- seen{host: req.Host, auth: req.Header.Get("Proxy-Authorization")}
		_, _ = conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\nLEFTOVER"))
	}()

	proxyURL, err := url.Parse("http://user:secret@" + ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	dialer := NewHTTPProxyDialer(&Profile{Name: "test"}, proxyURL)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := dialer.establishHTTPProxyTunnel(ctx, "chatgpt.com:443")
	if err != nil {
		t.Fatalf("establish tunnel: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	select {
	case got := <-gotCh:
		if got.host != "chatgpt.com:443" {
			t.Fatalf("CONNECT host = %q", got.host)
		}
		if got.auth == "" {
			t.Fatal("missing Proxy-Authorization")
		}
	case <-ctx.Done():
		t.Fatal("proxy did not receive CONNECT")
	}

	buf := make([]byte, 8)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != "LEFTOVER" {
		t.Fatalf("leftover = %q", buf[:n])
	}
}

func TestHTTPSProxyTunnelHandshakesThenCONNECT(t *testing.T) {
	certPEM, keyPEM, err := generateTestProxyCert()
	if err != nil {
		t.Fatal(err)
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(certPEM) {
		t.Fatal("append test CA")
	}

	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	var sawCONNECT atomic.Bool
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		req, err := http.ReadRequest(bufio.NewReader(conn))
		if err != nil {
			return
		}
		if req.Method == http.MethodConnect && req.Host == "api.openai.com:443" {
			sawCONNECT.Store(true)
		}
		_, _ = conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	}()

	proxyURL, err := url.Parse("https://user:secret@" + ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	dialer := NewHTTPProxyDialer(&Profile{Name: "test"}, proxyURL)
	dialer.proxyTLSConfig = &tls.Config{RootCAs: pool}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := dialer.establishHTTPProxyTunnel(ctx, "api.openai.com:443")
	if err != nil {
		t.Fatalf("https proxy tunnel: %v", err)
	}
	_ = conn.Close()
	if !sawCONNECT.Load() {
		t.Fatal("HTTPS proxy did not receive CONNECT after TLS handshake")
	}
}

func TestHTTPSProxyPlaintextPortReturnsHandshakeError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		_, _ = conn.Write([]byte("NOT-TLS"))
		_ = conn.Close()
	}()

	proxyURL, err := url.Parse("https://" + ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	dialer := NewHTTPProxyDialer(&Profile{Name: "test"}, proxyURL)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = dialer.establishHTTPProxyTunnel(ctx, "api.openai.com:443")
	if err == nil {
		t.Fatal("expected TLS handshake error against plaintext proxy")
	}
	if want := "tls handshake to https proxy"; !containsString(err.Error(), want) {
		t.Fatalf("error %q does not contain %q", err, want)
	}
}

func generateTestProxyCert() (certPEM, keyPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}),
		nil
}

func containsString(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || bytes.Contains([]byte(s), []byte(sub)))
}

func TestSOCKS5DialRespectsCanceledContext(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = io.Copy(io.Discard, conn)
	}()

	proxyURL, err := url.Parse("socks5://" + ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	dialer := NewSOCKS5ProxyDialer(&Profile{Name: "test"}, proxyURL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	_, err = dialer.DialTLSContext(ctx, "tcp", "chatgpt.com:443")
	if err == nil {
		t.Fatal("expected canceled SOCKS5 dial to fail")
	}
	if time.Since(start) > time.Second {
		t.Fatalf("canceled SOCKS5 dial took %s", time.Since(start))
	}
}

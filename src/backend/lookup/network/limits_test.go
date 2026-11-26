package network

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"typonamer/config"
	"typonamer/lookup/lookuperror"
)

func TestAnonymousLookupsLeaveCapacity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	public := WithPublic(ctx)
	for i := 0; i < 3; i++ {
		release, err := Acquire(public, "rdap")
		if err != nil {
			t.Fatal(err)
		}
		defer release()
	}

	blocked, stop := context.WithTimeout(public, 60*time.Millisecond)
	defer stop()
	if release, err := Acquire(blocked, "rdap"); !errors.Is(err, context.DeadlineExceeded) {
		if release != nil {
			release()
		}
		t.Fatalf("fourth anonymous request should wait, got %v", err)
	}

	release, err := Acquire(ctx, "rdap")
	if err != nil {
		t.Fatalf("authenticated work could not use the fourth slot: %v", err)
	}
	defer release()
}

func TestRetryAfterAndCancellation(t *testing.T) {
	RetryAfter("rdap", "fixture", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	if Wait(ctx, "rdap", "fixture") != context.DeadlineExceeded {
		t.Fatal("ignored Retry-After or deadline")
	}
	until := time.Now().Add(time.Second).UTC().Format(time.RFC1123)
	RetryAfter("rdap", "date-fixture", until)
	ctx2, cancel2 := context.WithCancel(context.Background())
	cancel2()
	if Wait(ctx2, "rdap", "date-fixture") != context.Canceled {
		t.Fatal("ignored cancellation")
	}

	// Oversized Retry-After headers are capped so an upstream cannot stall workers indefinitely.
	RetryAfter("rdap", "capped-fixture", "86400")
	mu.Lock()
	remaining := time.Until(next["rdap:capped-fixture"])
	delete(next, "rdap:capped-fixture")
	mu.Unlock()
	if remaining <= 0 || remaining > maxRetryAfter+time.Second {
		t.Fatalf("RetryAfter was not bounded: %v", remaining)
	}
}

func TestResponseBoundaries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, strings.Repeat("x", 2*1024*1024+1)) }))
	defer server.Close()
	client := &http.Client{Transport: Transport{Base: http.DefaultTransport, Protocol: "fixture"}}
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if _, err = io.ReadAll(resp.Body); err == nil {
		t.Fatal("unbounded response")
	}
	tls := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("untrusted")) }))
	defer tls.Close()
	if resp, err := client.Get(tls.URL); err == nil {
		resp.Body.Close()
		t.Fatal("accepted untrusted TLS certificate")
	}
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer slow.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", slow.URL, nil)
	if resp, err := client.Do(req); err == nil {
		resp.Body.Close()
		t.Fatal("ignored timeout")
	}
}

func TestSOCKS5RequiresConfiguredHostAndPort(t *testing.T) {
	for _, cfg := range []config.Config{
		{SocketProxyHost: "  ", SocketProxyPort: 1080},
		{SocketProxyHost: "127.0.0.1", SocketProxyPort: 0},
		{SocketProxyHost: "::1", SocketProxyPort: 65536},
		{SocketProxyHost: "http://proxy.example", SocketProxyPort: 22228},
	} {
		if _, err := SOCKS5Dialer(cfg); !errors.Is(err, lookuperror.ErrorProxyNotConfigured) {
			t.Fatalf("invalid endpoint accepted: %v", err)
		}
	}
}

func TestAcquireWakesImmediatelyOnRelease(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	releases := make([]func(), 0, 4)
	for i := 0; i < 4; i++ {
		rel, err := Acquire(ctx, "rdap")
		if err != nil {
			t.Fatal(err)
		}
		releases = append(releases, rel)
	}
	defer func() {
		for _, rel := range releases {
			if rel != nil {
				rel()
			}
		}
	}()

	acquired := make(chan time.Time, 1)
	go func() {
		rel, err := Acquire(ctx, "rdap")
		if err != nil {
			t.Errorf("waiter failed: %v", err)
			return
		}
		acquired <- time.Now()
		rel()
	}()
	time.Sleep(15 * time.Millisecond)
	releasedAt := time.Now()
	releases[0]()
	releases[0] = nil

	select {
	case got := <-acquired:
		if got.Sub(releasedAt) > 200*time.Millisecond {
			t.Fatalf("event-driven acquire woke too slowly: %v", got.Sub(releasedAt))
		}
	case <-time.After(time.Second):
		t.Fatal("waiter did not wake after release")
	}
}

func TestProxyErrorClassificationAndSanitization(t *testing.T) {
	secret := "super-secret-proxy-pass"
	cases := []struct {
		raw  string
		want string
	}{
		{"socks connect tcp 1.2.3.4:44445->a:443: unexpected protocol version 72", "HTTP 代理端口"},
		{"socks connect tcp 1.2.3.4:22228->a:443: authentication failed", "认证失败"},
		{"socks connect tcp 1.2.3.4:22228->a:443: unknown error connection not allowed by ruleset", "代理拒绝连接"},
		{"dial error with " + secret, "[redacted]"},
	}
	for _, tc := range cases {
		err := classifyProxyError(errors.New(tc.raw), secret)
		if !errors.Is(err, lookuperror.ErrorConnectToProxy) || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), secret) {
			t.Fatalf("unexpected proxy classification for %q: %v", tc.raw, err)
		}
	}

	// Point SOCKS5 dialer at a TCP listener that replies HTTP/1.1 400 (simulating an HTTP proxy port like 44445).
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, e := ln.Accept()
		if e != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 8)
		_, _ = conn.Read(buf)
		_, _ = conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
	}()
	tcpAddr := ln.Addr().(*net.TCPAddr)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = TestSOCKS5Proxy(ctx, config.Config{SocketProxyHost: tcpAddr.IP.String(), SocketProxyPort: tcpAddr.Port})
	if !errors.Is(err, lookuperror.ErrorConnectToProxy) || !strings.Contains(err.Error(), "HTTP 代理端口") {
		t.Fatalf("expected HTTP-port mismatch error, got: %v", err)
	}
}

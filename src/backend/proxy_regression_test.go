package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestLoginLimiterUsesForwardedClientBehindTrustedProxy guards finding 1.1:
// behind Caddy, the login limiter must count each forwarded client separately
// instead of putting every visitor in the proxy's bucket.
func TestLoginLimiterUsesForwardedClientBehindTrustedProxy(t *testing.T) {
	t.Setenv("POWERWHOIS_TRUSTED_PROXIES", "127.0.0.1")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	app, _ := newApp(ctx, nil, nil)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go app.Listener(ln)
	defer app.ShutdownWithContext(ctx)

	login := func(client string) int {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+ln.Addr().String()+"/api/login",
			strings.NewReader(`{"username":"nobody","password":"test-only-wrong"}`))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-For", client+", 127.0.0.1")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return resp.StatusCode
	}
	for i := 0; i < 10; i++ {
		if code := login("203.0.113.7"); code != http.StatusUnauthorized {
			t.Fatalf("attempt %d = HTTP %d, want 401", i+1, code)
		}
	}
	if code := login("203.0.113.7"); code != http.StatusTooManyRequests {
		t.Fatalf("eleventh failure from one client = HTTP %d, want 429", code)
	}
	// A front proxy that appends to X-Forwarded-For keeps whatever the client
	// wrote on the left. A forged entry there must not escape the lockout.
	if code := login("203.0.113.99, 203.0.113.7"); code != http.StatusTooManyRequests {
		t.Fatalf("forged leftmost X-Forwarded-For escaped the lockout: HTTP %d, want 429", code)
	}
	if code := login("10.0.0.5, 203.0.113.7"); code != http.StatusTooManyRequests {
		t.Fatalf("forged private X-Forwarded-For escaped the lockout: HTTP %d, want 429", code)
	}
	if code := login("203.0.113.8"); code != http.StatusUnauthorized {
		t.Fatalf("another client was locked out: HTTP %d, want 401", code)
	}
}

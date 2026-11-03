package network

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

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

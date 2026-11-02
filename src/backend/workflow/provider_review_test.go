package workflow

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestProviderReviewHostNormalizationAndCancellation(t *testing.T) {
	g := &hostGate{next: map[string]time.Time{}, interval: time.Millisecond}
	g.deferHost("ReGiStRy.Example:443", time.Minute)
	for _, host := range []string{"registry.example", "REGISTRY.EXAMPLE:443", "registry.example.:0443"} {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		err := g.wait(ctx, host)
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("%s bypassed shared cooldown: %v", host, err)
		}
	}
	if err := g.wait(context.Background(), "registry.example:8443"); err != nil {
		t.Fatalf("separate port should remain independent: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := g.wait(ctx, "unused.example"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled caller acquired a free provider slot: %v", err)
	}
}

func TestProviderReview429And503BodyFailuresKeepCooldown(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		broken bool
	}{
		{"oversized429", http.StatusTooManyRequests, false},
		{"truncated429", http.StatusTooManyRequests, true},
		{"oversized503", http.StatusServiceUnavailable, false},
		{"truncated503", http.StatusServiceUnavailable, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Retry-After", "60")
				if tc.broken {
					w.Header().Set("Content-Length", "100000")
				}
				w.WriteHeader(tc.status)
				if tc.broken {
					fmt.Fprint(w, "partial")
				} else {
					fmt.Fprint(w, strings.Repeat("x", (128<<10)+1))
				}
			}))
			defer server.Close()
			p := NewRDAP(time.Millisecond, time.Second)
			p.client.Transport = server.Client().Transport
			p.endpoints = map[string]string{"com": server.URL + "/domain/"}
			p.maxAttempts = 1
			started := time.Now()
			r := p.Lookup(context.Background(), "first.com")
			if r.Status != "error" || r.HTTPStatus != tc.status || len(r.Attempts) != 1 || r.Attempts[0].Error == "" {
				t.Fatalf("missing response failure evidence: %+v", r)
			}
			u, _ := url.Parse(server.URL)
			p.gate.mu.Lock()
			until := p.gate.next[providerHost(u.Host)]
			p.gate.mu.Unlock()
			if until.Before(started.Add(59 * time.Second)) {
				t.Fatalf("body failure lost Retry-After: %v", until)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			r = p.Lookup(ctx, "second.com")
			if calls.Load() != 1 || len(r.Attempts) != 0 || !strings.Contains(r.Error, "context deadline exceeded") {
				t.Fatalf("another query bypassed cooldown/cancellation: calls=%d result=%+v", calls.Load(), r)
			}
		})
	}
}

func TestProviderReview429HeadersApplyBeforeBodyRead(t *testing.T) {
	headers := make(chan struct{})
	releaseBody := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
		w.(http.Flusher).Flush()
		close(headers)
		<-releaseBody
		fmt.Fprint(w, `{"errorCode":429}`)
	}))
	defer server.Close()
	defer close(releaseBody)
	p := NewRDAP(time.Millisecond, time.Second)
	p.client.Transport = server.Client().Transport
	p.endpoints = map[string]string{"com": server.URL + "/domain/"}
	p.maxAttempts = 1
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan Result, 1)
	go func() { done <- p.Lookup(ctx, "first.com") }()
	select {
	case <-headers:
	case <-time.After(time.Second):
		t.Fatal("response headers were not received")
	}
	u, _ := url.Parse(server.URL)
	deadline := time.Now().Add(time.Second)
	for {
		p.gate.mu.Lock()
		until := p.gate.next[providerHost(u.Host)]
		p.gate.mu.Unlock()
		if until.After(time.Now().Add(50 * time.Second)) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("shared cooldown was not applied while response body was blocked")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case r := <-done:
		if r.Status != "error" || r.HTTPStatus != 429 || len(r.Attempts) != 1 {
			t.Fatalf("canceled body read lost response evidence: %+v", r)
		}
	case <-time.After(time.Second):
		t.Fatal("body read did not stop on cancellation")
	}
}

func TestProviderReviewRedirectRejectsHTTPDowngrade(t *testing.T) {
	var calls atomic.Int32
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
	}))
	defer plain.Close()
	redirect := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	p := NewRDAP(time.Millisecond, time.Second)
	p.client.Transport = redirect.Client().Transport
	p.endpoints = map[string]string{"com": redirect.URL + "/domain/"}
	p.maxAttempts = 1
	r := p.Lookup(context.Background(), "first.com")
	if calls.Load() != 0 || r.Status != "error" || !strings.Contains(r.Error, "unsafe RDAP redirect") {
		t.Fatalf("HTTPS redirect verification was bypassed: calls=%d result=%+v", calls.Load(), r)
	}
}

type providerReviewObserved struct {
	*RDAP
	entered, returned chan string
}

func (p *providerReviewObserved) Lookup(ctx context.Context, domain string) Result {
	p.entered <- domain
	r := p.RDAP.Lookup(ctx, domain)
	p.returned <- domain
	return r
}

func TestProviderReviewRedirectCooldownSharedAcrossJobs(t *testing.T) {
	arrivals := make(chan string, 4)
	final := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		domain := strings.TrimPrefix(r.URL.Path, "/domain/")
		arrivals <- domain
		if domain == "first.com" {
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"errorCode":429}`)
			return
		}
		fmt.Fprintf(w, `{"objectClassName":"domain","ldhName":%q}`, domain)
	}))
	defer final.Close()
	redirect := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	p := NewRDAP(time.Millisecond, 2*time.Second)
	p.client.Transport = final.Client().Transport
	p.endpoints = map[string]string{"com": redirect.URL + "/domain/", "net": final.URL + "/domain/"}
	p.maxAttempts = 1
	observed := &providerReviewObserved{p, make(chan string, 4), make(chan string, 4)}
	s := testStore(t)
	svc, err := NewService(s, observed, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	jobs := make([]Job, 0, 2)
	for _, domain := range []string{"first.com", "second.net"} {
		j := imported(t, s, domain)
		j, err = svc.Generate(context.Background(), j.ID, GenerateOptions{Rules: []string{"www"}, IncludeSeeds: true, MaxCandidates: 1})
		if err != nil {
			t.Fatal(err)
		}
		jobs = append(jobs, j)
	}
	if _, err = svc.Action(jobs[0].ID, "start"); err != nil {
		t.Fatal(err)
	}
	waitJob(t, s, jobs[0].ID, "completed")
	<-observed.entered
	<-observed.returned
	select {
	case domain := <-arrivals:
		if domain != "first.com" {
			t.Fatalf("unexpected first request: %s", domain)
		}
	case <-time.After(time.Second):
		t.Fatal("first job did not follow the HTTPS redirect")
	}
	items, _, err := s.Page(jobs[0].ID, "items", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	wantProvider := final.URL + "/domain/first.com"
	if !strings.Contains(string(items[0]), `"provider":"`+wantProvider+`"`) || strings.Contains(string(items[0]), redirect.URL) {
		t.Fatalf("redirect evidence does not name actual final provider: %s", items[0])
	}
	u, _ := url.Parse(final.URL)
	p.gate.mu.Lock()
	until := p.gate.next[providerHost(u.Host)]
	p.gate.mu.Unlock()
	if until.Before(time.Now().Add(50 * time.Second)) {
		t.Fatalf("redirected 429 cooled down the wrong provider: %v", until)
	}
	if _, err = svc.Action(jobs[1].ID, "start"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-observed.entered:
	case <-time.After(time.Second):
		t.Fatal("second job did not reach provider")
	}
	select {
	case domain := <-arrivals:
		t.Fatalf("other job bypassed final provider cooldown: %s", domain)
	case <-time.After(30 * time.Millisecond):
	}
	if _, err = svc.Action(jobs[1].ID, "cancel"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-observed.returned:
	case <-time.After(time.Second):
		t.Fatal("cancel did not interrupt redirected provider's shared wait")
	}
}

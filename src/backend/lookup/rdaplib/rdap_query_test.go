package rdaplib

import (
	"errors"
	"github.com/openrdap/rdap"
	"github.com/openrdap/rdap/bootstrap"
	"github.com/openrdap/rdap/bootstrap/cache"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"typonamer/lookup/lookuperror"
	"typonamer/lookup/lookupinfo"
	"typonamer/lookup/network"
)

func TestRegistryResponses(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		code       int
		want       error
	}{
		{"redacted", `{"objectClassName":"domain","ldhName":"example.com"}`, 200, nil},
		{"missing", `{"errorCode":404}`, 404, lookuperror.ErrorWhoisNotFound},
		{"empty negative answer", "", 404, lookuperror.ErrorWhoisNotFound},
		{"wrong domain", `{"objectClassName":"domain","ldhName":"other.com"}`, 200, lookuperror.ErrorParseWhoisResponse},
		{"server failed", `{"errorCode":500}`, 500, lookuperror.ErrorWhoisServerFailed},
		{"rate limit", `{"errorCode":429}`, 429, lookuperror.ErrorRateLimited},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/registry" {
					http.Redirect(w, r, "/registry", http.StatusTemporaryRedirect)
					return
				}
				w.Header().Set("Content-Type", "application/rdap+json")
				w.WriteHeader(tc.code)
				w.Write([]byte(tc.body))
			}))
			defer server.Close()
			address, _ := url.Parse(server.URL)
			rc := rdap.Client{HTTP: &http.Client{Transport: network.Transport{Base: http.DefaultTransport, Protocol: "test-rdap"}}}
			resp, err := rc.Do(&rdap.Request{Type: rdap.DomainRequest, Query: "example.com", Server: address})
			// A failed alternate must not replace the next registry's answer, even when its body is empty.
			resp.HTTP = append([]*rdap.HTTPResponse{{URL: "https://alternate.invalid/", Response: &http.Response{StatusCode: 429}, Body: []byte("slow down")}}, resp.HTTP...)
			info, err := parseResponse("example.com", lookupinfo.DomainInfo{LookupType: "rdap"}, resp, err)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
			if info.Source != server.URL+"/registry" || info.RawResponse != tc.body {
				t.Fatalf("wrong response evidence: %+v", info)
			}
		})
	}
}

func TestHTTP404RequiresRegistryEvidence(t *testing.T) {
	for _, tc := range []struct {
		mediaType, body string
		want            error
	}{
		{"text/html", "<html>Page not found</html>", lookuperror.ErrorWhoisServerFailed},
		{"application/json", `{"message":"unknown route"}`, lookuperror.ErrorWhoisServerFailed},
		{"application/json", "", lookuperror.ErrorWhoisServerFailed},
		{"application/rdap+json", `{"errorCode":500}`, lookuperror.ErrorWhoisServerFailed},
		{"application/jsonp", "", lookuperror.ErrorWhoisServerFailed},
		{"application/json; charset=utf-8", `{"errorCode":404}`, lookuperror.ErrorWhoisNotFound},
	} {
		t.Run(tc.mediaType, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.mediaType)
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(tc.body))
			}))
			defer server.Close()
			address, _ := url.Parse(server.URL)
			rc := rdap.Client{HTTP: server.Client()}
			resp, err := rc.Do(&rdap.Request{Type: rdap.DomainRequest, Query: "example.com", Server: address})
			info, err := parseResponse("example.com", lookupinfo.DomainInfo{}, resp, err)
			if !errors.Is(err, tc.want) || info.RawResponse != tc.body {
				t.Fatalf("got %v, body %q; want %v, %q", err, info.RawResponse, tc.want, tc.body)
			}
		})
	}
}

func TestEventDatesUseUTCAndIgnoreMalformedEvents(t *testing.T) {
	created, expires := getDomainDates([]rdap.Event{
		{Action: "registration", Date: "2026-01-02T00:00:01.123+08:00"},
		{Action: "registration", Date: "invalid"},
		{Action: "expiration", Date: "2027-01-02T03:04:05Z"},
	})
	if created != "2026-01-01 16:00:01" || expires != "2027-01-02 03:04:05" {
		t.Fatalf("unexpected dates %q, %q", created, expires)
	}
}

func TestSharedBootstrapCacheConcurrentAccess(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sharedBootstrapCache.SetTimeout(time.Hour)
			_ = sharedBootstrapCache.Save("dns.json", []byte(`{"version":"1.0"}`))
			_ = sharedBootstrapCache.State("dns.json")
			_, _ = sharedBootstrapCache.Load("dns.json")
		}()
	}
	wg.Wait()
}

func TestSharedBootstrapCacheAvoidsRedownloadAcrossClients(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"version":"1.0","services":[[["com"],["https://rdap.verisign.com/com/v1/"]]]}`))
	}))
	defer srv.Close()
	baseURL, _ := url.Parse(srv.URL)
	memCache := &safeRegistryCache{inner: cache.NewMemoryCache()}

	for i := 0; i < 3; i++ {
		bc := &bootstrap.Client{HTTP: srv.Client(), BaseURL: baseURL, Cache: memCache}
		ans, err := bc.Lookup(&bootstrap.Question{RegistryType: bootstrap.DNS, Query: "example.com"})
		if err != nil || len(ans.URLs) == 0 {
			t.Fatalf("attempt %d failed: %+v, %v", i, ans, err)
		}
	}
	if hits.Load() != 1 {
		t.Fatalf("expected 1 bootstrap download across 3 fresh clients, got %d", hits.Load())
	}
}

func TestParseResponseHandlesRDAP404CloseErrorAndProxyErrors(t *testing.T) {
	// Verified TLS 404 with application/rdap+json and a body-close TLS error (Verisign quirk) -> not_found.
	resp404 := &rdap.Response{
		HTTP: []*rdap.HTTPResponse{{
			URL: "https://rdap.verisign.com/com/v1/domain/missing.com",
			Response: &http.Response{
				StatusCode: http.StatusNotFound,
				Header:     http.Header{"Content-Type": []string{"application/rdap+json"}},
			},
			Error: errors.New("remote error: tls: bad record MAC"),
		}},
	}
	_, err := parseResponse("missing.com", lookupinfo.DomainInfo{LookupType: "rdap"}, resp404, &rdap.ClientError{Type: rdap.NoWorkingServers})
	if !errors.Is(err, lookuperror.ErrorWhoisNotFound) {
		t.Fatalf("expected ErrorWhoisNotFound for RDAP 404 with TLS close error, got %v", err)
	}

	// Non-RDAP HTML 404 with body error must NOT be treated as not_found.
	respHTML404 := &rdap.Response{
		HTTP: []*rdap.HTTPResponse{{
			URL: "https://broken.invalid/domain/missing.com",
			Response: &http.Response{
				StatusCode: http.StatusNotFound,
				Header:     http.Header{"Content-Type": []string{"text/html"}},
			},
			Error: errors.New("unexpected EOF"),
		}},
	}
	_, err = parseResponse("missing.com", lookupinfo.DomainInfo{LookupType: "rdap"}, respHTML404, &rdap.ClientError{Type: rdap.NoWorkingServers})
	if !errors.Is(err, lookuperror.ErrorWhoisServerFailed) {
		t.Fatalf("HTML 404 with read error should stay server failure, got %v", err)
	}

	// Proxy dial failure inside openrdap NoWorkingServers must propagate ErrorConnectToProxy.
	respProxyErr := &rdap.Response{
		HTTP: []*rdap.HTTPResponse{{
			URL:   "https://rdap.verisign.com/com/v1/domain/example.com",
			Error: errors.Join(lookuperror.ErrorConnectToProxy, errors.New("auth failed")),
		}},
	}
	_, err = parseResponse("example.com", lookupinfo.DomainInfo{LookupType: "rdap"}, respProxyErr, &rdap.ClientError{Type: rdap.NoWorkingServers})
	if !errors.Is(err, lookuperror.ErrorConnectToProxy) {
		t.Fatalf("proxy failure was masked: %v", err)
	}

	// Network timeout inside NoWorkingServers must preserve net.Error for retry/timeout classification.
	timeoutErr := &net.DNSError{Err: "i/o timeout", IsTimeout: true}
	respNetErr := &rdap.Response{
		HTTP: []*rdap.HTTPResponse{{
			URL:   "https://rdap.verisign.com/com/v1/domain/example.com",
			Error: timeoutErr,
		}},
	}
	_, err = parseResponse("example.com", lookupinfo.DomainInfo{LookupType: "rdap"}, respNetErr, &rdap.ClientError{Type: rdap.NoWorkingServers})
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("net.Error timeout lost from NoWorkingServers: %v", err)
	}
}

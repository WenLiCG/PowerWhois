package rdaplib

import (
	"errors"
	"github.com/openrdap/rdap"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
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
		{"wrong domain", `{"objectClassName":"domain","ldhName":"other.com"}`, 200, lookuperror.ErrorParseWhoisResponse},
		{"server failed", `{"errorCode":500}`, 500, lookuperror.ErrorWhoisServerFailed},
		{"rate limit", `{"errorCode":429}`, 429, lookuperror.ErrorRateLimited},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/rdap+json")
				w.WriteHeader(tc.code)
				w.Write([]byte(tc.body))
			}))
			defer server.Close()
			address, _ := url.Parse(server.URL)
			rc := rdap.Client{HTTP: &http.Client{Transport: network.Transport{Base: http.DefaultTransport, Protocol: "test-rdap"}}}
			resp, err := rc.Do(&rdap.Request{Type: rdap.DomainRequest, Query: "example.com", Server: address})
			info, err := parseResponse("example.com", lookupinfo.DomainInfo{LookupType: "rdap"}, resp, err)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
			if info.Source == "" {
				t.Fatal("source missing")
			}
		})
	}
}

package lookuper

import (
	"context"
	"errors"
	"testing"
	"typonamer/constant"
	"typonamer/lookup/lookuperror"
	"typonamer/lookup/lookupinfo"
)

func TestStatusEvidence(t *testing.T) {
	cases := []struct {
		name, kind string
		err        error
		ns         []string
		want       string
	}{
		{"DNS absence", constant.LookupTypeDNS, lookuperror.ErrorNsNotFound, nil, "unknown"},
		{"DNS failure", constant.LookupTypeDNS, lookuperror.ErrorDnsServerFailed, nil, "error"},
		{"timeout", constant.LookupTypeRDAP, context.DeadlineExceeded, nil, "timeout"},
		{"explicit not found", constant.LookupTypeRDAP, lookuperror.ErrorWhoisNotFound, nil, "not_found"},
		{"redacted positive", constant.LookupTypeRDAP, nil, nil, "registered"},
		{"DNS delegated", constant.LookupTypeDNS, nil, []string{"ns.example.com"}, "registered"},
		{"parse error", constant.LookupTypeWhois, lookuperror.ErrorParseWhoisResponse, nil, "unknown"},
		{"unhandled error", "custom", errors.New("bad response"), nil, "error"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := Result("example.com", lookupinfo.DomainInfo{LookupType: c.kind, NameServer: c.ns, Source: "fixture"}, c.err)
			if r.Status != c.want || r.Source != "fixture" || r.QueriedAt == "" {
				t.Fatalf("%+v", r)
			}
		})
	}
}

func TestUnsupportedTLDKeepsLocalEvidence(t *testing.T) {
	for _, mode := range []string{constant.WhoisQuery, constant.WhoisQueryWithProxy} {
		r := Query(context.Background(), "google.cf", mode, 1, 0)
		if r.Status != "unknown" || r.Source != "local-validation" || r.QueriedAt == "" || r.QueryError == "" {
			t.Fatalf("unsupported route lacks evidence: %+v", r)
		}
	}
}

func TestDelegatedAcademicUKDoesNotUseNominetAbsence(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // A local coverage decision must not require any network request.
	for _, domain := range []string{"ox.ac.uk", "google.ac.uk", "missing-test.ac.uk"} {
		info, err := Whois(ctx, domain, "uk", false)
		r := Result(domain, info, err)
		if !errors.Is(err, lookuperror.ErrorNotSupportedTld) || r.Status != "unknown" || r.Source != "local-validation" {
			t.Fatalf("delegated namespace incorrectly classified: %+v %v", r, err)
		}
	}
}

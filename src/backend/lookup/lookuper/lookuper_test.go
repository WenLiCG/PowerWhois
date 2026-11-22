package lookuper

import (
	"testing"

	"typonamer/config"
	"typonamer/constant"
	"typonamer/lookup/lookuperror"
	"typonamer/lookup/lookupinfo"
)

func TestResultRequiresRegistrationEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, mode, want string
		err              error
	}{
		{"DNS without NS", "dns", "Unknown", lookuperror.ErrorNsNotFound},
		{"DNS failed", "dns", "Error", lookuperror.ErrorDnsServerFailed},
		{"DNS timeout", "dns", "Error", lookuperror.ErrorDnsTimeout},
		{"RDAP object with redacted fields", "rdap", "Taken", nil},
		{"registry says reserved", "whois", "Reserved", lookuperror.ErrorReservedDomain},
		{"registry says not found", "whois", "Free", lookuperror.ErrorWhoisNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Result("example.com", lookupinfo.DomainInfo{LookupType: tc.mode}, tc.err); got.RegisterStatus != tc.want {
				t.Fatalf("status = %s, want %s", got.RegisterStatus, tc.want)
			}
		})
	}
	cfg := config.Config{WhoisApis: []config.WhoisApi{{ApiName: "private"}}}
	if ValidMode("private", false, cfg) || ValidMode(constant.WhoisQueryWithProxy, false, cfg) ||
		!ValidMode(constant.MixedQuery, false, cfg) || !ValidMode("private", true, cfg) {
		t.Fatal("unexpected lookup mode permissions")
	}
}

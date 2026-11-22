package rdaplib

import (
	"errors"
	"net/http"
	"testing"
	"typonamer/lookup/lookuperror"
	"typonamer/lookup/lookupinfo"
)

func TestRDAPDomainWithoutOptionalFields(t *testing.T) {
	result := lookupinfo.DomainInfo{DomainName: "example.com", LookupType: "rdap"}
	body := "{\"rdapConformance\":[\"rdap_level_0\"],\"objectClassName\":\"domain\",\"ldhName\":\"example.com\"}"
	parsed, err := parseResponse(result, body, http.StatusOK)
	if err != nil || parsed.DomainName != result.DomainName || parsed.RawResponse == "" {
		t.Fatalf("valid but sparse domain response: %+v, %v", parsed, err)
	}
	if _, err := parseResponse(result, "", http.StatusTooManyRequests); !errors.Is(err, lookuperror.ErrorRateLimited) {
		t.Fatalf("429 = %v", err)
	}
}

package whoislib

import (
	"errors"
	"regexp"
	"testing"
	"typonamer/lookup/lookuperror"
)

func TestOrdinalDatesDoNotCorruptMonthNames(t *testing.T) {
	matcher := WhoisInfoMatcher{ReCreationDate: regexp.MustCompile(`Created: (.*)`), ReExpiryDate: regexp.MustCompile(`Expires: (.*)`), DateTimeLayout: "2 January 2006"}
	info, e := ParseWhoisResponse("Created: 1st August 2020\nExpires: 23rd August 2027", "example.com", matcher)
	if e != nil || info.CreationDate != "2020-08-01 00:00:00" || info.ExpiryDate != "2027-08-23 00:00:00" {
		t.Fatalf("date parsing %+v %v", info, e)
	}
}

func TestReservedResponsesAreNotAbsence(t *testing.T) {
	for tld, response := range map[string]string{"qa": "Reserved by QDR", "ae": "Domain test.ae has been reserved by aeDA Regulator"} {
		_, e := ParseWhoisResponse(response, "test."+tld, WhoisMatchers[tld])
		if !errors.Is(e, lookuperror.ErrorReservedDomain) {
			t.Fatalf("%s: %v", tld, e)
		}
		_, e = ParseWhoisResponse("No Data Found", "test."+tld, WhoisMatchers[tld])
		if e == nil || e.Error() != "domain not found" {
			t.Fatalf("legitimate absence no longer recognized: %v", e)
		}
	}
}

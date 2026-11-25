package whoislib

import (
	"errors"
	"regexp"
	"testing"

	"github.com/dromara/carbon/v2"
	"typonamer/lookup/lookuperror"
	"typonamer/utils"
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

func TestCNRegistrationDatesIgnoreUnavailableProcessTimezone(t *testing.T) {
	carbon.SetDefault(carbon.Default{Timezone: "Unavailable/Zone"})
	t.Cleanup(func() { carbon.SetDefault(carbon.Default{Timezone: carbon.Local}) })
	for _, suffix := range []string{"cn", "中国", "中國"} {
		domain, err := utils.TrimAndGetMainDomain("example." + suffix)
		if err != nil {
			t.Fatal(err)
		}
		tld, _, err := utils.GetTld(domain)
		if err != nil || WhoisSupportedTlds[tld] == "" {
			t.Fatalf("no registry for %s: %v", domain, err)
		}
		info, err := ParseWhoisResponse("Registration Time: 2026-01-02 15:04:05\nExpiration Time: unavailable", domain, WhoisMatchers[tld])
		if err != nil || info.CreationDate != "2026-01-02 15:04:05" || info.ExpiryDate != "" {
			t.Fatalf("dates for %s: %+v, %v", domain, info, err)
		}
	}
}

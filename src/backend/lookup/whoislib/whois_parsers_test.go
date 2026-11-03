package whoislib

import (
	"regexp"
	"testing"
)

func TestOrdinalDatesDoNotCorruptMonthNames(t *testing.T) {
	matcher := WhoisInfoMatcher{ReCreationDate: regexp.MustCompile(`Created: (.*)`), ReExpiryDate: regexp.MustCompile(`Expires: (.*)`), DateTimeLayout: "2 January 2006"}
	info, e := ParseWhoisResponse("Created: 1st August 2020\nExpires: 23rd August 2027", "example.com", matcher)
	if e != nil || info.CreationDate != "2020-08-01 00:00:00" || info.ExpiryDate != "2027-08-23 00:00:00" {
		t.Fatalf("date parsing %+v %v", info, e)
	}
}

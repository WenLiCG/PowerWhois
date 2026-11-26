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

func TestRateLimitedWhoisResponsesAndISOTimezoneOffsets(t *testing.T) {
	for _, sample := range []string{
		"Query rate of 100 queries per hour exceeded for your IP.",
		"% Error: Too many requests, please try again later.",
		"WHOIS limit exceeded",
	} {
		_, err := ParseWhoisResponse(sample, "example.cn", WhoisMatchers["cn"])
		if !errors.Is(err, lookuperror.ErrorRateLimited) {
			t.Fatalf("rate limit text %q misclassified as %v", sample, err)
		}
	}

	// Registered domains whose footers mention rate limits must remain registered.
	registeredWithFooter := "Sponsoring Registrar: Example Corp\nRegistration Time: 2025-05-01T08:00:00+08:00\nExpiration Time: 2027-05-01T08:00:00+08:00\nName Server: ns1.example.cn\n% Notice: query rate limit applies."
	info, err := ParseWhoisResponse(registeredWithFooter, "example.cn", WhoisMatchers["cn"])
	if err != nil || info.CreationDate != "2025-05-01 00:00:00" || info.ExpiryDate != "2027-05-01 00:00:00" {
		t.Fatalf("registered domain with footer or +08:00 offset failed: %+v, %v", info, err)
	}
}

func TestMalformedStatusAndStatusOnlyDomainParsing(t *testing.T) {
	// Malformed status/NS lines must never panic on empty SplitAndTrim slices.
	malformed := "Sponsoring Registrar: Example\nDomain Status: http\nDomain Status: -\nName Server:    \n"
	info, err := ParseWhoisResponse(malformed, "example.cn", WhoisMatchers["cn"])
	if err != nil || info.Registrar != "Example" {
		t.Fatalf("malformed status/NS failed: %+v, %v", info, err)
	}

	// Parked .de domain with Status: connect and no nameservers must remain registered.
	deConnect := "Domain: parked.de\nStatus: connect\n"
	deInfo, err := ParseWhoisResponse(deConnect, "parked.de", WhoisMatchers["de"])
	if err != nil || len(deInfo.DomainStatus) != 1 || deInfo.DomainStatus[0] != "connect" {
		t.Fatalf("status-only .de domain failed: %+v, %v", deInfo, err)
	}
}

func TestLiveRegistryMatcherUpdates(t *testing.T) {
	// .md v3.0.1 taken and free
	mdTaken := "Domain  name:   google.md\nDomain state:   OK\nRegistered on:  2006-05-02\nExpires on:     2027-05-02\nNameServer:     ns1.google.com\nNameServer:     ns2.google.com\n"
	info, err := ParseWhoisResponse(mdTaken, "google.md", WhoisMatchers["md"])
	if err != nil || len(info.NameServer) != 2 || info.CreationDate != "2006-05-02 00:00:00" || info.ExpiryDate != "2027-05-02 00:00:00" {
		t.Fatalf(".md taken failed: %+v, %v", info, err)
	}
	if _, err := ParseWhoisResponse("% Whois Server v3.0.1\n\nNo entries found [ No match for ]\n", "free.md", WhoisMatchers["md"]); err == nil || err.Error() != "domain not found" {
		t.Fatalf(".md free failed: %v", err)
	}

	// .kz KazNIC dotted format
	kzTaken := "Domain Name............: google.kz\nName Server............: ns1.google.com\nName Server............: ns2.google.com\nDomain created.........: 1999-06-07 14:01:43 (GMT+0:00)\nExpiration date........: 2027-06-06 19:00:00 (GMT+0:00)\nDomain status..........: ok - Normal state.\nCurrent Registrar......: KAZNIC\n"
	info, err = ParseWhoisResponse(kzTaken, "google.kz", WhoisMatchers["kz"])
	if err != nil || info.Registrar != "KAZNIC" || len(info.NameServer) != 2 || info.CreationDate != "1999-06-07 14:01:43" || info.ExpiryDate != "2027-06-06 19:00:00" {
		t.Fatalf(".kz taken failed: %+v, %v", info, err)
	}

	// .co, .in, .do, .cat free responses
	freeCases := map[string]string{
		"co":  "The queried object does not exist: DOMAIN NOT FOUND\n",
		"in":  ">>> Domain free.in is available for registration\n",
		"do":  "Domain Name: free.com.do\nThe queried object does not exist: No Object Found\n",
		"cat": "The queried object does not exist: no matching objects found\n",
	}
	for tld, raw := range freeCases {
		if _, err := ParseWhoisResponse(raw, "free."+tld, WhoisMatchers[tld]); err == nil || err.Error() != "domain not found" {
			t.Fatalf(".%s free failed: %v", tld, err)
		}
	}

	// .cat taken
	catTaken := "Domain Name: google.cat\nCreation Date: 2006-02-14T08:55:44.988Z\nRegistry Expiry Date: 2027-02-14T08:55:44.988Z\nRegistrar: MarkMonitor\n"
	info, err = ParseWhoisResponse(catTaken, "google.cat", WhoisMatchers["cat"])
	if err != nil || info.Registrar != "MarkMonitor" || info.CreationDate != "2006-02-14 08:55:44" {
		t.Fatalf(".cat taken failed: %+v, %v", info, err)
	}

	// .nz conflicted/restricted domain maps to reserved rather than parse error
	nzRestricted := "Error code: 01044\nError message: The domain name requested has usage restrictions applied to it.\n"
	if _, err := ParseWhoisResponse(nzRestricted, "google.nz", WhoisMatchers["nz"]); !errors.Is(err, lookuperror.ErrorReservedDomain) {
		t.Fatalf(".nz restricted failed: %v", err)
	}
}

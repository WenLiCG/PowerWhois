package typo

import "testing"

func TestWrongTLDsKeepRequestedDomainLabel(t *testing.T) {
	if _, err := Generate(Request{Domain: "example.com", TLDs: []string{"other.net"}}); err == nil {
		t.Fatal("accepted a registered-domain label as part of a suffix")
	}
	got, err := Generate(Request{Domain: "example.com", TLDs: []string{".co.uk", "中国"}})
	if err != nil || len(got.Domains) != 2 || got.Domains[0] != "example.co.uk" || got.Domains[1] != "example.xn--fiqs8s" {
		t.Fatalf("valid suffixes: %+v, %v", got, err)
	}
}

func TestGeneratedVariantsExcludeTheSourceDomain(t *testing.T) {
	// Swapping "oo" reproduces google.com, and the default suffix list selects .com.
	got, err := Generate(Request{Domain: "Google.com", Types: []string{"reverseLetter"}, TLDs: []string{"com", "net"}})
	if err != nil {
		t.Fatal(err)
	}
	for kind, domains := range got.Groups {
		for _, d := range domains {
			if d == "google.com" {
				t.Fatalf("group %s contains the source domain: %v", kind, domains)
			}
		}
	}
	for _, d := range got.Domains {
		if d == "google.com" {
			t.Fatalf("candidates contain the source domain: %v", got.Domains)
		}
	}
	if len(got.Groups["wrongTlds"]) != 1 || got.Groups["wrongTlds"][0] != "google.net" || len(got.Groups["reverseLetter"]) != 4 {
		t.Fatalf("unexpected variants: %+v", got.Groups)
	}
}

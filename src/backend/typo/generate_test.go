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

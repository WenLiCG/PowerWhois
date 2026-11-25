package jobs

import (
	"strings"
	"testing"
	"typonamer/config"
)

func TestDefaultConcurrency(t *testing.T) {
	for _, mode := range []string{"dns", "whois", "mixed"} {
		r, err := Normalize(Request{Domains: []string{"example.com"}, Mode: mode})
		if err != nil || r.Concurrency != 15 {
			t.Fatalf("%s: %+v %v", mode, r, err)
		}
	}
	c := config.GetConfig()
	if c.DnsConcurrency != 15 || c.WhoisConcurrency != 15 || c.WebCheckConcurrencyLimit != 15 {
		t.Fatalf("protocol defaults: dns=%d whois=%d web=%d", c.DnsConcurrency, c.WhoisConcurrency, c.WebCheckConcurrencyLimit)
	}
}

func TestInvalidDomainErrorNamesTheValue(t *testing.T) {
	_, err := Normalize(Request{Domains: []string{"example.com", "example.org", "bad_domain!.com"}})
	if err == nil || err.Error() != `invalid domain at line 3: "bad_domain!.com"` {
		t.Fatalf("got %v", err)
	}
	_, err = Normalize(Request{Domains: []string{strings.Repeat("域", 200) + " .com"}})
	if err == nil || !strings.HasPrefix(err.Error(), "invalid domain at line 1: ") || len([]rune(err.Error())) > 120 {
		t.Fatalf("long values must be truncated: %v", err)
	}
}

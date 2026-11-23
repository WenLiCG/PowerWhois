package jobs

import (
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

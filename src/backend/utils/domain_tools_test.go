package utils

import "testing"

func TestTrimDomainRejectsIdentityChanges(t *testing.T) {
	for _, raw := range []string{"-example.com", "exa_mple.com", "exam ple.com", "https://example.com", "example..com", "=example.com"} {
		if got, err := TrimDomain(raw); err == nil {
			t.Errorf("accepted invalid input %q as %q", raw, got)
		}
	}
}

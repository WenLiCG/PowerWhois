package whoislib

import "testing"

func TestRateLimitedResponse(t *testing.T) {
	for raw, want := range map[string]bool{
		"%% Query rate limit exceeded. Please try again later.\n": true,
		"Too many requests from your IP\n":                        true,
		"WHOIS LIMIT EXCEEDED - SEE WWW.EXAMPLE.TEST\n":           true,
		"You have exceeded your query quota\n":                    true,
		"Status: free\n":                                          false,
		"No match for \"EXAMPLE.TEST\".\n":                        false,
		"Domain Name: EXAMPLE.TEST\nRegistrar: Example\n":         false,
	} {
		if got := RateLimitedResponse(raw); got != want {
			t.Errorf("RateLimitedResponse(%q) = %v, want %v", raw, got, want)
		}
	}
}

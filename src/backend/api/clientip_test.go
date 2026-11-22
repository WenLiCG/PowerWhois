package api

import (
	"net/netip"
	"strings"
	"testing"
)

func TestResolveClientIPResistsForgedForwardedFor(t *testing.T) {
	compose, err := ParseTrustedProxies("10.0.0.0/8, 172.16.0.0/12 192.168.0.0/16 fc00::/7")
	if err != nil {
		t.Fatal(err)
	}
	caddy := netip.MustParseAddr("172.18.0.3")
	for _, tc := range []struct {
		name      string
		remote    netip.Addr
		forwarded string
		want      string
	}{
		{"untrusted peer ignores the header", netip.MustParseAddr("198.51.100.4"), "10.0.0.1", "198.51.100.4"},
		{"shipped chain: Nginx overwrites, Caddy appends", caddy, "203.0.113.7, 172.18.0.1", "203.0.113.7"},
		{"forged private entry before an appending proxy", caddy, "10.0.0.5, 203.0.113.7, 172.18.0.1", "203.0.113.7"},
		{"forged public entry before an appending proxy", caddy, "198.51.100.1, 203.0.113.7, 172.18.0.1", "203.0.113.7"},
		{"malformed forged entry", caddy, "not-an-ip, 203.0.113.7", "203.0.113.7"},
		{"IPv4-mapped client", caddy, "::ffff:203.0.113.7", "203.0.113.7"},
		{"LAN client behind trusted proxies", caddy, "192.168.1.5, 172.18.0.1", "192.168.1.5"},
		{"no header from a trusted peer", caddy, "", "172.18.0.3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var forwarded []string
			if tc.forwarded != "" {
				forwarded = strings.Split(tc.forwarded, ",")
			}
			if got := resolveClientIP(tc.remote, forwarded, compose).String(); got != tc.want {
				t.Fatalf("client = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestParseTrustedProxies(t *testing.T) {
	defaults, err := ParseTrustedProxies("")
	if err != nil || !isTrusted(netip.MustParseAddr("127.0.0.1"), defaults) || !isTrusted(netip.MustParseAddr("::1"), defaults) ||
		isTrusted(netip.MustParseAddr("10.0.0.1"), defaults) {
		t.Fatalf("unset value should trust loopback only: %v, %v", defaults, err)
	}
	single, err := ParseTrustedProxies("192.0.2.10")
	if err != nil || !isTrusted(netip.MustParseAddr("192.0.2.10"), single) || isTrusted(netip.MustParseAddr("192.0.2.11"), single) {
		t.Fatalf("single address = %v, %v", single, err)
	}
	if _, err := ParseTrustedProxies("10.0.0.0/8 nonsense"); err == nil {
		t.Fatal("accepted an invalid entry")
	}
}

func TestLimitKeyGroupsIPv6By64(t *testing.T) {
	if limitKey("2001:db8:1:2::5") != limitKey("2001:db8:1:2:ffff::9") {
		t.Fatal("addresses in one /64 got different keys")
	}
	if limitKey("2001:db8:1:2::5") == limitKey("2001:db8:1:3::5") {
		t.Fatal("different /64 blocks share a key")
	}
	if limitKey("203.0.113.7") != "203.0.113.7" || limitKey("::ffff:203.0.113.7") != "203.0.113.7" {
		t.Fatal("IPv4 keys should be the address")
	}
}

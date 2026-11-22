package api

import (
	"fmt"
	"net/netip"
	"strings"
	"sync/atomic"

	"github.com/gofiber/fiber/v2"
)

// trustedProxies holds the ranges whose X-Forwarded-For entries are believed.
var trustedProxies atomic.Pointer[[]netip.Prefix]

var loopbackOnly = []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("::1/128")}

// ParseTrustedProxies reads addresses or CIDR ranges separated by spaces or
// commas. An empty value trusts loopback only, which suits a reverse proxy on
// the same host and is harmless without one.
func ParseTrustedProxies(value string) ([]netip.Prefix, error) {
	fields := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' })
	if len(fields) == 0 {
		return loopbackOnly, nil
	}
	prefixes := make([]netip.Prefix, 0, len(fields))
	for _, field := range fields {
		if prefix, err := netip.ParsePrefix(field); err == nil {
			prefixes = append(prefixes, prefix.Masked())
			continue
		}
		addr, err := netip.ParseAddr(field)
		if err != nil {
			return nil, fmt.Errorf("invalid POWERWHOIS_TRUSTED_PROXIES entry %q", field)
		}
		addr = normalizeAddr(addr)
		prefixes = append(prefixes, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return prefixes, nil
}

// SetTrustedProxies replaces the trusted ranges used by ClientIP.
func SetTrustedProxies(prefixes []netip.Prefix) { trustedProxies.Store(&prefixes) }

func currentTrustedProxies() []netip.Prefix {
	if stored := trustedProxies.Load(); stored != nil {
		return *stored
	}
	return loopbackOnly
}

func normalizeAddr(addr netip.Addr) netip.Addr { return addr.Unmap().WithZone("") }

func isTrusted(addr netip.Addr, prefixes []netip.Prefix) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// resolveClientIP returns the nearest address that is not a trusted proxy,
// reading X-Forwarded-For from the right. Each trusted proxy appends the peer
// it saw, so entries a client wrote itself sit on the left and are never
// reached while an untrusted hop remains. This holds whether front proxies
// overwrite the header (as the shipped Nginx file does) or append to it.
func resolveClientIP(remote netip.Addr, forwarded []string, prefixes []netip.Prefix) netip.Addr {
	client := normalizeAddr(remote)
	if !isTrusted(client, prefixes) {
		return client
	}
	for i := len(forwarded) - 1; i >= 0; i-- {
		addr, err := netip.ParseAddr(strings.TrimSpace(forwarded[i]))
		if err != nil {
			// Everything to the right was written by trusted proxies. Do not
			// look past malformed input to entries a client may control.
			break
		}
		client = normalizeAddr(addr)
		if !isTrusted(client, prefixes) {
			return client
		}
	}
	return client
}

// ClientIP is the proxy-aware client address used for rate limits.
func ClientIP(c *fiber.Ctx) string {
	remote, ok := netip.AddrFromSlice(c.Context().RemoteIP())
	if !ok {
		return ""
	}
	var forwarded []string
	for _, value := range c.Request().Header.PeekAll(fiber.HeaderXForwardedFor) {
		forwarded = append(forwarded, strings.Split(string(value), ",")...)
	}
	return resolveClientIP(remote, forwarded, currentTrustedProxies()).String()
}

// limitKey groups IPv6 clients by /64, the smallest block a subscriber
// usually controls, so rotating addresses inside it does not reset limits.
func limitKey(ip string) string {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return ip
	}
	addr = normalizeAddr(addr)
	if addr.Is4() {
		return addr.String()
	}
	return netip.PrefixFrom(addr, 64).Masked().String()
}

// LimitKey is the key for request rate limiters.
func LimitKey(c *fiber.Ctx) string { return limitKey(ClientIP(c)) }

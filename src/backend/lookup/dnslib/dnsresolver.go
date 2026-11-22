package dnslib

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"typonamer/constant"
	"typonamer/lookup/lookuperror"
	"typonamer/lookup/lookupinfo"
	"typonamer/lookup/network"

	"github.com/miekg/dns"
)

var rootServers = []string{
	"a.root-servers.net.", "b.root-servers.net.", "c.root-servers.net.", "d.root-servers.net.",
	"e.root-servers.net.", "f.root-servers.net.", "g.root-servers.net.", "h.root-servers.net.",
	"i.root-servers.net.", "j.root-servers.net.", "k.root-servers.net.", "l.root-servers.net.", "m.root-servers.net.",
}

type delegation struct {
	servers []string
	expires time.Time
}

var cacheMu sync.Mutex
var tldCache = map[string]delegation{}

func NsCheck(ctx context.Context, domain, tld string) (result lookupinfo.DomainInfo, resultErr error) {
	result = lookupinfo.DomainInfo{LookupType: constant.LookupTypeDNS, DomainName: domain}
	release, err := network.Acquire(ctx, "dns:"+tld, 4)
	if err != nil {
		return result, err
	}
	defer release()

	var trace strings.Builder
	fmt.Fprintf(&trace, "DNS NS trace for %s\n", domain)
	defer func() { result.RawResponse = trace.String() }()
	msg := new(dns.Msg).SetQuestion(dns.Fqdn(domain), dns.TypeNS)
	msg.RecursionDesired = false
	target := msg.Question[0].Name
	zone, servers := ".", rootServers
	cacheMu.Lock()
	cached := tldCache[tld]
	cacheMu.Unlock()
	if time.Now().Before(cached.expires) {
		zone, servers = dns.Fqdn(tld), cached.servers
	}
	udp := dns.Client{Net: "udp", Timeout: 1500 * time.Millisecond}
	for depth := 0; depth < 8; depth++ {
		var next []string
		for _, server := range servers {
			if ctx.Err() != nil {
				return result, lookuperror.ErrorDnsTimeout
			}
			reply := exchange(ctx, &udp, msg, server)
			if reply == nil {
				fmt.Fprintf(&trace, "%s via %s: no response\n", zone, server)
				continue
			}
			fmt.Fprintf(&trace, "%s via %s: %s\n", zone, server, dns.RcodeToString[reply.Rcode])
			if reply.Rcode != dns.RcodeSuccess && reply.Rcode != dns.RcodeNameError {
				continue
			}
			var nextZone string
			var ttl uint32
			if reply.Rcode == dns.RcodeSuccess {
				for _, records := range [][]dns.RR{reply.Answer, reply.Ns} {
					for _, rr := range records {
						ns, ok := rr.(*dns.NS)
						if !ok {
							continue
						}
						name := strings.ToLower(ns.Hdr.Name)
						if name == target {
							result.NameServer = append(result.NameServer, strings.TrimSuffix(strings.ToLower(ns.Ns), "."))
							continue
						}
						if name != zone && dns.IsSubDomain(zone, name) && dns.IsSubDomain(name, target) {
							if len(name) > len(nextZone) {
								nextZone, next, ttl = name, nil, ns.Hdr.Ttl
							}
							if name == nextZone {
								next = append(next, ns.Ns)
								ttl = min(ttl, ns.Hdr.Ttl)
							}
						}
					}
				}
			}
			if len(result.NameServer) > 0 {
				fmt.Fprintln(&trace, strings.Join(result.NameServer, ", "))
				return result, nil
			}
			if nextZone != "" {
				zone = nextZone
				if zone == dns.Fqdn(tld) && ttl != 0 {
					cacheMu.Lock()
					tldCache[tld] = delegation{next, time.Now().Add(min(time.Hour, time.Duration(ttl)*time.Second))}
					cacheMu.Unlock()
				}
				break
			}
			if reply.Authoritative && reply.Rcode == dns.RcodeNameError {
				return result, lookuperror.ErrorNsNotFound
			}
			if reply.Authoritative && reply.Rcode == dns.RcodeSuccess {
				for _, rr := range reply.Ns {
					if _, ok := rr.(*dns.SOA); ok && dns.IsSubDomain(rr.Header().Name, target) {
						return result, lookuperror.ErrorNsNotFound
					}
				}
			}
		}
		if len(next) == 0 {
			break
		}
		servers = next
	}
	if ctx.Err() != nil {
		return result, lookuperror.ErrorDnsTimeout
	}
	return result, lookuperror.ErrorDnsServerFailed
}

func exchange(ctx context.Context, udp *dns.Client, msg *dns.Msg, host string) *dns.Msg {
	addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil
	}
	for _, address := range addresses {
		if !address.IsGlobalUnicast() || address.IsPrivate() {
			continue
		}
		addr := net.JoinHostPort(address.String(), "53")
		reply, _, err := udp.ExchangeContext(ctx, msg, addr)
		if err == nil && reply.Truncated {
			tcp := *udp
			tcp.Net = "tcp"
			reply, _, err = tcp.ExchangeContext(ctx, msg, addr)
		}
		if err == nil {
			return reply
		}
	}
	return nil
}

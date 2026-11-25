package dnslib

import (
	"context"
	"fmt"
	"net"
	"slices"
	"strings"
	"sync"
	"time"
	"typonamer/lookup/network"

	"typonamer/config"
	"typonamer/constant"
	"typonamer/log"
	"typonamer/lookup/lookuperror"
	"typonamer/lookup/lookupinfo"

	"github.com/duke-git/lancet/v2/strutil"
	"github.com/miekg/dns"
)

type NsCache struct {
	TldNsMap map[string][]string
	mux      sync.RWMutex
}

var (
	rootServers = []string{
		"a.root-servers.net.",
		"b.root-servers.net.",
		"c.root-servers.net.",
		"d.root-servers.net.",
		"e.root-servers.net.",
		"f.root-servers.net.",
		"g.root-servers.net.",
		"h.root-servers.net.",
		"i.root-servers.net.",
		"j.root-servers.net.",
		"k.root-servers.net.",
		"l.root-servers.net.",
		"m.root-servers.net.",
	}

	TldNsCache = NsCache{
		TldNsMap: make(map[string][]string),
	}
)

func HasTldNsCache(tld string) bool {
	_, ok := lookupTldNsCache(tld)
	return ok
}

func AddTldNsCache(tld string, nameServers []string) {
	if len(nameServers) == 0 {
		return
	}
	TldNsCache.mux.Lock()
	defer TldNsCache.mux.Unlock()
	TldNsCache.TldNsMap[strings.ToLower(tld)] = slices.Clone(nameServers)
}

func GetTldNsCache(tld string) []string {
	ns, _ := lookupTldNsCache(tld)
	return ns
}

func lookupTldNsCache(tld string) ([]string, bool) {
	TldNsCache.mux.RLock()
	defer TldNsCache.mux.RUnlock()
	ns, ok := TldNsCache.TldNsMap[strings.ToLower(tld)]
	if !ok || len(ns) == 0 {
		return []string{}, false
	}
	return slices.Clone(ns), true
}

func NsCheckContext(ctx context.Context, domain string) (lookupinfo.DomainInfo, error) {
	log.Debugf("Resolving NS record for domain %s", domain)

	domainInfo := lookupinfo.DomainInfo{
		LookupType: constant.LookupTypeDNS,
		DomainName: domain,
	}

	cfg := config.GetConfig()
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.DnsTimeout)*time.Second)
	defer cancel()
	release, err := network.Acquire(ctx, "dns")
	if err != nil {
		return domainInfo, err
	}
	defer release()
	var lastErr error
	answered := false

	nsRecords := make([]string, 0)

	parts := strings.Split(strutil.Trim(domain, "."), ".")
	if len(parts) < 2 {
		return domainInfo, fmt.Errorf("invalid domain: %s", domain)
	}
	tld := parts[len(parts)-1]

	// Create DNS client
	dnsClient := new(dns.Client)
	dnsClient.Net = "udp"
	dnsClient.Timeout = time.Duration(cfg.DnsTimeout) * time.Second

	// Create DNS NS query message
	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(domain), dns.TypeNS)
	msg.RecursionDesired = false

	// Initialize the nameservers
	level := 0
	rawTrace := "┌─ DNS Resolution Trace\n"
	rawTrace += fmt.Sprintf("├─ Target: %s\n", domain)
	rawTrace += fmt.Sprintf("├─ Root Servers: \n")
	for _, rootNs := range rootServers {
		rawTrace += fmt.Sprintf("│  ├─ %s\n", rootNs)
	}

	nextNameservers := rootServers
	if cachedNs, ok := lookupTldNsCache(tld); ok {
		log.Debugf("Using cached NS records for '%s': %v", tld, cachedNs)
		nextNameservers = cachedNs

		rawTrace += "│\n"
		indent := strings.Repeat("│  ", level)
		rawTrace += fmt.Sprintf("%s├─ Level %d: Query for %s\n", indent, level+1, domain)
		rawTrace += fmt.Sprintf("%s├─ Got answer for: %s\n", indent, tld)
		rawTrace += fmt.Sprintf("%s├─ Found nameservers: \n", indent)
		for _, ns := range nextNameservers {
			rawTrace += fmt.Sprintf("%s│  ├─ %s\n", indent, ns)
		}
		rawTrace += fmt.Sprintf("%s└─ Via: %s\n", indent, nextNameservers[0])
		level++
	}

	for {
		var responseNS []string
		foundNS := false

		indent := strings.Repeat("│  ", level)
		rawTrace += "│\n"
		rawTrace += fmt.Sprintf("%s├─ Level %d: Query for %s\n", indent, level+1, domain)

		for _, nameserver := range nextNameservers {
			domainInfo.Source = nameserver
			if err := network.Wait(ctx, "dns", nameserver); err != nil {
				return domainInfo, err
			}
			response, _, err := dnsClient.ExchangeContext(ctx, msg, net.JoinHostPort(strutil.Trim(nameserver, "."), "53"))
			if err == nil && response != nil && response.Truncated {
				tcp := *dnsClient
				tcp.Net = "tcp"
				response, _, err = tcp.ExchangeContext(ctx, msg, net.JoinHostPort(strutil.Trim(nameserver, "."), "53"))
			}
			if err != nil {
				lastErr = err
				log.Debugf("Failed to query DNS for domain %s using nameserver %s: %s", domain, nameserver, err)
				continue
			}

			if response != nil {
				if response.Rcode == dns.RcodeSuccess || response.Rcode == dns.RcodeNameError {
					answered = true
				} else {
					lastErr = lookuperror.ErrorDnsServerFailed
				}
				if response.Rcode == dns.RcodeNameError {
					domainInfo.RawResponse = response.String()
					return domainInfo, lookuperror.ErrorNsNotFound
				}
				log.Debugf("DNS query for %s using nameserver %s Rcode is %d", domain, nameserver, response.Rcode)
				if response.Rcode == dns.RcodeSuccess {
					records := response.Answer
					if len(records) == 0 {
						records = response.Ns
					}

					isTldNs := false

					if len(records) > 0 {
						hdrName := ""
						for _, rr := range records {
							if ns, ok := rr.(*dns.NS); ok {
								foundNS = true
								responseNS = append(responseNS, ns.Ns)
								hdrName = strutil.Trim(ns.Hdr.Name, ".")
								if strings.EqualFold(hdrName, domain) {
									nsRecords = append(nsRecords, strutil.Trim(ns.Ns, "."))
								} else if strings.EqualFold(hdrName, tld) {
									isTldNs = true
								}
								log.Debugf("Found NS record for domain %s from nameserver %s is: %v", ns.Hdr.Name, nameserver, ns.Ns)
							}
						}
						if foundNS {
							rawTrace += fmt.Sprintf("%s├─ Got answer for: %s\n", indent, hdrName)
							rawTrace += fmt.Sprintf("%s├─ Found nameservers: \n", indent)
							for _, ns := range responseNS {
								rawTrace += fmt.Sprintf("%s│  ├─ %s\n", indent, ns)
							}
							rawTrace += fmt.Sprintf("%s└─ Via: %s\n", indent, nameserver)

							if isTldNs {
								if !HasTldNsCache(tld) {
									log.Infof("Adding NS records for '%s' to cache: %v", tld, responseNS)
									AddTldNsCache(tld, responseNS)
								}
							}
							break
						}
					}
				}
			} else {
				log.Warnf("DNS query for %s using nameserver %s response is nil", domain, nameserver)
			}
		}

		if len(nsRecords) > 0 {
			// Found the nameservers for the target domain
			log.Infof("Found NS record for %s are: %v", domain, nsRecords)
			break
		}

		if !foundNS {
			log.Infof("No nameservers found for %s", domain)
			rawTrace += fmt.Sprintf("%s└─ No nameservers found for %s\n", indent, domain)
			break
		}

		// Use the nameservers found in the current level to resolve the next level
		answered = false
		nextNameservers = responseNS
		level++

		if level > 5 {
			log.Warnf("Failed to resolve NS record for domain %s", domain)
			break
		}
	}

	rawTrace += "│\n"
	rawTrace += "└─ Resolution Complete\n"

	domainInfo.NameServer = nsRecords
	domainInfo.RawResponse = rawTrace

	if len(nsRecords) == 0 {
		if ctx.Err() != nil {
			return domainInfo, ctx.Err()
		}
		if !answered {
			if lastErr != nil {
				return domainInfo, lastErr
			}
			return domainInfo, lookuperror.ErrorDnsServerFailed
		}
		return domainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorNsNotFound, domain)
	}

	return domainInfo, nil
}

func NsCheck(domain string) (lookupinfo.DomainInfo, error) {
	return NsCheckContext(context.Background(), domain)
}

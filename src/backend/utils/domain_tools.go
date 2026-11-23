package utils

import (
	"errors"
	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"
	"net"
	"strings"

	"github.com/duke-git/lancet/v2/strutil"
)

var domainSuffixSet = func() map[string]bool {
	set := make(map[string]bool, len(DomainSuffixes))
	for _, suffix := range DomainSuffixes {
		set[suffix] = true
	}
	return set
}()

// TrimAndGetMainDomain trims and parses a domain string, and returns the main domain string
func TrimAndGetMainDomain(domain string) (string, error) {
	domainStr, err := TrimDomain(domain)
	if err != nil {
		return "", err
	}

	mainDomain, err := GetDomainSuffixPlusOne(domainStr)
	if err != nil {
		return "", err
	}

	return mainDomain, nil
}

func TrimDomain(domain string) (string, error) {
	domain = strings.TrimSuffix(strings.TrimSpace(domain), ".")
	ascii, err := idna.Lookup.ToASCII(domain)
	if err != nil || len(ascii) > 253 || !strings.Contains(ascii, ".") || net.ParseIP(ascii) != nil {
		return "", errors.New("invalid domain name")
	}
	ascii = strings.ToLower(ascii)
	for _, label := range strings.Split(ascii, ".") {
		if len(label) < 1 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", errors.New("invalid domain label")
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return "", errors.New("invalid domain character")
			}
		}
	}
	return ascii, nil
}

func GetDomainSuffix(domain string) (string, error) {
	// Use ICANN boundaries for registration; private hosting suffixes are not registries.
	for candidate := domain; ; {
		suffix, icann := publicsuffix.PublicSuffix(candidate)
		if icann {
			return suffix, nil
		}
		dot := strings.IndexByte(candidate, '.')
		if dot < 0 {
			break
		}
		candidate = candidate[dot+1:]
	}
	parts := strutil.SplitAndTrim(domain, ".")

	maxLen := len(parts)

	for n := maxLen; n > 0; n-- {
		suffix := strings.Join(parts[len(parts)-n:], ".")
		if domainSuffixSet[suffix] {
			return suffix, nil
		}
	}

	return "", errors.New("domain suffix not found")
}

func GetDomainSuffixPlusOne(domain string) (string, error) {
	domainSuffix, err := GetDomainSuffix(domain)
	if err != nil {
		return "", err
	}

	if domain == domainSuffix {
		return "", errors.New("domain is invalid")
	}

	prefix := strings.TrimSuffix(domain, "."+domainSuffix)
	return prefix[strings.LastIndexByte(prefix, '.')+1:] + "." + domainSuffix, nil
}

func GetTld(domain string) (tld string, suffix string, err error) {
	domainSuffix, err := GetDomainSuffix(domain)
	if err != nil {
		return "", "", err
	}

	suffix = domainSuffix

	// If the TLD is not as expected (e.g., "com.cn"), read the domain from right to left and take the part to the right of the first dot as the TLD
	tld = domainSuffix
	if strings.Contains(domainSuffix, ".") {
		parts := strings.Split(domainSuffix, ".")
		tld = parts[len(parts)-1]
	}

	return tld, suffix, nil
}

func GetSld(domain string) string {
	domainStr, err := TrimAndGetMainDomain(domain)
	if err != nil {
		return ""
	}

	parts := strings.Split(domainStr, ".")
	if len(parts) > 0 {
		return parts[0]
	}

	return ""
}

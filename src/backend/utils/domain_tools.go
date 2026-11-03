package utils

import (
	"errors"
	"fmt"
	"strings"

	"github.com/duke-git/lancet/v2/slice"
	"github.com/duke-git/lancet/v2/strutil"
)

var replaces = map[string]string{
	"~":  "",
	"!":  "",
	"@":  "",
	"#":  "",
	"$":  "",
	"%":  "",
	"^":  "",
	"&":  "",
	"*":  "",
	"(":  "",
	")":  "",
	"_":  "",
	"+":  "",
	"=":  "",
	"[":  "",
	"]":  "",
	"{":  "",
	"}":  "",
	"\\": "",
	"|":  "",
	":":  "",
	";":  "",
	"'":  "",
	"`":  "",
	"<":  "",
	">":  "",
	",":  "",
	"/":  "",
	"?":  "",
}

// var specialTreatmentDomain = []string{"gov.hu", "com.be", "gov.se"}

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
	if domain == "" {
		return "", errors.New("domain is empty")
	}

	// remove whitespaces from the domain string
	domainStr1 := strutil.RemoveWhiteSpace(domain, true)
	if domainStr1 == "" {
		return "", errors.New("domain is empty")
	}

	// remove special characters from the domain string
	domainStr2 := strutil.ReplaceWithMap(domainStr1, replaces)
	if domainStr2 == "" {
		return "", errors.New("domain is invalid")
	}

	// convert the domain string to lowercase
	domainStr3 := strings.ToLower(domainStr2)

	// Trim the domain string to remove "-"
	domainStr4 := strings.Trim(domainStr3, "-")
	if strings.Contains(domainStr4, ".") {
		return domainStr4, nil
	}

	return "", errors.New("domain is invalid")
}

func GetDomainSuffix(domain string) (string, error) {
	parts := strutil.SplitAndTrim(domain, ".")

	maxLen := 3
	if len(parts) < 3 {
		maxLen = len(parts)
	}

	for n := maxLen; n > 0; n-- {
		suffix := strings.Join(parts[len(parts)-n:], ".")
		fmt.Println("suffix: ", suffix)
		if slice.Contain(DomainSuffixes, suffix) {
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

	parts := strutil.SplitAndTrim(domain, fmt.Sprintf(".%s", domainSuffix), ".")
	if len(parts) == 0 {
		return "", errors.New("domain is invalid")
	}

	domainPrefix := parts[0]

	if strings.Contains(domainPrefix, ".") {
		domainPrefixParts := strutil.SplitAndTrim(domainPrefix, ".")
		return fmt.Sprintf("%s.%s", domainPrefixParts[len(domainPrefixParts)-1], domainSuffix), nil
	} else {
		return fmt.Sprintf("%s.%s", domainPrefix, domainSuffix), nil
	}
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

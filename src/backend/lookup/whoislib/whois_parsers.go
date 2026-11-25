package whoislib

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"typonamer/constant"
	"typonamer/lookup/lookuperror"
	"typonamer/lookup/lookupinfo"

	"github.com/dromara/carbon/v2"
	"github.com/duke-git/lancet/v2/strutil"
)

var ordinalDate = regexp.MustCompile(`\b([0-9]{1,2})(?:st|nd|rd|th)\b`)
var whoisRateLimitPattern = regexp.MustCompile(`(?i)(rate\s*limit|query\s*(?:limit|rate)|limit\s*exceeded|too\s*many\s*requests|(?:rate|limit|queries).*exceeded|exceeded.*(?:rate|limit|queries)|connection\s*limit|try\s*again\s*later|queried\s*too\s*fast|number\s*of\s*requests)`)

var whoisResponseReplaceList = map[string]string{
	"\r\n": "\n",
}

// ParseWhoisResponse parses the WHOIS response and returns the DomainInfo struct.
func ParseWhoisResponse(response string, domain string, matcher WhoisInfoMatcher) (lookupinfo.DomainInfo, error) {
	domainInfo := lookupinfo.DomainInfo{
		LookupType:  constant.LookupTypeWhois,
		DomainName:  domain,
		RawResponse: response,
	}

	// Clean up the response by replacing unwanted characters with a newline.
	responseContent := strutil.ReplaceWithMap(response, whoisResponseReplaceList)
	if strings.HasPrefix(responseContent, "Reserved by QDR") || strings.Contains(responseContent, "has been reserved by aeDA Regulator") {
		return domainInfo, lookuperror.ErrorReservedDomain
	}

	if matcher.ReFree != nil {
		if matcher.ReFree.MatchString(responseContent) {
			return domainInfo, errors.New("domain not found")
		}
	}

	// Extract the registrar from the response.
	if matcher.ReRegistrar != nil {
		matchRegistrar := matcher.ReRegistrar.FindStringSubmatch(responseContent)
		if len(matchRegistrar) > 1 {
			domainInfo.Registrar = strutil.Trim(matchRegistrar[1])
		}
	}

	// Extract the domain status from the response.
	if matcher.ReDomainStatus != nil {
		matchDomainStatuses := matcher.ReDomainStatus.FindAllStringSubmatch(responseContent, -1)
		if len(matchDomainStatuses) > 0 {
			// domainInfo.DomainStatus = make([]string, len(matchDomainStatuses))
			for _, match := range matchDomainStatuses {
				if strings.Contains(match[1], ",") {
					status := strutil.SplitAndTrim(match[1], ",")
					domainInfo.DomainStatus = append(domainInfo.DomainStatus, status...)
				} else if strings.Contains(match[1], "http") {
					status := strutil.SplitAndTrim(match[1], "http")
					domainInfo.DomainStatus = append(domainInfo.DomainStatus, status[0])
				} else if strings.Contains(match[1], "-") {
					status := strutil.SplitAndTrim(match[1], "-")
					domainInfo.DomainStatus = append(domainInfo.DomainStatus, status[0])
				} else {
					domainInfo.DomainStatus = append(domainInfo.DomainStatus, strutil.Trim(match[1]))
				}
			}
		}
	}

	domainInfo.CreationDate = whoisDate(responseContent, matcher.ReCreationDate, matcher.DateTimeLayoutForCreationDate, matcher.DateTimeLayout)
	domainInfo.ExpiryDate = whoisDate(responseContent, matcher.ReExpiryDate, matcher.DateTimeLayoutForExpiryDate, matcher.DateTimeLayout)

	// Extract the nameservers from the response.
	if matcher.ReNameServer != nil {
		matchNameServers := matcher.ReNameServer.FindAllStringSubmatch(responseContent, -1)
		if len(matchNameServers) > 0 {
			// domainInfo.NameServer = make([]string, len(matchNameServers))
			for _, match := range matchNameServers {
				nameServerItemStr := strutil.Trim(match[1])
				if strings.Contains(nameServerItemStr, "\n") {
					nss := strutil.SplitAndTrim(nameServerItemStr, "\n")
					for _, ns := range nss {
						if strings.Contains(ns, " ") {
							nsSubs := strutil.SplitAndTrim(ns, " ")
							if strings.Contains(nsSubs[0], ".") {
								domainInfo.NameServer = append(domainInfo.NameServer, strutil.Trim(nsSubs[0], "."))
							}
						} else {
							if strings.Contains(ns, ".") {
								domainInfo.NameServer = append(domainInfo.NameServer, strutil.Trim(ns, "."))
							}
						}
					}
				} else if strings.Contains(nameServerItemStr, " ") {
					nss := strutil.SplitAndTrim(nameServerItemStr, " ")
					if strings.Contains(nss[0], ".") {
						domainInfo.NameServer = append(domainInfo.NameServer, strutil.Trim(nss[0], "."))
					}
				} else {
					if strings.Contains(nameServerItemStr, ".") {
						domainInfo.NameServer = append(domainInfo.NameServer, strutil.Trim(nameServerItemStr, "."))
					}
				}
			}
		}
	}

	if domainInfo.Registrar == "" && domainInfo.CreationDate == "" && domainInfo.ExpiryDate == "" && len(domainInfo.NameServer) == 0 {
		if whoisRateLimitPattern.MatchString(responseContent) {
			return domainInfo, lookuperror.ErrorRateLimited
		}
		return domainInfo, errors.New("no domain info found in whois response")
	}

	return domainInfo, nil
}

func whoisDate(response string, re *regexp.Regexp, layout, fallbackLayout string) string {
	if re == nil {
		return ""
	}
	match := re.FindStringSubmatch(response)
	if len(match) < 2 {
		return ""
	}
	value := ordinalDate.ReplaceAllString(strings.TrimSpace(match[1]), "${1}")
	if layout == "" {
		layout = fallbackLayout
	}
	if layout != "" {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC().Format(time.DateTime)
		}
		return carbon.ParseByLayout(value, layout, carbon.UTC).ToDateTimeString()
	}
	for _, isoLayout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(isoLayout, value); err == nil {
			return parsed.UTC().Format(time.DateTime)
		}
	}
	date := carbon.Parse(value, carbon.UTC)
	if prefix, _, ok := strings.Cut(value, "+"); date.Error != nil && ok {
		date = carbon.Parse(strings.TrimSpace(prefix), carbon.UTC)
	}
	return date.ToDateTimeString()
}

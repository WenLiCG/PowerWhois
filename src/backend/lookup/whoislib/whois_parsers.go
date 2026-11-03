package whoislib

import (
	"errors"
	"regexp"
	"strings"
	"typonamer/constant"
	"typonamer/lookup/lookupinfo"

	"github.com/dromara/carbon/v2"
	"github.com/duke-git/lancet/v2/strutil"
)

var ordinalDate = regexp.MustCompile(`\b([0-9]{1,2})(?:st|nd|rd|th)\b`)

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

	// Extract the creation date from the response.
	if matcher.ReCreationDate != nil {
		matchCreationDate := matcher.ReCreationDate.FindStringSubmatch(responseContent)
		if len(matchCreationDate) > 1 {
			matchCreationDateStr := ordinalDate.ReplaceAllString(strutil.Trim(matchCreationDate[1]), "${1}")
			if matcher.DateTimeLayoutForCreationDate != "" {
				domainInfo.CreationDate = carbon.SetTimezone(carbon.UTC).ParseByLayout(matchCreationDateStr, matcher.DateTimeLayoutForCreationDate).ToDateTimeString()
			} else if matcher.DateTimeLayout != "" {
				domainInfo.CreationDate = carbon.SetTimezone(carbon.UTC).ParseByLayout(matchCreationDateStr, matcher.DateTimeLayout).ToDateTimeString()
			} else {
				creationDate := carbon.SetTimezone(carbon.UTC).Parse(matchCreationDateStr).ToDateTimeString()
				if creationDate != "" {
					domainInfo.CreationDate = creationDate
				} else {
					if strings.Contains(matchCreationDateStr, "+") {
						dateTimStrs := strutil.SplitAndTrim(matchCreationDateStr, "+")
						domainInfo.CreationDate = carbon.SetTimezone(carbon.UTC).Parse(dateTimStrs[0]).ToDateTimeString()
					}
				}
			}
		}
	}

	// Extract the expiry date from the response.
	if matcher.ReExpiryDate != nil {
		matchExpiryDate := matcher.ReExpiryDate.FindStringSubmatch(responseContent)
		if len(matchExpiryDate) > 1 {
			matchExpiryDateStr := ordinalDate.ReplaceAllString(strutil.Trim(matchExpiryDate[1]), "${1}")
			if matcher.DateTimeLayoutForExpiryDate != "" {
				domainInfo.ExpiryDate = carbon.SetTimezone(carbon.UTC).ParseByLayout(matchExpiryDateStr, matcher.DateTimeLayoutForExpiryDate).ToDateTimeString()
			} else if matcher.DateTimeLayout != "" {
				domainInfo.ExpiryDate = carbon.SetTimezone(carbon.UTC).ParseByLayout(matchExpiryDateStr, matcher.DateTimeLayout).ToDateTimeString()
			} else {
				expiryDate := carbon.SetTimezone(carbon.UTC).Parse(matchExpiryDateStr).ToDateTimeString()
				if expiryDate != "" {
					domainInfo.ExpiryDate = expiryDate
				} else {
					if strings.Contains(matchExpiryDateStr, "+") {
						dateTimStrs := strutil.SplitAndTrim(matchExpiryDateStr, "+")
						domainInfo.ExpiryDate = carbon.SetTimezone(carbon.UTC).Parse(dateTimStrs[0]).ToDateTimeString()
					}
				}
			}
		}
	}

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
		return domainInfo, errors.New("no domain info found in whois response")
	}

	return domainInfo, nil
}

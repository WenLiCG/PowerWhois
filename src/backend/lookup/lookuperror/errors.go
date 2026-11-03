package lookuperror

import "errors"

var (
	ErrorRateLimited              = errors.New("registry rate limited")
	ErrorInvalidDomainName        = errors.New("invalid domain name")
	ErrorWhoisTimeout             = errors.New("whois query timeout")
	ErrorNotSupportedTld          = errors.New("not supported tld")
	ErrorWhoisNotFound            = errors.New("whois not found")
	ErrorNsNotFound               = errors.New("dns ns record not found")
	ErrorWhoisServerFailed        = errors.New("whois server failed")
	ErrorConnectToProxy           = errors.New("connect to proxy failed")
	ErrorNoContentInWhoisResponse = errors.New("no content in whois response")
	ErrorNoParseRuleForTld        = errors.New("no parsing rule for tld")
	ErrorParseWhoisResponse       = errors.New("parse whois response failed")
	ErrorInvalidQueryType         = errors.New("invalid query type")
	ErrorInvalidLookupType        = errors.New("invalid lookup type")
	ErrorNoWhoisServerForTld      = errors.New("no whois server for tld")

	ErrorCustomizeApiServerResponse = errors.New("customize api server response error")
	ErrorCustomizeApiWhoisResult    = errors.New("customize api whois result error")
)

var (
	ErrorDnsTimeout      = errors.New("dns query timeout")
	ErrorDnsServerFailed = errors.New("dns server failed")
)

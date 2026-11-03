package utils

import (
	"errors"
	"strings"
	"typonamer/constant"
	"typonamer/log"
	"typonamer/lookup/lookuperror"
	"typonamer/lookup/lookupinfo"

	"github.com/bytedance/sonic"
	"github.com/duke-git/lancet/v2/slice"
	"github.com/duke-git/lancet/v2/strutil"
	"github.com/jszwec/csvutil"
)

func GetDnsLite(nameServers []string) string {
	var dnsLite string
	if len(nameServers) > 0 {
		nsString := strutil.Trim(nameServers[0], ".")
		dnsMainDomain, err := TrimAndGetMainDomain(nsString)
		if err != nil {
			return ""
		}
		dnsLite = dnsMainDomain
	}
	return dnsLite
}

// GetDomainHumanStatus takes a list of domain status strings and returns the human-readable status string.
// It first converts the domain status strings to lowercase and removes any whitespace.
// If the domain status contains "registarahold", it returns "Expired".
// If the domain status contains "redemptionperiod", it returns "RedemptionPeriod".
// If the domain status contains "pendingdelete" or "delegated", it returns "PendingDelete".
// Otherwise, it returns "Active".
func GetDomainHumanStatus(domainStatus []string) string {
	if len(domainStatus) > 0 {
		// convert the domain status strings to lowercase and remove any whitespace
		lowerDomainStatus := slice.Map(domainStatus, LowerString)
		noSpaceDomainStatus := slice.Map(lowerDomainStatus, RemoveWhiteSpace)

		// check if the domain status contains any of the following strings
		if slice.Contain(noSpaceDomainStatus, "registarahold") {
			return constant.DomainStatusExpired
		} else if slice.Contain(noSpaceDomainStatus, "pendingdelete") || slice.Contain(noSpaceDomainStatus, "delegated") {
			return constant.DomainStatusPendingDelete
		} else if slice.Contain(noSpaceDomainStatus, "redemptionperiod") {
			return constant.DomainStatusRedemptionPeriod
		} else {
			return constant.DomainStatusActive
		}
	} else {
		return constant.DomainStatusUnknown
	}
}

// GetDomainHumanError takes an error and returns the human-readable error message.
// The returned string is in Chinese.
func GetDomainHumanError(err error) string {
	if err != nil {
		switch {
		case errors.Is(err, lookuperror.ErrorInvalidDomainName):
			return "域名无效"
		case errors.Is(err, lookuperror.ErrorWhoisTimeout):
			return "Whois查询超时"
		case errors.Is(err, lookuperror.ErrorNotSupportedTld):
			return "后缀不支持"
		case errors.Is(err, lookuperror.ErrorWhoisServerFailed):
			return "Whois查询失败"
		case errors.Is(err, lookuperror.ErrorConnectToProxy):
			return "代理连接失败"
		case errors.Is(err, lookuperror.ErrorNoContentInWhoisResponse):
			return "Whois响应无内容"
		case errors.Is(err, lookuperror.ErrorNoParseRuleForTld):
			return "无法解析查询结果"
		case errors.Is(err, lookuperror.ErrorParseWhoisResponse):
			return "解析Whois结果失败"
		case errors.Is(err, lookuperror.ErrorDnsTimeout):
			return "DNS查询超时"
		case errors.Is(err, lookuperror.ErrorDnsServerFailed):
			return "DNS服务器返回异常"
		case errors.Is(err, lookuperror.ErrorInvalidQueryType):
			return "查询类型错误"
		case errors.Is(err, lookuperror.ErrorInvalidLookupType):
			return "查询类型错误"
		case errors.Is(err, lookuperror.ErrorNoWhoisServerForTld):
			return "没有Whois服务器"
		case errors.Is(err, lookuperror.ErrorCustomizeApiServerResponse):
			return "自定义Whois API服务器返回异常"
		case errors.Is(err, lookuperror.ErrorCustomizeApiWhoisResult):
			return "自定义Whois API结果解析错误"
		default:
			return "其它错误"
		}
	} else {
		return ""
	}
}

// GetOrderedQueryResult takes a list of domain JSON strings from Redis and convert them back to QueryResult struct array.
// It sorts the array by the Order field in ascending order.
func GetOrderedQueryResult(domainResult []string) []lookupinfo.QueryResult {
	var queryResults []lookupinfo.QueryResult
	for _, domainJson := range domainResult {
		var queryItem lookupinfo.QueryResult
		err := sonic.Unmarshal([]byte(domainJson), &queryItem)
		if err != nil {
			log.Errorf("Error unmarshal query result: %s", err)
			continue
		}
		queryResults = append(queryResults, queryItem)
	}

	// Sort the queryResults by their Order in ascending order
	slice.SortByField(queryResults, "Order", "asc")

	return queryResults
}

// ConvertQueryResultToCSV takes a list of QueryResult struct and converts it to []QueryCsvResult.
// It then uses csvutil to marshal the []QueryCsvResult to a CSV byte array.
func ConvertQueryResultToCSV(queryResults []lookupinfo.QueryResult) ([]byte, error) {
	var csvResults []lookupinfo.QueryCsvResult
	for _, queryResult := range queryResults {
		viaProxy := ""
		if (queryResult.LookupType == constant.LookupTypeWhois) || (queryResult.LookupType == constant.LookupTypeRDAP) {
			if queryResult.ViaProxy {
				viaProxy = "Yes"
			} else {
				viaProxy = "No"
			}
		}
		csvResults = append(csvResults, lookupinfo.QueryCsvResult{
			Domain:          queryResult.Domain,
			LookupType:      queryResult.LookupType,
			ViaProxy:        viaProxy,
			Status:          queryResult.RegisterStatus,
			ErrorInfo:       queryResult.QueryError,
			CreatedDate:     queryResult.CreatedDate,
			ExpiryDate:      queryResult.ExpiryDate,
			NameServer:      slice.Join(queryResult.NameServer, ","),
			DnsLite:         queryResult.DnsLite,
			RawDomainStatus: slice.Join(queryResult.RawDomainStatus, ","),
			DomainStatus:    queryResult.DomainStatus,
		})
	}

	return csvutil.Marshal(csvResults)
}

func LowerString(_ int, v string) string {
	return strings.ToLower(v)
}

func RemoveWhiteSpace(_ int, v string) string {
	return strutil.RemoveWhiteSpace(v, true)
}

func GetFormattedTlds(tldList []string) []string {
	tlds := []string{}
	for _, tld := range tldList {
		tldStr := strutil.RemoveWhiteSpace(tld, true)
		tlds = append(tlds, strutil.Trim(tldStr, "."))
	}

	return tlds
}

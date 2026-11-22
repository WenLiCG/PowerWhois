package utils

import (
	"context"
	"errors"
	"strings"
	"typonamer/constant"
	"typonamer/lookup/lookuperror"

	"github.com/duke-git/lancet/v2/slice"
	"github.com/duke-git/lancet/v2/strutil"
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

func GetDomainHumanStatus(domainStatus []string) string {
	if len(domainStatus) > 0 {
		// convert the domain status strings to lowercase and remove any whitespace
		lowerDomainStatus := slice.Map(domainStatus, LowerString)
		noSpaceDomainStatus := slice.Map(lowerDomainStatus, RemoveWhiteSpace)

		// check if the domain status contains any of the following strings
		if slice.Contain(noSpaceDomainStatus, "registrarhold") {
			return constant.DomainStatusExpired
		} else if slice.Contain(noSpaceDomainStatus, "pendingdelete") {
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
		case errors.Is(err, context.Canceled):
			return "查询中断"
		case errors.Is(err, lookuperror.ErrorRateLimited):
			return "上游请求过多，请稍后再查"
		case errors.Is(err, context.DeadlineExceeded) || errors.Is(err, lookuperror.ErrorWhoisTimeout):
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

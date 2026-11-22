package lookuper

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"strings"
	"time"

	"typonamer/config"
	"typonamer/constant"
	"typonamer/lookup/customize"
	"typonamer/lookup/dnslib"
	"typonamer/lookup/lookuperror"
	"typonamer/lookup/lookupinfo"
	"typonamer/lookup/rdaplib"
	"typonamer/lookup/whoislib"
	"typonamer/utils"
)

func ValidMode(mode string, allowPrivate bool, cfg config.Config) bool {
	switch mode {
	case constant.WhoisQuery, constant.DnsQuery, constant.MixedQuery:
		return true
	case constant.WhoisQueryWithProxy:
		return allowPrivate
	default:
		return allowPrivate && slices.ContainsFunc(cfg.WhoisApis, func(p config.WhoisApi) bool { return p.ApiName == mode })
	}
}

func Query(ctx context.Context, domain, mode string, allowPrivate bool, cfg config.Config) lookupinfo.QueryResult {
	canonical, inputErr := utils.TrimAndGetMainDomain(domain)
	if inputErr != nil {
		return Result(domain, lookupinfo.DomainInfo{}, lookuperror.ErrorInvalidDomainName)
	}
	domain = canonical
	if !ValidMode(mode, allowPrivate, cfg) {
		return Result(domain, lookupinfo.DomainInfo{}, lookuperror.ErrorInvalidQueryType)
	}

	tld, suffix, err := utils.GetTld(domain)
	if err != nil {
		return Result(domain, lookupinfo.DomainInfo{}, lookuperror.ErrorInvalidDomainName)
	}
	matches := func(tlds []string) bool { return slices.Contains(tlds, tld) || slices.Contains(tlds, suffix) }
	viaProxy := allowPrivate && (mode == constant.WhoisQueryWithProxy || matches(cfg.GlobalProxyTlds) ||
		(mode == constant.MixedQuery && matches(cfg.MixedProxyTlds)))

	if mode == constant.DnsQuery || (mode == constant.MixedQuery && matches(cfg.MixedDnsTlds)) {
		return dnsResult(ctx, domain, tld, cfg.DnsTimeout)
	}
	var servers []*url.URL
	var bootErr error
	var provider config.WhoisApi
	for _, p := range cfg.WhoisApis {
		if p.ApiName == mode {
			provider = p
			break
		}
	}
	if provider.ApiName == "" {
		if suffix == "ac.uk" {
			// Jisc runs ac.uk separately from the .uk registration service.
			if mode == constant.MixedQuery {
				return dnsResult(ctx, domain, tld, cfg.DnsTimeout)
			}
			return Result(domain, lookupinfo.DomainInfo{}, lookuperror.ErrorNotSupportedTld)
		}
		bootCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.WhoisTimeout)*time.Second)
		servers, bootErr = rdaplib.Bootstrap(bootCtx, domain)
		cancel()
	}

	attempts := 1
	if cfg.RetryOnTimeout {
		attempts += cfg.RetryMax
	}
	var info lookupinfo.DomainInfo
	timeout := time.Duration(cfg.WhoisTimeout) * time.Second
	for attempt := 0; attempt < attempts; attempt++ {
		if ctx.Err() != nil {
			return Result(domain, info, ctx.Err())
		}
		// Each client applies the timeout after it holds an upstream slot.
		switch {
		case provider.ApiName != "":
			info, err = customize.CustomizeLookup(ctx, domain, provider, timeout)
		case len(servers) != 0:
			info, err = rdaplib.RDAPQuery(ctx, domain, servers, viaProxy, cfg)
		default:
			info, err = whoislib.WhoisQuery(ctx, domain, tld, viaProxy, cfg)
		}
		if ctx.Err() != nil {
			return Result(domain, info, ctx.Err())
		}
		if errors.Is(err, lookuperror.ErrorNotSupportedTld) || errors.Is(err, lookuperror.ErrorNoParseRuleForTld) {
			if mode == constant.MixedQuery {
				return dnsResult(ctx, domain, tld, cfg.DnsTimeout)
			}
			if bootErr != nil {
				err = bootErr
			}
		}
		// A rate-limited upstream is paused by the client; the next attempt waits for it.
		retryable := errors.Is(err, context.DeadlineExceeded) || errors.Is(err, lookuperror.ErrorWhoisTimeout) ||
			errors.Is(err, lookuperror.ErrorConnectToProxy) || errors.Is(err, lookuperror.ErrorRateLimited)
		if !retryable || attempt+1 == attempts {
			break
		}
		select {
		case <-ctx.Done():
			return Result(domain, info, ctx.Err())
		case <-time.After(time.Duration(cfg.RetryInterval) * time.Second):
		}
	}
	return Result(domain, info, err)
}

func dnsResult(ctx context.Context, domain, tld string, timeout int) lookupinfo.QueryResult {
	info, err := dnslib.NsCheck(ctx, domain, tld, time.Duration(timeout)*time.Second)
	return Result(domain, info, err)
}

func Result(domain string, info lookupinfo.DomainInfo, err error) lookupinfo.QueryResult {
	result := lookupinfo.QueryResult{
		Domain: domain, LookupType: info.LookupType, ViaProxy: info.ViaProxy, RawResponse: info.RawResponse,
		CreatedDate: info.CreationDate, ExpiryDate: info.ExpiryDate, RawDomainStatus: info.DomainStatus,
	}
	for _, ns := range info.NameServer {
		result.NameServer = append(result.NameServer, strings.ToLower(ns))
	}
	result.DnsLite = utils.GetDnsLite(result.NameServer)
	if len(info.DomainStatus) != 0 {
		result.DomainStatus = utils.GetDomainHumanStatus(info.DomainStatus)
	}
	switch {
	case errors.Is(err, lookuperror.ErrorWhoisNotFound):
		result.RegisterStatus = constant.DomainRegisterStatusFree
		result.QueryError = "注册查询未找到记录，能否购买请以注册商为准"
	case errors.Is(err, lookuperror.ErrorReservedDomain):
		result.RegisterStatus = constant.DomainRegisterStatusReserved
		result.QueryError = "注册局保留"
	case errors.Is(err, lookuperror.ErrorNsNotFound) || (err == nil && info.LookupType == constant.LookupTypeDNS && len(info.NameServer) == 0):
		result.RegisterStatus = constant.DomainRegisterStatusUnknown
		result.QueryError = "DNS 无委派记录，无法确认注册状态"
	case err != nil:
		result.RegisterStatus = constant.DomainRegisterStatusError
		result.QueryError = utils.GetDomainHumanError(err)
	case info.CustomizedResult == constant.DomainRegisterStatusFree:
		result.RegisterStatus = constant.DomainRegisterStatusFree
		result.QueryError = "可用性由配置的供应商报告"
	case info.LookupType == "":
		result.RegisterStatus = constant.DomainRegisterStatusUnknown
		result.QueryError = "缺少查询证据"
	default:
		result.RegisterStatus = constant.DomainRegisterStatusTaken
	}
	return result
}

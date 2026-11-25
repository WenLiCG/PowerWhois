package lookuper

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"typonamer/config"
	"typonamer/constant"
	"typonamer/log"
	"typonamer/lookup/customize"
	"typonamer/lookup/dnslib"
	"typonamer/lookup/lookuperror"
	"typonamer/lookup/lookupinfo"
	"typonamer/lookup/network"
	"typonamer/lookup/rdaplib"
	"typonamer/lookup/whoislib"
	"typonamer/utils"

	"github.com/duke-git/lancet/v2/maputil"
	"github.com/duke-git/lancet/v2/slice"
)

func lookup(ctx context.Context, domain string, queryType string) (lookupinfo.DomainInfo, error) {
	var errDomainInfo = lookupinfo.DomainInfo{
		DomainName: domain,
		Source:     "local-validation",
	}
	if network.IsPublic(ctx) && !publicMode(queryType) {
		return errDomainInfo, fmt.Errorf("代理和自定义查询需要管理员登录: %w", lookuperror.ErrorInvalidQueryType)
	}

	cfg := config.GetConfig()

	mainDomain, err := utils.TrimAndGetMainDomain(domain)
	if err != nil {
		return errDomainInfo, lookuperror.ErrorInvalidDomainName
	}
	// Get the TLD (Top-Level Domain) of the domain
	tld, suffix, err := utils.GetTld(mainDomain)
	if err != nil || tld == "" || suffix == "" {
		log.Error("Invalid domain name: ", domain)
		return errDomainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorInvalidDomainName, err)
	}

	switch queryType {
	case constant.WhoisQuery, constant.WhoisQueryWithProxy:
		useProxy := queryType == constant.WhoisQueryWithProxy || slice.Contain(cfg.GlobalProxyTlds, tld) || slice.Contain(cfg.GlobalProxyTlds, suffix)
		return Whois(ctx, mainDomain, tld, useProxy)
	case constant.DnsQuery:
		domainInfo, err := dnslib.NsCheckContext(ctx, mainDomain)
		return domainInfo, err
	case constant.MixedQuery:
		switch {
		case slice.Contain(cfg.MixedDnsTlds, tld) || slice.Contain(cfg.MixedDnsTlds, suffix):
			domainInfo, err := dnslib.NsCheckContext(ctx, mainDomain)
			return domainInfo, err
		case !rdaplib.SupportsTLD(tld) && !maputil.HasKey(whoislib.WhoisSupportedTlds, tld):
			domainInfo, err := dnslib.NsCheckContext(ctx, mainDomain)
			return domainInfo, err
		case slice.Contain(cfg.MixedProxyTlds, tld) || slice.Contain(cfg.MixedProxyTlds, suffix):
			domainInfo, err := Whois(ctx, mainDomain, tld, true)
			return domainInfo, err
		case slice.Contain(cfg.GlobalProxyTlds, tld) || slice.Contain(cfg.GlobalProxyTlds, suffix):
			domainInfo, err := Whois(ctx, mainDomain, tld, true)
			return domainInfo, err
		default:
			domainInfo, err := Whois(ctx, mainDomain, tld, false)
			return domainInfo, err
		}
	default:
		domainInfo, err := customize.CustomizeLookupContext(ctx, mainDomain, queryType)
		return domainInfo, err
	}
}

func Whois(ctx context.Context, domain, tld string, viaProxy bool) (lookupinfo.DomainInfo, error) {
	// Jisc manages ac.uk separately; Nominet's RDAP 404 is not registration evidence there.
	if strings.HasSuffix(domain, ".ac.uk") {
		return lookupinfo.DomainInfo{DomainName: domain, Source: "local-validation"}, fmt.Errorf("%w: ac.uk requires a Jisc registry adapter", lookuperror.ErrorNotSupportedTld)
	}
	viaProxy = viaProxy && !network.IsPublic(ctx)
	if rdaplib.SupportsTLD(tld) {
		return rdaplib.RDAPQueryContext(ctx, domain, tld, viaProxy)
	}
	if !maputil.HasKey(whoislib.WhoisSupportedTlds, tld) {
		return lookupinfo.DomainInfo{DomainName: domain, Source: "local-validation"}, fmt.Errorf("%w: %s", lookuperror.ErrorNotSupportedTld, tld)
	}
	return whoislib.WhoisQueryContext(ctx, domain, tld, viaProxy)
}

func Lookup(domain, mode string) (lookupinfo.DomainInfo, error) {
	return lookup(context.Background(), domain, mode)
}

func publicMode(mode string) bool {
	switch mode {
	case constant.WhoisQuery, constant.DnsQuery, constant.MixedQuery:
		return true
	}
	return false
}

func ValidMode(mode string) bool {
	if publicMode(mode) || mode == constant.WhoisQueryWithProxy {
		return true
	}
	for _, a := range config.GetConfig().WhoisApis {
		if a.ApiName == mode {
			return true
		}
	}
	return false
}

// Query is the only result-classification and retry path used by all entry points.
func Query(ctx context.Context, domain, mode string, timeoutSeconds, retries int) lookupinfo.QueryResult {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	var info lookupinfo.DomainInfo
	var err error
	for attempt := 0; attempt <= retries; attempt++ {
		info, err = lookup(ctx, domain, mode)
		if err == nil || errors.Is(err, lookuperror.ErrorWhoisNotFound) || errors.Is(err, lookuperror.ErrorNsNotFound) || ctx.Err() != nil {
			break
		}
		var netErr net.Error
		if !errors.As(err, &netErr) && !errors.Is(err, lookuperror.ErrorWhoisTimeout) && !errors.Is(err, lookuperror.ErrorDnsTimeout) && !errors.Is(err, lookuperror.ErrorRateLimited) {
			break
		}
		if attempt < retries {
			select {
			case <-ctx.Done():
			case <-time.After(time.Duration(max(1, config.GetConfig().RetryInterval)) * time.Second):
			}
		}
	}
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	return Result(domain, info, err)
}

func Result(domain string, info lookupinfo.DomainInfo, err error) lookupinfo.QueryResult {
	r := lookupinfo.QueryResult{Domain: domain, LookupType: info.LookupType, ViaProxy: info.ViaProxy, RawResponse: info.RawResponse, Source: info.Source, QueriedAt: time.Now().UTC().Format(time.RFC3339Nano), Status: "unknown", RegisterStatus: "Unknown", NameServer: []string{}}
	if r.Source == "" {
		r.Source = info.LookupType
	}
	switch {
	case errors.Is(err, lookuperror.ErrorWhoisNotFound):
		r.Status = "not_found"
		r.RegisterStatus = "NotFound"
	case errors.Is(err, lookuperror.ErrorNsNotFound):
		r.QueryError = "DNS has no delegation; registration status is unknown"
	case err != nil:
		r.Status = "error"
		r.RegisterStatus = "Error"
		r.QueryError = err.Error()
		var netErr net.Error
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, lookuperror.ErrorWhoisTimeout) || errors.Is(err, lookuperror.ErrorDnsTimeout) || (errors.As(err, &netErr) && netErr.Timeout()) {
			r.Status = "timeout"
			r.RegisterStatus = "Timeout"
		}
		if errors.Is(err, lookuperror.ErrorReservedDomain) || errors.Is(err, lookuperror.ErrorNotSupportedTld) || errors.Is(err, lookuperror.ErrorNoParseRuleForTld) || errors.Is(err, lookuperror.ErrorParseWhoisResponse) {
			r.Status = "unknown"
			r.RegisterStatus = "Unknown"
		}
	case info.CustomizedResult == constant.DomainRegisterStatusFree:
		r.Status = "not_found"
		r.RegisterStatus = "NotFound"
	case info.LookupType == constant.LookupTypeDNS:
		if len(info.NameServer) > 0 {
			r.Status = "registered"
			r.RegisterStatus = "Taken"
		} else {
			r.QueryError = "DNS has no delegation; registration status is unknown"
		}
	case info.LookupType == constant.LookupTypeWhois || info.LookupType == constant.LookupTypeRDAP || info.CustomizedResult == constant.DomainRegisterStatusTaken:
		r.Status = "registered"
		r.RegisterStatus = "Taken"
	}
	r.CreatedDate = info.CreationDate
	r.ExpiryDate = info.ExpiryDate
	for _, ns := range info.NameServer {
		r.NameServer = append(r.NameServer, strings.ToLower(ns))
	}
	r.DnsLite = utils.GetDnsLite(r.NameServer)
	r.RawDomainStatus = info.DomainStatus
	r.DomainStatus = utils.GetDomainHumanStatus(info.DomainStatus)
	return r
}

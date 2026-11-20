package lookuper

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"typonamer/config"
	"typonamer/constant"
	"typonamer/log"
	"typonamer/lookup/customize"
	"typonamer/lookup/dnslib"
	"typonamer/lookup/lookuperror"
	"typonamer/lookup/lookupinfo"
	"typonamer/lookup/rdaplib"
	"typonamer/lookup/whoislib"
	"typonamer/utils"

	"github.com/duke-git/lancet/v2/maputil"
	"github.com/duke-git/lancet/v2/retry"
	"github.com/duke-git/lancet/v2/slice"
)

func Lookup(domain string, queryType string) (lookupinfo.DomainInfo, error) {
	var errDomainInfo = lookupinfo.DomainInfo{
		DomainName: domain,
	}

	cfg := config.GetConfig()

	// Get the TLD (Top-Level Domain) of the domain
	tld, suffix, err := utils.GetTld(domain)
	if err != nil || tld == "" || suffix == "" {
		log.Error("Invalid domain name: ", domain)
		return errDomainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorInvalidDomainName, err)
	}

	// Get the main domain
	mainDomain, err := utils.TrimAndGetMainDomain(domain)
	if err != nil {
		log.Error("Invalid domain name: ", domain)
		return errDomainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorInvalidDomainName, err)
	}

	switch queryType {
	case constant.WhoisQuery:
		if !slice.Contain(rdaplib.RdapSupportedTlds, tld) && !maputil.HasKey(whoislib.WhoisSupportedTlds, tld) {
			log.Error("Not supported TLD: ", tld)
			return errDomainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorNotSupportedTld, tld)
		}

		useProxy := false
		if !useProxy {
			if slice.Contain(cfg.GlobalProxyTlds, tld) {
				log.Debugf("%s is the TLD that needs to go through proxy, forcing the whois query to go through proxy", tld)
				useProxy = true
			}
			if slice.Contain(cfg.GlobalProxyTlds, suffix) {
				log.Debugf("%s is the TLD that needs to go through proxy, forcing the whois query to go through proxy", suffix)
				useProxy = true
			}
		}
		domainInfo, err := Whois(mainDomain, tld, useProxy)
		return domainInfo, err
	case constant.WhoisQueryWithProxy:
		if !slice.Contain(rdaplib.RdapSupportedTlds, tld) && !maputil.HasKey(whoislib.WhoisSupportedTlds, tld) {
			log.Error("Not supported TLD: ", tld)
			return errDomainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorNotSupportedTld, tld)
		}

		domainInfo, err := Whois(mainDomain, tld, true)
		return domainInfo, err
	case constant.DnsQuery:
		domainInfo, err := dnslib.NsCheck(mainDomain)
		return domainInfo, err
	case constant.MixedQuery:
		switch {
		case slice.Contain(cfg.MixedDnsTlds, tld) || slice.Contain(cfg.MixedDnsTlds, suffix):
			domainInfo, err := dnslib.NsCheck(mainDomain)
			return domainInfo, err
		case !slice.Contain(rdaplib.RdapSupportedTlds, tld) && !maputil.HasKey(whoislib.WhoisSupportedTlds, tld):
			domainInfo, err := dnslib.NsCheck(mainDomain)
			return domainInfo, err
		case slice.Contain(cfg.MixedProxyTlds, tld) || slice.Contain(cfg.MixedProxyTlds, suffix):
			domainInfo, err := Whois(mainDomain, tld, true)
			return domainInfo, err
		case slice.Contain(cfg.GlobalProxyTlds, tld) || slice.Contain(cfg.GlobalProxyTlds, suffix):
			domainInfo, err := Whois(mainDomain, tld, true)
			return domainInfo, err
		default:
			domainInfo, err := Whois(mainDomain, tld, false)
			return domainInfo, err
		}
	default:
		domainInfo, err := customize.CustomizeLookup(mainDomain, queryType)
		return domainInfo, err
	}
}

func Whois(mainDomain string, tld string, useProxy bool) (lookupinfo.DomainInfo, error) {
	var domainInfo = lookupinfo.DomainInfo{
		DomainName: mainDomain,
		LookupType: constant.LookupTypeWhois,
		ViaProxy:   useProxy,
	}

	cfg := config.GetConfig()
	if useProxy && (strings.TrimSpace(cfg.SocketProxyHost) == "" || cfg.SocketProxyPort <= 0) {
		return domainInfo, lookuperror.ErrorProxyNotConfigured
	}

	// If the RDAP server for the TLD is known, query the RDAP information for the domain
	if slice.Contain(rdaplib.RdapSupportedTlds, tld) {
		domainInfo.LookupType = constant.LookupTypeRDAP

		if cfg.RetryOnTimeout {
			var rdapErr error
			getDomainInfo := func() error {
				domainInfo, rdapErr = rdaplib.RDAPQuery(mainDomain, tld, useProxy)
				if rdapErr != nil {
					switch {
					case errors.Is(rdapErr, lookuperror.ErrorConnectToProxy):
						return rdapErr
					case errors.Is(rdapErr, lookuperror.ErrorWhoisTimeout):
						return rdapErr
					case errors.Is(rdapErr, lookuperror.ErrorWhoisServerFailed):
						return rdapErr
					default:
						return nil
					}
				}
				return nil
			}

			err := retry.Retry(getDomainInfo, retry.RetryTimes(uint(cfg.RetryMax)), retry.RetryWithLinearBackoff(time.Second*time.Duration(cfg.RetryInterval)))

			if err != nil {
				log.Errorf("Failed to query RDAP for domain %s with error: %s", mainDomain, err)
			}

			if rdapErr != nil {
				return domainInfo, rdapErr
			}

			log.Debugf("RDAP query result: \n%+v", domainInfo)

			return domainInfo, nil
		} else {
			domainInfo, err := rdaplib.RDAPQuery(mainDomain, tld, useProxy)
			if err != nil {
				return domainInfo, err
			}

			log.Debugf("RDAP query result: \n%+v", domainInfo)

			return domainInfo, nil
		}

		// If the WHOIS server for the TLD is known, query the WHOIS information for the domain
	} else if maputil.HasKey(whoislib.WhoisSupportedTlds, tld) {
		domainInfo.LookupType = constant.LookupTypeWhois

		if cfg.RetryOnTimeout {
			var whoisErr error
			getDomainInfo := func() error {
				domainInfo, whoisErr = whoislib.WhoisQuery(mainDomain, tld, useProxy)
				if whoisErr != nil {
					switch {
					case errors.Is(whoisErr, lookuperror.ErrorConnectToProxy):
						return whoisErr
					case errors.Is(whoisErr, lookuperror.ErrorWhoisTimeout):
						return whoisErr
					case errors.Is(whoisErr, lookuperror.ErrorWhoisServerFailed):
						return whoisErr
					case errors.Is(whoisErr, lookuperror.ErrorNoContentInWhoisResponse):
						return whoisErr
					default:
						return nil
					}
				}
				return nil
			}

			err := retry.Retry(getDomainInfo, retry.RetryTimes(uint(cfg.RetryMax)), retry.RetryWithLinearBackoff(time.Second*time.Duration(cfg.RetryInterval)))

			if err != nil {
				log.Errorf("Failed to query WHOIS for domain %s with error: %s", mainDomain, err)
			}

			if whoisErr != nil {
				return domainInfo, whoisErr
			}

			log.Debugf("WHOIS query domain result: \n%+v", domainInfo)

			return domainInfo, nil
		} else {
			domainInfo, err := whoislib.WhoisQuery(mainDomain, tld, useProxy)
			if err != nil {
				return domainInfo, err
			}

			log.Debugf("WHOIS query domain result: \n%+v", domainInfo)

			return domainInfo, nil
		}
	} else {
		log.Error("No RDAP or WHOIS server known for TLD: ", tld)
		return domainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorNoWhoisServerForTld, tld)
	}
}

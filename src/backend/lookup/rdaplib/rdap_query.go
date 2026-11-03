package rdaplib

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"typonamer/config"
	"typonamer/constant"
	"typonamer/log"
	"typonamer/lookup/lookuperror"
	"typonamer/lookup/lookupinfo"

	"github.com/duke-git/lancet/v2/slice"
	"github.com/duke-git/lancet/v2/strutil"
	"github.com/openrdap/rdap"
	"github.com/openrdap/rdap/bootstrap"
	"github.com/openrdap/rdap/bootstrap/cache"
	"golang.org/x/net/proxy"
)

const (
	domainFreeStatus = "free"
)

// RDAPQuery function is used to query the RDAP (Registration Data Access Protocol) information for a given domain.
//
// RDAP is a protocol used to retrieve information about domain names and
// Internet number resources. It is designed to be a replacement for the
// WHOIS protocol, which is used to retrieve information about domain names.
func RDAPQuery(domain string, tld string, useProxy bool) (lookupinfo.DomainInfo, error) {
	log.Debugf("Querying RDAP for domain: %s", domain)

	var domainInfo = lookupinfo.DomainInfo{
		LookupType: constant.LookupTypeRDAP,
		ViaProxy:   useProxy,
	}

	cfg := config.GetConfig()

	httpClient := &http.Client{
		Timeout: time.Duration(cfg.WhoisTimeout) * time.Second,
		Transport: &http.Transport{
			DisableKeepAlives:   true,
			TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
			TLSHandshakeTimeout: time.Duration(cfg.WhoisTimeout) * time.Second,
		},
	}

	if useProxy {
		// Set up the proxy server
		proxyServer := net.JoinHostPort(cfg.SocketProxyHost, strconv.Itoa(cfg.SocketProxyPort))
		if cfg.SocketProxyAuth {
			proxyAuth := proxy.Auth{
				User:     cfg.SocketProxyUser,
				Password: cfg.SocketProxyPassword,
			}
			dialer, err := proxy.SOCKS5("tcp", proxyServer, &proxyAuth, proxy.Direct)
			if err == nil {
				httpClient.Transport = &http.Transport{Dial: dialer.Dial}
			} else {
				log.Warnf("Failed to create proxy dialer with auth: %s", err)
				return domainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorConnectToProxy, err.Error())
			}
		} else {
			dialer, err := proxy.SOCKS5("tcp", proxyServer, nil, proxy.Direct)
			if err == nil {
				httpClient.Transport = &http.Transport{Dial: dialer.Dial}
			} else {
				log.Warnf("Failed to create proxy dialer: %s", err)
				return domainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorConnectToProxy, err.Error())
			}
		}
	}

	// Set up the RDAP client
	rdapReq := &rdap.Request{
		Type:    rdap.DomainRequest,
		Query:   domain,
		Timeout: time.Duration(cfg.WhoisTimeout) * time.Second,
	}

	rdapClient := &rdap.Client{
		HTTP: httpClient,
		Bootstrap: &bootstrap.Client{
			Cache: cache.NewDiskCache(),
		},
	}

	// Do the RDAP query
	rdapResp, err := rdapClient.Do(rdapReq)
	if err != nil {
		log.Debugf("Failed to query RDAP for domain %s: %s", domain, err)

		domainInfo.RawResponse = err.Error()

		if strutil.ContainsAny(err.Error(), []string{"No RDAP servers responded successfully"}) {
			return domainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorWhoisTimeout, err.Error())
		} else if strutil.ContainsAny(err.Error(), []string{"No RDAP servers found for"}) {
			return domainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorNotSupportedTld, tld)
		} else if strutil.ContainsAny(err.Error(), []string{"RDAP server returned 404"}) {

			return domainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorWhoisNotFound, domain)
		} else if strutil.ContainsAny(err.Error(), []string{"Server returned error code"}) {
			return domainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorWhoisServerFailed, err.Error())
		} else {
			log.Errorf("Failed to query RDAP for domain %s: %s", domain, err)
			return domainInfo, err
		}
	}

	// Parse the RDAP response
	if domainResult, ok := rdapResp.Object.(*rdap.Domain); ok {
		log.Debugf("RDAP response of domain %s is: \n%+v", domain, domainResult)

		domainInfo := ParseRDAPResponseforDomain(domainResult)
		domainInfo.RawResponse = getWhoisStyleRawResponse(rdapResp)
		domainInfo.ViaProxy = useProxy

		if slice.Contain(domainInfo.DomainStatus, domainFreeStatus) && len(domainInfo.NameServer) == 0 {
			return domainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorWhoisNotFound, domain)
		}

		if domainInfo.Registrar == "" && domainInfo.CreationDate == "" && domainInfo.ExpiryDate == "" && len(domainInfo.NameServer) == 0 {
			return domainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorWhoisNotFound, domain)
		}

		return domainInfo, nil
	} else {
		log.Error("RDAP server returned unexpected response for domain: ", domain)
		return domainInfo, fmt.Errorf("%w: %s", lookuperror.ErrorWhoisServerFailed, "RDAP server returned unexpected response")
	}
}

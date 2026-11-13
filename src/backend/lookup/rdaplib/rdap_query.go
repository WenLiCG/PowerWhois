package rdaplib

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
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

	var proxyDialFailed atomic.Bool
	var proxyDials sync.WaitGroup
	var proxyDialMu sync.Mutex
	proxyDialsStopped := false
	if useProxy {
		proxyServer := net.JoinHostPort(cfg.SocketProxyHost, strconv.Itoa(cfg.SocketProxyPort))
		var auth *proxy.Auth
		if cfg.SocketProxyAuth {
			auth = &proxy.Auth{User: cfg.SocketProxyUser, Password: cfg.SocketProxyPassword}
		}
		timeout := time.Duration(cfg.WhoisTimeout) * time.Second
		dialer, err := proxy.SOCKS5("tcp", proxyServer, auth, &net.Dialer{Timeout: timeout})
		if err != nil {
			log.Warn("代理拨号失败")
			return domainInfo, lookuperror.ErrorConnectToProxy
		}
		contextDialer, ok := dialer.(proxy.ContextDialer)
		if !ok {
			log.Warn("代理拨号失败")
			return domainInfo, lookuperror.ErrorConnectToProxy
		}
		httpClient.Transport.(*http.Transport).DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			// Transport can return before a timed-out dial finishes. Wait for
			// registered dials before classifying the RDAP result.
			proxyDialMu.Lock()
			if proxyDialsStopped {
				proxyDialMu.Unlock()
				return nil, context.Canceled
			}
			proxyDials.Add(1)
			proxyDialMu.Unlock()
			defer proxyDials.Done()
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			conn, err := contextDialer.DialContext(ctx, network, address)
			if err != nil {
				proxyDialFailed.Store(true)
				log.Warn("代理拨号失败")
				return nil, lookuperror.ErrorConnectToProxy
			}
			return conn, nil
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
	proxyDialMu.Lock()
	proxyDialsStopped = true
	proxyDialMu.Unlock()
	proxyDials.Wait()
	if err != nil {
		if proxyDialFailed.Load() {
			domainInfo.RawResponse = lookuperror.ErrorConnectToProxy.Error()
			return domainInfo, lookuperror.ErrorConnectToProxy
		}
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

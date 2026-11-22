package rdaplib

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"typonamer/config"
	"typonamer/constant"
	"typonamer/lookup/lookuperror"
	"typonamer/lookup/lookupinfo"
	"typonamer/lookup/network"

	"github.com/openrdap/rdap"
	"github.com/openrdap/rdap/bootstrap"
	"golang.org/x/net/idna"
	"golang.org/x/net/proxy"
)

var bootstrapSlot = make(chan struct{}, 1)
var registry = &bootstrap.Client{HTTP: &http.Client{Transport: network.DirectTransport(), Timeout: 5 * time.Second}}

var directClient = &http.Client{Transport: network.DirectTransport(), CheckRedirect: checkRedirect}
var proxyMu sync.Mutex
var proxiedClient *http.Client
var proxySettings struct {
	address, user, password string
	auth                    bool
}

func checkRedirect(req *http.Request, via []*http.Request) error {
	if req.URL.Scheme != "https" || len(via) >= 5 {
		return http.ErrUseLastResponse
	}
	return nil
}

func Bootstrap(ctx context.Context, domain string) ([]*url.URL, error) {
	select {
	case bootstrapSlot <- struct{}{}:
		defer func() { <-bootstrapSlot }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	q := &bootstrap.Question{RegistryType: bootstrap.DNS, Query: domain}
	answer, err := registry.Lookup(q.WithContext(ctx))
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, lookuperror.ErrorWhoisServerFailed
	}
	var servers []*url.URL
	for _, u := range answer.URLs {
		if u.Scheme == "https" && u.Host != "" {
			servers = append(servers, u)
		}
	}
	return servers, nil
}

func RDAPQuery(ctx context.Context, domain string, servers []*url.URL, useProxy bool, cfg config.Config) (lookupinfo.DomainInfo, error) {
	info := lookupinfo.DomainInfo{DomainName: domain, LookupType: constant.LookupTypeRDAP, ViaProxy: useProxy}
	client := directClient
	if useProxy {
		var err error
		client, err = proxyClient(cfg)
		if err != nil {
			return info, err
		}
	}

	lastErr := lookuperror.ErrorNotSupportedTld
	for _, server := range servers {
		request := (&rdap.Request{Type: rdap.DomainRequest, Query: domain, Server: server}).URL()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, request.String(), nil)
		if err != nil {
			continue
		}
		req.Header.Set("Accept", "application/rdap+json, application/json")
		release, err := network.Acquire(ctx, "rdap:"+server.Hostname(), 4)
		if err != nil {
			return info, err
		}
		raw, status, err := network.ReadResponse(client, req)
		release()
		if ctx.Err() != nil {
			return info, ctx.Err()
		}
		if err != nil {
			lastErr = lookuperror.ErrorWhoisServerFailed
			if useProxy {
				lastErr = lookuperror.ErrorConnectToProxy
			}
			continue
		}
		parsed, err := parseResponse(info, raw, status)
		switch err {
		case nil, lookuperror.ErrorWhoisNotFound, lookuperror.ErrorRateLimited:
			return parsed, err
		default:
			lastErr = err
		}
	}
	return info, lastErr
}

func parseResponse(info lookupinfo.DomainInfo, raw string, status int) (lookupinfo.DomainInfo, error) {
	switch status {
	case http.StatusNotFound:
		return info, lookuperror.ErrorWhoisNotFound
	case http.StatusTooManyRequests:
		return info, lookuperror.ErrorRateLimited
	}
	if status < 200 || status >= 300 {
		return info, lookuperror.ErrorWhoisServerFailed
	}
	obj, err := rdap.NewDecoder([]byte(raw)).Decode()
	domain, ok := obj.(*rdap.Domain)
	if err != nil || !ok {
		return info, lookuperror.ErrorParseWhoisResponse
	}
	reportedName := domain.LDHName
	if reportedName == "" {
		reportedName, _ = idna.Lookup.ToASCII(domain.UnicodeName)
	}
	if reportedName != "" && !strings.EqualFold(strings.TrimSuffix(reportedName, "."), info.DomainName) {
		return info, lookuperror.ErrorParseWhoisResponse
	}
	parsed := ParseRDAPResponseforDomain(domain)
	parsed.DomainName = info.DomainName
	parsed.RawResponse = raw
	parsed.ViaProxy = info.ViaProxy
	return parsed, nil
}

func proxyClient(cfg config.Config) (*http.Client, error) {
	if cfg.SocketProxyHost == "" {
		return nil, lookuperror.ErrorConnectToProxy
	}
	addr := net.JoinHostPort(cfg.SocketProxyHost, strconv.Itoa(cfg.SocketProxyPort))
	proxyMu.Lock()
	defer proxyMu.Unlock()
	if proxiedClient != nil && proxySettings.address == addr && proxySettings.auth == cfg.SocketProxyAuth &&
		proxySettings.user == cfg.SocketProxyUser && proxySettings.password == cfg.SocketProxyPassword {
		return proxiedClient, nil
	}
	var auth *proxy.Auth
	if cfg.SocketProxyAuth {
		auth = &proxy.Auth{User: cfg.SocketProxyUser, Password: cfg.SocketProxyPassword}
	}
	dialer, err := proxy.SOCKS5("tcp", addr, auth, &net.Dialer{})
	if err != nil {
		return nil, lookuperror.ErrorConnectToProxy
	}
	transport := network.DirectTransport()
	transport.DialContext = dialer.(proxy.ContextDialer).DialContext
	if proxiedClient != nil {
		proxiedClient.CloseIdleConnections()
	}
	proxySettings.address, proxySettings.auth = addr, cfg.SocketProxyAuth
	proxySettings.user, proxySettings.password = cfg.SocketProxyUser, cfg.SocketProxyPassword
	proxiedClient = &http.Client{Transport: transport, CheckRedirect: checkRedirect}
	return proxiedClient, nil
}

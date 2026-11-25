package rdaplib

import (
	"context"
	"errors"
	"fmt"
	"github.com/openrdap/rdap"
	"github.com/openrdap/rdap/bootstrap"
	"github.com/openrdap/rdap/bootstrap/cache"
	"net/http"
	"strings"
	"sync"
	"time"
	"typonamer/config"
	"typonamer/constant"
	"typonamer/lookup/lookuperror"
	"typonamer/lookup/lookupinfo"
	"typonamer/lookup/network"
)

type safeRegistryCache struct {
	mu    sync.RWMutex
	inner *cache.MemoryCache
}

func (c *safeRegistryCache) Load(filename string) ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.inner.Load(filename)
}

func (c *safeRegistryCache) Save(filename string, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.inner.Save(filename, data)
}

func (c *safeRegistryCache) State(filename string) cache.FileState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.inner.State(filename)
}

func (c *safeRegistryCache) SetTimeout(timeout time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.inner.SetTimeout(timeout)
}

var (
	directRDAPTransport = func() *http.Transport {
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.MaxIdleConns = 64
		tr.MaxIdleConnsPerHost = 8
		tr.IdleConnTimeout = 60 * time.Second
		return tr
	}()
	sharedBootstrapCache = &safeRegistryCache{inner: cache.NewMemoryCache()}
)

func RDAPQueryContext(ctx context.Context, domain, tld string, useProxy bool) (lookupinfo.DomainInfo, error) {
	info := lookupinfo.DomainInfo{DomainName: domain, LookupType: constant.LookupTypeRDAP, ViaProxy: useProxy}
	cfg := config.GetConfig()
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.RdapTimeout)*time.Second)
	defer cancel()
	tr := directRDAPTransport
	if useProxy {
		dialer, err := network.SOCKS5Dialer(cfg)
		if err != nil {
			info.Source = "local-validation"
			return info, err
		}
		proxyTr := directRDAPTransport.Clone()
		proxyTr.Proxy = nil
		proxyTr.DialContext = dialer.DialContext
		defer proxyTr.CloseIdleConnections()
		tr = proxyTr
	}
	release, err := network.Acquire(ctx, "rdap")
	if err != nil {
		return info, err
	}
	defer release()
	client := &http.Client{Transport: network.Transport{Base: tr, Protocol: "rdap"}, Timeout: time.Duration(cfg.RdapTimeout) * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 || req.URL.Scheme != "https" {
				return fmt.Errorf("unsafe RDAP redirect")
			}
			return nil
		}}
	rc := &rdap.Client{HTTP: client, Bootstrap: &bootstrap.Client{HTTP: client, Cache: sharedBootstrapCache}}
	resp, err := rc.Do((&rdap.Request{Type: rdap.DomainRequest, Query: domain, Timeout: time.Duration(cfg.RdapTimeout) * time.Second}).WithContext(ctx))
	parsed, err := parseResponse(domain, info, resp, err)
	if ctx.Err() != nil {
		return parsed, ctx.Err()
	}
	return parsed, err
}

func parseResponse(domain string, info lookupinfo.DomainInfo, resp *rdap.Response, err error) (lookupinfo.DomainInfo, error) {
	if resp != nil {
		for _, h := range resp.HTTP {
			info.Source = h.URL
			if len(h.Body) > 0 {
				info.RawResponse = string(h.Body)
			}
			if h.Response != nil && h.Response.StatusCode == 429 {
				return info, lookuperror.ErrorRateLimited
			}
		}
	}
	if err != nil {
		var ce *rdap.ClientError
		if errors.As(err, &ce) {
			switch ce.Type {
			case rdap.ObjectDoesNotExist:
				return info, lookuperror.ErrorWhoisNotFound
			case rdap.BootstrapNoMatch, rdap.BootstrapNotSupported:
				return info, lookuperror.ErrorNotSupportedTld
			}
		}
		return info, fmt.Errorf("%w: %s", lookuperror.ErrorWhoisServerFailed, err)
	}
	d, ok := resp.Object.(*rdap.Domain)
	if !ok || !strings.EqualFold(d.LDHName, domain) {
		return info, lookuperror.ErrorParseWhoisResponse
	}
	parsed := ParseRDAPResponseforDomain(d)
	parsed.Source = info.Source
	parsed.ViaProxy = info.ViaProxy
	parsed.RawResponse = getWhoisStyleRawResponse(resp)
	// A valid domain object remains registered even when all contact data is redacted.
	return parsed, nil
}
func RDAPQuery(domain, tld string, viaProxy bool) (lookupinfo.DomainInfo, error) {
	return RDAPQueryContext(context.Background(), domain, tld, viaProxy)
}

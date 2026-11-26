package rdaplib

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/openrdap/rdap"
	"github.com/openrdap/rdap/bootstrap"
	"github.com/openrdap/rdap/bootstrap/cache"
	"mime"
	"net"
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
	state := c.inner.State(filename)
	// Each RDAP query instantiates a fresh bootstrap.Client whose local registries map is empty.
	// openrdap only loads from Cache when State returns ShouldReload; returning Good would skip
	// Cache.Load and force a full network re-download of data.iana.org/rdap/dns.json every time.
	if state == cache.Good {
		return cache.ShouldReload
	}
	return state
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
	var dialer interface {
		DialContext(context.Context, string, string) (net.Conn, error)
	}
	if useProxy {
		proxyDialer, err := network.SOCKS5Dialer(cfg)
		if err != nil {
			info.Source = "local-validation"
			return info, err
		}
		dialer = proxyDialer
	}
	release, err := network.Acquire(ctx, "rdap")
	if err != nil {
		return info, err
	}
	defer release()
	// Apply per-attempt RDAP timeout only after acquiring a concurrency slot.
	attemptCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.RdapTimeout)*time.Second)
	defer cancel()
	tr := directRDAPTransport
	if useProxy {
		proxyTr := directRDAPTransport.Clone()
		proxyTr.Proxy = nil
		proxyTr.DialContext = dialer.DialContext
		defer proxyTr.CloseIdleConnections()
		tr = proxyTr
	}
	client := &http.Client{Transport: network.Transport{Base: tr, Protocol: "rdap"}, Timeout: time.Duration(cfg.RdapTimeout) * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 || req.URL.Scheme != "https" {
				return fmt.Errorf("unsafe RDAP redirect")
			}
			return nil
		}}
	rc := &rdap.Client{HTTP: client, Bootstrap: &bootstrap.Client{HTTP: client, Cache: sharedBootstrapCache}}
	resp, err := rc.Do((&rdap.Request{Type: rdap.DomainRequest, Query: domain, Timeout: time.Duration(cfg.RdapTimeout) * time.Second}).WithContext(attemptCtx))
	parsed, err := parseResponse(domain, info, resp, err)
	if attemptCtx.Err() != nil && !errors.Is(err, lookuperror.ErrorConnectToProxy) && !errors.Is(err, lookuperror.ErrorWhoisNotFound) {
		return parsed, attemptCtx.Err()
	}
	return parsed, err
}

func registryNotFound(h *rdap.HTTPResponse) bool {
	mediaType, _, err := mime.ParseMediaType(h.Response.Header.Get("Content-Type"))
	if err != nil || (mediaType != "application/rdap+json" && mediaType != "application/json") {
		return false
	}
	// Some registries send an empty RDAP 404 and close the stream with a TLS error.
	if len(h.Body) == 0 {
		return mediaType == "application/rdap+json"
	}
	var reply struct {
		ErrorCode int `json:"errorCode"`
	}
	return json.Unmarshal(h.Body, &reply) == nil && reply.ErrorCode == http.StatusNotFound
}

func parseResponse(domain string, info lookupinfo.DomainInfo, resp *rdap.Response, err error) (lookupinfo.DomainInfo, error) {
	// The SDK tries alternate registry URLs until one answers. Evidence belongs to the final attempt.
	var lastHTTPErr error
	if resp != nil && len(resp.HTTP) > 0 {
		h := resp.HTTP[len(resp.HTTP)-1]
		info.Source = h.URL
		info.RawResponse = string(h.Body)
		lastHTTPErr = h.Error
		if h.Response != nil {
			if req := h.Response.Request; req != nil && req.URL != nil {
				info.Source = req.URL.String()
			}
			if h.Response.StatusCode == http.StatusTooManyRequests {
				return info, lookuperror.ErrorRateLimited
			}
			if h.Response.StatusCode == http.StatusNotFound {
				if registryNotFound(h) {
					return info, lookuperror.ErrorWhoisNotFound
				}
				return info, fmt.Errorf("%w: HTTP 404 without an RDAP negative response", lookuperror.ErrorWhoisServerFailed)
			}
		}
		if lastHTTPErr != nil && errors.Is(lastHTTPErr, lookuperror.ErrorConnectToProxy) {
			return info, lastHTTPErr
		}
	}
	if err != nil {
		if errors.Is(err, lookuperror.ErrorConnectToProxy) {
			return info, err
		}
		var ce *rdap.ClientError
		if errors.As(err, &ce) {
			switch ce.Type {
			case rdap.BootstrapNoMatch, rdap.BootstrapNotSupported:
				return info, lookuperror.ErrorNotSupportedTld
			case rdap.NoWorkingServers:
				if lastHTTPErr != nil {
					return info, fmt.Errorf("%w: %w", lookuperror.ErrorWhoisServerFailed, lastHTTPErr)
				}
			}
		}
		return info, fmt.Errorf("%w: %w", lookuperror.ErrorWhoisServerFailed, err)
	}
	d, ok := resp.Object.(*rdap.Domain)
	if !ok || !strings.EqualFold(strings.TrimSuffix(strings.TrimSpace(d.LDHName), "."), domain) {
		return info, lookuperror.ErrorParseWhoisResponse
	}
	parsed := ParseRDAPResponseforDomain(d)
	parsed.Source = info.Source
	parsed.ViaProxy = info.ViaProxy
	parsed.RawResponse = info.RawResponse
	// A valid domain object remains registered even when all contact data is redacted.
	return parsed, nil
}
func RDAPQuery(domain, tld string, viaProxy bool) (lookupinfo.DomainInfo, error) {
	return RDAPQueryContext(context.Background(), domain, tld, viaProxy)
}

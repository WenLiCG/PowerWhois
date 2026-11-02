package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Provider interface {
	Lookup(context.Context, string) Result
}
type hostGate struct {
	mu       sync.Mutex
	next     map[string]time.Time
	interval time.Duration
}

func providerHost(host string) string {
	u := url.URL{Host: host}
	name, port := strings.ToLower(strings.TrimSuffix(u.Hostname(), ".")), u.Port()
	if n, err := strconv.Atoi(port); err == nil {
		port = strconv.Itoa(n)
	}
	if port == "" || port == "443" {
		return name
	}
	return net.JoinHostPort(name, port)
}

func (g *hostGate) wait(ctx context.Context, host string) error {
	host = providerHost(host)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		g.mu.Lock()
		now := time.Now()
		next := g.next[host]
		if !now.Before(next) {
			g.next[host] = now.Add(g.interval)
			g.mu.Unlock()
			return nil
		}
		g.mu.Unlock()
		if err := sleep(ctx, time.Until(next)); err != nil {
			return err
		}
	}
}
func (g *hostGate) deferHost(host string, delay time.Duration) {
	host = providerHost(host)
	g.mu.Lock()
	defer g.mu.Unlock()
	until := time.Now().Add(delay)
	if until.After(g.next[host]) {
		g.next[host] = until
	}
}
func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// RDAP uses only GET against IANA-advertised HTTPS registry endpoints. It does
// not call DNS, registrar availability APIs, registration APIs or purchase APIs.
type RDAP struct {
	client       *http.Client
	gate         *hostGate
	mu           sync.Mutex
	endpoints    map[string]string
	bootstrapURL string
	timeout      time.Duration
	backoff      time.Duration
	maxAttempts  int
}

func NewRDAP(interval, timeout time.Duration) *RDAP {
	p := &RDAP{gate: &hostGate{next: map[string]time.Time{}, interval: interval}, endpoints: map[string]string{}, bootstrapURL: "https://data.iana.org/rdap/dns.json", timeout: timeout, backoff: time.Second, maxAttempts: 3}
	p.client = &http.Client{Timeout: timeout, Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, MaxConnsPerHost: 10, MaxIdleConns: 10, IdleConnTimeout: 30 * time.Second, TLSHandshakeTimeout: timeout}, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || req.URL.Scheme != "https" || req.URL.User != nil {
			return fmt.Errorf("unsafe RDAP redirect")
		}
		return p.gate.wait(req.Context(), req.URL.Host)
	}}
	return p
}
func (p *RDAP) endpoint(ctx context.Context, domain string) (string, error) {
	tld := domain[strings.LastIndex(domain, ".")+1:]
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.endpoints) == 0 {
		req, e := http.NewRequestWithContext(ctx, http.MethodGet, p.bootstrapURL, nil)
		if e != nil {
			return "", e
		}
		resp, e := p.client.Do(req)
		if e != nil {
			return "", e
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return "", fmt.Errorf("IANA bootstrap returned HTTP %d", resp.StatusCode)
		}
		var doc struct {
			Services [][][]string `json:"services"`
		}
		if e = json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&doc); e != nil {
			return "", e
		}
		for _, service := range doc.Services {
			if len(service) != 2 {
				continue
			}
			for _, base := range service[1] {
				u, e := url.Parse(base)
				if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
					continue
				}
				for _, t := range service[0] {
					if p.endpoints[t] == "" {
						p.endpoints[t] = strings.TrimRight(base, "/") + "/domain/"
					}
				}
				break
			}
		}
		if len(p.endpoints) == 0 {
			return "", fmt.Errorf("empty IANA bootstrap")
		}
	}
	return p.endpoints[tld], nil
}
func (p *RDAP) Lookup(ctx context.Context, domain string) Result {
	result := Result{Status: "error", Attempts: []Attempt{}}
	bootCtx, bootCancel := context.WithTimeout(ctx, p.timeout)
	base, err := p.endpoint(bootCtx, domain)
	bootCancel()
	if err != nil {
		result.CheckedAt = time.Now().UTC()
		result.Provider = p.bootstrapURL
		result.Error = err.Error()
		result.Attempts = append(result.Attempts, Attempt{CheckedAt: result.CheckedAt, Provider: p.bootstrapURL, Error: result.Error})
		return result
	}
	if base == "" {
		result.Status = "unknown"
		result.Error = "no authoritative HTTPS RDAP provider for TLD"
		result.CheckedAt = time.Now().UTC()
		return result
	}
	provider := base + url.PathEscape(domain)
	u, _ := url.Parse(provider)
	for attempt := 0; attempt < p.maxAttempts; attempt++ {
		if err = p.gate.wait(ctx, u.Host); err != nil {
			result.Error = err.Error()
			break
		}
		started := time.Now().UTC()
		a := Attempt{CheckedAt: started, Provider: provider}
		callCtx, cancel := context.WithTimeout(ctx, p.timeout)
		req, e := http.NewRequestWithContext(callCtx, http.MethodGet, provider, nil)
		if e != nil {
			cancel()
			result.Error = e.Error()
			break
		}
		req.Header.Set("Accept", "application/rdap+json, application/json")
		req.Header.Set("User-Agent", "PowerWhois-local/1.0 (read-only RDAP)")
		resp, e := p.client.Do(req)
		retry := false
		delay := p.backoff * time.Duration(1<<attempt)
		result.Status = "error"
		result.Evidence = ""
		result.Error = ""
		result.HTTPStatus = 0
		if e != nil {
			if failed, ok := e.(*url.Error); ok {
				a.Provider = failed.URL
			}
			a.Error = e.Error()
			result.Error = a.Error
			retry = true
		} else {
			responseURL := req.URL
			if resp.Request != nil && resp.Request.URL != nil {
				responseURL = resp.Request.URL
			}
			a.Provider = responseURL.String()
			a.HTTPStatus = resp.StatusCode
			result.HTTPStatus = resp.StatusCode
			// Honor response headers before a slow, broken or oversized body is read.
			if resp.StatusCode == 429 || resp.StatusCode >= 500 {
				result.Error = fmt.Sprintf("provider HTTP %d", resp.StatusCode)
				retry = true
				if d, ok := retryAfter(resp.Header.Get("Retry-After")); ok && d > delay {
					delay = d
				}
				p.gate.deferHost(responseURL.Host, delay)
			}
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, (128<<10)+1))
			resp.Body.Close()
			if readErr != nil {
				result.Error = readErr.Error()
				retry = true
			} else if len(body) > 128<<10 {
				result.Error = "RDAP response exceeds evidence limit"
			} else {
				result.Evidence = string(body)
				var obj struct {
					ObjectClassName string `json:"objectClassName"`
					LDHName         string `json:"ldhName"`
					ErrorCode       int    `json:"errorCode"`
				}
				parseErr := json.Unmarshal(body, &obj)
				switch {
				case retry:
				case resp.StatusCode == 404 && len(bytes.TrimSpace(body)) == 0:
					// RFC 7480 section 5.3 makes the negative response body optional.
					result.Status = "not_found"
				case parseErr != nil:
					result.Error = "malformed RDAP JSON"
				case resp.StatusCode == 200 && obj.ObjectClassName == "domain":
					got, validationErr := Normalize(obj.LDHName)
					if validationErr == nil && got == domain {
						result.Status = "registered"
					} else {
						result.Error = "RDAP domain identity missing or mismatched"
					}
				case resp.StatusCode == 404 && obj.ErrorCode == 404:
					result.Status = "not_found"
				default:
					result.Error = fmt.Sprintf("unverified RDAP response HTTP %d", resp.StatusCode)
				}
			}
			a.Error = result.Error
		}
		cancel()
		a.DurationMS = time.Since(started).Milliseconds()
		result.Attempts = append(result.Attempts, a)
		result.CheckedAt = time.Now().UTC()
		result.Provider = a.Provider
		if !retry || attempt+1 == p.maxAttempts || ctx.Err() != nil {
			break
		}
		if err = sleep(ctx, delay); err != nil {
			result.Error = err.Error()
			break
		}
	}
	return result
}
func retryAfter(v string) (time.Duration, bool) {
	if n, e := strconv.Atoi(v); e == nil && n >= 0 {
		return time.Duration(n) * time.Second, true
	}
	if d, e := http.ParseTime(v); e == nil {
		return time.Until(d), true
	}
	return 0, false
}

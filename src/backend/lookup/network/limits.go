package network

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"typonamer/config"
)

var mu sync.Mutex
var active = map[string]slots{}
var next = map[string]time.Time{}

type slots struct {
	total  int
	public int
}

type publicKey struct{}

// WithPublic marks anonymous Web lookups so they leave capacity for authenticated work.
func WithPublic(ctx context.Context) context.Context {
	return context.WithValue(ctx, publicKey{}, true)
}

func IsPublic(ctx context.Context) bool {
	public, _ := ctx.Value(publicKey{}).(bool)
	return public
}

func settings(protocol string) (int, int) {
	c := config.GetConfig()
	if strings.HasPrefix(protocol, "custom:") {
		for _, api := range c.WhoisApis {
			if "custom:"+api.ApiName == protocol {
				return api.ConcurrencyLimit, c.RdapRate
			}
		}
	}
	switch protocol {
	case "dns":
		return c.DnsConcurrency, c.DnsRate
	case "whois":
		return c.WhoisConcurrency, c.WhoisRate
	default:
		return c.RdapConcurrency, c.RdapRate
	}
}

func Acquire(ctx context.Context, protocol string) (func(), error) {
	public := IsPublic(ctx)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		limit, _ := settings(protocol)
		publicLimit := max(1, limit-max(1, limit/4))
		mu.Lock()
		used := active[protocol]
		if used.total < limit && (!public || used.public < publicLimit) {
			used.total++
			if public {
				used.public++
			}
			active[protocol] = used
			mu.Unlock()
			return func() {
				mu.Lock()
				defer mu.Unlock()
				used := active[protocol]
				used.total--
				if public {
					used.public--
				}
				active[protocol] = used
			}, nil
		}
		mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// Limit each upstream independently. Recheck after sleep so Retry-After also delays queued requests.
func Wait(ctx context.Context, protocol, host string) error {
	key := protocol + ":" + host
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, rate := settings(protocol)
		mu.Lock()
		wait := time.Until(next[key])
		if wait <= 0 {
			next[key] = time.Now().Add(time.Second / time.Duration(max(1, rate)))
			mu.Unlock()
			return nil
		}
		mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
}

func RetryAfter(protocol, host, value string) {
	until, err := http.ParseTime(value)
	if seconds, e := strconv.Atoi(value); e == nil && seconds >= 0 {
		until = time.Now().Add(time.Duration(seconds) * time.Second)
		err = nil
	}
	if err != nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	key := protocol + ":" + host
	if until.After(next[key]) {
		next[key] = until
	}
}

type Transport struct {
	Base     http.RoundTripper
	Protocol string
}

func (t Transport) RoundTrip(r *http.Request) (*http.Response, error) {
	if err := Wait(r.Context(), t.Protocol, r.URL.Host); err != nil {
		return nil, err
	}
	resp, err := t.Base.RoundTrip(r)
	if err == nil {
		resp.Body = http.MaxBytesReader(nil, resp.Body, 2*1024*1024)
	}
	if err == nil && (resp.StatusCode == 429 || resp.StatusCode == 503) {
		RetryAfter(t.Protocol, r.URL.Host, resp.Header.Get("Retry-After"))
	}
	return resp, err
}

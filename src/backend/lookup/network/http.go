package network

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const MaxResponseBytes = 2 << 20

// DefaultRateLimitCooldown applies when a rate-limited upstream gives no usable Retry-After.
const DefaultRateLimitCooldown = 15 * time.Second

// ReadResponse checks the transport status separately from the provider's business status.
func ReadResponse(client *http.Client, req *http.Request) (string, int, error) {
	body, status, _, err := ReadResponseHeader(client, req)
	return body, status, err
}

// ReadResponseHeader is ReadResponse that also returns the response headers.
// Headers are returned for non-2xx responses so callers can read Retry-After.
func ReadResponseHeader(client *http.Client, req *http.Request) (string, int, http.Header, error) {
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", resp.StatusCode, resp.Header, nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes+1))
	if err != nil {
		return "", resp.StatusCode, resp.Header, err
	}
	if len(body) > MaxResponseBytes {
		return "", resp.StatusCode, resp.Header, fmt.Errorf("provider response exceeded %d bytes", MaxResponseBytes)
	}
	return string(body), resp.StatusCode, resp.Header, nil
}

// RetryAfter reads a Retry-After header in seconds or HTTP-date form.
// It returns DefaultRateLimitCooldown when the header is absent or invalid,
// and never more than MaxCooldown.
func RetryAfter(header http.Header, now time.Time) time.Duration {
	value := ""
	if header != nil {
		value = strings.TrimSpace(header.Get("Retry-After"))
	}
	if value == "" {
		return DefaultRateLimitCooldown
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		switch {
		case seconds <= 0:
			return DefaultRateLimitCooldown
		case seconds >= int(MaxCooldown/time.Second):
			return MaxCooldown
		}
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil {
		if wait := at.Sub(now); wait > 0 {
			return min(wait, MaxCooldown)
		}
	}
	return DefaultRateLimitCooldown
}

func DirectTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return transport
}

func NoRedirect(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

var PrivateLookupClient = &http.Client{Transport: DirectTransport(), CheckRedirect: NoRedirect, Timeout: time.Minute}

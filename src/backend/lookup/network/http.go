package network

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

const MaxResponseBytes = 2 << 20

// ReadResponse checks the transport status separately from the provider's business status.
func ReadResponse(client *http.Client, req *http.Request) (string, int, error) {
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", resp.StatusCode, nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes+1))
	if err != nil {
		return "", resp.StatusCode, err
	}
	if len(body) > MaxResponseBytes {
		return "", resp.StatusCode, fmt.Errorf("provider response exceeded %d bytes", MaxResponseBytes)
	}
	return string(body), resp.StatusCode, nil
}

func DirectTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return transport
}

func NoRedirect(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

var PrivateLookupClient = &http.Client{Transport: DirectTransport(), CheckRedirect: NoRedirect, Timeout: time.Minute}

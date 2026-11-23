package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Client struct {
	URL   string
	Token string
	HTTP  *http.Client
}
type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string { return fmt.Sprintf("HTTP %d %s: %s", e.Status, e.Code, e.Message) }
func FromEnv() (*Client, error) {
	base := os.Getenv("POWERWHOIS_URL")
	if base == "" {
		base = "http://127.0.0.1:8080"
	}
	u, e := url.Parse(base)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("invalid POWERWHOIS_URL")
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "https" && (u.Scheme != "http" || (u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()))) {
		return nil, fmt.Errorf("remote POWERWHOIS_URL requires HTTPS")
	}
	token := os.Getenv("POWERWHOIS_QUERY_TOKEN")
	if path := os.Getenv("POWERWHOIS_TOKEN_FILE"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("cannot read token file")
		}
		token = strings.TrimSpace(string(data))
	}
	if len(token) < 32 {
		return nil, fmt.Errorf("set POWERWHOIS_QUERY_TOKEN or POWERWHOIS_TOKEN_FILE (at least 32 characters)")
	}
	return &Client{URL: strings.TrimRight(base, "/"), Token: token, HTTP: &http.Client{Timeout: 125 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *Client) Do(ctx context.Context, method, path string, body interface{}, key string) ([]byte, error) {
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.URL+"/api/v1"+path, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("invalid API request")
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API connection failed; for task creation retry using the same idempotency key")
	}
	defer resp.Body.Close()
	result, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024*1024+1))
	if err != nil || len(result) > 32*1024*1024 {
		return nil, fmt.Errorf("API response unreadable or too large")
	}
	if resp.StatusCode >= 300 {
		var payload struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		json.Unmarshal(result, &payload)
		return nil, &Error{Status: resp.StatusCode, Code: payload.Error.Code, Message: payload.Error.Message}
	}
	return result, nil
}

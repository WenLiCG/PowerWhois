package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"typonamer/client"
)

type QueryArgs struct {
	Domains     []string `json:"domains" jsonschema:"Domain names, one per item"`
	Mode        string   `json:"mode,omitempty" jsonschema:"whois, dns, mixed, proxy, or a configured query API name"`
	Concurrency int      `json:"concurrency,omitempty" jsonschema:"Workers; defaults to 15, bounded by server configuration"`
	Timeout     int      `json:"timeout,omitempty" jsonschema:"Overall seconds per domain, 1 to 120"`
	Retries     int      `json:"retries,omitempty"`
	Key         string   `json:"idempotencyKey,omitempty" jsonschema:"Reuse for task submission after connection loss"`
}
type TypoArgs struct {
	Domain string   `json:"domain"`
	Types  []string `json:"types" jsonschema:"www, skipLetter, doubleLetter, reverseLetter, insertedLetter, wrongHorizontalKey, wrongVerticalKey, customizedReplace"`
	TLDs   []string `json:"tlds,omitempty"`
}
type TaskArgs struct {
	ID string `json:"id" jsonschema:"Task UUID returned by create_task"`
}
type PageArgs struct {
	ID     string `json:"id"`
	Offset int    `json:"offset,omitempty" jsonschema:"Input index cursor, start at zero"`
	Limit  int    `json:"limit,omitempty" jsonschema:"1 to 1000"`
}
type ListArgs struct {
	Offset int `json:"offset,omitempty"`
	Limit  int `json:"limit,omitempty"`
}

func newServer(c *client.Client) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "powerwhois", Version: "1.0.0"}, nil)
	call := func(ctx context.Context, method, path string, body interface{}, key string) (*mcp.CallToolResult, map[string]interface{}, error) {
		raw, e := c.Do(ctx, method, path, body, key)
		if e != nil {
			return nil, nil, e
		}
		var out map[string]interface{}
		if e = json.Unmarshal(raw, &out); e != nil {
			return nil, nil, fmt.Errorf("invalid API response")
		}
		return nil, out, nil
	}
	mcp.AddTool(s, &mcp.Tool{Name: "query_domains", Description: "Query up to 100 domains. DNS absence is unknown; not_found is not a registration guarantee."}, func(ctx context.Context, _ *mcp.CallToolRequest, a QueryArgs) (*mcp.CallToolResult, map[string]interface{}, error) {
		return call(ctx, "POST", "/query", map[string]interface{}{"domains": a.Domains, "mode": a.Mode, "concurrency": a.Concurrency, "timeout": a.Timeout, "retries": a.Retries}, "")
	})
	mcp.AddTool(s, &mcp.Tool{Name: "create_task", Description: "Start a durable domain-query batch and return its task ID immediately. Queries continue after disconnection; use idempotencyKey for retry-safe submission."}, func(ctx context.Context, _ *mcp.CallToolRequest, a QueryArgs) (*mcp.CallToolResult, map[string]interface{}, error) {
		return call(ctx, "POST", "/tasks", map[string]interface{}{"domains": a.Domains, "mode": a.Mode, "concurrency": a.Concurrency, "timeout": a.Timeout, "retries": a.Retries}, a.Key)
	})
	mcp.AddTool(s, &mcp.Tool{Name: "generate_typos", Description: "Generate domain typo candidates using the same generator as Web; performs no registration."}, func(ctx context.Context, _ *mcp.CallToolRequest, a TypoArgs) (*mcp.CallToolResult, map[string]interface{}, error) {
		return call(ctx, "POST", "/typo", a, "")
	})
	mcp.AddTool(s, &mcp.Tool{Name: "list_tasks", Description: "List shared query tasks, newest first, with pagination."}, func(ctx context.Context, _ *mcp.CallToolRequest, a ListArgs) (*mcp.CallToolResult, map[string]interface{}, error) {
		if a.Limit == 0 {
			a.Limit = 100
		}
		return call(ctx, "GET", fmt.Sprintf("/tasks?offset=%d&limit=%d", a.Offset, a.Limit), nil, "")
	})
	for _, op := range []string{"get", "pause", "resume", "cancel", "retry"} {
		operation := op
		mcp.AddTool(s, &mcp.Tool{Name: operation + "_task", Description: map[string]string{"get": "Read task state and counts.", "pause": "Pause a query task; retain all completed results.", "resume": "Resume a paused or failed task; skip committed results.", "cancel": "Cancel a query task; keep partial results. Canceled tasks cannot resume.", "retry": "Create a new task for failed, timeout and unknown results; preserve the original."}[operation]}, func(ctx context.Context, _ *mcp.CallToolRequest, a TaskArgs) (*mcp.CallToolResult, map[string]interface{}, error) {
			method, path := "GET", "/tasks/"+url.PathEscape(a.ID)
			if operation != "get" {
				method = "POST"
				path += "/" + operation
			}
			return call(ctx, method, path, nil, "")
		})
	}
	for _, op := range []string{"results", "export"} {
		operation := op
		mcp.AddTool(s, &mcp.Tool{Name: operation + "_task", Description: "Read a JSON page of completed results. nextOffset is an input-index cursor; active pages may be empty. Re-read from zero after completion for a full export."}, func(ctx context.Context, _ *mcp.CallToolRequest, a PageArgs) (*mcp.CallToolResult, map[string]interface{}, error) {
			if a.Limit == 0 {
				a.Limit = 100
			}
			return call(ctx, "GET", fmt.Sprintf("/tasks/%s/%s?offset=%d&limit=%d", url.PathEscape(a.ID), operation, a.Offset, a.Limit), nil, "")
		})
	}
	return s
}
func main() {
	listen := flag.String("http", "", "Optional loopback HTTP bind, e.g. 127.0.0.1:8081; default stdio")
	flag.Parse()
	c, e := client.FromEnv()
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(2)
	}
	s := newServer(c)
	if *listen == "" {
		e = s.Run(context.Background(), &mcp.StdioTransport{})
	} else {
		host, _, err := net.SplitHostPort(*listen)
		ip := net.ParseIP(host)
		if err != nil || ip == nil || !ip.IsLoopback() {
			fmt.Fprintln(os.Stderr, "MCP HTTP must bind a loopback IP; use a TLS reverse proxy remotely")
			os.Exit(2)
		}
		token := os.Getenv("POWERWHOIS_MCP_TOKEN")
		if len(token) < 32 {
			fmt.Fprintln(os.Stderr, "POWERWHOIS_MCP_TOKEN must have at least 32 characters")
			os.Exit(2)
		}
		handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
		server := &http.Server{Addr: *listen, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/mcp" {
				http.NotFound(w, r)
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" {
				u, err := url.Parse(origin)
				if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || !strings.EqualFold(u.Host, r.Host) {
					http.Error(w, "origin forbidden", 403)
					return
				}
			}
			supplied := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || subtle.ConstantTimeCompare([]byte(supplied), []byte(token)) != 1 {
				http.Error(w, "unauthorized", 401)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 4*1024*1024)
			handler.ServeHTTP(w, r)
		})}
		e = server.ListenAndServe()
	}
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}

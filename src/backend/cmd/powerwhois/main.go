package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"typonamer/client"
)

const help = `PowerWhois: vendor-neutral domain queries. JSON is written only to stdout.
Usage:
  powerwhois query [--mode whois|dns|mixed|proxy] [options] domain...
  powerwhois typo --domain example.com --types skipLetter,doubleLetter [--tlds net,org]
  powerwhois tasks create [options] domain...
  powerwhois tasks list [--offset 0 --limit 100]
  powerwhois tasks get|pause|resume|cancel|retry --id TASK_ID
  powerwhois tasks results|export --id TASK_ID [--offset 0 --limit 1000] [--format json|csv]
Options (flags precede domain arguments):
  --file PATH       One domain per line; - reads stdin (also accepts positional domains).
  --concurrency N   Workers (default 16, bounded by server configuration).
  --timeout N       Overall seconds per domain including retries (default 30, maximum 120).
  --retries N       Transient-query retries (default 0, maximum 5).
  --key STRING     Idempotency key for task creation; reuse after a lost response.
  --json           Explicit JSON output (the default); errors go to stderr.
Credentials: POWERWHOIS_QUERY_TOKEN or POWERWHOIS_TOKEN_FILE. Never pass a secret as a flag.
Server: POWERWHOIS_URL (default http://127.0.0.1:8080); remote URLs require HTTPS.
Examples:
  powerwhois query --mode dns example.com example.org
  powerwhois tasks create --file domains.txt --mode mixed --key inventory-2026-11-03
  powerwhois tasks results --id TASK_ID --limit 1000
  powerwhois tasks export --id TASK_ID --format csv --offset 0 --limit 1000 > results-page.csv
Long tasks continue on the server after this process exits. Page cursors use input indexes;
active pages may be empty. Re-read from offset 0 after completion for a complete export.
Exit: 0 success; 2 usage/input; 3 authentication; 4 not found; 5 conflict; 1 transport/server.
A successful query can contain timeout/error results; inspect each result's status.
`

func run(args []string, in io.Reader, out, errOut io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "help" {
		fmt.Fprint(out, help)
		return 0
	}
	command := args[0]
	args = args[1:]
	action := ""
	if command == "tasks" {
		if len(args) == 0 {
			fmt.Fprint(errOut, help)
			return 2
		}
		action = args[0]
		args = args[1:]
	}
	fs := flag.NewFlagSet("powerwhois", flag.ContinueOnError)
	fs.SetOutput(errOut)
	mode := fs.String("mode", "whois", "query mode")
	file := fs.String("file", "", "input file or -")
	id := fs.String("id", "", "task ID")
	key := fs.String("key", "", "idempotency key")
	domain := fs.String("domain", "", "typo domain")
	types := fs.String("types", "skipLetter", "comma-separated typo types")
	tlds := fs.String("tlds", "", "comma-separated suffixes")
	concurrency := fs.Int("concurrency", 16, "workers")
	timeout := fs.Int("timeout", 30, "seconds per domain")
	retries := fs.Int("retries", 0, "retry count")
	offset := fs.Int("offset", 0, "input-index cursor")
	limit := fs.Int("limit", 100, "page size")
	format := fs.String("format", "json", "json or csv")
	fs.Bool("json", true, "JSON output")
	fs.Usage = func() { fmt.Fprint(errOut, help) }
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	method, path := "GET", ""
	var body interface{}
	switch {
	case command == "query" || command == "tasks" && action == "create":
		domains := append([]string{}, fs.Args()...)
		if *file != "" {
			reader := in
			if *file != "-" {
				f, e := os.Open(*file)
				if e != nil {
					fmt.Fprintln(errOut, "cannot read input file")
					return 2
				}
				defer f.Close()
				reader = f
			}
			scanner := bufio.NewScanner(reader)
			scanner.Buffer(make([]byte, 4096), 4096)
			for scanner.Scan() {
				line := strings.TrimPrefix(strings.TrimSpace(scanner.Text()), "\ufeff")
				if line != "" {
					domains = append(domains, line)
				}
				if len(domains) > 1000000 {
					fmt.Fprintln(errOut, "too many domains")
					return 2
				}
			}
			if scanner.Err() != nil {
				fmt.Fprintln(errOut, "input contains unreadable or overlong lines")
				return 2
			}
		}
		if len(domains) == 0 {
			fmt.Fprintln(errOut, "supply domains or --file")
			return 2
		}
		method = "POST"
		path = "/query"
		if command == "tasks" {
			path = "/tasks"
		}
		body = map[string]interface{}{"domains": domains, "mode": *mode, "concurrency": *concurrency, "timeout": *timeout, "retries": *retries}
	case command == "typo":
		method = "POST"
		path = "/typo"
		suffixes := []string{}
		if *tlds != "" {
			suffixes = strings.Split(*tlds, ",")
		}
		body = map[string]interface{}{"domain": *domain, "types": strings.Split(*types, ","), "tlds": suffixes}
	case command == "tasks":
		if action == "list" {
			path = "/tasks"
		} else {
			if *id == "" {
				fmt.Fprintln(errOut, "--id required")
				return 2
			}
			path = "/tasks/" + url.PathEscape(*id)
		}
		switch action {
		case "get":
		case "list", "results", "export":
			if action != "list" {
				path += "/" + action
			}
			path += fmt.Sprintf("?offset=%d&limit=%d", *offset, *limit)
			if action == "export" {
				path += "&format=" + url.QueryEscape(*format)
			}
		case "pause", "resume", "cancel", "retry":
			method = "POST"
			path += "/" + action
		default:
			fmt.Fprintln(errOut, "unknown tasks command")
			return 2
		}
	default:
		fmt.Fprintln(errOut, "unknown command; use --help")
		return 2
	}
	c, e := client.FromEnv()
	if e != nil {
		fmt.Fprintln(errOut, e)
		return 2
	}
	data, e := c.Do(context.Background(), method, path, body, *key)
	if e != nil {
		fmt.Fprintln(errOut, e)
		var apiErr *client.Error
		if errors.As(e, &apiErr) {
			switch apiErr.Status {
			case 400, 422:
				return 2
			case 401, 403:
				return 3
			case 404:
				return 4
			case 409:
				return 5
			}
		}
		return 1
	}
	if !(command == "tasks" && action == "export" && *format == "csv") && !json.Valid(data) {
		fmt.Fprintln(errOut, "server returned invalid JSON")
		return 1
	}
	if _, e = out.Write(data); e != nil {
		fmt.Fprintln(errOut, "output write failed")
		return 1
	}
	fmt.Fprintln(out)
	return 0
}
func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

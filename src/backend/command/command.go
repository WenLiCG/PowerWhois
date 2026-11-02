// Package command is a thin HTTP client; workflow owns the shared business logic.
package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"typonamer/workflow"
)

func Run(args []string, out, stderr io.Writer) int {
	fail := func(code int, e error) int {
		_ = json.NewEncoder(stderr).Encode(map[string]string{"error": e.Error()})
		return code
	}
	if len(args) == 0 {
		args = []string{"serve"}
	}
	action := args[0]
	f := flag.NewFlagSet(action, flag.ContinueOnError)
	f.SetOutput(stderr)
	address := f.String("url", "http://127.0.0.1:8080", "API server")
	listen := f.String("listen", "127.0.0.1:8080", "loopback bind address")
	data := f.String("data", "data/powerwhois.db", "server database")
	web := f.String("web", "", "built frontend directory; empty uses embedded dashboard")
	workers := f.Int("workers", 10, "global workers, maximum 10")
	interval := f.Duration("provider-interval", time.Second, "minimum interval per registry host")
	timeout := f.Duration("provider-timeout", 10*time.Second, "timeout per provider attempt")
	id := f.String("job", "", "job ID")
	file := f.String("file", "", "input XLSX or export destination")
	sheet := f.String("sheet", "", "worksheet name")
	column := f.String("column", "B", "domain column letter")
	header := f.Bool("header", false, "skip first row")
	source := f.String("source", "", "source provenance")
	rules := f.String("rules", "", "comma-separated typo rules")
	tlds := f.String("tlds", "com,net,org,co", "TLD mutation list")
	limit := f.Int("max-candidates", 500000, "legal unique candidate cap")
	include := f.Bool("include-seeds", false, "also scan original seeds")
	after := f.String("after", "", "pagination cursor")
	pageSize := f.Int("limit", 100, "page size (maximum 1000)")
	denominator := f.String("denominator", "all", "XLSX ratio denominator: all or known")
	if e := f.Parse(args[1:]); e != nil {
		return 2
	}
	if len(f.Args()) > 0 {
		return fail(2, fmt.Errorf("unexpected arguments"))
	}
	if action == "serve" {
		host, _, e := net.SplitHostPort(*listen)
		if e != nil {
			return fail(2, e)
		}
		ip := net.ParseIP(host)
		if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return fail(2, fmt.Errorf("server requires a loopback listen address"))
		}
		if *interval < 100*time.Millisecond || *timeout < time.Second || *timeout > time.Minute {
			return fail(2, fmt.Errorf("provider-interval must be >=100ms; provider-timeout must be 1s..1m"))
		}
		store, e := workflow.Open(*data)
		if e != nil {
			return fail(1, e)
		}
		defer store.Close()
		svc, e := workflow.NewService(store, workflow.NewRDAP(*interval, *timeout), *workers)
		if e != nil {
			return fail(2, e)
		}
		defer svc.Close()
		server := &http.Server{Addr: *listen, Handler: svc.Handler(*web), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: time.Minute, MaxHeaderBytes: 16 << 10}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		server.BaseContext = func(net.Listener) context.Context { return ctx }
		_ = json.NewEncoder(stderr).Encode(map[string]string{"listen": *listen, "database": *data})
		listenErr := make(chan error, 1)
		go func() { listenErr <- server.ListenAndServe() }()
		select {
		case e = <-listenErr:
			if e != nil && e != http.ErrServerClosed {
				return fail(1, e)
			}
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			e = server.Shutdown(shutdown)
			cancel()
			if e != nil {
				_ = server.Close()
				return fail(1, e)
			}
		}
		return 0
	}
	base, e := url.Parse(*address)
	if e != nil || base.Scheme != "http" || base.Host == "" {
		return fail(2, fmt.Errorf("url must be a loopback HTTP URL"))
	}
	host := base.Hostname()
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return fail(2, fmt.Errorf("CLI API URL must be loopback"))
	}
	path := "/api/v1/jobs"
	method := http.MethodGet
	var body io.Reader
	contentType := "application/json"
	if action != "import" && action != "jobs" && *id == "" {
		return fail(2, fmt.Errorf("--job is required"))
	}
	if *id != "" {
		if strings.ContainsAny(*id, "/?#\\") {
			return fail(2, fmt.Errorf("invalid job ID"))
		}
		path += "/" + *id
	}
	query := url.Values{}
	switch action {
	case "jobs":
		query.Set("after", *after)
		query.Set("limit", fmt.Sprint(*pageSize))
	case "status":
	case "results", "seeds", "origins":
		kind := "items"
		if action == "seeds" {
			kind = "seeds"
		}
		if action == "origins" {
			kind = "origins"
		}
		path += "/" + kind
		query.Set("after", *after)
		query.Set("limit", fmt.Sprint(*pageSize))
	case "import":
		if *file == "" {
			return fail(2, fmt.Errorf("--file is required"))
		}
		input, e := os.Open(*file)
		if e != nil {
			return fail(1, e)
		}
		defer input.Close()
		pr, pw := io.Pipe()
		mp := multipart.NewWriter(pw)
		contentType = mp.FormDataContentType()
		body = pr
		defer pr.Close()
		go func() {
			err := func() error {
				for k, v := range map[string]string{"sheet": *sheet, "column": *column, "header": fmt.Sprint(*header), "source": *source} {
					if e := mp.WriteField(k, v); e != nil {
						return e
					}
				}
				part, e := mp.CreateFormFile("file", filepath.Base(*file))
				if e != nil {
					return e
				}
				if _, e = io.Copy(part, input); e != nil {
					return e
				}
				return mp.Close()
			}()
			_ = pw.CloseWithError(err)
		}()
		path = "/api/v1/jobs/import"
		method = http.MethodPost
	case "generate":
		o := workflow.GenerateOptions{TLDs: strings.Split(*tlds, ","), MaxCandidates: *limit, IncludeSeeds: *include}
		if *rules != "" {
			o.Rules = strings.Split(*rules, ",")
		}
		b, _ := json.Marshal(o)
		body = bytes.NewReader(b)
		path += "/generate"
		method = http.MethodPost
	case "start", "pause", "resume", "cancel":
		path += "/" + action
		method = http.MethodPost
	case "export":
		if *file == "" {
			return fail(2, fmt.Errorf("--file is required"))
		}
		path += "/export"
		query.Set("denominator", *denominator)
	default:
		return fail(2, fmt.Errorf("commands: serve import generate start status results seeds jobs pause resume cancel export"))
	}
	endpoint := strings.TrimRight(*address, "/") + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, e := http.NewRequest(method, endpoint, body)
	if e != nil {
		return fail(2, e)
	}
	req.Header.Set("Content-Type", contentType)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, e := client.Do(req)
	if e != nil {
		return fail(1, e)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 65536))
		return fail(1, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(message))))
	}
	if action == "export" {
		tmp, e := os.CreateTemp(filepath.Dir(*file), ".powerwhois-*.xlsx")
		if e != nil {
			return fail(1, e)
		}
		defer os.Remove(tmp.Name())
		_, e = io.Copy(tmp, resp.Body)
		if e == nil {
			e = tmp.Sync()
		}
		closeErr := tmp.Close()
		if e != nil {
			return fail(1, e)
		}
		if closeErr != nil {
			return fail(1, closeErr)
		}
		// shortcut: atomic no-replace publication needs hard links; use HTTP export on filesystems without them.
		if e = os.Link(tmp.Name(), *file); errors.Is(e, os.ErrExist) {
			return fail(2, fmt.Errorf("export destination exists; choose a new path"))
		} else if e != nil {
			return fail(1, e)
		}
		_ = json.NewEncoder(out).Encode(map[string]string{"file": *file})
		return 0
	}
	if _, e = io.Copy(out, resp.Body); e != nil {
		return fail(1, e)
	}
	return 0
}

package jobs

import (
	"bufio"
	"context"
	"encoding/json"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"typonamer/internal/testutil"
	"typonamer/lookup/lookupinfo"
	"typonamer/utils"
)

// Opt-in full user corpus run; uses a private Redis and no external DNS/WHOIS calls.
func TestAttachmentFullCorpus(t *testing.T) {
	path := os.Getenv("POWERWHOIS_ATTACHMENT_TXT")
	if path == "" {
		t.Skip("set POWERWHOIS_ATTACHMENT_TXT for the full corpus regression")
	}
	output := os.Getenv("POWERWHOIS_ATTACHMENT_EVIDENCE")
	if output == "" {
		t.Fatal("POWERWHOIS_ATTACHMENT_EVIDENCE required")
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	raw, valid := []string{}, []string{}
	rejected := []map[string]any{}
	seen := map[string]bool{}
	scanner := bufio.NewScanner(f)
	line := 0
	for scanner.Scan() {
		line++
		d := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "\ufeff"))
		if d == "" {
			continue
		}
		raw = append(raw, d)
		n, e := utils.TrimAndGetMainDomain(d)
		if e != nil {
			rejected = append(rejected, map[string]any{"line": line, "input": d, "error": e.Error()})
			continue
		}
		if !seen[n] {
			seen[n] = true
			valid = append(valid, n)
		}
	}
	if err = scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(raw) < 500000 || len(valid) < 500000 || len(rejected) == 0 {
		t.Fatalf("unexpected corpus raw=%d valid=%d rejected=%d", len(raw), len(valid), len(rejected))
	}
	os.MkdirAll(output, 0700)
	write := func(name string, v any) {
		b, e := json.MarshalIndent(v, "", "  ")
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(output, name), b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	write("txt-rejected.json", rejected)
	if err = os.WriteFile(filepath.Join(output, "txt-valid-unique.txt"), []byte(strings.Join(valid, "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	r := testutil.Redis(t)
	s := New(r, "attachments:")
	ctx := context.Background()
	states := []string{"registered", "not_found", "unknown", "timeout", "error"}
	expected := map[string]int{}
	for _, d := range valid {
		expected[states[crc32.ChecksumIEEE([]byte(d))%5]]++
	}
	s.Query = func(ctx context.Context, d, m string, timeout, retries int) lookupinfo.QueryResult {
		return lookupinfo.QueryResult{Domain: d, Status: states[crc32.ChecksumIEEE([]byte(d))%5], Source: "isolated-corpus-fixture", QueriedAt: "2026-11-03T00:00:00Z"}
	}
	started := time.Now()
	stop := start(t, s)
	defer func() {
		if stop != nil {
			stop()
		}
	}()
	<-s.Ready
	if _, _, e := s.Create(ctx, Request{Domains: raw, Mode: "dns"}, "original"); e == nil {
		t.Fatal("invalid original accepted")
	}
	list, e := s.List(ctx, 0, 100)
	if e != nil || len(list) != 0 {
		t.Fatal("invalid original left task state", e)
	}
	task, _, e := s.Create(ctx, Request{Domains: valid, Mode: "dns"}, "full-corpus")
	if e != nil {
		t.Fatal(e)
	}
	if task.Total != len(valid) || task.Concurrency != 15 {
		t.Fatalf("unexpected task %+v", task)
	}
	waitTask(t, s, task.ID, func(t Task) bool { return t.Completed >= 300 })
	paused, e := s.Control(ctx, task.ID, "pause")
	if e != nil {
		t.Fatal(e)
	}
	time.Sleep(100 * time.Millisecond)
	stable, e := s.Get(ctx, task.ID)
	if e != nil || stable.Completed != paused.Completed {
		t.Fatal("paused progress changed", e)
	}
	// Stop and reopen the worker against the same Redis; the task must stay paused.
	stop()
	stop = nil
	s = New(r, "attachments:")
	s.Query = func(ctx context.Context, d, m string, timeout, retries int) lookupinfo.QueryResult {
		return lookupinfo.QueryResult{Domain: d, Status: states[crc32.ChecksumIEEE([]byte(d))%5], Source: "isolated-corpus-fixture", QueriedAt: "2026-11-03T00:00:00Z"}
	}
	stop = start(t, s)
	<-s.Ready
	current, e := s.Get(ctx, task.ID)
	if e != nil || current.State != "paused" || current.Completed != paused.Completed {
		t.Fatal("paused task changed across restart", e)
	}
	if _, e = s.Control(ctx, task.ID, "resume"); e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(8 * time.Minute)
	nextLog := time.Now()
	for {
		current, e = s.Get(ctx, task.ID)
		if e != nil {
			t.Fatal(e)
		}
		if time.Now().After(nextLog) {
			t.Logf("progress %d/%d (%s)", current.Completed, current.Total, current.State)
			nextLog = time.Now().Add(20 * time.Second)
		}
		if current.State == "completed" {
			break
		}
		if current.State == "failed" || time.Now().After(deadline) {
			t.Fatalf("full corpus failed %+v", current)
		}
		time.Sleep(200 * time.Millisecond)
	}
	if current.Completed != len(valid) {
		t.Fatal("incomplete")
	}
	for k, n := range expected {
		if current.Counts[k] != n {
			t.Fatalf("count %s: %d != %d", k, current.Counts[k], n)
		}
	}
	// Read every page and assert input order, uniqueness and status, including the final page.
	count, pages := 0, 0
	for offset := 0; offset >= 0; {
		p, e := s.Results(ctx, task.ID, offset, 1000, false)
		if e != nil {
			t.Fatal(e)
		}
		for _, item := range p.Items {
			if item.Order != count || item.Domain != valid[count] || item.Status != states[crc32.ChecksumIEEE([]byte(valid[count]))%5] {
				t.Fatalf("result mismatch at %d", count)
			}
			count++
		}
		if p.NextOffset != -1 && p.NextOffset <= offset {
			t.Fatal("cursor did not progress")
		}
		offset = p.NextOffset
		pages++
	}
	if count != len(valid) {
		t.Fatal("export missing rows", count)
	}
	write("full-corpus-mock.json", map[string]any{"rawRows": len(raw), "validUnique": len(valid), "rejectedRows": len(rejected), "duplicateOrNormalizedRows": len(raw) - len(rejected) - len(valid), "task": current, "verifiedResultRows": count, "pages": pages, "elapsedSeconds": time.Since(started).Seconds(), "pausedRestartResume": true, "externalQueries": false})
	t.Logf("full corpus verified: raw=%d validUnique=%d rejected=%d pages=%d duration=%s", len(raw), len(valid), len(rejected), pages, time.Since(started))
}

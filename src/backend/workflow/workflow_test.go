package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func fixture(t *testing.T, domains ...string) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	if err := f.SetCellStr("Sheet1", "B1", "Domain"); err != nil {
		t.Fatal(err)
	}
	for i, d := range domains {
		if err := f.SetCellStr("Sheet1", fmt.Sprintf("B%d", i+2), d); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.SetCellFormula("Sheet1", fmt.Sprintf("B%d", len(domains)+2), "-bad.example"); err != nil {
		t.Fatal(err)
	}
	b, e := f.WriteToBuffer()
	if e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func testStore(t *testing.T) *Store {
	t.Helper()
	s, e := Open(filepath.Join(t.TempDir(), "jobs.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func imported(t *testing.T, s *Store, domains ...string) Job {
	t.Helper()
	j, e := s.Import(context.Background(), bytes.NewReader(fixture(t, domains...)), ImportOptions{Column: "B", Header: true, Source: "synthetic regression fixture"})
	if e != nil {
		t.Fatal(e)
	}
	return j
}
func generated(t *testing.T, s *Store, domains ...string) Job {
	t.Helper()
	j := imported(t, s, domains...)
	j, e := s.Generate(context.Background(), j.ID, GenerateOptions{Rules: []string{"omission", "repetition", "transposition"}, IncludeSeeds: true, MaxCandidates: 500})
	if e != nil {
		t.Fatal(e)
	}
	return j
}

func TestExcelRoundtripAndDeterminism(t *testing.T) {
	s := testStore(t)
	j := imported(t, s, "example.com", "EXAMPLE.COM", "-example.com", "exa_mple.com")
	if j.InputRows != 5 || j.UniqueSeeds != 1 || j.ValidRows != 2 || j.InvalidRows != 3 {
		t.Fatalf("wrong import counts: %+v", j)
	}
	opts := GenerateOptions{Rules: []string{"omission", "repetition", "transposition"}, MaxCandidates: 500}
	j, e := s.Generate(context.Background(), j.ID, opts)
	if e != nil {
		t.Fatal(e)
	}
	other := imported(t, s, "example.com", "EXAMPLE.COM", "-example.com", "exa_mple.com")
	other, e = s.Generate(context.Background(), other.ID, opts)
	if e != nil {
		t.Fatal(e)
	}
	a, _, _ := s.Page(j.ID, "items", "", 1000)
	b, _, _ := s.Page(other.ID, "items", "", 1000)
	if !bytes.Equal(mustJSON(a), mustJSON(b)) {
		t.Fatal("generation is nondeterministic")
	}
	if j.Origins <= j.Candidates {
		t.Fatal("duplicate source row mapping lost")
	}
	var out bytes.Buffer
	if e = s.Export(j.ID, "known", &out); e != nil {
		t.Fatal(e)
	}
	f, e := excelize.OpenReader(&out)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	for _, sheet := range []string{"Summary", "Results", "Sources", "InvalidInputs"} {
		rows, e := f.GetRows(sheet)
		if e != nil || len(rows) < 2 {
			t.Fatalf("missing %s: %v", sheet, e)
		}
	}
	rows, _ := f.GetRows("InvalidInputs")
	for _, r := range rows[1:] {
		if len(r) > 2 && strings.Contains(r[2], "bad.example") && !strings.HasPrefix(r[2], "'=") {
			t.Fatalf("formula injection not escaped: %v", r)
		}
	}
	if got, _ := f.GetCellValue("Summary", "B3"); got != "TRUE" && got != "1" {
		t.Fatalf("partial report not marked: %q", got)
	}
}
func mustJSON(v interface{}) []byte { b, _ := json.Marshal(v); return b }

func TestLeaseRecoveryAndPersistenceFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.db")
	s, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	j := generated(t, s, "example.com")
	if _, e = s.Transition(j.ID, "start"); e != nil {
		t.Fatal(e)
	}
	work, e := s.claim()
	if e != nil || work == nil {
		t.Fatal(e)
	}
	s.beforeCommit = func() error { return errors.New("injected disk failure") }
	if e = s.finish(*work, Result{Status: "registered"}); e == nil {
		t.Fatal("expected persist failure")
	}
	s.beforeCommit = nil
	j, _ = s.Get(j.ID)
	p, a, e := s.stats(j.ID)
	if e != nil || j.Completed != 0 || p+a != j.Candidates || a != 1 {
		t.Fatal("queue or result lost after rollback")
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	s, e = Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	j, _ = s.Get(j.ID)
	p, a, e = s.stats(j.ID)
	if e != nil || j.State != "paused" || a != 0 || p != j.Candidates {
		t.Fatalf("failed recovery: %+v pending=%d active=%d err=%v", j, p, a, e)
	}
	if _, e = s.Transition(j.ID, "resume"); e != nil {
		t.Fatal(e)
	}
	newWork, e := s.claim()
	if e != nil {
		t.Fatal(e)
	}
	if e = s.finish(*work, Result{Status: "not_found"}); e != nil {
		t.Fatal(e)
	}
	j, _ = s.Get(j.ID)
	if j.Completed != 0 {
		t.Fatal("stale lease committed")
	}
	if e = s.finish(*newWork, Result{Status: "registered"}); e != nil {
		t.Fatal(e)
	}
	if e = s.finish(*newWork, Result{Status: "registered"}); e != nil {
		t.Fatal(e)
	}
	j, _ = s.Get(j.ID)
	if j.Completed != 1 {
		t.Fatal("result not idempotent")
	}
}

type timedTransport struct {
	inner  http.RoundTripper
	mu     sync.Mutex
	starts []time.Time
}

func (t *timedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	t.mu.Lock()
	t.starts = append(t.starts, time.Now())
	t.mu.Unlock()
	return t.inner.RoundTrip(r)
}

type controlledProvider struct {
	active, max atomic.Int32
	calls       atomic.Int32
	delay       time.Duration
}

func (p *controlledProvider) Lookup(ctx context.Context, d string) Result {
	n := p.active.Add(1)
	defer p.active.Add(-1)
	for old := p.max.Load(); n > old; old = p.max.Load() {
		if p.max.CompareAndSwap(old, n) {
			break
		}
	}
	p.calls.Add(1)
	_ = sleep(ctx, p.delay)
	return Result{Status: "registered", Provider: "synthetic", CheckedAt: time.Now().UTC()}
}
func waitJob(t *testing.T, s *Store, id, state string) Job {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		j, e := s.Get(id)
		if e != nil {
			t.Fatal(e)
		}
		if j.State == state {
			return j
		}
		time.Sleep(10 * time.Millisecond)
	}
	j, _ := s.Get(id)
	t.Fatalf("wanted %s, got %+v", state, j)
	return j
}
func TestServiceGlobalConcurrencyPauseCancel(t *testing.T) {
	s := testStore(t)
	p := &controlledProvider{delay: 15 * time.Millisecond}
	svc, e := NewService(s, p, 10)
	if e != nil {
		t.Fatal(e)
	}
	defer svc.Close()
	a := generated(t, s, "example.com")
	b := generated(t, s, "anotherdomain.com")
	for _, id := range []string{a.ID, b.ID} {
		if _, e = svc.Action(id, "start"); e != nil {
			t.Fatal(e)
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = svc.Action(a.ID, "start") }()
	}
	wg.Wait()
	if _, e = svc.Action(a.ID, "pause"); e != nil {
		t.Fatal(e)
	}
	paused, _ := s.Get(a.ID)
	time.Sleep(40 * time.Millisecond)
	after, _ := s.Get(a.ID)
	if paused.Completed != after.Completed {
		t.Fatal("paused job still commits results")
	}
	if _, e = svc.Action(a.ID, "resume"); e != nil {
		t.Fatal(e)
	}
	waitJob(t, s, a.ID, "completed")
	waitJob(t, s, b.ID, "completed")
	if p.max.Load() > 10 || p.max.Load() < 2 {
		t.Fatalf("global concurrency=%d", p.max.Load())
	}
	c := generated(t, s, "canceldomain.com")
	if _, e = svc.Action(c.ID, "start"); e != nil {
		t.Fatal(e)
	}
	if _, e = svc.Action(c.ID, "cancel"); e != nil {
		t.Fatal(e)
	}
	if _, e = svc.Action(c.ID, "resume"); e == nil {
		t.Fatal("canceled job resumed")
	}
	time.Sleep(40 * time.Millisecond)
	c, _ = s.Get(c.ID)
	if c.Completed != 0 {
		t.Fatal("canceled job committed")
	}
}

func TestRDAPClassificationRetryTimeoutAndRate(t *testing.T) {
	var mu sync.Mutex
	starts := map[string][]time.Time{}
	counts := map[string]int{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		domain := strings.TrimPrefix(r.URL.Path, "/domain/")
		mu.Lock()
		starts[r.Host] = append(starts[r.Host], time.Now())
		counts[domain]++
		n := counts[domain]
		mu.Unlock()
		w.Header().Set("Content-Type", "application/rdap+json")
		switch domain {
		case "registered.com":
			fmt.Fprint(w, `{"objectClassName":"domain","ldhName":"registered.com"}`)
		case "absent.com":
			w.WriteHeader(404)
			fmt.Fprint(w, `{"errorCode":404}`)
		case "empty.com":
			w.WriteHeader(404)
		case "html.com":
			w.WriteHeader(404)
			fmt.Fprint(w, `<html>not found</html>`)
		case "malformed.com":
			fmt.Fprint(w, `{`)
		case "limited.com":
			if n == 1 {
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(429)
				fmt.Fprint(w, `{"errorCode":429}`)
			} else {
				fmt.Fprint(w, `{"objectClassName":"domain","ldhName":"limited.com"}`)
			}
		case "timeout.com":
			select {
			case <-r.Context().Done():
			case <-time.After(200 * time.Millisecond):
			}
		default:
			fmt.Fprint(w, `{"objectClassName":"domain","ldhName":"other.com"}`)
		}
	}))
	defer server.Close()
	p := NewRDAP(10*time.Millisecond, 50*time.Millisecond)
	p.client = server.Client()
	transport := &timedTransport{inner: p.client.Transport}
	p.client.Transport = transport
	p.timeout = 50 * time.Millisecond
	p.backoff = time.Millisecond
	p.endpoints = map[string]string{"com": server.URL + "/domain/"}
	for domain, want := range map[string]string{"registered.com": "registered", "absent.com": "not_found", "empty.com": "not_found", "html.com": "error", "malformed.com": "error", "mismatch.com": "error", "timeout.com": "error", "unsupported.test": "unknown", "limited.com": "registered"} {
		r := p.Lookup(context.Background(), domain)
		if r.Status != want {
			t.Errorf("%s got %s want %s: %+v", domain, r.Status, want, r)
		}
		if domain == "limited.com" && (len(r.Attempts) != 2 || r.Attempts[0].HTTPStatus != 429) {
			t.Fatalf("missing 429 evidence: %+v", r)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	// TLS handshake variance can compress server arrival intervals. Measure
	// dispatches at the client boundary where the throttle is enforced.
	transport.mu.Lock()
	defer transport.mu.Unlock()
	for i := 1; i < len(transport.starts); i++ {
		if gap := transport.starts[i].Sub(transport.starts[i-1]); gap < 8*time.Millisecond {
			t.Fatalf("provider dispatch rate bypassed: %v", gap)
		}
	}
	if counts["timeout.com"] != 3 {
		t.Fatalf("timeout attempts=%d", counts["timeout.com"])
	}
}

func TestGenerationRollbackResumeAndCap(t *testing.T) {
	s := testStore(t)
	domains := []string{}
	for i := 0; i < 100; i++ {
		domains = append(domains, fmt.Sprintf("sourceexample%d.com", i))
	}
	j := imported(t, s, domains...)
	opts := GenerateOptions{Rules: []string{"omission", "transposition"}, MaxCandidates: 50000}
	ctx, cancel := context.WithCancel(context.Background())
	commits := 0
	s.beforeCommit = func() error {
		commits++
		if commits == 2 {
			cancel()
		}
		return nil
	}
	_, e := s.Generate(ctx, j.ID, opts)
	s.beforeCommit = nil
	if !errors.Is(e, context.Canceled) {
		t.Fatalf("expected interrupted generation: %v", e)
	}
	partial, e := s.Get(j.ID)
	if e != nil || partial.State != "generating" || partial.Candidates == 0 {
		t.Fatal("no durable generation checkpoint")
	}
	j, e = s.Generate(context.Background(), j.ID, opts)
	if e != nil {
		t.Fatal(e)
	}
	clean := imported(t, s, domains...)
	clean, e = s.Generate(context.Background(), clean.ID, opts)
	if e != nil {
		t.Fatal(e)
	}
	if j.Candidates != clean.Candidates || j.Origins != clean.Origins || j.RawCandidates != clean.RawCandidates {
		t.Fatal("resume counters differ from clean generation")
	}
	failed := imported(t, s, "example.com")
	s.beforeCommit = func() error { return errors.New("disk failure") }
	if _, e = s.Generate(context.Background(), failed.ID, opts); e == nil {
		t.Fatal("expected generation persistence failure")
	}
	s.beforeCommit = nil
	failed, _ = s.Get(failed.ID)
	if failed.Candidates != 0 || failed.State != "draft" {
		t.Fatal("uncommitted generation leaked into durable state")
	}
	capJob := imported(t, s, "example.com")
	capJob, e = s.Generate(context.Background(), capJob.ID, GenerateOptions{MaxCandidates: 3})
	if e != nil || capJob.Candidates != 3 || !capJob.Truncated {
		t.Fatalf("cap not enforced: %+v %v", capJob, e)
	}
	for _, rule := range Rules {
		if got := mutations("example.com", rule, []string{"net"}); len(got) == 0 {
			t.Errorf("rule %s has no output", rule)
		}
	}
}

func TestEndToEndFakeRDAPRecoveryAndExport(t *testing.T) {
	var mu sync.Mutex
	calls := map[string]int{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		domain := strings.TrimPrefix(r.URL.Path, "/domain/")
		mu.Lock()
		calls[domain]++
		attempt := calls[domain]
		mu.Unlock()
		switch {
		case strings.Contains(domain, "absent"):
			w.WriteHeader(404)
		case strings.Contains(domain, "malformed"):
			fmt.Fprint(w, "{")
		case strings.Contains(domain, "timeout"):
			select {
			case <-r.Context().Done():
			case <-time.After(time.Second):
			}
		case strings.Contains(domain, "limited") && attempt == 1:
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(429)
		default:
			fmt.Fprintf(w, `{"objectClassName":"domain","ldhName":%q}`, domain)
		}
	}))
	defer server.Close()
	p := NewRDAP(3*time.Millisecond, 50*time.Millisecond)
	p.client = server.Client()
	p.endpoints = map[string]string{"com": server.URL + "/domain/"}
	p.backoff = time.Millisecond
	path := filepath.Join(t.TempDir(), "e2e.db")
	store, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	j := imported(t, store, "registered.com", "absent.com", "limited.com", "timeout.com", "malformed.com", "REGISTERED.COM", "-invalid.com")
	j, e = store.Generate(context.Background(), j.ID, GenerateOptions{Rules: []string{"www"}, IncludeSeeds: true, MaxCandidates: 100})
	if e != nil {
		t.Fatal(e)
	}
	if j.Candidates != 10 || j.UniqueSeeds != 5 || j.InvalidRows != 2 {
		t.Fatalf("bad fixture import/generation: %+v", j)
	}
	svc, e := NewService(store, p, 3)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = svc.Action(j.ID, "start"); e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		j, _ = store.Get(j.ID)
		if j.Completed > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if j.Completed == 0 || j.Completed == j.Candidates {
		t.Fatal("did not capture partial in-flight job")
	}
	svc.Close()
	if e = store.Close(); e != nil {
		t.Fatal(e)
	}
	store, e = Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	j, e = store.Get(j.ID)
	if e != nil || j.State != "paused" {
		t.Fatal("scan was not recoverable")
	}
	pending, active, e := store.stats(j.ID)
	if e != nil || active != 0 || pending+j.Completed != j.Candidates {
		t.Fatal("in-flight recovery lost a domain")
	}
	svc, e = NewService(store, p, 3)
	if e != nil {
		t.Fatal(e)
	}
	defer svc.Close()
	if _, e = svc.Action(j.ID, "resume"); e != nil {
		t.Fatal(e)
	}
	j = waitJob(t, store, j.ID, "completed")
	if j.Completed != 10 || j.Counts["registered"] != 4 || j.Counts["not_found"] != 2 || j.Counts["error"] != 4 {
		t.Fatalf("incorrect final classifications: %+v", j)
	}
	items, _, e := store.Page(j.ID, "items", "", 100)
	if e != nil {
		t.Fatal(e)
	}
	saw429 := false
	for _, raw := range items {
		var c Candidate
		if e = json.Unmarshal(raw, &c); e != nil {
			t.Fatal(e)
		}
		for _, a := range c.Result.Attempts {
			if a.HTTPStatus == 429 {
				saw429 = true
			}
		}
	}
	if !saw429 {
		t.Fatal("retry attempt evidence lost")
	}
	var out bytes.Buffer
	if e = store.Export(j.ID, "known", &out); e != nil {
		t.Fatal(e)
	}
	x, e := excelize.OpenReader(&out)
	if e != nil {
		t.Fatal(e)
	}
	defer x.Close()
	rows, e := x.GetRows("Results")
	if e != nil || len(rows) != 11 {
		t.Fatal("end-to-end XLSX results lost rows")
	}
}

func TestXLSXEvidenceLimitPreservesAPIRecord(t *testing.T) {
	s := testStore(t)
	j := generated(t, s, "example.com")
	_, e := s.Transition(j.ID, "start")
	if e != nil {
		t.Fatal(e)
	}
	work, e := s.claim()
	if e != nil {
		t.Fatal(e)
	}
	full := strings.Repeat("x", 33000)
	if e = s.finish(*work, Result{Status: "registered", Evidence: full, CheckedAt: time.Now().UTC()}); e != nil {
		t.Fatal(e)
	}
	var out bytes.Buffer
	if e = s.Export(j.ID, "all", &out); e != nil {
		t.Fatal(e)
	}
	f, e := excelize.OpenReader(&out)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	displayed, _ := f.GetCellValue("Results", "H2")
	flag, _ := f.GetCellValue("Results", "I2")
	if len(displayed) > 32767 || !strings.Contains(displayed, "truncated") || (flag != "TRUE" && flag != "1") {
		t.Fatal("Excel evidence truncation was not explicit")
	}
	page, _, e := s.Page(j.ID, "items", "", 1)
	if e != nil {
		t.Fatal(e)
	}
	var c Candidate
	if e = json.Unmarshal(page[0], &c); e != nil || c.Result.Evidence != full {
		t.Fatal("full result evidence lost")
	}
}

func TestPrivateNamespaceIsNotRegistrySuffix(t *testing.T) {
	if got, e := Normalize("blogspot.com"); e != nil || got != "blogspot.com" {
		t.Errorf("registered private-suffix host rejected: %q %v", got, e)
	}
	if got, e := publicRoot("site.blogspot.com"); e != nil || got != "blogspot.com" {
		t.Errorf("private subdomain treated as registry domain: %q %v", got, e)
	}
	s := testStore(t)
	j := imported(t, s, "blogspot.com", "site.blogspot.com")
	if j.UniqueSeeds != 1 || j.InvalidRows != 2 {
		t.Fatalf("wrong registry seed validation: %+v", j)
	}
}

func TestHTTPBoundaryAndPagination(t *testing.T) {
	s := testStore(t)
	svc, e := NewService(s, &controlledProvider{}, 2)
	if e != nil {
		t.Fatal(e)
	}
	defer svc.Close()
	j := generated(t, s, "example.com")
	h := svc.Handler("")
	for _, tc := range []struct {
		path, host, origin string
		want               int
	}{{"/api/v1/jobs/" + j.ID, "127.0.0.1:8080", "", 200}, {"/api/v1/jobs/" + j.ID + "/items?limit=2", "127.0.0.1:8080", "", 200}, {"/api/v1/jobs/" + j.ID + "/items?limit=1001", "127.0.0.1:8080", "", 400}, {"/api/v1/jobs", "evil.test", "", 403}, {"/api/v1/jobs", "127.0.0.1:8080", "https://evil.test", 403}} {
		r := httptest.NewRequest(http.MethodGet, tc.path, nil)
		r.Host = tc.host
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("%+v got %d: %s", tc, w.Code, w.Body)
		}
	}
	page, next, e := s.Page(j.ID, "items", "", 2)
	if e != nil || len(page) != 2 || next == "" {
		t.Fatal("pagination failed")
	}
	page2, _, e := s.Page(j.ID, "items", next, 2)
	if e != nil || bytes.Equal(page[1], page2[0]) {
		t.Fatal("cursor repeated data")
	}
}

func TestHostGateCooldownAndCancellation(t *testing.T) {
	g := &hostGate{next: map[string]time.Time{}, interval: 20 * time.Millisecond}
	if e := g.wait(context.Background(), "registry"); e != nil {
		t.Fatal(e)
	}
	finished := make(chan time.Time, 1)
	go func() { _ = g.wait(context.Background(), "registry"); finished <- time.Now() }()
	time.Sleep(3 * time.Millisecond)
	cooldownAt := time.Now()
	g.deferHost("registry", 100*time.Millisecond)
	if gap := (<-finished).Sub(cooldownAt); gap < 95*time.Millisecond {
		t.Fatalf("waiting request ignored provider cooldown: %v", gap)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if e := g.wait(ctx, "registry"); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatalf("gate ignored cancellation: %v", e)
	}
}

func TestRDAPRejectsUntrustedTLS(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"objectClassName":"domain","ldhName":"example.com"}`)
	}))
	defer server.Close()
	p := NewRDAP(time.Millisecond, time.Second)
	p.endpoints = map[string]string{"com": server.URL + "/domain/"}
	p.maxAttempts = 1
	r := p.Lookup(context.Background(), "example.com")
	if r.Status != "error" || !strings.Contains(r.Error, "certificate") {
		t.Fatalf("untrusted TLS accepted: %+v", r)
	}
}

package workflow

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
	bolt "go.etcd.io/bbolt"
)

func TestGenerationCannotOverwriteCommittedCancel(t *testing.T) {
	s := testStore(t)
	j := imported(t, s, "example.com")
	locked, release := make(chan struct{}), make(chan struct{})
	writerDone := make(chan error, 1)
	go func() {
		writerDone <- s.db.Update(func(tx *bolt.Tx) error {
			b, e := jobBucket(tx, j.ID)
			if e != nil {
				return e
			}
			canceled, e := readJob(b)
			if e != nil {
				return e
			}
			canceled.State = "canceled"
			if e = putJob(b, canceled); e != nil {
				return e
			}
			close(locked)
			<-release
			return nil
		})
	}()
	<-locked
	result := make(chan error, 1)
	started := make(chan struct{})
	go func() {
		close(started)
		_, e := s.Generate(context.Background(), j.ID, GenerateOptions{Rules: []string{"www"}})
		result <- e
	}()
	<-started
	// Hold an uncommitted cancel long enough for the old snapshot-read path to
	// reach its write. Atomic admission instead waits before reading the state.
	time.Sleep(30 * time.Millisecond)
	close(release)
	if e := <-writerDone; e != nil {
		t.Fatal(e)
	}
	if e := <-result; !errors.Is(e, ErrConflict) {
		t.Fatalf("generation overwrote cancel: %v", e)
	}
	j, _ = s.Get(j.ID)
	if j.State != "canceled" || j.Candidates != 0 {
		t.Fatalf("canceled job revived: %+v", j)
	}
}

func TestClaimDoesNotWriteWhileAllWorkIsInFlight(t *testing.T) {
	s := testStore(t)
	j := imported(t, s, "example.com")
	if _, err := s.Generate(context.Background(), j.ID, GenerateOptions{Rules: []string{"www"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Transition(j.ID, "start"); err != nil {
		t.Fatal(err)
	}
	work, err := s.claim()
	if err != nil || work == nil {
		t.Fatal("expected work", err)
	}
	s.beforeCommit = func() error { return errors.New("unexpected idle write") }
	if next, err := s.claim(); err != nil || next != nil {
		t.Fatal("idle worker opened a write transaction", err)
	}
	s.beforeCommit = nil
	if err := s.finish(*work, Result{Status: "registered"}); err != nil {
		t.Fatal(err)
	}
	current, err := s.Get(j.ID)
	if err != nil || current.State != "completed" {
		t.Fatal("completion failed", err)
	}
}

func TestInvalidUTF8DomainIsRejected(t *testing.T) {
	for _, raw := range []string{"\xff.com", "exam\xc0\xafple.com", "\xed\xa0\x80.com"} {
		if _, err := NormalizeSeed(raw); err == nil {
			t.Fatalf("invalid UTF-8 accepted: %q", raw)
		}
	}
}

func TestBrowserCrossOriginCannotStartJob(t *testing.T) {
	s := testStore(t)
	j := generated(t, s, "example.com")
	svc, e := NewService(s, &controlledProvider{}, 2)
	if e != nil {
		t.Fatal(e)
	}
	defer svc.Close()
	h := svc.Handler("")
	for _, tc := range []struct{ host, origin string }{{"evil.example", ""}, {"127.0.0.1:8080", "https://evil.example"}, {"127.0.0.1:8080", "null"}, {"localhost:8080", "http://127.0.0.1:8080"}} {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/"+j.ID+"/start", nil)
		r.Host = tc.host
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("cross-origin write accepted: %+v status=%d", tc, w.Code)
		}
		current, _ := s.Get(j.ID)
		if current.State != "ready" || current.Completed != 0 {
			t.Fatal("rejected request changed job")
		}
	}
}

func TestReadyGenerationRejectsChangedOptions(t *testing.T) {
	s := testStore(t)
	j := imported(t, s, "example.com")
	o := GenerateOptions{Rules: []string{"omission"}, MaxCandidates: 1}
	j, e := s.Generate(context.Background(), j.ID, o)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Generate(context.Background(), j.ID, o); e != nil {
		t.Fatal("same generation was not idempotent", e)
	}
	if _, e = s.Generate(context.Background(), j.ID, GenerateOptions{Rules: []string{"repetition"}, MaxCandidates: 500}); !errors.Is(e, ErrConflict) {
		t.Fatalf("changed generation silently accepted: %v", e)
	}
}

func TestConcurrentRepeatedLifecycleActions(t *testing.T) {
	s := testStore(t)
	domains := []string{}
	for i := 0; i < 80; i++ {
		domains = append(domains, fmt.Sprintf("lifecycleexample%d.com", i))
	}
	j := imported(t, s, domains...)
	j, e := s.Generate(context.Background(), j.ID, GenerateOptions{Rules: []string{"omission", "repetition"}, MaxCandidates: 5000})
	if e != nil {
		t.Fatal(e)
	}
	svc, e := NewService(s, &controlledProvider{delay: 20 * time.Millisecond}, 5)
	if e != nil {
		t.Fatal(e)
	}
	defer svc.Close()
	if _, e = svc.Action(j.ID, "start"); e != nil {
		t.Fatal(e)
	}
	repeat := func(action string) {
		t.Helper()
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); _, e := svc.Action(j.ID, action); errs <- e }()
		}
		wg.Wait()
		close(errs)
		for e := range errs {
			if e != nil {
				t.Fatal(action, e)
			}
		}
	}
	for i := 0; i < 8; i++ {
		repeat("pause")
		repeat("resume")
	}
	repeat("cancel")
	for _, action := range []string{"start", "resume", "pause"} {
		if _, e = svc.Action(j.ID, action); !errors.Is(e, ErrConflict) {
			t.Fatalf("terminal cancellation accepted %s: %v", action, e)
		}
	}
	j, _ = s.Get(j.ID)
	pending, active, e := s.stats(j.ID)
	if e != nil || j.State != "canceled" || pending+active+j.Completed != j.Candidates {
		t.Fatal("lifecycle race lost queue entries")
	}
}

func TestStreamedZIPRoundtripAndBounds(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	data := make([]byte, 8192)
	if _, e := rand.Read(data); e != nil {
		t.Fatal(e)
	}
	_ = f.SetCellStr("Sheet1", "A1", base64.StdEncoding.EncodeToString(data))
	var out bytes.Buffer
	if e := writeWorkbookStream(f, &out); e != nil {
		t.Fatal(e)
	}
	x, e := excelize.OpenReader(bytes.NewReader(out.Bytes()))
	if e != nil {
		t.Fatal(e)
	}
	x.Close()
	if e = writeWorkbookWithLimits(f, &bytes.Buffer{}, 128, 1<<20); e == nil {
		t.Fatal("archive size cap ignored")
	}
	if e = writeWorkbookWithLimits(f, &bytes.Buffer{}, 1<<20, 128); e == nil {
		t.Fatal("worksheet XML cap ignored")
	}
}

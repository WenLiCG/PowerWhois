package jobs

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"typonamer/internal/testutil"
	"typonamer/lookup/lookupinfo"
)

func waitTask(t *testing.T, s *Store, id string, predicate func(Task) bool) Task {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		task, e := s.Get(context.Background(), id)
		if e == nil && predicate(task) {
			return task
		}
		time.Sleep(10 * time.Millisecond)
	}
	task, _ := s.Get(context.Background(), id)
	t.Fatalf("timed out: %+v", task)
	return task
}
func start(t *testing.T, s *Store) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	return func() {
		cancel()
		select {
		case e := <-done:
			if e != nil {
				t.Errorf("worker: %v", e)
			}
		case <-time.After(5 * time.Second):
			t.Error("worker did not stop")
		}
	}
}
func TestDurableTasks(t *testing.T) {
	r := testutil.Redis(t)
	s := New(r, "test:")
	ctx := context.Background()
	var active, peak atomic.Int32
	s.Query = func(ctx context.Context, d, m string, timeout, retries int) lookupinfo.QueryResult {
		n := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); n > old; old = peak.Load() {
			if peak.CompareAndSwap(old, n) {
				break
			}
		}
		select {
		case <-ctx.Done():
		case <-time.After(10 * time.Millisecond):
		}
		status := "registered"
		if d == "timeout.com" {
			status = "timeout"
		}
		return lookupinfo.QueryResult{Domain: d, Status: status, Source: "fixture", QueriedAt: "2026-11-03T00:00:00Z"}
	}
	names := []string{"EXAMPLE.com", "example.com", "timeout.com"}
	for i := 0; i < 300; i++ {
		names = append(names, fmt.Sprintf("name%d.com", i))
	}
	req := Request{Domains: names, Mode: "mixed", Concurrency: 32, Timeout: 1}
	task, _, err := s.Create(ctx, req, "same-submission")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			same, reused, e := s.Create(ctx, req, "same-submission")
			if e != nil || !reused || same.ID != task.ID {
				t.Errorf("idempotency failed: %v", e)
			}
		}()
	}
	wg.Wait()
	req.Domains = []string{"different.com"}
	if _, _, e := s.Create(ctx, req, "same-submission"); e != ErrConflict {
		t.Fatalf("expected conflict: %v", e)
	}
	stop := start(t, s)
	waitTask(t, s, task.ID, func(t Task) bool { return t.Completed > 8 })
	if _, err = s.Control(ctx, task.ID, "pause"); err != nil {
		t.Fatal(err)
	}
	paused := waitTask(t, s, task.ID, func(t Task) bool { return t.State == "paused" })
	time.Sleep(60 * time.Millisecond)
	after, _ := s.Get(ctx, task.ID)
	if paused.Completed != after.Completed {
		t.Fatal("late results after pause")
	}
	if _, err = s.Control(ctx, task.ID, "resume"); err != nil {
		t.Fatal(err)
	}
	waitTask(t, s, task.ID, func(t Task) bool { return t.State == "running" })
	stop()
	// A fresh service resumes the same durable ID after interruption.
	s2 := New(r, "test:")
	s2.Query = s.Query
	stop2 := start(t, s2)
	defer stop2()
	final := waitTask(t, s2, task.ID, func(t Task) bool { return t.State == "completed" })
	if final.Total != 302 || final.Completed != 302 || final.Counts["timeout"] != 1 {
		t.Fatalf("wrong counts: %+v", final)
	}
	if peak.Load() > 32 {
		t.Fatalf("concurrency limit exceeded: %d", peak.Load())
	}
	page, err := s2.Results(ctx, task.ID, 0, 100)
	if err != nil || len(page.Items) != 100 || page.NextOffset != 100 {
		t.Fatalf("pagination: %+v %v", page, err)
	}
	retry, err := s2.RetryFailed(ctx, task.ID)
	if err != nil || retry.Total != 1 || retry.ID == task.ID || retry.ParentTaskID != task.ID {
		t.Fatalf("retry: %+v %v", retry, err)
	}
	canceled, _, err := s2.Create(ctx, Request{Domains: []string{"example.org"}, Mode: "dns"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s2.Control(ctx, canceled.ID, "cancel"); err != nil {
		t.Fatal(err)
	}
	if _, err = s2.Control(ctx, canceled.ID, "resume"); err != ErrConflict {
		t.Fatal("canceled task resumed")
	}
}
func TestLeaseExclusionAndInvalidInput(t *testing.T) {
	r := testutil.Redis(t)
	s := New(r, "lease:")
	ctx := context.Background()
	r.Set(ctx, "lease:worker", "existing", time.Minute)
	if s.Run(ctx) == nil {
		t.Fatal("second service acquired worker")
	}
	for _, input := range []string{"bad name.com", "https://example.com", "bad..com", "a.com\r\nInjected"} {
		if _, e := Normalize(Request{Domains: []string{input}}); e == nil {
			t.Fatalf("accepted %q", input)
		}
	}
}

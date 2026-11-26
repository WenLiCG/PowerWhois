package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/redis/go-redis/v9"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"typonamer/config"
	"typonamer/internal/testutil"
	"typonamer/lookup/lookupinfo"
)

func TestFailedPublicationNeverQueuesAnIncompleteTask(t *testing.T) {
	r := testutil.Redis(t)
	s := New(r, "publication:")
	ctx := context.Background()
	r.Set(ctx, s.Prefix+"tasks", "wrong-type", 0)
	task, _, err := s.Create(ctx, Request{Domains: []string{"example.com"}}, "same")
	if err == nil {
		t.Fatal("invalid index accepted")
	}
	if r.Exists(ctx, s.key(task.ID, "meta"), s.Prefix+"idem:same", s.key(task.ID, "input")).Val() != 0 || r.ZCard(ctx, s.Prefix+"queued").Val() != 0 {
		t.Fatal("failed publication left a visible task")
	}
	r.Del(ctx, s.Prefix+"tasks")
	task, reused, err := s.Create(ctx, Request{Domains: []string{"example.com"}}, "same")
	if err != nil || reused || task.ID == "" {
		t.Fatal(task, reused, err)
	}
}

type lostPublicationReply struct{ once atomic.Bool }

func (h *lostPublicationReply) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) { return next(ctx, network, addr) }
}
func (h *lostPublicationReply) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (h *lostPublicationReply) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		e := next(ctx, cmd)
		if e == nil && cmd.Name() == "eval" && cmd.Args()[1] == createScript && h.once.CompareAndSwap(false, true) {
			return errors.New("publication reply lost")
		}
		return e
	}
}
func TestLostPublicationReplyKeepsCommittedInputForIdempotentRetry(t *testing.T) {
	r := testutil.Redis(t)
	r.AddHook(&lostPublicationReply{})
	s := New(r, "lost-reply:")
	ctx := context.Background()
	req := Request{Domains: []string{"example.com", "example.org"}}
	first, _, e := s.Create(ctx, req, "same")
	if e == nil {
		t.Fatal("fault not injected")
	}
	same, reused, e := s.Create(ctx, req, "same")
	if e != nil || !reused || first.ID != same.ID || r.LLen(ctx, s.key(same.ID, "input")).Val() != 2 {
		t.Fatal("lost response destroyed committed input", e)
	}
}

func TestLostLeaseFencesControlsCreationAndFinalState(t *testing.T) {
	r := testutil.Redis(t)
	s := New(r, "fence:")
	ctx := context.Background()
	task, _, e := s.Create(ctx, Request{Domains: []string{"example.com"}}, "")
	if e != nil {
		t.Fatal(e)
	}
	paused, e := s.Control(ctx, task.ID, "pause")
	if e != nil {
		t.Fatal(e)
	}
	stored, e := s.Get(ctx, task.ID)
	if e != nil || paused.UpdatedAt != stored.UpdatedAt || paused.UpdatedAt == task.UpdatedAt {
		t.Fatal("pause did not return its persisted transition timestamp")
	}
	runctx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- s.Run(runctx) }()
	select {
	case <-s.Ready:
	case <-time.After(3 * time.Second):
		t.Fatal("startup")
	}
	defer func() { cancel(); <-done }()
	stored, e = s.Get(ctx, task.ID)
	if e != nil || stored.UpdatedAt != paused.UpdatedAt {
		t.Fatal("restart changed an untouched task timestamp")
	}
	r.Set(ctx, s.Prefix+"worker", "new-owner", time.Minute)
	if _, e = s.Control(ctx, task.ID, "resume"); e == nil {
		t.Fatal("stale control accepted")
	}
	if _, _, e = s.Create(ctx, Request{Domains: []string{"example.org"}}, "late"); e == nil {
		t.Fatal("stale creation accepted")
	}
	task.State = "completed"
	if s.save(ctx, task) == nil {
		t.Fatal("stale final state accepted")
	}
	actual, _ := s.Get(ctx, task.ID)
	if actual.State != "paused" {
		t.Fatal("stale writer changed state", actual.State)
	}
}

func TestResultFailureDoesNotLeaveResultWithoutCount(t *testing.T) {
	r := testutil.Redis(t)
	s := New(r, "commit:")
	ctx := context.Background()
	keys := []string{s.Prefix + "worker", s.key("fixture", "results"), s.key("fixture", "counts")}
	r.Set(ctx, keys[2], "wrong-type", 0)
	args := []interface{}{"", 0, `{"domain":"example.com","status":"registered"}`, "registered"}
	if r.Eval(ctx, resultScript, keys, args...).Err() == nil || r.HLen(ctx, keys[1]).Val() != 0 {
		t.Fatal("partial result commit")
	}
	r.Del(ctx, keys[2])
	r.HSet(ctx, keys[2], "registered", "9223372036854775807")
	if r.Eval(ctx, resultScript, keys, args...).Err() == nil || r.HLen(ctx, keys[1]).Val() != 0 {
		t.Fatal("count overflow left a result")
	}
	r.Del(ctx, keys[2])
	if e := r.Eval(ctx, resultScript, keys, args...).Err(); e != nil {
		t.Fatal(e)
	}
	r.Eval(ctx, resultScript, keys, args...)
	if r.HGet(ctx, keys[2], "registered").Val() != "1" {
		t.Fatal("duplicate counted twice")
	}
}

func TestInconsistentTaskKeepsEvidenceAndYieldsQueue(t *testing.T) {
	for _, missing := range []string{"input", "counts"} {
		t.Run(missing, func(t *testing.T) {
			ctx := context.Background()
			r := testutil.Redis(t)
			s := New(r, "restore:")
			s.Query = func(_ context.Context, domain, _ string, _, _ int) lookupinfo.QueryResult {
				return lookupinfo.QueryResult{Domain: domain, Status: "registered"}
			}
			task, _, err := s.Create(ctx, Request{Domains: []string{"first.com", "second.com"}}, "")
			if err != nil {
				t.Fatal(err)
			}
			input, results, counts := s.key(task.ID, "input"), s.key(task.ID, "results"), s.key(task.ID, "counts")
			saved := `{"domain":"first.com","status":"unknown","source":"original"}`
			if err := r.HSet(ctx, results, "0", saved).Err(); err != nil {
				t.Fatal(err)
			}
			if missing == "input" {
				if err := r.HSet(ctx, counts, "unknown", 1).Err(); err != nil {
					t.Fatal(err)
				}
				if first, err := r.LPop(ctx, input).Result(); err != nil || first != "first.com" {
					t.Fatal(first, err)
				}
			}
			stop := start(t, s)
			defer stop()
			failed := waitTask(t, s, task.ID, func(t Task) bool { return t.State == "failed" })
			if failed.Error == "" || r.HLen(ctx, results).Val() != 1 || r.HGet(ctx, results, "0").Val() != saved {
				t.Fatal("worker changed results before detecting inconsistent storage")
			}
			next, _, err := s.Create(ctx, Request{Domains: []string{"next.com"}}, "")
			if err != nil {
				t.Fatal(err)
			}
			waitTask(t, s, next.ID, func(t Task) bool { return t.State == "completed" })
			if missing == "input" {
				err = r.LPush(ctx, input, "first.com").Err()
			} else {
				err = r.HSet(ctx, counts, "unknown", 1).Err()
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.Control(ctx, task.ID, "resume"); err != nil {
				t.Fatal(err)
			}
			finished := waitTask(t, s, task.ID, func(t Task) bool { return t.State == "completed" })
			if finished.Completed != 2 || finished.Counts["registered"] != 1 || r.HGet(ctx, results, "0").Val() != saved {
				t.Fatal("resumed task did not retain its original result", finished)
			}
		})
	}
}

func TestResultPaginationBoundsBytesWithoutSkippingInputs(t *testing.T) {
	r := testutil.Redis(t)
	s := New(r, "pages:")
	ctx := context.Background()
	names := []string{}
	for i := 0; i < 17; i++ {
		names = append(names, fmt.Sprintf("large%d.com", i))
	}
	task, _, e := s.Create(ctx, Request{Domains: names}, "")
	if e != nil {
		t.Fatal(e)
	}
	for i, name := range names {
		data, _ := json.Marshal(lookupinfo.QueryResult{Order: i, Domain: name, Status: "error", NameServer: []string{strings.Repeat("x", 600000)}, RawResponse: strings.Repeat("x", 2*1024*1024)})
		r.HSet(ctx, s.key(task.ID, "results"), i, data)
	}
	// The 17 records total over 40 MiB. The server will drop a client that
	// buffers them in one reply, even if the HTTP page itself is later trimmed.
	if e := r.ConfigSet(ctx, "client-output-buffer-limit", "normal 16mb 0 0").Err(); e != nil {
		t.Fatal(e)
	}
	if e := r.Do(ctx, "ACL", "SETUSER", "default", "-evalsha", "-hstrlen").Err(); e != nil {
		t.Fatal(e)
	}
	seen := map[int]bool{}
	pages := 0
	for offset := 0; offset >= 0; {
		p, e := s.Results(ctx, task.ID, offset, 1000)
		if e != nil {
			t.Fatal(e)
		}
		data, _ := json.Marshal(p)
		if len(data) > 8*1024*1024 {
			t.Fatal("page too large", len(data))
		}
		for _, item := range p.Items {
			if seen[item.Order] {
				t.Fatal("duplicate page item")
			}
			seen[item.Order] = true
		}
		offset = p.NextOffset
		pages++
	}
	if len(seen) != 17 || pages < 2 {
		t.Fatal("byte pagination skipped results")
	}
	summary, e := s.Results(ctx, task.ID, 0, 1000, false)
	if e != nil || len(summary.Items) == 0 || summary.Items[0].RawResponse != "" || !summary.Items[0].RawResponseAvailable {
		t.Fatal("summary lost evidence reference", e)
	}
	task, e = s.Control(ctx, task.ID, "pause")
	if e != nil {
		t.Fatal(e)
	}
	// This task predates a lower concurrency cap in the current configuration.
	cap := config.GetConfig().BulkCheckConcurrencyLimit
	task.Concurrency = cap + 1
	if err := s.save(ctx, task); err != nil {
		t.Fatal(err)
	}
	retry, e := s.RetryFailed(ctx, task.ID)
	if e != nil || retry.Total != 17 || retry.ParentTaskID != task.ID || retry.Concurrency != cap {
		t.Fatal("retry lost results or exceeded the current cap", retry.Total, retry.Concurrency, e)
	}
	// Legacy evidence can exceed the raw limit while its summary remains readable.
	huge, _ := json.Marshal(lookupinfo.QueryResult{Domain: names[1], Order: 1, RawResponse: strings.Repeat("x", 16*1024*1024)})
	if e := r.HSet(ctx, s.key(task.ID, "results"), 1, huge).Err(); e != nil {
		t.Fatal(e)
	}
	first, e := s.Results(ctx, task.ID, 0, 3)
	if e != nil || len(first.Items) != 1 || first.NextOffset != 1 {
		t.Fatal("oversized legacy result hid the preceding row", e)
	}
	if _, e := s.Results(ctx, task.ID, 1, 1); !errors.Is(e, ErrResultTooLarge) {
		t.Fatal("expected a size error for full legacy evidence", e)
	}
	if e := r.ConfigSet(ctx, "client-output-buffer-limit", "normal 0 0 0").Err(); e != nil {
		t.Fatal(e)
	}
	tiny, e := s.Results(ctx, task.ID, 1, 2, false)
	if e != nil || len(tiny.Items) != 2 || !tiny.Items[0].RawResponseAvailable || tiny.NextOffset != 3 {
		t.Fatal("oversized raw evidence blocked its summary or the following row", e)
	}
	if e := r.HSet(ctx, s.key(task.ID, "results"), 1, "").Err(); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Results(ctx, task.ID, 1, 1, false); e == nil {
		t.Fatal("empty stored result silently treated as an unfinished input")
	}
}

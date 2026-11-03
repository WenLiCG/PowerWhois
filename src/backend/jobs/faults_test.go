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
	if _, e = s.Control(ctx, task.ID, "pause"); e != nil {
		t.Fatal(e)
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
	if _, e = s.Control(ctx, task.ID, "pause"); e != nil {
		t.Fatal(e)
	}
	retry, e := s.RetryFailed(ctx, task.ID)
	if e != nil || retry.Total != 17 || retry.ParentTaskID != task.ID {
		t.Fatal("retry skipped a byte-limited page", retry.Total, e)
	}
}

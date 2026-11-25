package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"typonamer/config"
	"typonamer/lookup/lookuper"
	"typonamer/lookup/lookupinfo"
	"typonamer/utils"
)

var ErrNotFound = errors.New("task not found")
var ErrConflict = errors.New("task state or idempotency key conflict")
var ErrNoRetryResults = errors.New("no failed or unknown results to retry")
var ErrResultTooLarge = errors.New("stored result exceeds safe response size; use summary results")
var ErrInvalidRequest = errors.New("invalid task request")

type Request struct {
	parentID    string
	Domains     []string `json:"domains"`
	Mode        string   `json:"mode"`
	Concurrency int      `json:"concurrency"`
	Timeout     int      `json:"timeout"`
	Retries     int      `json:"retries"`
}
type Task struct {
	ParentTaskID string         `json:"parentTaskId,omitempty"`
	ID           string         `json:"id"`
	State        string         `json:"state"`
	Mode         string         `json:"mode"`
	Total        int            `json:"total"`
	Completed    int            `json:"completed"`
	Counts       map[string]int `json:"counts"`
	Concurrency  int            `json:"concurrency"`
	Timeout      int            `json:"timeout"`
	Retries      int            `json:"retries"`
	CreatedAt    string         `json:"createdAt"`
	UpdatedAt    string         `json:"updatedAt"`
	Error        string         `json:"error,omitempty"`
}

func (t *Task) setState(state string) {
	t.State = state
	t.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
}

type Page struct {
	Items      []lookupinfo.QueryResult `json:"items"`
	Offset     int                      `json:"offset"`
	NextOffset int                      `json:"nextOffset"`
	Total      int                      `json:"total"`
}
type QueryFunc func(context.Context, string, string, int, int) lookupinfo.QueryResult

type Store struct {
	Ready    chan struct{}
	Redis    *redis.Client
	Prefix   string
	Query    QueryFunc
	mu       sync.Mutex
	cancel   context.CancelFunc
	activeID string
	owner    string
	ctx      context.Context
}

func New(r *redis.Client, prefix string) *Store {
	return &Store{Redis: r, Prefix: prefix, Query: lookuper.Query, Ready: make(chan struct{})}
}
func (s *Store) key(id, part string) string { return s.Prefix + "task:" + id + ":" + part }

func Normalize(r Request) (Request, error) {
	if len(r.Domains) == 0 || len(r.Domains) > 1000000 {
		return r, fmt.Errorf("domains must contain 1..1000000 names")
	}
	if r.Mode == "" {
		r.Mode = "whoisQuery"
	}
	switch r.Mode {
	case "dns":
		r.Mode = "dnsQuery"
	case "whois":
		r.Mode = "whoisQuery"
	case "mixed":
		r.Mode = "mixedQuery"
	case "proxy":
		r.Mode = "whoisQueryWithProxy"
	}
	if !lookuper.ValidMode(r.Mode) {
		return r, fmt.Errorf("unknown query mode")
	}
	if r.Concurrency == 0 {
		r.Concurrency = min(15, config.GetConfig().BulkCheckConcurrencyLimit)
	}
	if r.Timeout == 0 {
		r.Timeout = 30
	}
	if r.Concurrency < 1 || r.Concurrency > min(256, config.GetConfig().BulkCheckConcurrencyLimit) || r.Timeout < 1 || r.Timeout > 120 || r.Retries < 0 || r.Retries > 5 {
		return r, fmt.Errorf("invalid concurrency (server cap), timeout (1..120), or retries (0..5)")
	}
	seen := make(map[string]bool, len(r.Domains))
	domains := make([]string, 0, len(r.Domains))
	for i, d := range r.Domains {
		n, err := utils.TrimAndGetMainDomain(d)
		if err != nil {
			// Clients drop blank lines before submitting, so name the value as well as its position.
			if preview := []rune(d); len(preview) > 80 {
				d = string(preview[:80]) + "…"
			}
			return r, fmt.Errorf("invalid domain at line %d: %q", i+1, d)
		}
		if !seen[n] {
			seen[n] = true
			domains = append(domains, n)
		}
	}
	r.Domains = domains
	return r, nil
}

func (s *Store) Create(ctx context.Context, r Request, idempotency string) (Task, bool, error) {
	r, err := Normalize(r)
	if err != nil {
		return Task{}, false, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	if len(idempotency) > 128 {
		return Task{}, false, fmt.Errorf("%w: idempotency key exceeds 128 bytes", ErrInvalidRequest)
	}
	raw, _ := json.Marshal(r)
	sum := sha256.Sum256(raw)
	fingerprint := hex.EncodeToString(sum[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	idemKey := s.Prefix + "idem:" + idempotency
	if idempotency != "" {
		old, e := s.Redis.Get(ctx, idemKey).Result()
		if e != nil && e != redis.Nil {
			return Task{}, false, e
		}
		if e == nil {
			parts := strings.SplitN(old, ":", 2)
			if len(parts) != 2 || parts[0] != fingerprint {
				return Task{}, false, ErrConflict
			}
			t, e := s.Get(ctx, parts[1])
			return t, true, e
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	t := Task{ID: uuid.NewString(), ParentTaskID: r.parentID, State: "queued", Mode: r.Mode, Total: len(r.Domains), Concurrency: r.Concurrency, Timeout: r.Timeout, Retries: r.Retries, CreatedAt: now, UpdatedAt: now, Counts: map[string]int{}}
	data, _ := json.Marshal(t)
	inputKey := s.key(t.ID, "input")
	published := false
	defer func() {
		if !published {
			cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			// A lost publication response may hide success; never remove a published input.
			s.Redis.Eval(cleanup, `if redis.call('EXISTS',KEYS[1])==0 then return redis.call('DEL',KEYS[2]) end return 0`, []string{s.key(t.ID, "meta"), inputKey})
		}
	}()
	for i := 0; i < len(r.Domains); i += 1000 {
		end := min(i+1000, len(r.Domains))
		args := make([]interface{}, end-i)
		for j, d := range r.Domains[i:end] {
			args[j] = d
		}
		if e := s.Redis.Eval(ctx, `redis.call('RPUSH',KEYS[1],unpack(ARGV)); return redis.call('EXPIRE',KEYS[1],3600)`, []string{inputKey}, args...).Err(); e != nil {
			return Task{}, false, e
		}
	}
	useKey := "0"
	if idempotency != "" {
		useKey = "1"
	}
	err = s.Redis.Eval(ctx, createScript, []string{s.Prefix + "worker", s.key(t.ID, "meta"), inputKey, s.Prefix + "tasks", s.Prefix + "queued", idemKey}, s.owner, t.ID, data, fingerprint+":"+t.ID, len(r.Domains), useKey, time.Now().UnixMilli()).Err()
	published = err == nil
	return t, false, err
}

func (s *Store) Get(ctx context.Context, id string) (Task, error) {
	var t Task
	data, err := s.Redis.Get(ctx, s.key(id, "meta")).Bytes()
	if err == redis.Nil {
		return t, ErrNotFound
	}
	if err != nil {
		return t, err
	}
	if err = json.Unmarshal(data, &t); err != nil {
		return t, err
	}
	counts, err := s.Redis.HGetAll(ctx, s.key(id, "counts")).Result()
	if err != nil {
		return t, err
	}
	t.Counts = map[string]int{}
	for k, v := range counts {
		n, e := strconv.Atoi(v)
		if e != nil {
			return t, e
		}
		t.Counts[k] = n
		t.Completed += n
	}
	return t, nil
}
func (s *Store) List(ctx context.Context, offset, limit int) ([]Task, error) {
	ids, err := s.Redis.LRange(ctx, s.Prefix+"tasks", int64(offset), int64(offset+limit-1)).Result()
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []Task{}, nil
	}
	pipe := s.Redis.Pipeline()
	metas := make([]*redis.StringCmd, len(ids))
	counts := make([]*redis.MapStringStringCmd, len(ids))
	for i, id := range ids {
		metas[i] = pipe.Get(ctx, s.key(id, "meta"))
		counts[i] = pipe.HGetAll(ctx, s.key(id, "counts"))
	}
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}
	out := make([]Task, 0, len(ids))
	for i := range ids {
		var t Task
		data, e := metas[i].Bytes()
		if e == redis.Nil {
			return nil, ErrNotFound
		}
		if e != nil {
			return nil, e
		}
		if e = json.Unmarshal(data, &t); e != nil {
			return nil, e
		}
		rawCounts, e := counts[i].Result()
		if e != nil {
			return nil, e
		}
		t.Counts = map[string]int{}
		for k, v := range rawCounts {
			n, convErr := strconv.Atoi(v)
			if convErr != nil {
				return nil, convErr
			}
			t.Counts[k] = n
			t.Completed += n
		}
		out = append(out, t)
	}
	return out, nil
}

// Results advances by input index and also bounds each page by encoded bytes.
func (s *Store) Results(ctx context.Context, id string, offset, limit int, includeRaw ...bool) (Page, error) {
	t, err := s.Get(ctx, id)
	p := Page{Items: []lookupinfo.QueryResult{}, Offset: offset, NextOffset: -1, Total: t.Total}
	if err != nil {
		return p, err
	}
	if offset >= t.Total {
		return p, nil
	}
	end := offset + min(limit, t.Total-offset)
	const pageBudget = 8 * 1024 * 1024
	const recordLimit = 16 * 1024 * 1024
	summary := len(includeRaw) > 0 && !includeRaw[0]
	maxRaw := recordLimit
	if summary {
		// A legacy record can exceed the raw limit and still have a small summary.
		maxRaw = 0
	}
	used := 1024
	for base := offset; base < end; {
		vals, e := s.Redis.Eval(ctx, resultChunk, []string{s.key(id, "results")}, base, min(base+100, end), pageBudget, maxRaw).Slice()
		if e != nil {
			return p, e
		}
		for i, rawVal := range vals {
			if rawVal == nil {
				continue
			}
			strVal, ok := rawVal.(string)
			if !ok {
				// The script returned the size in place of an oversized raw record.
				if len(p.Items) == 0 {
					return p, ErrResultTooLarge
				}
				p.NextOffset = base + i
				return p, nil
			}
			data := []byte(strVal)
			var r lookupinfo.QueryResult
			if e = json.Unmarshal(data, &r); e != nil {
				return p, e
			}
			if summary {
				r.RawResponseAvailable = r.RawResponse != ""
				r.RawResponse = ""
				data, _ = json.Marshal(r)
			}
			if used+len(data) > pageBudget && len(p.Items) > 0 {
				p.NextOffset = base + i
				return p, nil
			}
			if len(data) > recordLimit {
				return p, ErrResultTooLarge
			}
			p.Items = append(p.Items, r)
			used += len(data)
		}
		base += len(vals)
	}
	if end < t.Total {
		p.NextOffset = end
	}
	return p, nil
}

// Controls and worker commits serialize on mu; a paused/canceled task cannot receive late results.
func (s *Store) Control(ctx context.Context, id, action string) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.Get(ctx, id)
	if err != nil {
		return t, err
	}
	switch action {
	case "pause":
		if t.State == "paused" {
			return t, nil
		}
		if t.State != "running" && t.State != "queued" {
			return t, ErrConflict
		}
		t.setState("paused")
	case "resume":
		if t.State == "running" || t.State == "queued" {
			return t, nil
		}
		if t.State != "paused" && t.State != "failed" {
			return t, ErrConflict
		}
		t.setState("queued")
		t.Error = ""
	case "cancel":
		if t.State == "canceled" {
			return t, nil
		}
		if t.State == "completed" {
			return t, ErrConflict
		}
		t.setState("canceled")
	default:
		return t, fmt.Errorf("unknown task action")
	}
	if err = s.save(ctx, t); err != nil {
		return t, err
	}
	if s.activeID == id && s.cancel != nil {
		s.cancel()
	}
	return t, nil
}
func (s *Store) save(ctx context.Context, t Task) error {
	t.Completed = 0
	t.Counts = nil
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	created, _ := time.Parse(time.RFC3339Nano, t.CreatedAt)
	err = s.Redis.Eval(ctx, saveScript, []string{s.Prefix + "worker", s.key(t.ID, "meta"), s.Prefix + "queued"}, s.owner, data, t.State, t.ID, created.UnixMilli()).Err()
	return err
}

func (s *Store) RetryFailed(ctx context.Context, id string) (Task, error) {
	t, err := s.Get(ctx, id)
	if err != nil {
		return t, err
	}
	if t.State == "running" || t.State == "queued" {
		return t, ErrConflict
	}
	domains := []string{}
	for offset := 0; offset >= 0; {
		page, e := s.Results(ctx, id, offset, 1000, false)
		if e != nil {
			return t, e
		}
		for _, r := range page.Items {
			if r.Status == "error" || r.Status == "timeout" || r.Status == "unknown" {
				domains = append(domains, r.Domain)
			}
		}
		offset = page.NextOffset
	}
	if len(domains) == 0 {
		return t, ErrNoRetryResults
	}
	concurrency := min(t.Concurrency, config.GetConfig().BulkCheckConcurrencyLimit)
	task, _, err := s.Create(ctx, Request{parentID: id, Domains: domains, Mode: t.Mode, Concurrency: concurrency, Timeout: t.Timeout, Retries: t.Retries}, "")
	return task, err
}

// Run owns a renewable lease. One backend per namespace keeps scheduling and control simple.
// shortcut: jobs execute in FIFO order, add fair multi-job scheduling only for multi-tenant deployments.
func (s *Store) Run(ctx context.Context) error {
	s.owner = uuid.NewString()
	lease := s.Prefix + "worker"
	ok, err := s.Redis.SetNX(ctx, lease, s.owner, 15*time.Second).Result()
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("another backend owns the task worker lease")
	}
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	s.ctx = ctx
	defer s.Redis.Eval(context.Background(), `if redis.call('GET',KEYS[1])==ARGV[1] then return redis.call('DEL',KEYS[1]) end return 0`, []string{lease}, s.owner)
	leaseErrors := make(chan error, 1)
	go func() {
		tick := time.NewTicker(3 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				n, e := s.Redis.Eval(ctx, `if redis.call('GET',KEYS[1])==ARGV[1] then return redis.call('PEXPIRE',KEYS[1],15000) end return 0`, []string{lease}, s.owner).Int()
				if e != nil || n != 1 {
					leaseErrors <- fmt.Errorf("worker lease lost")
					stop()
					return
				}
			}
		}
	}()
	if err := s.ImportLegacy(ctx); err != nil {
		return err
	}
	ids, err := s.Redis.LRange(ctx, s.Prefix+"tasks", 0, -1).Result()
	if err != nil {
		return err
	}
	for _, id := range ids {
		t, e := s.Get(ctx, id)
		if e != nil {
			return e
		}
		if t.State == "running" {
			t.setState("queued")
		}
		// Reconcile the queue from durable metadata after an interrupted state write.
		if e = s.save(ctx, t); e != nil {
			return e
		}
	}
	close(s.Ready)
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			select {
			case e := <-leaseErrors:
				return e
			default:
				return nil
			}
		case <-tick.C:
		}
		ids, err = s.Redis.ZRange(ctx, s.Prefix+"queued", 0, 0).Result()
		if err != nil {
			return err
		}
		for i := len(ids) - 1; i >= 0; i-- {
			t, e := s.Get(ctx, ids[i])
			if e != nil {
				return e
			}
			if t.State == "queued" {
				if e = s.runTask(ctx, t.ID); e != nil {
					return e
				}
				break
			}
		}
	}
}

func (s *Store) runTask(parent context.Context, id string) error {
	s.mu.Lock()
	t, err := s.Get(parent, id)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	if t.State != "queued" {
		s.mu.Unlock()
		return nil
	}
	t.setState("running")
	if err = s.save(parent, t); err != nil {
		s.mu.Unlock()
		return err
	}
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	s.activeID = id
	s.mu.Unlock()
	defer cancel()
	type item struct {
		index  int
		domain string
	}
	ch := make(chan item)
	var wg sync.WaitGroup
	failures := make(chan error, 1)
	fail := func(e error) {
		select {
		case failures <- e:
		default:
		}
		cancel()
	}
	for i := 0; i < t.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for v := range ch {
				if ctx.Err() != nil {
					return
				}
				r := s.Query(ctx, v.domain, t.Mode, t.Timeout, t.Retries)
				r.Order = v.index
				if ctx.Err() != nil {
					return
				}
				data, e := json.Marshal(r)
				if e != nil {
					fail(e)
					return
				}
				s.mu.Lock()
				if ctx.Err() == nil {
					// Result + count are one atomic commit, fenced by the worker lease. Inputs are never removed.
					_, e = s.Redis.Eval(ctx, resultScript, []string{s.Prefix + "worker", s.key(id, "results"), s.key(id, "counts")}, s.owner, v.index, data, r.Status).Result()
				}
				s.mu.Unlock()
				if e != nil {
					fail(e)
					return
				}
			}
		}()
	}
feed:
	for offset := 0; offset < t.Total; offset += 1000 {
		names, e := s.Redis.LRange(ctx, s.key(id, "input"), int64(offset), int64(min(offset+999, t.Total-1))).Result()
		if e != nil {
			fail(e)
			break
		}
		fields := make([]string, len(names))
		for i := range names {
			fields[i] = strconv.Itoa(offset + i)
		}
		pipe := s.Redis.Pipeline()
		done := make([]*redis.BoolCmd, len(fields))
		for i, field := range fields {
			done[i] = pipe.HExists(ctx, s.key(id, "results"), field)
		}
		_, e = pipe.Exec(ctx)
		if e != nil {
			fail(e)
			break
		}
		for i, d := range names {
			if done[i].Val() {
				continue
			}
			select {
			case <-ctx.Done():
				break feed
			case ch <- item{offset + i, d}:
			}
		}
	}
	close(ch)
	wg.Wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancel = nil
	s.activeID = ""
	if parent.Err() != nil {
		return nil
	}
	t, err = s.Get(parent, id)
	if err != nil {
		return err
	}
	if t.State != "running" {
		return nil
	}
	select {
	case err = <-failures:
		t.setState("failed")
		t.Error = "task storage or worker failed; completed results retained"
	default:
		if t.Completed == t.Total {
			t.setState("completed")
		} else {
			t.setState("queued")
		}
	}
	return s.save(parent, t)
}

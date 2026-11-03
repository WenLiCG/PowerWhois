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

type Request struct {
	Domains     []string `json:"domains"`
	Mode        string   `json:"mode"`
	Concurrency int      `json:"concurrency"`
	Timeout     int      `json:"timeout"`
	Retries     int      `json:"retries"`
}
type Task struct {
	ID          string         `json:"id"`
	State       string         `json:"state"`
	Mode        string         `json:"mode"`
	Total       int            `json:"total"`
	Completed   int            `json:"completed"`
	Counts      map[string]int `json:"counts"`
	Concurrency int            `json:"concurrency"`
	Timeout     int            `json:"timeout"`
	Retries     int            `json:"retries"`
	CreatedAt   string         `json:"createdAt"`
	UpdatedAt   string         `json:"updatedAt"`
	Error       string         `json:"error,omitempty"`
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
		r.Concurrency = min(16, config.GetConfig().BulkCheckConcurrencyLimit)
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
			return r, fmt.Errorf("invalid domain at line %d", i+1)
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
		return Task{}, false, err
	}
	if len(idempotency) > 128 {
		return Task{}, false, fmt.Errorf("idempotency key exceeds 128 characters")
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
	t := Task{ID: uuid.NewString(), State: "queued", Mode: r.Mode, Total: len(r.Domains), Concurrency: r.Concurrency, Timeout: r.Timeout, Retries: r.Retries, CreatedAt: now, UpdatedAt: now, Counts: map[string]int{}}
	data, _ := json.Marshal(t)
	_, err = s.Redis.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.Set(ctx, s.key(t.ID, "meta"), data, 0)
		for i := 0; i < len(r.Domains); i += 1000 {
			end := min(i+1000, len(r.Domains))
			a := make([]interface{}, end-i)
			for j, d := range r.Domains[i:end] {
				a[j] = d
			}
			p.RPush(ctx, s.key(t.ID, "input"), a...)
		}
		p.LPush(ctx, s.Prefix+"tasks", t.ID)
		p.ZAdd(ctx, s.Prefix+"queued", redis.Z{Score: float64(time.Now().UnixMilli()), Member: t.ID})
		if idempotency != "" {
			p.Set(ctx, idemKey, fingerprint+":"+t.ID, 0)
		}
		return nil
	})
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
	out := make([]Task, 0, len(ids))
	for _, id := range ids {
		t, e := s.Get(ctx, id)
		if e != nil {
			return nil, e
		}
		out = append(out, t)
	}
	return out, nil
}
func (s *Store) Results(ctx context.Context, id string, offset, limit int) (Page, error) {
	t, err := s.Get(ctx, id)
	p := Page{Items: []lookupinfo.QueryResult{}, Offset: offset, NextOffset: -1, Total: t.Total}
	if err != nil {
		return p, err
	}
	end := min(offset+limit, t.Total)
	if offset >= end {
		return p, nil
	}
	fields := make([]string, end-offset)
	for i := range fields {
		fields[i] = strconv.Itoa(offset + i)
	}
	values, err := s.Redis.HMGet(ctx, s.key(id, "results"), fields...).Result()
	if err != nil {
		return p, err
	}
	for _, v := range values {
		if v == nil {
			continue
		}
		var r lookupinfo.QueryResult
		if err = json.Unmarshal([]byte(v.(string)), &r); err != nil {
			return p, err
		}
		p.Items = append(p.Items, r)
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
		t.State = "paused"
	case "resume":
		if t.State == "running" || t.State == "queued" {
			return t, nil
		}
		if t.State != "paused" && t.State != "failed" {
			return t, ErrConflict
		}
		t.State = "queued"
		t.Error = ""
	case "cancel":
		if t.State == "canceled" {
			return t, nil
		}
		if t.State == "completed" {
			return t, ErrConflict
		}
		t.State = "canceled"
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
	t.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	t.Completed = 0
	t.Counts = nil
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	_, err = s.Redis.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.Set(ctx, s.key(t.ID, "meta"), data, 0)
		if t.State == "queued" {
			created, _ := time.Parse(time.RFC3339Nano, t.CreatedAt)
			p.ZAdd(ctx, s.Prefix+"queued", redis.Z{Score: float64(created.UnixMilli()), Member: t.ID})
		} else {
			p.ZRem(ctx, s.Prefix+"queued", t.ID)
		}
		return nil
	})
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
	for offset := 0; offset < t.Total; offset += 1000 {
		page, e := s.Results(ctx, id, offset, 1000)
		if e != nil {
			return t, e
		}
		for _, r := range page.Items {
			if r.Status == "error" || r.Status == "timeout" || r.Status == "unknown" {
				domains = append(domains, r.Domain)
			}
		}
	}
	if len(domains) == 0 {
		return t, ErrNoRetryResults
	}
	task, _, err := s.Create(ctx, Request{Domains: domains, Mode: t.Mode, Concurrency: t.Concurrency, Timeout: t.Timeout, Retries: t.Retries}, "")
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
			t.State = "queued"
			if e = s.save(ctx, t); e != nil {
				return e
			}
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
	t.State = "running"
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
					_, e = s.Redis.Eval(ctx, `if redis.call('GET',KEYS[1])~=ARGV[1] then return redis.error_reply('lease lost') end if redis.call('HSETNX',KEYS[2],ARGV[2],ARGV[3])==1 then redis.call('HINCRBY',KEYS[3],ARGV[4],1) end return 1`, []string{s.Prefix + "worker", s.key(id, "results"), s.key(id, "counts")}, s.owner, v.index, data, r.Status).Result()
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
		done, e := s.Redis.HMGet(ctx, s.key(id, "results"), fields...).Result()
		if e != nil {
			fail(e)
			break
		}
		for i, d := range names {
			if done[i] != nil {
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
		t.State = "failed"
		t.Error = "task storage or worker failed; completed results retained"
	default:
		if t.Completed == t.Total {
			t.State = "completed"
		} else {
			t.State = "queued"
		}
	}
	return s.save(parent, t)
}

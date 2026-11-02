package workflow

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Service struct {
	Store      *Store
	provider   Provider
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	mu         sync.Mutex
	jobs       map[string]context.CancelFunc
	contexts   map[string]context.Context
	generating map[string]bool
	failure    error
}

func NewService(s *Store, p Provider, workers int) (*Service, error) {
	if workers < 1 || workers > 10 {
		return nil, fmt.Errorf("workers must be 1..10")
	}
	ctx, cancel := context.WithCancel(context.Background())
	svc := &Service{Store: s, provider: p, ctx: ctx, cancel: cancel, jobs: map[string]context.CancelFunc{}, contexts: map[string]context.Context{}, generating: map[string]bool{}}
	for i := 0; i < workers; i++ {
		svc.wg.Add(1)
		go svc.worker()
	}
	return svc, nil
}
func (s *Service) Health() error { s.mu.Lock(); defer s.mu.Unlock(); return s.failure }
func (s *Service) Close()        { s.cancel(); s.wg.Wait() }
func (s *Service) Generate(ctx context.Context, id string, o GenerateOptions) (Job, error) {
	s.mu.Lock()
	if s.generating[id] {
		s.mu.Unlock()
		return Job{}, fmt.Errorf("%w: generation already active", ErrConflict)
	}
	s.generating[id] = true
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.generating, id); s.mu.Unlock() }()
	return s.Store.Generate(ctx, id, o)
}
func (s *Service) Action(id, action string) (Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failure != nil {
		return Job{}, s.failure
	}
	j, err := s.Store.Transition(id, action)
	if err != nil {
		return j, err
	}
	if j.State == "running" {
		if s.contexts[id] == nil {
			ctx, cancel := context.WithCancel(s.ctx)
			s.contexts[id] = ctx
			s.jobs[id] = cancel
		}
	} else {
		if cancel := s.jobs[id]; cancel != nil {
			cancel()
		}
		delete(s.contexts, id)
		delete(s.jobs, id)
	}
	return j, nil
}
func (s *Service) fail(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failure = fmt.Errorf("persistence failure; stop and restart after fixing storage: %w", err)
	s.cancel()
}
func (s *Service) worker() {
	defer s.wg.Done()
	for s.ctx.Err() == nil {
		s.mu.Lock()
		work, err := s.Store.claim()
		var ctx context.Context
		if work != nil {
			ctx = s.contexts[work.JobID]
		}
		s.mu.Unlock()
		if err != nil {
			s.fail(err)
			return
		}
		if work == nil {
			if sleep(s.ctx, 30*time.Millisecond) != nil {
				return
			}
			continue
		}
		if ctx == nil {
			continue
		}
		result := s.provider.Lookup(ctx, work.Domain)
		if ctx.Err() != nil {
			continue
		}
		switch result.Status {
		case "registered", "not_found", "unknown", "error":
		default:
			result.Status = "error"
			result.Error = "invalid provider status"
		}
		if result.CheckedAt.IsZero() {
			result.CheckedAt = time.Now().UTC()
		}
		if err = s.Store.finish(*work, result); err != nil {
			s.fail(err)
			return
		}
	}
}

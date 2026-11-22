package network

import (
	"context"
	"sync"
)

var slotsMu sync.Mutex
var slots = map[string]*slot{}

type slot struct {
	active  int
	waiters int
	next    chan struct{}
}

// Acquire shares a concurrency budget across all callers of the same upstream.
func Acquire(ctx context.Context, key string, limit int) (func(), error) {
	slotsMu.Lock()
	defer slotsMu.Unlock()
	s := slots[key]
	if s == nil {
		s = &slot{next: make(chan struct{})}
		slots[key] = s
	}
	s.waiters++
	defer func() {
		s.waiters--
		if s.active == 0 && s.waiters == 0 {
			delete(slots, key)
		}
	}()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if s.active < limit {
			s.active++
			return func() {
				slotsMu.Lock()
				defer slotsMu.Unlock()
				s.active--
				close(s.next)
				s.next = make(chan struct{})
				if s.active == 0 && s.waiters == 0 {
					delete(slots, key)
				}
			}, nil
		}
		next := s.next
		slotsMu.Unlock()
		select {
		case <-ctx.Done():
		case <-next:
		}
		slotsMu.Lock()
	}
}

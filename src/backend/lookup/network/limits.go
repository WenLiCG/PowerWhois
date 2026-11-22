package network

import (
	"context"
	"sync"
	"time"
)

// MaxCooldown bounds how long one upstream rate-limit reply can pause that upstream.
const MaxCooldown = 2 * time.Minute

var slotsMu sync.Mutex
var slots = map[string]*slot{}
var cooldowns = map[string]time.Time{}

type slot struct {
	active  int
	waiters int
	next    chan struct{}
}

// Acquire shares a concurrency budget across all callers of the same upstream.
// It also waits while the upstream is cooling down after a rate-limit reply.
// Callers should start their request timeout only after Acquire returns, so
// that time spent queuing is not reported as an upstream timeout.
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
		if until, ok := cooldowns[key]; ok {
			if wait := time.Until(until); wait > 0 {
				slotsMu.Unlock()
				timer := time.NewTimer(wait)
				select {
				case <-ctx.Done():
				case <-timer.C:
				}
				timer.Stop()
				slotsMu.Lock()
				continue
			}
			delete(cooldowns, key)
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

// Cooldown stops new requests to an upstream for d, capped at MaxCooldown.
// Requests already in flight are not interrupted.
func Cooldown(key string, d time.Duration) {
	if d <= 0 {
		return
	}
	until := time.Now().Add(min(d, MaxCooldown))
	slotsMu.Lock()
	defer slotsMu.Unlock()
	if until.After(cooldowns[key]) {
		cooldowns[key] = until
	}
}

// CoolingDown reports the remaining cooldown for an upstream.
func CoolingDown(key string) time.Duration {
	slotsMu.Lock()
	defer slotsMu.Unlock()
	return max(0, time.Until(cooldowns[key]))
}

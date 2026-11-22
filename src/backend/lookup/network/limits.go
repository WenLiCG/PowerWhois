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
	public  int
	waiters int
	next    chan struct{}
}

type publicKey struct{}

// WithPublic marks work done for an anonymous visitor. Acquire limits such
// work to PublicLimit of each upstream's slots, keeping the rest for
// administrators, bulk jobs and API clients.
func WithPublic(ctx context.Context) context.Context {
	return context.WithValue(ctx, publicKey{}, true)
}

// IsPublic reports whether ctx carries anonymous work.
func IsPublic(ctx context.Context) bool {
	public, _ := ctx.Value(publicKey{}).(bool)
	return public
}

// PublicLimit is how many of an upstream's limit slots anonymous work may
// hold at once. At least one slot stays reserved whenever limit allows it, so
// privileged work always has capacity without raising the total sent to the
// upstream. With the registry limits in use (3 or 4) anonymous work gets 2 or 3.
func PublicLimit(limit int) int {
	if limit <= 1 {
		return limit
	}
	return limit - max(1, limit/4)
}

// Acquire shares a concurrency budget across all callers of the same upstream.
// Anonymous work (see WithPublic) is held to PublicLimit of it.
// It also waits while the upstream is cooling down after a rate-limit reply.
// Callers should start their request timeout only after Acquire returns, so
// that time spent queuing is not reported as an upstream timeout.
func Acquire(ctx context.Context, key string, limit int) (func(), error) {
	public := IsPublic(ctx)
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
		if s.active < limit && (!public || s.public < PublicLimit(limit)) {
			s.active++
			if public {
				s.public++
			}
			return func() {
				slotsMu.Lock()
				defer slotsMu.Unlock()
				s.active--
				if public {
					s.public--
				}
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

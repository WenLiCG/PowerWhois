package api

import (
	"net/netip"
	"sync"
	"time"
)

// connectionsPerIP caps concurrent WebSocket sessions from one public address,
// so one client cannot take every server slot and lock the administrator out.
const connectionsPerIP = 16

// limitedIP reports whether per-IP limits apply. Loopback, private and
// unparsable addresses are exempt: they are either local users or a reverse
// proxy whose client address was not forwarded (POWERWHOIS_TRUSTED_PROXIES
// unset), and limiting them would put every visitor in one shared bucket.
func limitedIP(ip string) bool {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	return !(addr.IsLoopback() || addr.IsPrivate() || addr.IsUnspecified() || addr.IsLinkLocalUnicast())
}

type connectionCounter struct {
	mu     sync.Mutex
	counts map[string]int
}

func (c *connectionCounter) acquire(ip string, limit int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.counts == nil {
		c.counts = map[string]int{}
	}
	if c.counts[ip] >= limit {
		return false
	}
	c.counts[ip]++
	return true
}

func (c *connectionCounter) release(ip string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.counts[ip] <= 1 {
		delete(c.counts, ip)
	} else {
		c.counts[ip]--
	}
}

// domainBudget is a per-IP token bucket measured in domains per minute.
type domainBudget struct {
	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens  float64
	updated time.Time
}

// take charges n domains to ip. A request is admitted when the bucket holds
// at least min(n, perMinute) tokens, so one full-size batch is always possible;
// the balance may go negative, which delays the next request.
func (d *domainBudget) take(ip string, n, perMinute int, now time.Time) bool {
	if n <= 0 || perMinute <= 0 {
		return n <= 0
	}
	capacity := float64(perMinute)
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.buckets == nil {
		d.buckets = map[string]*bucket{}
	}
	if len(d.buckets) > 4096 {
		for key, b := range d.buckets {
			if now.Sub(b.updated) > time.Minute {
				delete(d.buckets, key)
			}
		}
	}
	b := d.buckets[ip]
	if b == nil {
		b = &bucket{tokens: capacity, updated: now}
		d.buckets[ip] = b
	}
	elapsed := now.Sub(b.updated).Minutes()
	if elapsed > 0 {
		b.tokens = min(capacity, b.tokens+elapsed*capacity)
		b.updated = now
	}
	if b.tokens < min(float64(n), capacity) {
		return false
	}
	b.tokens -= float64(n)
	return true
}

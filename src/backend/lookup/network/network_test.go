package network

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestCooldownDelaysNewAcquisitions(t *testing.T) {
	const key = "test:cooldown"
	Cooldown(key, 150*time.Millisecond)
	if CoolingDown(key) <= 0 {
		t.Fatal("cooldown was not recorded")
	}
	start := time.Now()
	release, err := Acquire(context.Background(), key, 1)
	if err != nil {
		t.Fatal(err)
	}
	release()
	if waited := time.Since(start); waited < 100*time.Millisecond {
		t.Fatalf("Acquire returned after %v during a 150ms cooldown", waited)
	}

	Cooldown(key, time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := Acquire(ctx, key, 1); err == nil {
		t.Fatal("Acquire ignored context cancellation during cooldown")
	}
	if remaining := CoolingDown(key); remaining > MaxCooldown {
		t.Fatalf("cooldown %v exceeds MaxCooldown", remaining)
	}
}

func TestAcquireLimitsConcurrency(t *testing.T) {
	const key = "test:limit"
	first, err := Acquire(context.Background(), key, 1)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := Acquire(ctx, key, 1); err == nil {
		t.Fatal("second caller exceeded the limit")
	}
	first()
	second, err := Acquire(context.Background(), key, 1)
	if err != nil {
		t.Fatal(err)
	}
	second()
}

// TestAnonymousWorkCannotTakeReservedUpstreamSlots guards verification finding
// F3: anonymous lookups share each upstream's real slots, but some stay reserved
// for administrators, and the total never exceeds the upstream limit.
func TestAnonymousWorkCannotTakeReservedUpstreamSlots(t *testing.T) {
	for _, limit := range []int{3, 4} {
		key := fmt.Sprintf("test:whois-reserve-%d", limit)
		public := WithPublic(context.Background())
		var releases []func()
		for range PublicLimit(limit) {
			release, err := Acquire(public, key, limit)
			if err != nil {
				t.Fatal(err)
			}
			releases = append(releases, release)
		}
		short, cancel := context.WithTimeout(public, 50*time.Millisecond)
		if _, err := Acquire(short, key, limit); err == nil {
			t.Fatalf("limit %d: anonymous work took more than %d slots", limit, PublicLimit(limit))
		}
		cancel()
		for i := PublicLimit(limit); i < limit; i++ {
			adminCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			release, err := Acquire(adminCtx, key, limit)
			cancel()
			if err != nil {
				t.Fatalf("limit %d: administrator could not use reserved slot %d: %v", limit, i+1, err)
			}
			releases = append(releases, release)
		}
		full, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		if _, err := Acquire(full, key, limit); err == nil {
			t.Fatalf("limit %d: total exceeded the upstream limit", limit)
		}
		cancel()
		// Freeing an anonymous slot lets anonymous work continue.
		releases[0]()
		again, cancel := context.WithTimeout(public, 50*time.Millisecond)
		release, err := Acquire(again, key, limit)
		cancel()
		if err != nil {
			t.Fatalf("limit %d: anonymous slot was not returned: %v", limit, err)
		}
		release()
		for _, release := range releases[1:] {
			release()
		}
	}
	// Administrators may use every slot when no anonymous work is waiting.
	var releases []func()
	for range 3 {
		release, err := Acquire(context.Background(), "test:admin-all", 3)
		if err != nil {
			t.Fatal(err)
		}
		releases = append(releases, release)
	}
	for _, release := range releases {
		release()
	}
	if PublicLimit(1) != 1 || PublicLimit(3) != 2 || PublicLimit(4) != 3 || PublicLimit(7) != 6 {
		t.Fatal("unexpected public share")
	}
}

func TestRetryAfter(t *testing.T) {
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		value string
		want  time.Duration
	}{
		{"", DefaultRateLimitCooldown},
		{"7", 7 * time.Second},
		{"0", DefaultRateLimitCooldown},
		{"-3", DefaultRateLimitCooldown},
		{"999999999999", MaxCooldown},
		{"soon", DefaultRateLimitCooldown},
		{now.Add(30 * time.Second).Format(http.TimeFormat), 30 * time.Second},
		{now.Add(-time.Minute).Format(http.TimeFormat), DefaultRateLimitCooldown},
	} {
		header := http.Header{}
		if tc.value != "" {
			header.Set("Retry-After", tc.value)
		}
		if got := RetryAfter(header, now); got != tc.want {
			t.Errorf("Retry-After %q = %v, want %v", tc.value, got, tc.want)
		}
	}
	if got := RetryAfter(nil, now); got != DefaultRateLimitCooldown {
		t.Errorf("nil header = %v", got)
	}
}

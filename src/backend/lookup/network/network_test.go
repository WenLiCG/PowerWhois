package network

import (
	"context"
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

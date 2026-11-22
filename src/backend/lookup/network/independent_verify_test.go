package network

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestIndependentVerifyReservedSlotDespitePublicWaiters checks that anonymous
// waiters do not sit ahead of an administrator on a slot the anonymous share
// is not allowed to take.
func TestIndependentVerifyReservedSlotDespitePublicWaiters(t *testing.T) {
	const key = "independent:reserve-waiters"
	const limit = 4
	public := WithPublic(context.Background())
	var releases []func()
	for range PublicLimit(limit) {
		release, err := Acquire(public, key, limit)
		if err != nil {
			t.Fatal(err)
		}
		releases = append(releases, release)
	}
	ctx, cancel := context.WithCancel(public)
	defer cancel()
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := Acquire(ctx, key, limit)
			if err == nil {
				release()
				t.Error("anonymous waiter took a slot reserved from anonymous work")
			}
		}()
	}
	time.Sleep(50 * time.Millisecond)
	adminCtx, stop := context.WithTimeout(context.Background(), 200*time.Millisecond)
	release, err := Acquire(adminCtx, key, limit)
	stop()
	if err != nil {
		t.Fatalf("administrator did not get the reserved slot while anonymous waiters were queued: %v", err)
	}
	release()
	cancel()
	wg.Wait()
	for _, release := range releases {
		release()
	}
}

func TestIndependentVerifyCancelledWaiterDoesNotLeakSlot(t *testing.T) {
	const key = "independent:cancel-leak"
	holder, err := Acquire(context.Background(), key, 1)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := Acquire(ctx, key, 1)
		done <- err
	}()
	time.Sleep(30 * time.Millisecond)
	cancel()
	if err := <-done; err == nil {
		t.Fatal("cancelled waiter acquired the only slot")
	}
	holder()
	release, err := Acquire(context.Background(), key, 1)
	if err != nil {
		t.Fatal(err)
	}
	release()
}

func TestIndependentVerifyAcquireDoesNotExceedLimit(t *testing.T) {
	const key = "independent:race-limit"
	const limit = 4
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	public := WithPublic(ctx)
	var held atomic.Int32
	var maxHeld atomic.Int32
	var wg sync.WaitGroup
	for i := range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			caller := ctx
			if i%3 == 0 {
				caller = public
			}
			for {
				release, err := Acquire(caller, key, limit)
				if err != nil {
					return
				}
				n := held.Add(1)
				for {
					old := maxHeld.Load()
					if n <= old || maxHeld.CompareAndSwap(old, n) {
						break
					}
				}
				if n > limit {
					t.Errorf("held %d, limit %d", n, limit)
				}
				time.Sleep(time.Millisecond)
				held.Add(-1)
				release()
			}
		}()
	}
	wg.Wait()
	if maxHeld.Load() > limit {
		t.Fatalf("max held %d, limit %d", maxHeld.Load(), limit)
	}
	if maxHeld.Load() < 1 {
		t.Fatal("no caller acquired a slot before cancellation")
	}
}

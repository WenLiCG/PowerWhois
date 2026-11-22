package testredis

import (
	"context"
	"io"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// Start uses a private, socket-only server so existing Redis data cannot be touched.
func Start(t *testing.T) *redis.Client {
	t.Helper()
	bin, err := exec.LookPath("redis-server")
	if err != nil {
		t.Skip("redis-server is required for this integration test")
	}
	dir := t.TempDir()
	socket := filepath.Join(dir, "r.sock")
	cmd := exec.Command(bin, "--port", "0", "--unixsocket", socket, "--dir", dir, "--save", "", "--appendonly", "no")
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })

	client := redis.NewClient(&redis.Options{
		Addr: socket, Network: "unix", DialTimeout: 50 * time.Millisecond, MaxRetries: -1, ContextTimeoutEnabled: true,
	})
	t.Cleanup(func() { client.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		if err := client.Ping(ctx).Err(); err == nil {
			return client
		}
		select {
		case <-ctx.Done():
			t.Fatalf("private Redis did not start: %v", ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
}

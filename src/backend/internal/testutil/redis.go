package testutil

import (
	"context"
	"github.com/redis/go-redis/v9"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Every test owns a separate Redis process and Unix socket; no developer data is touched.
func Redis(t *testing.T) *redis.Client {
	t.Helper()
	path, err := exec.LookPath("redis-server")
	if err != nil {
		t.Fatal("tests require redis-server (run in WSL or Linux)")
	}
	socket := filepath.Join(t.TempDir(), "redis.sock")
	cmd := exec.Command(path, "--port", "0", "--unixsocket", socket, "--save", "", "--appendonly", "no")
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	r := redis.NewClient(&redis.Options{Network: "unix", Addr: socket})
	t.Cleanup(func() { r.Close(); cmd.Process.Kill(); cmd.Wait() })
	for i := 0; i < 100; i++ {
		if r.Ping(context.Background()).Err() == nil {
			return r
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("Redis did not start")
	return nil
}

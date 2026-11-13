// RDAP proxy regression: run only in a socket-capable, offline test environment.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/fasthttp/websocket"
)

func TestRDAPProxyWebSocketFullProcess(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "typonamer")
	build := exec.Command("go", "build", "-trimpath", "-o", bin, ".")
	build.Dir = backendDir(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	binary, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, host, mode, want string
		proxyHits              int32
	}{
		{name: "MissingHost", mode: "ok", want: "未配置代理"},
		{name: "ProxyRefused", host: "127.0.0.1", mode: "closed", want: "代理连接失败"},
		{name: "ProxyHandshakeHang", host: "127.0.0.1", mode: "hang", want: "代理连接失败", proxyHits: 1},
		{name: "AuthenticationRejected", host: "127.0.0.1", mode: "auth", want: "代理连接失败", proxyHits: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rdapTrap, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { rdapTrap.Close() })
			var rdapHits atomic.Int32
			go func() {
				for {
					c, err := rdapTrap.Accept()
					if err != nil {
						return
					}
					rdapHits.Add(1)
					go func() {
						defer c.Close()
						// A direct RDAP fallback must hit this loopback trap.
						c.SetDeadline(time.Now().Add(5 * time.Second))
						io.Copy(io.Discard, c)
					}()
				}
			}()
			upstream := rdapTrap.Addr().String()
			port, hits := proxyFixture(t, tc.mode, upstream)
			if tc.mode == "closed" {
				closed, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				port = closed.Addr().(*net.TCPAddr).Port
				closed.Close()
			}
			redis, err := miniredis.Run()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(redis.Close)
			probe, err := net.Listen("tcp", ":8080")
			if err != nil {
				t.Fatalf("full-process test requires free port 8080: %v", err)
			}
			probe.Close()
			runDir := t.TempDir()
			// v0.9.1 uses HOME/.openrdap (not XDG). A fresh local-only
			// registry prevents IANA discovery and any real RDAP access.
			cacheDir := filepath.Join(runDir, ".openrdap")
			if err := os.MkdirAll(cacheDir, 0700); err != nil {
				t.Fatal(err)
			}
			registry := fmt.Sprintf(`{"version":"1.0","publication":"2026-11-13T00:00:00Z","services":[[["com"],["http://%s/"]]]}`, upstream)
			if err := os.WriteFile(filepath.Join(cacheDir, "dns.json"), []byte(registry), 0600); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if got := rdapHits.Load(); got != 0 {
					t.Errorf("direct RDAP trap hits=%d, want 0", got)
				}
				if got := hits.Load(); got != tc.proxyHits {
					t.Errorf("proxy hits=%d, want %d", got, tc.proxyHits)
				}
			})
			// config.init reads config.yaml beside the executable, not from cmd.Dir.
			runBin := filepath.Join(runDir, "typonamer")
			if err := os.WriteFile(runBin, binary, 0700); err != nil {
				t.Fatal(err)
			}
			cfg := fmt.Sprintf("LogLevel: Debug\nAuthUsername: admin\nAuthExpireDays: 1\nWhoisTimeout: 2\nDnsTimeout: 2\nRetryOnTimeout: false\nBulkCheckConcurrencyLimit: 1\nWebCheckConcurrencyLimit: 1\nWebCheckDomainLimit: 5\nSocketProxyHost: %q\nSocketProxyPort: %d\nSocketProxyAuth: %t\nSocketProxyUser: %s\nSocketProxyPassword: %s\n", tc.host, port, true, proxyTestUser, proxyTestPassword)
			if err := os.WriteFile(filepath.Join(runDir, "config.yaml"), []byte(cfg), 0600); err != nil {
				t.Fatal(err)
			}
			output, err := os.Create(filepath.Join(runDir, "process.log"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { output.Close() })
			// Register before Start so startup failures also include both logs.
			t.Cleanup(func() {
				if !t.Failed() {
					return
				}
				for _, path := range []string{output.Name(), filepath.Join(runDir, "logs", "typonamer.log")} {
					body, err := os.ReadFile(path)
					if err != nil {
						t.Logf("full-process log %s: %v", path, err)
						continue
					}
					t.Logf("full-process log %s:\n%s", path, body)
				}
			})
			cmd := exec.Command(runBin)
			cmd.Dir = runDir
			cmd.Env = startupEnv(map[string]string{"HOME": runDir, "XDG_CACHE_HOME": filepath.Join(runDir, "xdg-cache"), "POWERWHOIS_ADMIN_PASSWORD": testEnvPassword, "POWERWHOIS_JWT_SECRET": testEnvJWT, "REDIS_HOST": redis.Host(), "REDIS_PORT": redis.Port(), "REDIS_DB": "0"})
			for _, key := range []string{"GOEXPERIMENT", "GOFLAGS", "GOCACHE"} {
				if value, ok := os.LookupEnv(key); ok {
					cmd.Env = append(cmd.Env, key+"="+value)
				}
			}
			cmd.Stdout = output
			cmd.Stderr = output
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan struct{})
			var processErr error
			go func() { processErr = cmd.Wait(); close(done) }()
			t.Cleanup(func() {
				_ = cmd.Process.Kill()
				<-done
				for _, path := range []string{output.Name(), filepath.Join(runDir, "logs", "typonamer.log")} {
					body, err := os.ReadFile(path)
					if err != nil {
						t.Errorf("read log: %v", err)
						continue
					}
					for _, secret := range []string{proxyTestUser, proxyTestPassword} {
						if strings.Contains(string(body), secret) {
							t.Errorf("proxy credential leaked in %s", filepath.Base(path))
						}
					}
				}
			})
			assertAlive := func() {
				t.Helper()
				select {
				case <-done:
					t.Fatalf("typonamer exited unexpectedly (exit code %d): %v", cmd.ProcessState.ExitCode(), processErr)
				default:
				}
			}
			dialer := websocket.Dialer{HandshakeTimeout: time.Second}
			deadline := time.Now().Add(15 * time.Second)
			var conn *websocket.Conn
			for {
				assertAlive()
				conn, _, err = dialer.Dial("ws://127.0.0.1:8080/app/ws", nil)
				if err == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("WebSocket startup: %v", err)
				}
				select {
				case <-done:
					assertAlive()
				case <-time.After(50 * time.Millisecond):
				}
			}
			defer conn.Close()
			started := time.Now()
			const maxQueryDuration = 6 * time.Second // WhoisTimeout*2 + 2 seconds
			conn.SetReadDeadline(started.Add(maxQueryDuration))
			data := map[string]any{"queryType": "whoisQueryWithProxy", "domains": []string{"fixture.com"}}
			if err := conn.WriteJSON(map[string]any{"event": "webCheck", "data": data}); err != nil {
				t.Fatal(err)
			}
			for {
				_, raw, err := conn.ReadMessage()
				if err != nil {
					t.Fatal(err)
				}
				for _, secret := range []string{proxyTestUser, proxyTestPassword} {
					if strings.Contains(string(raw), secret) {
						t.Error("proxy credential leaked in WebSocket message")
					}
				}
				var msg struct {
					Event string                     `json:"event"`
					Data  map[string]json.RawMessage `json:"data"`
				}
				// Other events can carry arrays, so inspect the envelope first.
				var envelope struct {
					Event string `json:"event"`
				}
				if err := json.Unmarshal(raw, &envelope); err != nil {
					t.Fatal(err)
				}
				if envelope.Event != "webCheckResult" {
					continue
				}
				if err := json.Unmarshal(raw, &msg); err != nil {
					t.Fatal(err)
				}
				if elapsed := time.Since(started); elapsed > maxQueryDuration {
					t.Errorf("query took %v, maximum %v", elapsed, maxQueryDuration)
				}
				t.Logf("webCheckResult: %s", raw)
				for field, want := range map[string]string{"queryError": tc.want, "domain": "fixture.com", "lookupType": "rdap"} {
					var got string
					if err := json.Unmarshal(msg.Data[field], &got); err != nil || got != want {
						t.Errorf("%s=%s, want %q", field, msg.Data[field], want)
					}
				}
				if got := hits.Load(); got != tc.proxyHits {
					t.Errorf("configured proxy hits=%d, want %d", got, tc.proxyHits)
				}
				if got := rdapHits.Load(); got != 0 {
					t.Errorf("direct RDAP hits=%d, want %d", got, 0)
				}
				break
			}

		})
	}
}

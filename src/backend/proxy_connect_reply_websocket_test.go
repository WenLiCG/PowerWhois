// SOCKS CONNECT reply regressions: run only in a socket-capable, offline test environment.
package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/fasthttp/websocket"
)

func TestProxyConnectReplyWebSocketFullProcess(t *testing.T) {
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
	// Expected diagnostic fields are literals, independent of implementation mappings.
	for _, tc := range []struct {
		name, domain, lookupType, target, wantRep string
		reply                                     byte
	}{
		{"WHOIS_REP1", "fixture.cn", "whois", "whois.cnnic.cn:43", "rep=1", 1},
		{"WHOIS_REP2", "fixture.cn", "whois", "whois.cnnic.cn:43", "rep=2", 2},
		{"WHOIS_REP3", "fixture.cn", "whois", "whois.cnnic.cn:43", "rep=3", 3},
		{"WHOIS_REP4", "fixture.cn", "whois", "whois.cnnic.cn:43", "rep=4", 4},
		{"WHOIS_REP5", "fixture.cn", "whois", "whois.cnnic.cn:43", "rep=5", 5},
		{"WHOIS_REP6", "fixture.cn", "whois", "whois.cnnic.cn:43", "rep=6", 6},
		{"WHOIS_REP7", "fixture.cn", "whois", "whois.cnnic.cn:43", "rep=7", 7},
		{"WHOIS_REP8", "fixture.cn", "whois", "whois.cnnic.cn:43", "rep=8", 8},
		{"RDAP_REP1", "fixture.com", "rdap", "127.0.0.1:443", "rep=1", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// No upstream exists: rejection must complete before any body parsing.
			port, hits := proxyFixtureWithReply(t, "", "", tc.target, tc.reply)
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
			registry := `{"version":"1.0","publication":"2026-11-13T00:00:00Z","services":[[["com"],["https://127.0.0.1:443/"]]]}`
			if err := os.WriteFile(filepath.Join(cacheDir, "dns.json"), []byte(registry), 0600); err != nil {
				t.Fatal(err)
			}
			// config.init reads config.yaml beside the executable, not from cmd.Dir.
			runBin := filepath.Join(runDir, "typonamer")
			if err := os.WriteFile(runBin, binary, 0700); err != nil {
				t.Fatal(err)
			}
			cfg := fmt.Sprintf("LogLevel: Debug\nAuthUsername: admin\nAuthExpireDays: 1\nWhoisTimeout: 2\nDnsTimeout: 2\nRetryOnTimeout: false\nBulkCheckConcurrencyLimit: 1\nWebCheckConcurrencyLimit: 1\nWebCheckDomainLimit: 5\nSocketProxyHost: %q\nSocketProxyPort: %d\nSocketProxyAuth: %t\nSocketProxyUser: %s\nSocketProxyPassword: %s\n", "127.0.0.1", port, true, proxyTestUser, proxyTestPassword)
			if err := os.WriteFile(filepath.Join(runDir, "config.yaml"), []byte(cfg), 0600); err != nil {
				t.Fatal(err)
			}
			output, err := os.Create(filepath.Join(runDir, "process.log"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { output.Close() })
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
				if got := hits.Load(); got != 1 {
					t.Errorf("proxy hits=%d, want 1", got)
				}
				var logs strings.Builder
				for _, path := range []string{output.Name(), filepath.Join(runDir, "logs", "typonamer.log")} {
					body, err := os.ReadFile(path)
					if err != nil {
						t.Errorf("read log %s: %v", filepath.Base(path), err)
						continue
					}
					logs.Write(body)
					logs.WriteByte('\n')
				}
				text := logs.String()
				if !regexp.MustCompile(`\b` + regexp.QuoteMeta(tc.wantRep) + `($|[^[:alnum:]_])`).MatchString(text) {
					t.Errorf("logs missing literal %q", tc.wantRep)
				}
				for _, forbidden := range []string{
					"general SOCKS server failure", "connection not allowed by ruleset",
					"network unreachable", "host unreachable", "connection refused", "TTL expired",
					"command not supported", "address type not supported", "socks connect",
					proxyTestUser, proxyTestPassword,
				} {
					if strings.Contains(text, forbidden) {
						t.Error("logs contain forbidden proxy error text or test credentials")
					}
				}
				if tc.lookupType == "whois" {
					for _, marker := range []string{"Whois query raw result:", "Failed to parse WHOIS response", "panic:"} {
						if strings.Contains(text, marker) {
							t.Errorf("CONNECT rejection entered body handling: %s", marker)
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
			data := map[string]any{"queryType": "whoisQueryWithProxy", "domains": []string{tc.domain}}
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
				assertAlive() // Receipt plus liveness catches the .cn parser panic regression.
				expected := map[string]string{"queryError": "代理连接失败", "domain": tc.domain, "lookupType": tc.lookupType}
				for field, want := range expected {
					var got string
					if err := json.Unmarshal(msg.Data[field], &got); err != nil || got != want {
						t.Errorf("%s=%s, want %q", field, msg.Data[field], want)
					}
				}
				break
			}

		})
	}
}

package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/fasthttp/websocket"
)

// Exercise the real main (including carbon defaults), not the parser in the
// testing process. All RDAP discovery and responses are local fixtures.
func TestRDAPDatesWebSocketFullProcess(t *testing.T) {
	cases := []struct {
		name, domain, registration, expiration, created, expiry string
	}{
		{"Z", "z-date.com", "1995-08-14T04:00:00Z", "2027-08-13T04:00:00Z", "1995-08-14 04:00:00", "2027-08-13 04:00:00"},
		{"Offset", "offset.com", "1995-08-14T12:00:00+08:00", "2027-08-13T12:00:00+08:00", "1995-08-14 04:00:00", "2027-08-13 04:00:00"},
		{"Fractional", "frac.com", "1995-08-14T04:00:00.481Z", "2027-08-13T04:00:00.481Z", "1995-08-14 04:00:00", "2027-08-13 04:00:00"},
		{"Malformed", "bad-date.com", "not-a-date", "also-bad", "", ""},
		{"Missing", "no-date.com", "", "", "", ""},
	}
	probe, err := net.Listen("tcp", ":8080")
	if err != nil {
		t.Fatalf("full-process test requires free port 8080: %v", err)
	}
	probe.Close()
	// The real scheduler init connects to Redis before main can listen on 8080.
	// Keep Redis local and alive until after the child has been stopped.
	redis, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start local Redis fixture: %v", err)
	}
	t.Cleanup(redis.Close)
	var mu sync.Mutex
	hits := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, tc := range cases {
			if r.URL.Path != "/domain/"+tc.domain {
				continue
			}
			mu.Lock()
			hits[tc.domain]++
			mu.Unlock()
			events := []map[string]string{{"eventAction": "registration"}, {"eventAction": "expiration"}}
			if tc.registration != "" {
				events[0]["eventDate"] = tc.registration
				events[1]["eventDate"] = tc.expiration
			}
			w.Header().Set("Content-Type", "application/rdap+json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"objectClassName": "domain", "ldhName": tc.domain, "status": []string{"active"},
				"nameservers": []map[string]string{{"objectClassName": "nameserver", "ldhName": "ns1.fixture.invalid"}},
				"entities":    []map[string]any{{"objectClassName": "entity", "handle": "TEST-REGISTRAR", "roles": []string{"registrar"}}},
				"events":      events,
			})
			return
		}
		t.Errorf("unexpected local RDAP request: %s", r.URL.Path)
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	dir := t.TempDir()
	bin := filepath.Join(dir, "typonamer")
	build := exec.Command("go", "build", "-trimpath", "-o", bin, ".") // Match production; inherits GOEXPERIMENT/GOFLAGS/GOCACHE.
	build.Dir = backendDir(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	write := func(path string, body []byte) {
		t.Helper()
		if err := os.WriteFile(path, body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(dir, "config.yaml"), []byte(`LogLevel: Info
AuthUsername: admin
AuthExpireDays: 1
WhoisTimeout: 10
DnsTimeout: 10
RetryOnTimeout: false
RetryInterval: 1
RetryMax: 1
BulkCheckConcurrencyLimit: 1
WebCheckConcurrencyLimit: 1
WebCheckDomainLimit: 5
`))
	home := filepath.Join(dir, "home")
	cacheDir := filepath.Join(home, ".openrdap")
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		t.Fatal(err)
	}
	registry, err := json.Marshal(map[string]any{"version": "1.0", "publication": "2026-11-13T00:00:00Z", "services": []any{[]any{[]string{"com"}, []string{server.URL + "/"}}}})
	if err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(cacheDir, "dns.json"), registry) // Fresh mtime prevents IANA refresh.
	output, err := os.Create(filepath.Join(dir, "process.log"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { output.Close() })
	// Registered before Start so startup failures also include both log sources.
	// Cleanup runs after the child is reaped and before TempDir is removed.
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		for _, path := range []string{output.Name(), filepath.Join(dir, "logs", "typonamer.log")} {
			body, err := os.ReadFile(path)
			if err != nil {
				t.Logf("full-process log %s: %v", path, err)
				continue
			}
			t.Logf("full-process log %s:\n%s", path, body)
		}
	})
	cmd := exec.Command(bin)
	cmd.Dir = dir
	cmd.Env = startupEnv(map[string]string{
		"HOME": home, "POWERWHOIS_ADMIN_PASSWORD": testEnvPassword, "POWERWHOIS_JWT_SECRET": testEnvJWT,
		"REDIS_HOST": redis.Host(), "REDIS_PORT": redis.Port(), "REDIS_DB": "0",
	})
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
	})
	assertAlive := func(t *testing.T) {
		t.Helper()
		select {
		case <-done:
			t.Fatalf("typonamer exited unexpectedly: %v", processErr)
		default:
		}
	}
	dialer := websocket.Dialer{HandshakeTimeout: time.Second}
	deadline := time.Now().Add(15 * time.Second)
	for {
		assertAlive(t)
		conn, _, dialErr := dialer.Dial("ws://127.0.0.1:8080/app/ws", nil)
		if dialErr == nil {
			conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("WebSocket startup: %v", dialErr)
		}
		time.Sleep(50 * time.Millisecond)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertAlive(t)
			conn, _, err := dialer.Dial("ws://127.0.0.1:8080/app/ws", nil)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
			_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := conn.WriteJSON(map[string]any{"event": "webCheck", "data": map[string]any{"queryType": "whoisQuery", "domains": []string{tc.domain}}}); err != nil {
				t.Fatal(err)
			}
			var receivedMessages [][]byte
			t.Cleanup(func() {
				if t.Failed() {
					for i, raw := range receivedMessages {
						t.Logf("raw WebSocket message %d: %s", i+1, raw)
					}
				}
			})
			for {
				_, raw, err := conn.ReadMessage()
				if len(raw) > 0 {
					receivedMessages = append(receivedMessages, raw)
				}
				if err != nil {
					t.Fatalf("reading webCheckResult: %v", err)
				}
				var message struct {
					Event string          `json:"event"`
					Data  json.RawMessage `json:"data"`
				}
				if err := json.Unmarshal(raw, &message); err != nil {
					t.Fatalf("decoding WebSocket message: %v", err)
				}
				if message.Event != "webCheckResult" {
					continue
				}
				var data map[string]json.RawMessage
				if err := json.Unmarshal(message.Data, &data); err != nil {
					t.Fatalf("decoding webCheckResult data: %v", err)
				}
				mu.Lock()
				count := hits[tc.domain]
				mu.Unlock()
				if count == 0 {
					t.Fatal("local fake RDAP server was not queried")
				}
				for field, want := range map[string]string{"registerStatus": "Taken", "lookupType": "rdap", "createdDate": tc.created, "expiryDate": tc.expiry} {
					raw, exists := data[field]
					var got string
					if !exists || json.Unmarshal(raw, &got) != nil || strings.TrimSpace(string(raw)) == "null" {
						t.Errorf("webCheckResult missing string field %s: %s", field, raw)
						continue
					}
					if got != want {
						t.Errorf("webCheckResult.%s = %q, want %q", field, got, want)
					}
				}
				var nameservers []string
				if err := json.Unmarshal(data["nameServer"], &nameservers); err != nil || len(nameservers) != 1 || nameservers[0] != "ns1.fixture.invalid" {
					t.Errorf("webCheckResult.nameServer = %s, want [ns1.fixture.invalid]", data["nameServer"])
				}
				assertAlive(t)
				t.Logf("local RDAP hits=%d; received webCheckResult; process still running", count)
				break
			}
		})
	}
}

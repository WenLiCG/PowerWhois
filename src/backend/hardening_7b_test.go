package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/fasthttp/websocket"
)

func TestPW7bDebugProcessLogsOmitCredentialsAndFrames(t *testing.T) {
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
	write(filepath.Join(dir, "config.yaml"), []byte(`LogLevel: Debug
AuthUsername: pw7b-test-only-admin-user
AuthExpireDays: 1
WhoisTimeout: 10
DnsTimeout: 10
RetryOnTimeout: false
RetryInterval: 1
RetryMax: 1
BulkCheckConcurrencyLimit: 1
WebCheckConcurrencyLimit: 1
WebCheckDomainLimit: 5
SocketProxyUser: pw7b-test-only-proxy-user
SocketProxyPassword: pw7b-test-only-proxy-password
`))
	home := filepath.Join(dir, "home")
	cacheDir := filepath.Join(home, ".openrdap")
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		t.Fatal(err)
	}
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
		"APP_DEV": "true", "HOME": home, "POWERWHOIS_ADMIN_PASSWORD": testEnvPassword, "POWERWHOIS_JWT_SECRET": testEnvJWT,
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

	login, err := http.Post("http://127.0.0.1:8080/api/login", "application/json", strings.NewReader(`{"username":"pw7b-test-only-admin-user","password":"`+testEnvPassword+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	rawLogin, err := io.ReadAll(login.Body)
	login.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if login.StatusCode != 200 {
		t.Fatalf("login status=%d, want 200", login.StatusCode)
	}
	var result map[string]any
	if err := json.Unmarshal(rawLogin, &result); err != nil {
		t.Fatal(err)
	}
	// Login must actually succeed, not merely return an HTTP 200 error envelope.
	if token, ok := result["token"].(string); !ok || token == "" {
		t.Fatalf("login did not return token: %s", rawLogin)
	}
	conn, _, err := dialer.Dial("ws://127.0.0.1:8080/app/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	for _, frame := range []string{
		`{"event":"adminAuth","data":"pw7-test-only-ws-token-xyz"}`,
		`{"event":"webCheck","data":{"domains":[],"queryType":"whoisQuery","note":"pw7b-test-only-ordinary-message"}}`,
		`{"event":"ping","data":"pw7b-test-only-ping-payload"}`,
	} {
		if err := conn.WriteMessage(websocket.TextMessage, []byte(frame)); err != nil {
			t.Fatal(err)
		}
	}
	// Pong is a barrier proving the preceding frames were handled before reading logs.
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		var message struct {
			Event string `json:"event"`
		}
		if err := json.Unmarshal(raw, &message); err != nil {
			t.Fatal(err)
		}
		if message.Event == "pong" {
			break
		}
	}
	assertAlive(t)
	for _, path := range []string{output.Name(), filepath.Join(dir, "logs", "typonamer.log")} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(data, []byte("\tDEBUG\t")) {
			t.Errorf("%s missing Debug-level control record", path)
		}
		if !bytes.Contains(data, []byte("Admin auth message")) {
			t.Errorf("%s missing handled-auth log control", path)
		}
		for _, secret := range []string{"pw7-test-only-ws-token-xyz", "pw7b-test-only-ordinary-message", "pw7b-test-only-ping-payload", "pw7b-test-only-admin-user", testEnvPassword, "pw7b-test-only-proxy-user", "pw7b-test-only-proxy-password"} {
			if bytes.Contains(data, []byte(secret)) {
				t.Errorf("%s contains forbidden credential/frame marker %q", path, secret)
			}
		}
	}
}

func TestPW7bGitignoreBackendBinary(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(backendDir(t), "..", "..", ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, line := range strings.Split(string(data), "\n") {
		if line == "src/backend/typonamer" {
			found = true
		}
	}
	if !found {
		t.Fatal("root .gitignore must contain literal line src/backend/typonamer")
	}
}

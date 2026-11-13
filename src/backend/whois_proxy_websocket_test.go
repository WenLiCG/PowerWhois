package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/fasthttp/websocket"
)

const proxyTestUser = "pw7-test-only-proxy-user-xyz"
const proxyTestPassword = "pw7-test-only-proxy-pass-xyz"

// The SOCKS fixture never resolves the requested hostname: its only upstream is
// a loopback WHOIS fixture. cn is WHOIS-only, so no RDAP/IANA access is needed.
func proxyFixture(t *testing.T, mode, upstream string) (int, *atomic.Int32) {
	return proxyFixtureForTarget(t, mode, upstream, "whois.cnnic.cn:43")
}

func proxyFixtureForTarget(t *testing.T, mode, upstream, expectedTarget string) (int, *atomic.Int32) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	hits := new(atomic.Int32)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			hits.Add(1)
			go func() {
				defer c.Close()
				c.SetDeadline(time.Now().Add(5 * time.Second))
				if mode == "hang" {
					c.SetDeadline(time.Now().Add(30 * time.Second))
					t.Cleanup(func() { c.Close() })
					io.Copy(io.Discard, c)
					return
				}
				var h [2]byte
				if _, err := io.ReadFull(c, h[:]); err != nil {
					return
				}
				methods := make([]byte, int(h[1]))
				if _, err := io.ReadFull(c, methods); err != nil {
					return
				}
				if h[0] != 5 {
					t.Error("fixture received non-SOCKS5 greeting")
					return
				}
				if mode == "auth" {
					c.Write([]byte{5, 2})
					if _, err := io.ReadFull(c, h[:]); err != nil {
						return
					}
					user := make([]byte, int(h[1]))
					if _, err := io.ReadFull(c, user); err != nil {
						return
					}
					var n [1]byte
					if _, err := io.ReadFull(c, n[:]); err != nil {
						return
					}
					password := make([]byte, int(n[0]))
					if _, err := io.ReadFull(c, password); err != nil {
						return
					}
					if string(user) != proxyTestUser || string(password) != proxyTestPassword {
						t.Error("unexpected fixture credentials")
					}
					c.Write([]byte{1, 1})
					return
				}
				c.Write([]byte{5, 0})
				var req [4]byte
				if _, err := io.ReadFull(c, req[:]); err != nil {
					return
				}
				if req[0] != 5 || req[1] != 1 || req[2] != 0 {
					t.Errorf("unexpected SOCKS request header %v", req)
					return
				}
				var host string
				switch req[3] {
				case 1:
					var ip [4]byte
					if _, err := io.ReadFull(c, ip[:]); err != nil {
						return
					}
					host = net.IP(ip[:]).String()
				case 3:
					var n [1]byte
					if _, err := io.ReadFull(c, n[:]); err != nil {
						return
					}
					name := make([]byte, int(n[0]))
					if _, err := io.ReadFull(c, name); err != nil {
						return
					}
					host = string(name)
				default:
					t.Errorf("unexpected SOCKS address type %d", req[3])
					return
				}
				var port [2]byte
				if _, err := io.ReadFull(c, port[:]); err != nil {
					return
				}
				target := net.JoinHostPort(host, fmt.Sprint(binary.BigEndian.Uint16(port[:])))
				if target != expectedTarget {
					t.Errorf("unexpected SOCKS target %q, want %q", target, expectedTarget)
					return
				}
				// Even the failure case attempts only a closed loopback upstream.
				remote, err := net.DialTimeout("tcp", upstream, time.Second)
				if err != nil {
					c.Write([]byte{5, 5, 0, 1, 0, 0, 0, 0, 0, 0})
					return
				}
				defer remote.Close()
				c.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0})
				go io.Copy(remote, c)
				io.Copy(c, remote)
			}()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port, hits
}

func TestWhoisProxyWebSocketFullProcess(t *testing.T) {
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
		zero, override         bool
		proxyHits, whoisHits   int32
	}{
		{name: "MissingHost", mode: "ok", want: "未配置代理"},
		{name: "WhitespaceHost", host: " \t ", mode: "ok", want: "未配置代理"},
		{name: "ProxyHandshakeHang", host: "127.0.0.1", mode: "hang", want: "代理连接失败", proxyHits: 1},
		{name: "MissingPort", host: "127.0.0.1", mode: "ok", zero: true, want: "未配置代理"},
		{name: "ProxyRefused", host: "127.0.0.1", mode: "closed", want: "代理连接失败"},
		{name: "AuthenticationRejected", host: "127.0.0.1", mode: "auth", want: "代理连接失败", proxyHits: 1},
		{name: "ProxySuccess", host: "127.0.0.1", mode: "ok", proxyHits: 1, whoisHits: 1},
		{name: "WhoisRefused", host: "127.0.0.1", mode: "upstream-closed", want: "Whois查询超时", proxyHits: 1},
		{name: "RequestCannotOverrideConfig", host: "127.0.0.1", mode: "ok", override: true, proxyHits: 1, whoisHits: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			whois, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { whois.Close() })
			var whoisHits atomic.Int32
			go func() {
				for {
					c, err := whois.Accept()
					if err != nil {
						return
					}
					whoisHits.Add(1)
					go func() {
						defer c.Close()
						c.SetDeadline(time.Now().Add(5 * time.Second))
						line, err := bufio.NewReader(c).ReadString('\n')
						if err != nil {
							return
						}
						if line != "fixture.cn\r\n" {
							t.Errorf("WHOIS query=%q", line)
						}
						io.WriteString(c, "Domain Name: fixture.cn\r\nSponsoring Registrar: Fixture\r\nDomain Status: ok\r\nName Server: ns1.fixture.invalid\r\n")
					}()
				}
			}()
			upstream := whois.Addr().String()
			if tc.mode == "upstream-closed" {
				whois.Close()
			}
			port, hits := proxyFixture(t, tc.mode, upstream)
			if tc.mode == "closed" {
				closed, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				port = closed.Addr().(*net.TCPAddr).Port
				closed.Close()
			}
			if tc.zero {
				port = 0
			}
			decoyPort, decoyHits := proxyFixture(t, "ok", upstream)
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
			cmd.Env = startupEnv(map[string]string{"HOME": runDir, "POWERWHOIS_ADMIN_PASSWORD": testEnvPassword, "POWERWHOIS_JWT_SECRET": testEnvJWT, "REDIS_HOST": redis.Host(), "REDIS_PORT": redis.Port(), "REDIS_DB": "0"})
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
			data := map[string]any{"queryType": "whoisQueryWithProxy", "domains": []string{"fixture.cn"}}
			if tc.override {
				data["socketProxyHost"] = "127.0.0.1"
				data["socketProxyPort"] = decoyPort
				data["SocketProxyHost"] = "127.0.0.1"
				data["SocketProxyPort"] = decoyPort
				data["config"] = map[string]any{"socketProxyHost": "127.0.0.1", "socketProxyPort": decoyPort}
			}
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
				for field, want := range map[string]string{"queryError": tc.want, "domain": "fixture.cn", "lookupType": "whois"} {
					var got string
					if err := json.Unmarshal(msg.Data[field], &got); err != nil || got != want {
						t.Errorf("%s=%s, want %q", field, msg.Data[field], want)
					}
				}
				if tc.want == "" {
					if string(msg.Data["viaProxy"]) != "true" {
						t.Errorf("viaProxy=%s, want true", msg.Data["viaProxy"])
					}
					if string(msg.Data["registerStatus"]) != `"Taken"` {
						t.Errorf("registerStatus=%s, want Taken", msg.Data["registerStatus"])
					}
				}
				if got := hits.Load(); got != tc.proxyHits {
					t.Errorf("configured proxy hits=%d, want %d", got, tc.proxyHits)
				}
				if got := whoisHits.Load(); got != tc.whoisHits {
					t.Errorf("WHOIS hits=%d, want %d", got, tc.whoisHits)
				}
				if got := decoyHits.Load(); got != 0 {
					t.Errorf("request-supplied proxy hits=%d, want 0", got)
				}
				break
			}
			t.Logf("config proxy port=%s; WHOIS hits=%d; decoy hits=%d", strconv.Itoa(port), whoisHits.Load(), decoyHits.Load())
		})
	}
}

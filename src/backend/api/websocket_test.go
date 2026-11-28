package api

import (
	"encoding/json"
	ws "github.com/fasthttp/websocket"
	"github.com/gofiber/contrib/socketio"
	"github.com/gofiber/fiber/v2"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"typonamer/config"
)

func TestWebSocketAuthorizationAndLimits(t *testing.T) {
	t.Setenv("POWERWHOIS_ADMIN_USER", "ws-fixture")
	t.Setenv("POWERWHOIS_ADMIN_PASSWORD", "fixture-password-only")
	t.Setenv("POWERWHOIS_JWT_SECRET", strings.Repeat("x", 40))
	path := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(path, []byte("logLevel: Error\n"), 0600)
	if e := config.Load(path); e != nil {
		t.Fatal(e)
	}
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use("/app/ws", WebSocketUpgrade)
	app.Get("/app/ws", socketio.New(WebSocketHandler))
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { done <- app.Listener(listener) }()
	defer func() { app.Shutdown(); <-done }()
	address := "ws://" + listener.Addr().String() + "/app/ws"
	if conn, resp, e := ws.DefaultDialer.Dial(address, http.Header{"Origin": []string{"https://foreign.example"}}); e == nil {
		conn.Close()
		t.Fatal("foreign origin accepted")
	} else if resp == nil || resp.StatusCode != 403 {
		t.Fatal("origin check", e)
	}
	if conn, resp, e := ws.DefaultDialer.Dial(address, http.Header{"Origin": []string{"null"}}); e == nil {
		conn.Close()
		t.Fatal("null origin accepted")
	} else if resp == nil || resp.StatusCode != 403 {
		t.Fatal("null origin check", e)
	}
	conn, _, e := ws.DefaultDialer.Dial(address, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Close()
	send := func(event string, data any) {
		t.Helper()
		if e := conn.WriteJSON(map[string]any{"event": event, "data": data}); e != nil {
			t.Fatal(e)
		}
	}
	expect := func(event, text string) {
		t.Helper()
		conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		var m struct {
			Event string          `json:"event"`
			Data  json.RawMessage `json:"data"`
		}
		if e := conn.ReadJSON(&m); e != nil || m.Event != event || !strings.Contains(string(m.Data), text) {
			t.Fatalf("response %+v %v", m, e)
		}
	}
	send("register", Register{Domains: []string{"example.com"}, Confirmed: true})
	expect("registerError", "需要管理员")
	send("webCheck", WebCheck{QueryType: "whoisQueryWithProxy", Domains: []string{"example.com"}})
	expect("webCheckDomains", "example.com")
	expect("webCheckResult", "需要管理员登录")
	send("adminAuth", strings.Repeat("query-only-token", 4))
	send("register", Register{Confirmed: true})
	expect("registerError", "需要管理员")
	token, e := generateToken(config.GetConfig())
	if e != nil {
		t.Fatal(e)
	}
	send("adminAuth", token)
	send("register", Register{Domains: []string{"example.com"}})
	expect("registerError", "明确确认")
	send("webCheck", WebCheck{QueryType: "dnsQuery", Domains: []string{"https://example.com"}})
	expect("webCheckError", "格式错误")
	send("webCheck", WebCheck{QueryType: "dnsQuery", Domains: []string{"example.com", "bad name.com"}})
	expect("webCheckError", "第 2 项")
	send("webCheck", WebCheck{QueryType: "dnsQuery", Domains: make([]string, 501)})
	expect("webCheckError", "数量超过")
	// Swapping "aa" and selecting the source suffix only reproduce the source domain.
	send("typoCheck", TypoCheck{Domain: "aa.com", TypoType: []string{"reverseLetter"}, CcTlds: []string{"com"}, QueryType: "dnsQuery"})
	expect("typoCheckError", "没有生成")
	send("webCheckStop", nil)
	expect("webCheckStopped", "")
	send("ping", nil)
	expect("pong", "")
}

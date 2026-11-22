package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"typonamer/config"
	"typonamer/internal/testredis"
	"typonamer/lookup/lookupinfo"
	"typonamer/lookup/network"

	"github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

const testAdminPassword = "test-only-admission-password"

// admissionServer runs the real WebSocket handler and HTTP routes on a
// loopback listener. Loopback is a trusted proxy, so X-Forwarded-For sets the
// client address the way Caddy does in production.
func admissionServer(t *testing.T, client *redis.Client, extraConfig string) string {
	t.Helper()
	oldGen, oldStore, oldProxies := authGeneration.Load(), authStore.Load(), currentTrustedProxies()
	t.Cleanup(func() {
		authGeneration.Store(oldGen)
		authStore.Store(oldStore)
		SetTrustedProxies(oldProxies)
		dropTickets()
	})
	SetTrustedProxies(loopbackOnly)
	t.Setenv("POWERWHOIS_ADMIN_PASSWORD", testAdminPassword)
	t.Setenv("POWERWHOIS_JWT_SECRET", "test-only-admission-secret")
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("AuthUsername: admin\n"+extraConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.Load(path); err != nil {
		t.Fatal(err)
	}
	if client != nil {
		if err := InitAuth(context.Background(), client); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	upgrade, drain := WebSocketHandler(ctx, nil, client)
	app.Get("/app/ws", upgrade)
	app.Route("/api", func(router fiber.Router) { ApiRoute(router, nil, client) })
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go app.Listener(ln)
	t.Cleanup(func() {
		cancel()
		stop, done := context.WithTimeout(context.Background(), 3*time.Second)
		defer done()
		app.ShutdownWithContext(stop)
		drain(stop)
		ln.Close()
	})
	return ln.Addr().String()
}

func httpCall(t *testing.T, addr, method, path, token, body string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, "http://"+addr+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

func adminLogin(t *testing.T, addr string) string {
	t.Helper()
	code, raw := httpCall(t, addr, http.MethodPost, "/api/login", "", `{"username":"admin","password":"`+testAdminPassword+`"}`)
	var login struct{ Token string }
	if code != http.StatusOK || json.Unmarshal(raw, &login) != nil || login.Token == "" {
		t.Fatalf("login = HTTP %d", code)
	}
	return login.Token
}

func issueTicket(t *testing.T, addr, token string) string {
	t.Helper()
	code, raw := httpCall(t, addr, http.MethodPost, "/api/admin/ws-ticket", token, "")
	var ticket struct{ Ticket string }
	if code != http.StatusOK || json.Unmarshal(raw, &ticket) != nil || ticket.Ticket == "" {
		t.Fatalf("ticket = HTTP %d %s", code, raw)
	}
	return ticket.Ticket
}

type wsMessage struct {
	Event string
	Data  json.RawMessage
}

func dialWS(addr, forwardedFor, query string) (*websocket.Conn, error) {
	header := http.Header{}
	if forwardedFor != "" {
		header.Set("X-Forwarded-For", forwardedFor)
	}
	conn, _, err := websocket.DefaultDialer.Dial("ws://"+addr+"/app/ws"+query, header)
	if err != nil {
		return nil, err
	}
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	return conn, nil
}

// pingWS confirms the server accepted the session; a rejected connection
// receives "close 1013: server busy" instead of a pong.
func pingWS(conn *websocket.Conn) error {
	if err := conn.WriteJSON(map[string]any{"event": "ping"}); err != nil {
		return err
	}
	for {
		var message wsMessage
		if err := conn.ReadJSON(&message); err != nil {
			return err
		}
		if message.Event == "pong" {
			return nil
		}
	}
}

func readEvent(t *testing.T, conn *websocket.Conn, event string) wsMessage {
	t.Helper()
	for {
		var message wsMessage
		if err := conn.ReadJSON(&message); err != nil {
			t.Fatalf("waiting for %s: %v", event, err)
		}
		if message.Event == event {
			return message
		}
	}
}

// TestTicketAdmitsAdministratorWhilePublicPoolIsFull guards verification
// finding F1: anonymous clients fill every public slot before authenticating.
func TestTicketAdmitsAdministratorWhilePublicPoolIsFull(t *testing.T) {
	for _, scenario := range []struct {
		name      string
		anonymous func(i int) string
	}{
		{"four public addresses", func(i int) string { return fmt.Sprintf("203.0.113.%d", 1+i/connectionsPerIP) }},
		{"one direct private client", func(int) string { return "" }},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			addr := admissionServer(t, nil, "")
			var open []*websocket.Conn
			defer func() {
				for _, conn := range open {
					conn.Close()
				}
			}()
			for i := range publicConnections {
				conn, err := dialWS(addr, scenario.anonymous(i), "")
				if err != nil {
					t.Fatal(err)
				}
				open = append(open, conn)
				if err := pingWS(conn); err != nil {
					t.Fatalf("anonymous connection %d: %v", i+1, err)
				}
			}
			extra, err := dialWS(addr, "198.51.100.40", "")
			if err != nil {
				t.Fatal(err)
			}
			defer extra.Close()
			if err := pingWS(extra); err == nil {
				t.Fatal("the public pool accepted more than its limit")
			}

			token := adminLogin(t, addr)
			ticket := issueTicket(t, addr, token)
			admin, err := dialWS(addr, "198.51.100.40", "?ticket="+ticket)
			if err != nil {
				t.Fatal(err)
			}
			defer admin.Close()
			if result := readEvent(t, admin, "adminAuthResult"); string(result.Data) != "true" {
				t.Fatalf("reserved connection was not an administrator session: %s", result.Data)
			}
			if err := pingWS(admin); err != nil {
				t.Fatalf("administrator locked out while the public pool was full: %v", err)
			}

			// A ticket works once; reusing it falls back to the full public pool.
			replay, err := dialWS(addr, "198.51.100.41", "?ticket="+ticket)
			if err != nil {
				t.Fatal(err)
			}
			defer replay.Close()
			if err := pingWS(replay); err == nil {
				t.Fatal("a redeemed ticket admitted a second connection")
			}
		})
	}
}

func TestTicketsRequireAValidSessionAndExpireOnLogout(t *testing.T) {
	addr := admissionServer(t, nil, "")
	if code, _ := httpCall(t, addr, http.MethodPost, "/api/admin/ws-ticket", "", ""); code != http.StatusUnauthorized {
		t.Fatalf("anonymous ticket request = HTTP %d, want 401", code)
	}
	token := adminLogin(t, addr)
	unused := issueTicket(t, addr, token)
	used := issueTicket(t, addr, token)
	admin, err := dialWS(addr, "", "?ticket="+used)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if result := readEvent(t, admin, "adminAuthResult"); string(result.Data) != "true" {
		t.Fatal("ticket did not grant administrator access")
	}
	// silent stops reading after setup, so it never answers the server's close
	// frame; only the server can free its slot.
	silent, err := dialWS(addr, "", "?ticket="+issueTicket(t, addr, token))
	if err != nil {
		t.Fatal(err)
	}
	defer silent.Close()
	if err := pingWS(silent); err != nil {
		t.Fatal(err)
	}
	if code, _ := httpCall(t, addr, http.MethodPost, "/api/admin/logout", token, ""); code != http.StatusNoContent {
		t.Fatalf("logout = HTTP %d", code)
	}
	// The reserved connection is closed so its slot returns to administrators.
	admin.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		var message wsMessage
		err := admin.ReadJSON(&message)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				t.Fatal("the reserved connection stayed open after logout")
			}
			break
		}
		if message.Event == "adminAuthResult" && string(message.Data) != "false" {
			t.Fatalf("unexpected authorization after logout: %s", message.Data)
		}
	}
	if _, ok := redeemTicket(unused); ok {
		t.Fatal("a ticket issued before logout is still valid")
	}
	if _, ok := redeemTicket("never-issued"); ok {
		t.Fatal("an unknown ticket was accepted")
	}

	// The closed session must give its reserved slot back on the server, not
	// only appear closed to the client: all slots are usable again.
	token = adminLogin(t, addr)
	var reserved []*websocket.Conn
	defer func() {
		for _, conn := range reserved {
			conn.Close()
		}
	}()
	for i := range adminConnections {
		var lastErr error
		for attempt := 0; attempt < 30; attempt++ {
			conn, err := dialWS(addr, "", "?ticket="+issueTicket(t, addr, token))
			if err == nil {
				if err = pingWS(conn); err == nil {
					reserved = append(reserved, conn)
					break
				}
				conn.Close()
			}
			lastErr = err
			time.Sleep(100 * time.Millisecond)
		}
		if len(reserved) != i+1 {
			t.Fatalf("reserved slot %d was not released after logout: %v", i+1, lastErr)
		}
	}
	over, err := dialWS(addr, "", "?ticket="+issueTicket(t, addr, token))
	if err != nil {
		t.Fatal(err)
	}
	defer over.Close()
	if err := pingWS(over); err == nil {
		t.Fatalf("more than %d reserved connections were admitted", adminConnections)
	}
}

// TestShutdownClosesIdleSessions: closing a hijacked fasthttp connection is a
// no-op, so shutdown used to wait for each idle client's next message.
func TestShutdownClosesIdleSessions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	upgrade, drain := WebSocketHandler(ctx, nil, nil)
	app.Get("/app/ws", upgrade)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go app.Listener(ln)
	defer app.Shutdown()
	conn, err := dialWS(ln.Addr().String(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := pingWS(conn); err != nil {
		t.Fatal(err)
	}
	cancel()
	stop, done := context.WithTimeout(context.Background(), 2*time.Second)
	defer done()
	start := time.Now()
	if err := drain(stop); err != nil {
		t.Fatalf("drain did not finish with an idle session open: %v after %v", err, time.Since(start))
	}
}

// TestLogoutStopsUnsubmittedRegistrations guards verification finding F2.
func TestLogoutStopsUnsubmittedRegistrations(t *testing.T) {
	client := testredis.Start(t)
	firstArrived, releaseFirst := make(chan struct{}), make(chan struct{})
	var requests atomic.Int32
	var seen sync.Map
	registrar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Store(r.URL.Query().Get("domain"), true)
		if requests.Add(1) == 1 {
			close(firstArrived)
			<-releaseFirst
		}
		io.WriteString(w, "fixture-success")
	}))
	defer registrar.Close()
	defer func() {
		select {
		case <-releaseFirst:
		default:
			close(releaseFirst)
		}
	}()
	addr := admissionServer(t, client, "RegisterApis:\n - ApiName: fixture\n   ApiUrl: '"+registrar.URL+
		"/?domain={domain}'\n   SuccessText: [fixture-success]\n   FailText: [fixture-fail]\n   ConcurrencyLimit: 1\n")
	token := adminLogin(t, addr)
	conn, err := dialWS(addr, "203.0.113.9", "")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.WriteJSON(map[string]any{"event": "adminAuth", "data": token})
	if result := readEvent(t, conn, "adminAuthResult"); string(result.Data) != "true" {
		t.Fatal("administrator authentication failed")
	}
	const operation = "c773af68-facd-4250-bdab-c57eedb246fc"
	conn.WriteJSON(map[string]any{"event": "register", "data": map[string]any{
		"registerType": "fixture", "operationId": operation, "domains": []string{"example.com", "example.net"},
	}})
	select {
	case <-firstArrived:
	case <-time.After(3 * time.Second):
		t.Fatal("first registrar request did not arrive")
	}
	if code, _ := httpCall(t, addr, http.MethodPost, "/api/admin/logout", token, ""); code != http.StatusNoContent {
		t.Fatalf("logout = HTTP %d", code)
	}
	close(releaseFirst)

	ctx := context.Background()
	deadline := time.Now().Add(3 * time.Second)
	var stored string
	for time.Now().Before(deadline) {
		stored = client.HGet(ctx, "powerwhois:registration:example.com", operation).Val()
		if strings.Contains(stored, `"registerStatus":"success"`) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(stored, `"registerStatus":"success"`) {
		t.Fatalf("the outcome of the request already sent was not stored: %q", stored)
	}
	time.Sleep(300 * time.Millisecond) // give a wrongly started second request time to arrive
	if n := requests.Load(); n != 1 {
		t.Fatalf("registrar received %d requests; a domain not yet submitted was sent after logout", n)
	}
	if _, ok := seen.Load("example.net"); ok {
		t.Fatal("example.net reached the registrar after logout")
	}
	if client.Exists(ctx, "powerwhois:registration:example.net").Val() != 0 {
		t.Fatal("example.net was claimed although it was never submitted")
	}
}

// TestAnonymousLookupsUsePublicUpstreamShare guards verification finding F3 at
// the wiring level: anonymous web checks run with the public marker that
// network.Acquire uses to keep reserved upstream slots for administrators.
func TestAnonymousLookupsUsePublicUpstreamShare(t *testing.T) {
	var mu sync.Mutex
	public := map[string]bool{}
	previous := lookupQuery
	lookupQuery = func(ctx context.Context, domain, _ string, _ bool, _ config.Config) lookupinfo.QueryResult {
		mu.Lock()
		public[domain] = network.IsPublic(ctx)
		mu.Unlock()
		return lookupinfo.QueryResult{Domain: domain, RegisterStatus: "Taken"}
	}
	t.Cleanup(func() { lookupQuery = previous })
	addr := admissionServer(t, nil, "")

	anonymous, err := dialWS(addr, "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer anonymous.Close()
	anonymous.WriteJSON(map[string]any{"event": "webCheck", "data": map[string]any{"queryType": "whoisQuery", "domains": []string{"anonymous.com"}}})
	readEvent(t, anonymous, "webCheckDone")

	token := adminLogin(t, addr)
	admin, err := dialWS(addr, "", "?ticket="+issueTicket(t, addr, token))
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	readEvent(t, admin, "adminAuthResult")
	admin.WriteJSON(map[string]any{"event": "webCheck", "data": map[string]any{"queryType": "whoisQuery", "domains": []string{"admin.com"}}})
	readEvent(t, admin, "webCheckDone")

	mu.Lock()
	defer mu.Unlock()
	if !public["anonymous.com"] || public["admin.com"] {
		t.Fatalf("public marker = %v, want anonymous only", public)
	}
}

package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"typonamer/config"
	"typonamer/internal/testredis"
	"typonamer/lookup/lookuper"
	"typonamer/lookup/lookupinfo"
	"typonamer/lookup/network"

	"github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

// Independent checks of the de0fad6 fixes. They use the ticket and identity
// paths introduced by that commit. The previous probe dialed an administrator
// with no credential and occupied upstream slots without the anonymous marker;
// those calls no longer describe the fixed behavior.

func TestIndependentVerifyReservedPoolWhilePublicIsFull(t *testing.T) {
	for _, scenario := range []struct {
		name      string
		anonymous func(i int) string
	}{
		{"four public addresses", func(i int) string { return "203.0.113." + strconv.Itoa(1+i/connectionsPerIP) }},
		// No X-Forwarded-For: the TCP peer is the loopback listener, a real
		// local source, which is exempt from the per-address cap.
		{"direct loopback source", func(int) string { return "" }},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			addr := admissionServer(t, nil, "")
			var open []*websocket.Conn
			t.Cleanup(func() {
				for _, conn := range open {
					conn.Close()
				}
			})
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
				t.Fatal("public pool accepted a 65th anonymous connection")
			}

			token := adminLogin(t, addr)
			var admins []*websocket.Conn
			t.Cleanup(func() {
				for _, conn := range admins {
					conn.Close()
				}
			})
			for i := range adminConnections {
				conn, err := dialWS(addr, "198.51.100.40", "?ticket="+issueTicket(t, addr, token))
				if err != nil {
					t.Fatal(err)
				}
				admins = append(admins, conn)
				if result := readEvent(t, conn, "adminAuthResult"); string(result.Data) != "true" {
					t.Fatalf("reserved connection %d was not admitted: %s", i+1, result.Data)
				}
				if err := pingWS(conn); err != nil {
					t.Fatalf("reserved connection %d could not be used: %v", i+1, err)
				}
			}
			ninth, err := dialWS(addr, "198.51.100.40", "?ticket="+issueTicket(t, addr, token))
			if err != nil {
				t.Fatal(err)
			}
			defer ninth.Close()
			if adminGranted(t, ninth) {
				t.Fatal("a 9th ticket was admitted into the reserved pool")
			}
			if err := pingWS(ninth); err == nil {
				t.Fatal("9th connection was accepted while both pools were full")
			}
		})
	}
}

func TestIndependentVerifyTicketReuseExpiryAndLogout(t *testing.T) {
	addr := admissionServer(t, nil, "")
	token := adminLogin(t, addr)
	code, raw := httpCall(t, addr, http.MethodPost, "/api/admin/ws-ticket", token, "")
	var issued struct {
		Ticket    string `json:"ticket"`
		ExpiresIn int    `json:"expiresIn"`
	}
	if code != http.StatusOK || json.Unmarshal(raw, &issued) != nil || issued.Ticket == "" || issued.ExpiresIn != int(ticketTTL/time.Second) {
		t.Fatalf("ticket response = %d %s", code, raw)
	}
	used, err := dialWS(addr, "", "?ticket="+issued.Ticket)
	if err != nil {
		t.Fatal(err)
	}
	defer used.Close()
	if result := readEvent(t, used, "adminAuthResult"); string(result.Data) != "true" {
		t.Fatal("first use did not admit an administrator")
	}
	replay, err := dialWS(addr, "", "?ticket="+issued.Ticket)
	if err != nil {
		t.Fatal(err)
	}
	defer replay.Close()
	if adminGranted(t, replay) {
		t.Fatal("reused ticket granted administrator admission")
	}

	unused := issueTicket(t, addr, token)
	if code, _ := httpCall(t, addr, http.MethodPost, "/api/admin/logout", token, ""); code != http.StatusNoContent {
		t.Fatalf("logout = %d", code)
	}
	revoked, err := dialWS(addr, "", "?ticket="+unused)
	if err != nil {
		t.Fatal(err)
	}
	defer revoked.Close()
	if adminGranted(t, revoked) {
		t.Fatal("ticket issued before logout still granted administrator admission")
	}

	token = adminLogin(t, addr)
	stale := issueTicket(t, addr, token)
	time.Sleep(ticketTTL + time.Second)
	expired, err := dialWS(addr, "", "?ticket="+stale)
	if err != nil {
		t.Fatal(err)
	}
	defer expired.Close()
	if adminGranted(t, expired) {
		t.Fatal("expired ticket granted administrator admission")
	}
	fresh, err := dialWS(addr, "", "?ticket="+issueTicket(t, addr, token))
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	if result := readEvent(t, fresh, "adminAuthResult"); string(result.Data) != "true" {
		t.Fatal("a newly issued ticket was rejected")
	}
}

func TestIndependentVerifyLogoutKeepsSentRegistrationOnly(t *testing.T) {
	client := testredis.Start(t)
	first := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	var domains sync.Map
	registrar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		domains.Store(r.URL.Query().Get("domain"), r.Header.Get("Idempotency-Key"))
		if calls.Add(1) == 1 {
			close(first)
			<-release
		}
		_, _ = io.WriteString(w, "fixture-success")
	}))
	defer registrar.Close()
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	addr := admissionServer(t, client, "RegisterApis:\n - ApiName: fixture\n   ApiUrl: '"+registrar.URL+
		"/?domain={domain}'\n   SuccessText: [fixture-success]\n   FailText: [fixture-fail]\n   ConcurrencyLimit: 1\n")
	token := adminLogin(t, addr)
	conn, err := dialWS(addr, "203.0.113.9", "?ticket="+issueTicket(t, addr, token))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if result := readEvent(t, conn, "adminAuthResult"); string(result.Data) != "true" {
		t.Fatal("ticket did not admit the registering session")
	}
	const operation = "c773af68-facd-4250-bdab-c57eedb246fc"
	if err := conn.WriteJSON(map[string]any{"event": "register", "data": map[string]any{
		"registerType": "fixture", "operationId": operation, "domains": []string{"example.com", "example.net"},
	}}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-first:
	case <-time.After(3 * time.Second):
		t.Fatal("mock registrar did not receive the first domain")
	}
	if code, _ := httpCall(t, addr, http.MethodPost, "/api/admin/logout", token, ""); code != http.StatusNoContent {
		t.Fatal("logout did not return 204")
	}
	close(release)
	stored := waitRedis(t, client, "powerwhois:registration:example.com", operation, `"registerStatus":"success"`)
	if !strings.Contains(stored, `"registerStatus":"success"`) {
		t.Fatalf("outcome of the request already sent was not stored: %q", stored)
	}
	if key, _ := domains.Load("example.com"); key != operation+":example.com" {
		t.Fatalf("idempotency key = %v", key)
	}
	time.Sleep(400 * time.Millisecond)
	if calls.Load() != 1 {
		t.Fatalf("registrar received %d requests after logout", calls.Load())
	}
	if _, ok := domains.Load("example.net"); ok {
		t.Fatal("example.net was submitted after logout")
	}
	if client.Exists(context.Background(), "powerwhois:registration:example.net").Val() != 0 {
		t.Fatal("example.net was claimed although it was not submitted")
	}
}

func TestIndependentVerifyAnonymousMarkerAndUpstreamShare(t *testing.T) {
	var hits atomic.Int32
	hold := make(chan struct{})
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		<-hold
		_, _ = io.WriteString(w, "TAKEN")
	}))
	defer provider.Close()
	defer close(hold)
	t.Setenv("POWERWHOIS_ADMIN_PASSWORD", testAdminPassword)
	t.Setenv("POWERWHOIS_JWT_SECRET", "test-only-admission-secret")
	t.Setenv("POWERWHOIS_API_KEYS", "independent-api-fixture-key-00001")
	path := t.TempDir() + "/config.yaml"
	yaml := "AuthUsername: admin\nWhoisApis:\n - ApiName: fixturelookup\n   ApiUrl: '" + provider.URL +
		"/?domain={domain}'\n   FreeText: [FREE]\n   TakenText: [TAKEN]\n   ConcurrencyLimit: 4\n"
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.Load(path); err != nil {
		t.Fatal(err)
	}
	cfg := config.GetConfig()
	const limit = 4
	public := network.WithPublic(context.Background())
	for range network.PublicLimit(limit) {
		go lookuper.Query(public, "example.com", "fixturelookup", true, cfg)
	}
	waitHits(t, &hits, int32(network.PublicLimit(limit)))

	blocked := make(chan struct{})
	go func() {
		ctx, cancel := context.WithTimeout(public, 200*time.Millisecond)
		defer cancel()
		lookuper.Query(ctx, "example.net", "fixturelookup", true, cfg)
		close(blocked)
	}()
	select {
	case <-blocked:
	case <-time.After(time.Second):
		t.Fatal("anonymous lookup past the public share did not return")
	}
	if got := hits.Load(); got != int32(network.PublicLimit(limit)) {
		t.Fatalf("anonymous work reached the provider %d times, public share is %d", got, network.PublicLimit(limit))
	}

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Route("/api", func(router fiber.Router) { ApiRoute(router, nil, nil) })
	go func() {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/check?domain=example.org&queryType=fixturelookup", nil)
		req.Header.Set("X-API-Key", "independent-api-fixture-key-00001")
		resp, err := app.Test(req, 5000)
		if err == nil {
			resp.Body.Close()
		}
	}()
	waitHits(t, &hits, int32(network.PublicLimit(limit))+1)
	if hits.Load() > limit {
		t.Fatalf("provider saw %d calls while holders were blocked, limit is %d", hits.Load(), limit)
	}

	previous := lookupQuery
	var mu sync.Mutex
	seen := map[string]bool{}
	lookupQuery = func(ctx context.Context, domain, _ string, _ bool, _ config.Config) lookupinfo.QueryResult {
		mu.Lock()
		seen[domain] = network.IsPublic(ctx)
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
	if !seen["anonymous.com"] || seen["admin.com"] {
		t.Fatalf("public marker on the WebSocket lookup path = %v", seen)
	}
}

func TestIndependentVerifyAPIKeyLimitIgnoresInvalidKeys(t *testing.T) {
	t.Setenv("POWERWHOIS_API_KEYS", "independent-api-fixture-key-00001")
	t.Cleanup(func() { setAPIKeys("") })
	t.Setenv("POWERWHOIS_ADMIN_PASSWORD", testAdminPassword)
	t.Setenv("POWERWHOIS_JWT_SECRET", "test-only-admission-secret")
	path := t.TempDir() + "/config.yaml"
	if err := os.WriteFile(path, []byte("AuthUsername: admin\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.Load(path); err != nil {
		t.Fatal(err)
	}
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Route("/api", func(router fiber.Router) { ApiRoute(router, nil, nil) })
	call := func(key, query string) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/check?"+query, nil)
		if key != "" {
			req.Header.Set("X-API-Key", key)
		}
		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	const valid = "independent-api-fixture-key-00001"
	if got := call("", "domain=bad..name&api_key="+valid); got != http.StatusUnauthorized {
		t.Fatalf("key in the query string was accepted: %d", got)
	}
	for i := range 5 {
		if got := call("wrong-key-not-configured-000", "domain=bad..name"); got != http.StatusUnauthorized {
			t.Fatalf("invalid key %d = %d", i, got)
		}
	}
	for i := 1; i <= 600; i++ {
		if got := call(valid, "domain=bad..name"); got != http.StatusBadRequest {
			t.Fatalf("valid key request %d = %d", i, got)
		}
	}
	if got := call(valid, "domain=bad..name"); got != http.StatusTooManyRequests {
		t.Fatalf("request 601 = %d, want 429", got)
	}
	if got := call("wrong-key-not-configured-000", "domain=bad..name"); got != http.StatusUnauthorized {
		t.Fatalf("invalid key after the valid key was limited = %d", got)
	}
}

// adminGranted reports whether the server pushed administrator admission
// before the client sent any message. An invalid ticket falls back to an
// ordinary connection and must not do so.
func adminGranted(t *testing.T, conn *websocket.Conn) bool {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	var message wsMessage
	if err := conn.ReadJSON(&message); err != nil {
		return false
	}
	return message.Event == "adminAuthResult" && string(message.Data) == "true"
}

func waitHits(t *testing.T, hits *atomic.Int32, want int32) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for hits.Load() < want {
		if time.Now().After(deadline) {
			t.Fatalf("provider hits = %d, want %d", hits.Load(), want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitRedis(t *testing.T, client *redis.Client, key, field, needle string) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		stored := client.HGet(context.Background(), key, field).Val()
		if strings.Contains(stored, needle) {
			return stored
		}
		time.Sleep(20 * time.Millisecond)
	}
	return client.HGet(context.Background(), key, field).Val()
}

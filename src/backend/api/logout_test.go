package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"typonamer/config"
	"typonamer/internal/testutil"
	"typonamer/register"

	ws "github.com/fasthttp/websocket"
	"github.com/gofiber/contrib/socketio"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

func TestLogoutStopsQueuedRegistrationsAndKeepsInFlightResult(t *testing.T) {
	ctx := context.Background()
	redis := testutil.Redis(t)
	if err := InitAuth(ctx, redis); err != nil {
		t.Fatal(err)
	}
	register.SetAuditStore(redis)
	t.Cleanup(func() {
		authStore.Store(nil)
		authGeneration.Store(0)
		register.SetAuditStore(nil)
	})

	reached := make(chan string, 2)
	finish := make(chan struct{})
	registrar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		domain := r.URL.Query().Get("domain")
		reached <- domain
		if domain == "example.com" {
			<-finish
		}
		w.Write([]byte("SUCCESS"))
	}))
	defer registrar.Close()
	release := func() {
		select {
		case <-finish:
		default:
			close(finish)
		}
	}
	defer release()

	t.Setenv("POWERWHOIS_ADMIN_USER", "logout-fixture")
	t.Setenv("POWERWHOIS_ADMIN_PASSWORD", "test-fixture-only")
	t.Setenv("POWERWHOIS_JWT_SECRET", strings.Repeat("fixture", 8))
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("logLevel: Error\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := config.Load(path); err != nil {
		t.Fatal(err)
	}
	cfg := config.GetConfig()
	cfg.RegisterApis = []config.RegisterApi{{
		ApiName: "fixture", ApiUrl: registrar.URL + "/?domain={domain}",
		SuccessText: []string{"SUCCESS"}, FailText: []string{"FAILED"}, ConcurrencyLimit: 1,
	}}
	if err := config.UpdateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	register.SetupLimiter()

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Route("/api", ApiRoute)
	app.Use("/app/ws", WebSocketUpgrade)
	app.Get("/app/ws", socketio.New(WebSocketHandler))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- app.Listener(listener) }()
	defer func() { app.Shutdown(); <-done }()
	addr := listener.Addr().String()
	conn, _, err := ws.DefaultDialer.Dial("ws://"+addr+"/app/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"name": cfg.AuthUsername,
		"exp":  time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte(cfg.JwtSecretKey))
	if err != nil {
		t.Fatal(err)
	}
	send := func(event string, data any) {
		t.Helper()
		if err := conn.WriteJSON(map[string]any{"event": event, "data": data}); err != nil {
			t.Fatal(err)
		}
	}
	send("adminAuth", token)
	send("register", Register{RegisterType: "fixture", Domains: []string{"example.com", "example.net"}, Confirmed: true})
	select {
	case domain := <-reached:
		if domain != "example.com" {
			t.Fatalf("unexpected first registration: %s", domain)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("first domain did not reach the mock registrar")
	}

	call := func(method, path string) int {
		t.Helper()
		req, err := http.NewRequest(method, "http://"+addr+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		client := &http.Client{Timeout: 4 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}
	if got := call(http.MethodPost, "/api/admin/logout"); got != http.StatusNoContent {
		t.Fatalf("logout status %d", got)
	}
	if got := call(http.MethodGet, "/api/admin/setting"); got != http.StatusUnauthorized {
		t.Fatalf("revoked JWT allowed HTTP access: %d", got)
	}
	release()
	conn.SetReadDeadline(time.Now().Add(4 * time.Second))
	for i, want := range []string{"success", "failed"} {
		var message struct {
			Event string
			Data  register.RegisterInfo
		}
		if err := conn.ReadJSON(&message); err != nil || message.Event != "registerResult" || message.Data.RegisterStatus != want {
			t.Fatalf("registration %d: %+v, %v", i, message, err)
		}
		if i == 0 {
			entry := redis.Get(ctx, "powerwhois:registration:"+message.Data.AuditID).Val()
			var record map[string]string
			if err := json.Unmarshal([]byte(entry), &record); err != nil || record["status"] != "success" {
				t.Fatalf("in-flight registration outcome not saved: %q, %v", entry, err)
			}
		} else if message.Data.AuditID != "" {
			t.Fatal("queued domain acquired an audit ID after logout")
		}
	}
	select {
	case domain := <-reached:
		t.Fatalf("unexpected registration after logout: %s", domain)
	default:
	}

	authGeneration.Store(0)
	if err := InitAuth(ctx, redis); err != nil || validAdminToken(token) {
		t.Fatalf("logout did not survive reload: %v", err)
	}
	newToken, err := generateToken(cfg)
	if err != nil || !validAdminToken(newToken) {
		t.Fatalf("new administrator token rejected: %v", err)
	}
}

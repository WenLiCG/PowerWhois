package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"typonamer/config"

	"github.com/gofiber/fiber/v2"
)

func TestLimitedIPExemptsLocalAddresses(t *testing.T) {
	for ip, want := range map[string]bool{
		"203.0.113.9": true, "2001:db8::1": true,
		"127.0.0.1": false, "10.1.2.3": false, "172.18.0.4": false, "192.168.1.1": false,
		"::1": false, "fd00::1": false, "::ffff:127.0.0.1": false, "": false, "not-an-ip": false,
	} {
		if got := limitedIP(ip); got != want {
			t.Errorf("limitedIP(%q) = %v, want %v", ip, got, want)
		}
	}
}

func TestDomainBudgetRefillsPerMinute(t *testing.T) {
	var budget domainBudget
	now := time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)
	if !budget.take("203.0.113.1", 100, 100, now) {
		t.Fatal("a full bucket rejected one full-size request")
	}
	if budget.take("203.0.113.1", 1, 100, now) {
		t.Fatal("an empty bucket accepted another request")
	}
	if !budget.take("203.0.113.2", 1, 100, now) {
		t.Fatal("one client's usage affected another client")
	}
	if !budget.take("203.0.113.1", 50, 100, now.Add(30*time.Second)) {
		t.Fatal("half a minute should refill half the budget")
	}
	// A batch larger than the budget is admitted once from a full bucket, then delays the next one.
	if !budget.take("203.0.113.3", 500, 100, now) || budget.take("203.0.113.3", 1, 100, now.Add(time.Minute)) {
		t.Fatal("oversized batch accounting is wrong")
	}
}

func TestConnectionCounter(t *testing.T) {
	var counter connectionCounter
	if !counter.acquire("a", 2) || !counter.acquire("a", 2) || counter.acquire("a", 2) {
		t.Fatal("limit not enforced")
	}
	counter.release("a")
	if !counter.acquire("a", 2) {
		t.Fatal("release did not free a slot")
	}
	counter.release("a")
	counter.release("a")
	if len(counter.counts) != 0 {
		t.Fatal("released addresses were not removed")
	}
}

func TestLogoutRevokesIssuedTokens(t *testing.T) {
	previous := authGeneration.Load()
	t.Cleanup(func() { authGeneration.Store(previous) })
	t.Setenv("POWERWHOIS_ADMIN_PASSWORD", "test-only-session-password")
	t.Setenv("POWERWHOIS_JWT_SECRET", "test-only-session-secret")
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("AuthUsername: admin\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.Load(path); err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	app.Route("/api", func(router fiber.Router) { ApiRoute(router, nil, nil) })
	call := func(method, target, token, body string) (int, []byte) {
		t.Helper()
		req := httptest.NewRequest(method, target, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, raw
	}
	if code, _ := call(http.MethodPost, "/api/login", "", `{"username":"admin","password":"wrong"}`); code != http.StatusUnauthorized {
		t.Fatalf("wrong password = HTTP %d, want 401", code)
	}
	code, raw := call(http.MethodPost, "/api/login", "", `{"username":"admin","password":"test-only-session-password"}`)
	var login struct{ Token string }
	if code != http.StatusOK || json.Unmarshal(raw, &login) != nil || login.Token == "" {
		t.Fatalf("login = HTTP %d", code)
	}
	if code, _ := call(http.MethodGet, "/api/admin/setting", login.Token, ""); code != http.StatusOK {
		t.Fatalf("fresh token = HTTP %d", code)
	}
	if code, _ := call(http.MethodPost, "/api/admin/logout", login.Token, ""); code != http.StatusNoContent {
		t.Fatalf("logout = HTTP %d", code)
	}
	if code, _ := call(http.MethodGet, "/api/admin/setting", login.Token, ""); code != http.StatusUnauthorized {
		t.Fatalf("revoked token = HTTP %d, want 401", code)
	}
	code, raw = call(http.MethodPost, "/api/login", "", `{"username":"admin","password":"test-only-session-password"}`)
	if code != http.StatusOK || json.Unmarshal(raw, &login) != nil {
		t.Fatal("could not sign in after logout")
	}
	if code, _ := call(http.MethodGet, "/api/admin/setting", login.Token, ""); code != http.StatusOK {
		t.Fatalf("token issued after logout = HTTP %d", code)
	}
}

func TestIntegrationAPIRequiresConfiguredKey(t *testing.T) {
	t.Setenv("POWERWHOIS_API_KEYS", "")
	t.Cleanup(func() { setAPIKeys("") })
	app := fiber.New()
	app.Route("/api", func(router fiber.Router) { ApiRoute(router, nil, nil) })
	status := func(key string) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/check?domain=bad..name", nil)
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
	if code := status("anything"); code != http.StatusNotFound {
		t.Fatalf("API without configured keys = HTTP %d, want 404", code)
	}
	setAPIKeys("short, test-only-integration-key-000001")
	if code := status("short"); code != http.StatusUnauthorized {
		t.Fatalf("short key was accepted: HTTP %d", code)
	}
	if code := status("test-only-integration-key-000002"); code != http.StatusUnauthorized {
		t.Fatalf("wrong key = HTTP %d, want 401", code)
	}
	// A valid key reaches the handler, which rejects the malformed domain without a lookup.
	if code := status("test-only-integration-key-000001"); code != http.StatusBadRequest {
		t.Fatalf("valid key with invalid domain = HTTP %d, want 400", code)
	}
}

package api

import (
	"bytes"
	"encoding/json"
	"github.com/gofiber/fiber/v2"
	"gopkg.in/yaml.v3"
	"io"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"typonamer/config"
)

func TestWebPasswordChangeSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("authPassword: 123456\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("POWERWHOIS_ADMIN_USER", "fixture")
	t.Setenv("POWERWHOIS_ADMIN_PASSWORD", "old-environment-password")
	t.Setenv("POWERWHOIS_JWT_SECRET", strings.Repeat("fixture!", 8))
	if err := config.Load(path); err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	app.Route("/api", ApiRoute)
	request := func(method, url, token string, payload any, want int) []byte {
		t.Helper()
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, url, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := app.Test(req, 5000)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != want {
			t.Fatalf("%s %s: status %d, want %d", method, url, resp.StatusCode, want)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return body
	}
	login := func(password string, want int) string {
		t.Helper()
		body := request("POST", "/api/login", "", LoginInfo{Username: "fixture", Password: password}, want)
		if want != 200 {
			return ""
		}
		var result struct{ Token string }
		if err := json.Unmarshal(body, &result); err != nil || result.Token == "" {
			t.Fatal("missing login token")
		}
		return result.Token
	}
	login("123456", 400)
	old := login("old-environment-password", 200)
	authenticatedBeforeChange := config.GetConfig()
	fields := config.PublicSettings()
	fields["secretActions"] = map[string]string{"authPassword": "set"}
	fields["authPassword"] = " \t "
	request("PUT", "/api/admin/setting", old, fields, 400)
	fields["secretActions"] = map[string]string{"authPassword": "clear"}
	request("PUT", "/api/admin/setting", old, fields, 400)
	fields["secretActions"] = map[string]string{"authPassword": "set"}
	fields["authPassword"] = "123456"
	body := request("PUT", "/api/admin/setting", old, fields, 200)
	if bytes.Contains(body, []byte("123456")) || bytes.Contains(body, []byte("authPasswordHash")) || bytes.Contains(body, []byte("old-environment-password")) {
		t.Fatal("password change disclosed credentials")
	}
	newToken := login("123456", 200)
	request("GET", "/api/admin/setting", old, nil, 401)
	lateToken, err := generateToken(authenticatedBeforeChange)
	if err != nil {
		t.Fatal(err)
	}
	request("GET", "/api/admin/setting", lateToken, nil, 401)
	login("old-environment-password", 400)
	saved, err := os.ReadFile(path)
	var disk struct {
		Password string `yaml:"authPassword"`
		Hash     string `yaml:"authPasswordHash"`
	}
	if err != nil || yaml.Unmarshal(saved, &disk) != nil || disk.Password != "" || disk.Hash == "" || bytes.Contains(saved, []byte("old-environment-password")) {
		t.Fatal("new credential not stored privately")
	}
	t.Setenv("POWERWHOIS_ADMIN_PASSWORD", "changed-bootstrap-password")
	if err := config.Load(path); err != nil {
		t.Fatal(err)
	}
	request("PUT", "/api/admin/setting", newToken, config.PublicSettings(), 200)
	if err := config.Load(path); err != nil {
		t.Fatal(err)
	}
	login("changed-bootstrap-password", 400)
	login("old-environment-password", 400)
	login("123456", 200)
	request("GET", "/api/admin/setting", old, nil, 401)
	request("GET", "/api/admin/setting", newToken, nil, 200)
}

func TestAdminSettingsNeverRoundTripCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(path, []byte("logLevel: Error\n"), 0600)
	t.Setenv("POWERWHOIS_ADMIN_USER", "fixture")
	t.Setenv("POWERWHOIS_ADMIN_PASSWORD", "fixture-password-strong")
	t.Setenv("POWERWHOIS_JWT_SECRET", strings.Repeat("fixture!", 8))
	if e := config.Load(path); e != nil {
		t.Fatal(e)
	}
	cfg := config.GetConfig()
	cfg.SocketProxyPassword = "proxy-fixture"
	cfg.WhoisApis = []config.WhoisApi{{ApiName: "fixture", ApiUrl: "https://example.com/?secret=private-fixture&domain={domain}", FreeText: []string{"missing"}, TakenText: []string{"exists"}, ConcurrencyLimit: 1}}
	if e := config.UpdateConfig(cfg); e != nil {
		t.Fatal(e)
	}
	token, e := generateToken(cfg)
	if e != nil {
		t.Fatal(e)
	}
	app := fiber.New()
	app.Route("/api", ApiRoute)
	call := func(method string, body []byte) []byte {
		req := httptest.NewRequest(method, "/api/admin/setting", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		response, e := app.Test(req)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		data, _ := io.ReadAll(response.Body)
		if response.StatusCode != 200 {
			t.Fatalf("settings failed %d %s", response.StatusCode, data)
		}
		for _, secret := range []string{cfg.AuthPassword, cfg.JwtSecretKey, cfg.SocketProxyPassword, "private-fixture"} {
			if strings.Contains(string(data), secret) {
				t.Fatal("settings exposed secret")
			}
		}
		return data
	}
	public := call("GET", nil)
	var fields map[string]interface{}
	json.Unmarshal(public, &fields)
	fields["secretActions"] = map[string]string{"authPassword": "keep", "jwtSecretKey": "keep", "socketProxyPassword": "keep"}
	fields["dnsRate"] = 7
	body, _ := json.Marshal(fields)
	call("PUT", body)
	actual := config.GetConfig()
	if actual.DnsRate != 7 || actual.AuthPassword != cfg.AuthPassword || actual.SocketProxyPassword != cfg.SocketProxyPassword || actual.WhoisApis[0].ApiUrl != cfg.WhoisApis[0].ApiUrl {
		t.Fatal("redacted update lost private configuration")
	}

	// Verify /api/admin/proxy/test requires auth, rejects http:// host, and classifies non-SOCKS5 HTTP port without leaking secrets.
	unauthReq := httptest.NewRequest("POST", "/api/admin/proxy/test", nil)
	if resp, err := app.Test(unauthReq); err != nil || resp.StatusCode != 401 {
		t.Fatalf("unauthenticated proxy test allowed: %v %v", resp, err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, e := ln.Accept()
		if e != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 8)
		_, _ = conn.Read(buf)
		_, _ = conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
	}()
	tcpAddr := ln.Addr().(*net.TCPAddr)
	testBody, _ := json.Marshal(map[string]any{
		"socketProxyHost": tcpAddr.IP.String(),
		"socketProxyPort": tcpAddr.Port,
		"socketProxyAuth": true,
		"socketProxyUser": "probe-user",
		"secretActions":   map[string]string{"socketProxyPassword": "keep"},
	})
	proxyReq := httptest.NewRequest("POST", "/api/admin/proxy/test", bytes.NewReader(testBody))
	proxyReq.Header.Set("Content-Type", "application/json")
	proxyReq.Header.Set("Authorization", "Bearer "+token)
	proxyResp, err := app.Test(proxyReq, 5000)
	if err != nil {
		t.Fatal(err)
	}
	defer proxyResp.Body.Close()
	proxyData, _ := io.ReadAll(proxyResp.Body)
	if proxyResp.StatusCode != 502 || !strings.Contains(string(proxyData), "HTTP 代理端口") || strings.Contains(string(proxyData), cfg.SocketProxyPassword) {
		t.Fatalf("unexpected proxy test response %d: %s", proxyResp.StatusCode, proxyData)
	}

	// Verify /api/admin/proxy/test also honors a direct socketProxyPassword when secretActions is omitted.
	go func() {
		conn, e := ln.Accept()
		if e != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 8)
		_, _ = conn.Read(buf)
		_, _ = conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
	}()
	directBody, _ := json.Marshal(map[string]any{
		"socketProxyHost":     tcpAddr.IP.String(),
		"socketProxyPort":     tcpAddr.Port,
		"socketProxyAuth":     true,
		"socketProxyUser":     "probe-user",
		"socketProxyPassword": "inline-secret-value",
	})
	directReq := httptest.NewRequest("POST", "/api/admin/proxy/test", bytes.NewReader(directBody))
	directReq.Header.Set("Content-Type", "application/json")
	directReq.Header.Set("Authorization", "Bearer "+token)
	directResp, err := app.Test(directReq, 5000)
	if err != nil {
		t.Fatal(err)
	}
	defer directResp.Body.Close()
	directData, _ := io.ReadAll(directResp.Body)
	if directResp.StatusCode != 502 || !strings.Contains(string(directData), "HTTP 代理端口") || strings.Contains(string(directData), "inline-secret-value") {
		t.Fatalf("unexpected direct proxy test response %d: %s", directResp.StatusCode, directData)
	}

	// Verify /api/web/setting exposes proxyConfigured only to authenticated admins.
	anonWeb, _ := app.Test(httptest.NewRequest("GET", "/api/web/setting", nil))
	anonBody, _ := io.ReadAll(anonWeb.Body)
	anonWeb.Body.Close()
	if strings.Contains(string(anonBody), `"proxyConfigured":true`) {
		t.Fatalf("anonymous web settings leaked proxyConfigured: %s", anonBody)
	}
}

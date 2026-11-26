package api

import (
	"bytes"
	"encoding/json"
	"github.com/gofiber/fiber/v2"
	"io"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"typonamer/config"
)

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
	token, e := generateToken(cfg.AuthUsername)
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

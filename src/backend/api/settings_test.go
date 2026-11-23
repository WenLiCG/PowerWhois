package api

import (
	"bytes"
	"encoding/json"
	"github.com/gofiber/fiber/v2"
	"io"
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
}

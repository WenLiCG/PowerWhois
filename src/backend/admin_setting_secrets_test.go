package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"typonamer/config"
)

const (
	pw5TestOnlyAdmin     = "pw5-red-test-only-admin-7db41"
	pw5TestOnlyJWT       = "pw5-red-test-only-jwt-82c6f"
	pw5TestOnlySocket    = "pw5-red-test-only-socket-39fa2"
	pw5TestOnlyNewAdmin  = "pw5-red-test-only-new-admin-b71c2"
	pw5TestOnlyNewJWT    = "pw5-red-test-only-new-jwt-f63a1"
	pw5TestOnlyNewSocket = "pw5-red-test-only-new-socket-90e4b"
)

// No parallel tests: config and its executable-relative yaml are process global.
type pw5Fixture struct {
	app         *fiber.App
	token, yaml string
}

func pw5Setup(t *testing.T, socket string) pw5Fixture {
	t.Helper()
	old := config.GetConfig()
	t.Setenv("POWERWHOIS_ADMIN_PASSWORD", pw5TestOnlyAdmin)
	t.Setenv("POWERWHOIS_JWT_SECRET", pw5TestOnlyJWT)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(filepath.Dir(exe), "config.yaml")
	// Only touch the disposable go-test executable directory, never repository yaml.
	if filepath.Dir(exe) == backendDir(t) {
		t.Fatal("test executable must be outside source directory")
	}
	previous, readErr := os.ReadFile(path)
	var mode os.FileMode
	if readErr == nil {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(readErr) {
		t.Fatal(readErr)
	}
	t.Cleanup(func() {
		// UpdateConfig sources these two fields from the environment. Restore the
		// saved in-memory values before t.Setenv restores the original environment.
		if old.AuthPassword != "" && old.JwtSecretKey != "" {
			_ = os.Setenv("POWERWHOIS_ADMIN_PASSWORD", old.AuthPassword)
			_ = os.Setenv("POWERWHOIS_JWT_SECRET", old.JwtSecretKey)
		}
		if err := config.UpdateConfig(old); err != nil {
			t.Errorf("restore config: %v", err)
		}
		if old.AuthPassword == "" && old.JwtSecretKey == "" {
			_ = os.Setenv("POWERWHOIS_ADMIN_PASSWORD", "")
			_ = os.Setenv("POWERWHOIS_JWT_SECRET", "")
			_ = config.ApplyRequiredSecrets()
		}
		if readErr == nil {
			if err := os.WriteFile(path, previous, mode); err != nil {
				t.Error(err)
			}
			if err := os.Chmod(path, mode); err != nil {
				t.Error(err)
			}
		} else if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			t.Error(err)
		}
	})
	if err := config.ApplyRequiredSecrets(); err != nil {
		t.Fatal(err)
	}
	seed := config.Config{LogLevel: "Info", AuthUsername: "pw5-test-only-admin-user", AuthExpireDays: 1, SocketProxyPassword: socket}
	if err := config.UpdateConfig(seed); err != nil {
		t.Fatal(err)
	}
	f := pw5Fixture{app: newApp(), yaml: path}
	f.token = pw5Login(t, f.app, pw5TestOnlyAdmin, 200)
	return f
}

func pw5Request(t *testing.T, app *fiber.App, method, path, token string, payload any, status int) []byte {
	t.Helper()
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != status {
		t.Fatalf("%s %s status=%d, want %d", method, path, resp.StatusCode, status)
	}
	return raw
}

func pw5Login(t *testing.T, app *fiber.App, password string, status int) string {
	t.Helper()
	raw := pw5Request(t, app, "POST", "/api/login", "", map[string]string{"username": "pw5-test-only-admin-user", "password": password}, status)
	pw5NoPlaintext(t, raw, pw5TestOnlyAdmin, pw5TestOnlyJWT, pw5TestOnlySocket)
	if status != 200 {
		return ""
	}
	result := pw5Object(t, raw)
	token, ok := result["token"].(string)
	if !ok || token == "" {
		t.Fatal("successful login must return a nonempty token")
	}
	return token
}

func pw5Object(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	return result
}
func pw5NoPlaintext(t *testing.T, raw []byte, secrets ...string) {
	t.Helper()
	for i, secret := range secrets {
		if strings.Contains(string(raw), secret) {
			t.Errorf("raw response contains test-only secret #%d", i+1)
		}
	}
}
func pw5Masked(t *testing.T, raw []byte, socket string) {
	t.Helper()
	pw5NoPlaintext(t, raw, pw5TestOnlyAdmin, pw5TestOnlyJWT, pw5TestOnlySocket, pw5TestOnlyNewAdmin, pw5TestOnlyNewJWT, pw5TestOnlyNewSocket)
	obj := pw5Object(t, raw)
	for field, want := range map[string]string{"authPassword": "__PW_SECRET_SET__", "jwtSecretKey": "__PW_SECRET_SET__", "socketProxyPassword": socket} {
		if obj[field] != want {
			t.Errorf("%s does not equal required public marker", field)
		}
	}
}
func pw5Unchanged(t *testing.T) {
	t.Helper()
	cfg := config.GetConfig()
	if cfg.AuthPassword != pw5TestOnlyAdmin {
		t.Error("stored admin password changed")
	}
	if cfg.JwtSecretKey != pw5TestOnlyJWT {
		t.Error("stored JWT secret changed")
	}
	if cfg.SocketProxyPassword != pw5TestOnlySocket {
		t.Error("stored Socket password overwritten")
	}
}

func TestPW5AdminSettingGETMasksSecrets(t *testing.T) {
	f := pw5Setup(t, pw5TestOnlySocket)
	pw5Masked(t, pw5Request(t, f.app, "GET", "/api/admin/setting", f.token, nil, 200), "__PW_SECRET_SET__")
	pw5Unchanged(t)
}
func TestPW5AdminSettingGETUnsetSocket(t *testing.T) {
	for _, socket := range []string{"", " \t\n "} {
		t.Run("unset_"+strings.ReplaceAll(socket, "\n", "newline"), func(t *testing.T) {
			f := pw5Setup(t, socket)
			pw5Masked(t, pw5Request(t, f.app, "GET", "/api/admin/setting", f.token, nil, 200), "")
		})
	}
}
func TestPW5AdminSettingRoundTrip(t *testing.T) {
	f := pw5Setup(t, pw5TestOnlySocket)
	raw := pw5Request(t, f.app, "GET", "/api/admin/setting", f.token, nil, 200)
	pw5Masked(t, raw, "__PW_SECRET_SET__")
	body := pw5Object(t, raw)
	body["authExpireDays"] = 7
	// Replay the actual GET view unchanged except for a nonsecret field.
	result := pw5Request(t, f.app, "PUT", "/api/admin/setting", f.token, body, 200)
	pw5Unchanged(t)
	if config.GetConfig().AuthExpireDays != 7 {
		t.Error("nonsecret update did not persist")
	}
	pw5Masked(t, result, "__PW_SECRET_SET__")
	pw5Login(t, f.app, pw5TestOnlyAdmin, 200)
	// A fresh app ensures JWT validation uses the stored key after the update.
	pw5Request(t, newApp(), "GET", "/api/admin/setting", f.token, nil, 200)
}
func TestPW5AdminSettingPUTPreservesSecrets(t *testing.T) {
	cases := []struct {
		name    string
		value   any
		missing bool
	}{
		{"empty", "", false}, {"marker", "__PW_SECRET_SET__", false},
		{"whitespace", " \t\n ", false}, {"padded_marker", " \t__PW_SECRET_SET__\n ", false},
		{"null", nil, false}, {"missing", nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := pw5Setup(t, pw5TestOnlySocket)
			data, err := json.Marshal(config.GetConfig())
			if err != nil {
				t.Fatal(err)
			}
			body := pw5Object(t, data)
			for _, key := range []string{"authPassword", "jwtSecretKey", "socketProxyPassword"} {
				if tc.missing {
					delete(body, key)
				} else {
					body[key] = tc.value
				}
			}
			raw := pw5Request(t, f.app, "PUT", "/api/admin/setting", f.token, body, 200)
			pw5Unchanged(t)
			pw5Masked(t, raw, "__PW_SECRET_SET__")
		})
	}
}
func TestPW5AdminSettingPUTUpdatesSocket(t *testing.T) {
	f := pw5Setup(t, pw5TestOnlySocket)
	body := config.GetConfig()
	body.SocketProxyPassword = pw5TestOnlyNewSocket
	raw := pw5Request(t, f.app, "PUT", "/api/admin/setting", f.token, body, 200)
	if config.GetConfig().SocketProxyPassword != pw5TestOnlyNewSocket {
		t.Error("new Socket password not stored")
	}
	pw5Masked(t, raw, "__PW_SECRET_SET__")
	pw5Masked(t, pw5Request(t, f.app, "GET", "/api/admin/setting", f.token, nil, 200), "__PW_SECRET_SET__")
}
func TestPW5AdminSettingPUTCannotReplaceEnvironmentSecrets(t *testing.T) {
	f := pw5Setup(t, pw5TestOnlySocket)
	body := config.GetConfig()
	body.AuthPassword = pw5TestOnlyNewAdmin
	body.JwtSecretKey = pw5TestOnlyNewJWT
	pw5Request(t, f.app, "PUT", "/api/admin/setting", f.token, body, 200)
	pw5Unchanged(t)
	pw5Login(t, newApp(), pw5TestOnlyNewAdmin, 400)
	pw5Login(t, newApp(), pw5TestOnlyAdmin, 200)
	pw5Request(t, newApp(), "GET", "/api/admin/setting", f.token, nil, 200)
	yaml, err := os.ReadFile(f.yaml)
	if err != nil {
		t.Fatal(err)
	}
	pw5NoPlaintext(t, yaml, pw5TestOnlyAdmin, pw5TestOnlyJWT, pw5TestOnlyNewAdmin, pw5TestOnlyNewJWT)
}
func TestPW5LoginAndWebSettingDoNotLeakSecrets(t *testing.T) {
	f := pw5Setup(t, pw5TestOnlySocket)
	pw5Login(t, f.app, pw5TestOnlyAdmin, 200)
	pw5NoPlaintext(t, pw5Request(t, f.app, "GET", "/api/web/setting", "", nil, 200), pw5TestOnlyAdmin, pw5TestOnlyJWT, pw5TestOnlySocket)
}
func TestPW5AdminSettingRequiresToken(t *testing.T) {
	f := pw5Setup(t, pw5TestOnlySocket)
	pw5Request(t, f.app, "GET", "/api/admin/setting", "", nil, 401)
	pw5Request(t, f.app, "PUT", "/api/admin/setting", "", config.GetConfig(), 401)
}
func TestPW5AdminSettingWritesYAMLMode0600(t *testing.T) {
	f := pw5Setup(t, pw5TestOnlySocket)
	// Remove the seed file so creation permissions cannot inherit a prior mode.
	if err := os.Remove(f.yaml); err != nil {
		t.Fatal(err)
	}
	body := config.GetConfig()
	body.AuthExpireDays = 3
	pw5Request(t, f.app, "PUT", "/api/admin/setting", f.token, body, 200)
	info, err := os.Stat(f.yaml)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("written yaml mode=%04o, want 0600", info.Mode().Perm())
	}
}

package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"typonamer/config"
	"typonamer/log"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

const (
	// Temporary values used only inside tests. They are not production secrets.
	testWrongPassword = "test-only-wrong-password"
	testFilePassword  = "test-only-file-password"
	testFileJWT       = "test-only-file-jwt"
	testEnvPassword   = "test-only-admin-password"
	testEnvJWT        = "test-only-jwt-secret"
)

func TestUnauthenticatedTuningReturns401(t *testing.T) {
	t.Setenv("APP_TUNING", "true")
	app := newApp()

	for _, path := range []string{"/api/tuning/debug/pprof/", "/api/tuning/monitor"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s status=%d, want 401", path, resp.StatusCode)
		}
	}
}

func TestTuningDisabledWhenAppTuningUnset(t *testing.T) {
	t.Setenv("APP_TUNING", "")
	app := newApp()

	for _, path := range []string{"/api/tuning/debug/pprof/", "/api/tuning/monitor"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("%s status=%d, want 404 while APP_TUNING is unset", path, resp.StatusCode)
		}
	}
}

func TestDeployComposeDoesNotForceTuningOn(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", "deploy", "docker-compose.yml")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(body), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.Contains(lower, "app_tuning") && strings.Contains(lower, "true") {
			t.Fatalf("deploy/docker-compose.yml forces tuning on: %s", trimmed)
		}
	}
}

func TestFailedLoginLogOmitsSubmittedPassword(t *testing.T) {
	log.SetLevel("debug")
	// Lumberjack creates the log file on the first write. Open it, then clear
	// it so the assertion sees only the failed-login line.
	log.Info("security-test-log-open")
	if err := log.ResetLogsFile(); err != nil {
		t.Fatal(err)
	}

	app := newApp()
	body := strings.NewReader(`{"username":"not-a-user","password":"` + testWrongPassword + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/login", body)
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status=%d, want 400", resp.StatusCode)
	}

	log.Sync()
	text := readAppLog(t)
	if !strings.Contains(text, "Login failed") {
		t.Fatalf("log has no login-failure record: %q", text)
	}
	if strings.Contains(text, testWrongPassword) {
		t.Fatalf("failed-login log contains the submitted password %q\nlog: %s", testWrongPassword, text)
	}
}

func TestAuthenticatedTuningReachesHandler(t *testing.T) {
	t.Setenv("APP_TUNING", "true")
	t.Setenv("POWERWHOIS_ADMIN_PASSWORD", testEnvPassword)
	t.Setenv("POWERWHOIS_JWT_SECRET", testEnvJWT)
	if err := config.ApplyRequiredSecrets(); err != nil {
		t.Fatal(err)
	}

	claims := jwt.MapClaims{
		"name": "admin",
		"exp":  time.Now().Add(time.Hour).Unix(),
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testEnvJWT))
	if err != nil {
		t.Fatal(err)
	}

	app := newApp()
	for _, path := range []string{"/api/tuning/debug/pprof/", "/api/tuning/monitor"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set(fiber.HeaderAuthorization, "Bearer "+signed)
		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusNotFound {
			t.Fatalf("%s status=%d, want the handler behind a valid admin token", path, resp.StatusCode)
		}
	}
}

func TestStartupRefusedWhenAdminPasswordMissing(t *testing.T) {
	assertStartupRefused(t, map[string]string{
		"POWERWHOIS_JWT_SECRET": testEnvJWT,
	}, "POWERWHOIS_ADMIN_PASSWORD")
}

func TestStartupRefusedWhenJWTSecretMissing(t *testing.T) {
	assertStartupRefused(t, map[string]string{
		"POWERWHOIS_ADMIN_PASSWORD": testEnvPassword,
	}, "POWERWHOIS_JWT_SECRET")
}

func assertStartupRefused(t *testing.T, env map[string]string, missingName string) {
	t.Helper()

	dir := t.TempDir()
	bin := filepath.Join(dir, "typonamer")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = backendDir(t)
	buildOut, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("go build: %v\n%s", err, buildOut)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(startupConfigYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin)
	cmd.Env = startupEnv(env)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	runErr := cmd.Run()
	text := output.String()
	if strings.Contains(text, testFilePassword) || strings.Contains(text, testFileJWT) {
		t.Fatalf("startup output contains a file-configured secret value")
	}
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("process still running after 15s; missing %s did not refuse startup\noutput: %s", missingName, text)
	}
	if runErr == nil {
		t.Fatalf("process exited 0; missing %s did not refuse startup\noutput: %s", missingName, text)
	}
	if !strings.Contains(text, "refusing to start") || !strings.Contains(text, missingName) {
		t.Fatalf("startup error does not name %s\nexit: %v\noutput: %s", missingName, runErr, text)
	}
}

func startupEnv(extra map[string]string) []string {
	env := []string{"PATH=" + os.Getenv("PATH")}
	for key, value := range extra {
		env = append(env, key+"="+value)
	}
	return env
}

func backendDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(file)
}

func readAppLog(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(filepath.Dir(exe), "logs", "typonamer.log")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

const startupConfigYAML = `LogLevel: Info
AuthUsername: admin
AuthPassword: test-only-file-password
AuthExpireDays: 1
JwtSecretKey: test-only-file-jwt
WhoisTimeout: 1
DnsTimeout: 1
RetryOnTimeout: false
RetryInterval: 1
RetryMax: 1
BulkCheckConcurrencyLimit: 1
WebCheckConcurrencyLimit: 1
WebCheckDomainLimit: 1
`

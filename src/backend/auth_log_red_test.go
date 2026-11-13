package main

import (
	"crypto"
	"encoding/json"
	"errors"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"typonamer/api"
	"typonamer/config"
	"typonamer/log"
)

// Dependency globals and the log file require sequential tests. Inject at the
// JWT library's existing seam, without adding any production hook.
func TestLoginSigningFailureLogOmitsUnderlyingError(t *testing.T) {
	log.SetLevel("Debug")
	log.Info("7c-open-log")
	if err := log.ResetLogsFile(); err != nil {
		t.Fatal(err)
	}
	oldHash, oldErr := jwt.SigningMethodHS256.Hash, jwt.ErrHashUnavailable
	jwt.SigningMethodHS256.Hash = crypto.Hash(0)
	jwt.ErrHashUnavailable = errors.New("pw7-7c-signing-error-private-marker-9081")
	defer func() { jwt.SigningMethodHS256.Hash = oldHash; jwt.ErrHashUnavailable = oldErr }()
	cfg := config.GetConfig()
	body, err := json.Marshal(map[string]string{"username": cfg.AuthUsername, "password": cfg.AuthPassword})
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	app.Post("/api/login", api.Login)
	req := httptest.NewRequest("POST", "/api/login", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 500 || string(raw) != "Internal Server Error" {
		t.Errorf("response=(%d,%q), want (500, Internal Server Error)", resp.StatusCode, raw)
	}
	log.Sync()
	text := readAppLog(t)
	if strings.Count(text, "Generate token error") != 2 {
		t.Error("expected both generation failure log sites to execute")
	}
	if strings.Contains(text, "pw7-7c-signing-error-private-marker-9081") {
		t.Error("underlying signing error leaked into log")
	}
}

// Public entry point used by WebSocket authentication. Malformed JSON claims
// fail in jwt.Parse, before the subsequent MapClaims type assertion.
func TestValidateTokenMalformedClaimsLogOmitsTokenAndParserDetail(t *testing.T) {
	log.SetLevel("Debug")
	log.Info("7c-open-log")
	if err := log.ResetLogsFile(); err != nil {
		t.Fatal(err)
	}
	const token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.InB3Ny03Yy1wcml2YXRlLWNsYWltcy1tYXJrZXIi.c2ln"
	ok, err := api.ValidateToken(token)
	if ok || err == nil {
		t.Fatal("malformed claims accepted")
	}
	if !strings.Contains(err.Error(), "cannot unmarshal string") {
		t.Fatalf("unexpected failure: %v", err)
	}
	log.Sync()
	text := readAppLog(t)
	if !strings.Contains(text, "Failed to parse token") {
		t.Error("missing parse failure log")
	}
	for _, forbidden := range []string{token, "pw7-7c-private-claims-marker", "cannot unmarshal string", "token is malformed"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("log contains forbidden token/parser detail %q", forbidden)
		}
	}
}

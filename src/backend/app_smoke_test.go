package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// TestSmokeMalformedLoginReturns400 proves the test harness can boot the real
// HTTP app and exercise an existing route. A malformed login body is rejected
// before any credential is checked.
func TestSmokeMalformedLoginReturns400(t *testing.T) {
	app := newApp()

	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader("{"))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusBadRequest)
	}
}

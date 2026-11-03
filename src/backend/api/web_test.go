package api

import (
	"github.com/gofiber/fiber/v2"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeWebServesSPAWithoutMaskingAPIErrors(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "index.html"), []byte("fixture-spa"), 0600)
	os.WriteFile(filepath.Join(root, "asset.js"), []byte("fixture-asset"), 0600)
	app := fiber.New()
	app.Get("/api/ping", func(c *fiber.Ctx) error { return c.SendString("pong") })
	if e := ServeWeb(app, root); e != nil {
		t.Fatal(e)
	}
	for path, want := range map[string]string{"/": "fixture-spa", "/task/example": "fixture-spa", "/asset.js": "fixture-asset", "/api/ping": "pong"} {
		response, e := app.Test(httptest.NewRequest("GET", path, nil))
		if e != nil {
			t.Fatal(e)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != 200 || string(body) != want {
			t.Fatalf("%s: %d %s", path, response.StatusCode, body)
		}
	}
	for _, path := range []string{"/api/missing", "/app/missing", "/missing.js"} {
		response, e := app.Test(httptest.NewRequest("GET", path, nil))
		if e != nil {
			t.Fatal(e)
		}
		response.Body.Close()
		if response.StatusCode != 404 {
			t.Fatal("SPA masked missing route", path)
		}
	}
	if ServeWeb(fiber.New(), t.TempDir()) == nil {
		t.Fatal("invalid root accepted")
	}
}

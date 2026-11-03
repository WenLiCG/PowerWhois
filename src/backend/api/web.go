package api

import (
	"fmt"
	"github.com/gofiber/fiber/v2"
	"os"
	"path/filepath"
	"strings"
)

// ServeWeb keeps a native installation on one loopback port without another web server.
func ServeWeb(app *fiber.App, root string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	index := filepath.Join(root, "index.html")
	if info, e := os.Stat(index); e != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("web root must contain index.html")
	}
	app.Use(func(c *fiber.Ctx) error {
		if c.Path() == "/api" || strings.HasPrefix(c.Path(), "/api/") || c.Path() == "/app" || strings.HasPrefix(c.Path(), "/app/") {
			return c.SendStatus(404)
		}
		return c.Next()
	})
	app.Static("/", root)
	app.Get("/*", func(c *fiber.Ctx) error {
		if filepath.Ext(c.Path()) != "" {
			return c.SendStatus(404)
		}
		return c.SendFile(index)
	})
	return nil
}

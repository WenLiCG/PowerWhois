package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"typonamer/database"
	"typonamer/register"
	"typonamer/scheduler"

	"typonamer/api"
	"typonamer/config"
	"typonamer/log"

	"github.com/dromara/carbon/v2"
	"github.com/gofiber/contrib/socketio"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/monitor"
	"github.com/gofiber/fiber/v2/middleware/pprof"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

const (
	appName    = "PowerWhois"
	appVersion = "2026.11"
	listenPort = "127.0.0.1:8080"
)

var (
	appTuning string = os.Getenv("APP_TUNING")
)

func init() {
	// init carbon config
	carbon.SetDefault(carbon.Default{
		Layout:       carbon.DateTimeLayout,
		Timezone:     carbon.PRC,
		WeekStartsAt: carbon.Monday,
		Locale:       "zh-CN",
	})

	// init logger
	// init log level
	cfg := config.GetConfig()
	log.SetLevel(cfg.LogLevel)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	path := os.Getenv("POWERWHOIS_CONFIG")
	if path == "" {
		exe, _ := os.Executable()
		path = filepath.Join(filepath.Dir(exe), "config.yaml")
	}
	if err := config.Load(path); err != nil {
		return err
	}
	register.SetupLimiter()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	r, err := database.GetRedisContext(ctx)
	if err != nil {
		return err
	}
	defer r.Close()
	if err := api.InitAuth(ctx, r); err != nil {
		return fmt.Errorf("cannot load administrator sessions: %w", err)
	}
	register.SetAuditStore(r)
	scheduler.Init(ctx, r)

	// ---------- Init Fiber App ----------
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ReadTimeout:           60 * time.Second,
		IdleTimeout:           60 * time.Second,
		AppName:               fmt.Sprintf("%s v%s", appName, appVersion),
		BodyLimit:             100 * 1024 * 1024, // 100MB
	})
	defer app.Shutdown()

	// 延迟同步日志
	defer log.Sync()

	app.Use(func(c *fiber.Ctx) error {
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "DENY")
		c.Set("Referrer-Policy", "same-origin")
		return c.Next()
	})
	app.Use(compress.New())
	app.Use(recover.New())
	app.Get("/healthz", func(c *fiber.Ctx) error {
		if err := r.Ping(c.UserContext()).Err(); err != nil {
			return fiber.ErrServiceUnavailable
		}
		return c.SendString("ok")
	})

	// Check if appTuning is true
	if appTuning != "" && strings.ToLower(appTuning) == "true" {
		app.Use("/api/tuning", api.LoginRequired())
		app.Use(pprof.New(pprof.Config{Prefix: "/api/tuning"})) // 启用 pprof
		app.Get("/api/tuning/monitor", monitor.New())           // 监控页面
	}

	limited := func(c *fiber.Ctx) error {
		return c.Status(429).JSON(fiber.Map{"error": fiber.Map{"code": "rate_limited", "message": "request limit reached; respect Retry-After"}})
	}
	app.Post("/api/login", limiter.New(limiter.Config{Max: 5, Expiration: time.Minute, LimitReached: limited}))
	app.Post("/api/v1/query", limiter.New(limiter.Config{Max: 30, Expiration: time.Minute, LimitReached: limited}))
	app.Post("/api/v1/tasks", limiter.New(limiter.Config{Max: 60, Expiration: time.Minute, LimitReached: limited}))
	app.Use("/api/v1", limiter.New(limiter.Config{Max: 1200, Expiration: time.Minute, LimitReached: limited}))
	// Setup WebSocket
	app.Use("/app/ws", api.WebSocketUpgrade)
	app.Get("/app/ws", socketio.New(api.WebSocketHandler))

	// Setup Fiber API Router
	app.Route("/api", api.ApiRoute, "api.")
	if webRoot := os.Getenv("POWERWHOIS_WEB_ROOT"); webRoot != "" {
		if err := api.ServeWeb(app, webRoot); err != nil {
			return err
		}
	}

	// ---------- Start Server ----------
	workerDone := make(chan error, 1)
	go func() {
		err := scheduler.Tasks.Run(ctx)
		if ctx.Err() != nil && errors.Is(err, context.Canceled) {
			err = nil
		}
		workerDone <- err
		stop()
	}()
	select {
	case <-scheduler.Tasks.Ready:
	case <-ctx.Done():
		return <-workerDone
	}
	go func() { <-ctx.Done(); app.Shutdown() }()
	address := os.Getenv("POWERWHOIS_LISTEN")
	if address == "" {
		address = listenPort
	}
	listenErr := app.Listen(address)
	stop()
	return errors.Join(listenErr, <-workerDone)
}

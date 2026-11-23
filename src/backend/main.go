package main

import (
	"context"
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
	path := os.Getenv("POWERWHOIS_CONFIG")
	if path == "" {
		exe, _ := os.Executable()
		path = filepath.Join(filepath.Dir(exe), "config.yaml")
	}
	if err := config.Load(path); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	register.SetupLimiter()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	r, err := database.GetRedis()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer r.Close()
	if err := api.InitAuth(ctx, r); err != nil {
		fmt.Fprintln(os.Stderr, "Cannot load administrator sessions:", err)
		os.Exit(1)
	}
	register.SetAuditStore(r)
	scheduler.Init(ctx, r)

	// ---------- Init Fiber App ----------
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ReadTimeout:           30 * time.Second,
		IdleTimeout:           60 * time.Second,
		AppName:               fmt.Sprintf("%s v%s", appName, appVersion),
		BodyLimit:             100 * 1024 * 1024, // 100MB
	})
	defer app.Shutdown()

	// 延迟同步日志
	defer log.Sync()

	app.Use(compress.New())
	app.Use(recover.New())

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
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	// ---------- Start Server ----------
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		if err := scheduler.Tasks.Run(ctx); err != nil {
			log.Error("Task worker stopped: ", err)
		}
		stop()
	}()
	select {
	case <-scheduler.Tasks.Ready:
	case <-ctx.Done():
		<-workerDone
		return
	}
	go func() { <-ctx.Done(); app.Shutdown() }()
	address := os.Getenv("POWERWHOIS_LISTEN")
	if address == "" {
		address = listenPort
	}
	if err := app.Listen(address); err != nil {
		log.Error("HTTP server stopped: ", err)
	}
	stop()
	<-workerDone
}

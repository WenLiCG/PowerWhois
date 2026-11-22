package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata"

	"typonamer/api"
	"typonamer/config"
	"typonamer/database"
	"typonamer/log"
	"typonamer/scheduler"

	"github.com/dromara/carbon/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/monitor"
	"github.com/gofiber/fiber/v2/middleware/pprof"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	defer log.Sync()
	if err := run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		log.Error("PowerWhois startup or shutdown failed: ", err)
		log.Sync()
		os.Exit(1)
	}
}

func run(parent context.Context) error {
	carbon.SetDefault(carbon.Default{Layout: carbon.DateTimeLayout, Timezone: carbon.PRC, WeekStartsAt: carbon.Monday, Locale: "zh-CN"})
	file := os.Getenv("POWERWHOIS_CONFIG")
	if file == "" {
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		file = filepath.Join(filepath.Dir(executable), "config.yaml")
	}
	if err := config.Load(file); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	client, err := database.GetRedis(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	addr := os.Getenv("POWERWHOIS_LISTEN")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer listener.Close()
	bulk, err := scheduler.NewBulkCheck(ctx, client)
	if err != nil {
		return err
	}
	app, drainSockets := newApp(ctx, bulk, client)
	serverErr := make(chan error, 1)
	go func() { serverErr <- app.Listener(listener) }()
	select {
	case <-ctx.Done():
	case err = <-serverErr:
	}
	cancel()
	deadline, stop := context.WithTimeout(context.Background(), 15*time.Second)
	defer stop()
	var closing errgroup.Group
	closing.Go(func() error { return bulk.Shutdown(deadline) })
	closing.Go(func() error { return app.ShutdownWithContext(deadline) })
	closing.Go(func() error { return drainSockets(deadline) })
	return errors.Join(err, closing.Wait())
}

func newApp(ctx context.Context, bulk *scheduler.BulkCheck, client *redis.Client) (*fiber.App, func(context.Context) error) {
	app := fiber.New(fiber.Config{
		AppName: "PowerWhois", BodyLimit: 64 << 10,
		StreamRequestBody: true, DisablePreParseMultipartForm: true,
		ReadTimeout: 2 * time.Minute, WriteTimeout: 3 * time.Minute, IdleTimeout: 20 * time.Second,
		ProxyHeader: fiber.HeaderXForwardedFor, EnableTrustedProxyCheck: true, EnableIPValidation: true,
		TrustedProxies: strings.Fields(os.Getenv("POWERWHOIS_TRUSTED_PROXIES")),
	})
	app.Use(recover.New())
	app.Use(compress.New())
	if origin := os.Getenv("POWERWHOIS_WEB_ORIGIN"); origin != "" {
		app.Use(cors.New(cors.Config{AllowOrigins: origin, ExposeHeaders: fiber.HeaderContentDisposition}))
	}
	if strings.EqualFold(os.Getenv("APP_TUNING"), "true") {
		app.Use("/api/tuning", api.LoginRequired)
		app.Use(pprof.New(pprof.Config{Prefix: "/api/tuning"}))
		app.Get("/api/tuning/monitor", monitor.New())
	}
	upgrade, drain := api.WebSocketHandler(ctx, bulk, client)
	app.Get("/app/ws", limiter.New(limiter.Config{Max: 120, Expiration: time.Minute}), upgrade)
	app.Route("/api", func(router fiber.Router) { api.ApiRoute(router, bulk, client) }, "api.")
	return app, drain
}

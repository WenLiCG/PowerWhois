package main

import (
	"fmt"
	"os"
	"strings"

	"typonamer/api"
	"typonamer/config"
	"typonamer/log"

	"github.com/dromara/carbon/v2"
	"github.com/gofiber/contrib/socketio"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/monitor"
	"github.com/gofiber/fiber/v2/middleware/pprof"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

const (
	appName    = "TypoNamer"
	appVersion = "2025.09.02"
	listenPort = ":8080"
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
	app := newApp()
	defer app.Shutdown()

	// 延迟同步日志
	defer log.Sync()

	// ---------- Start Server ----------
	if err := app.Listen(listenPort); err != nil {
		panic(err)
	}
}

// newApp builds the HTTP server. APP_TUNING is read when the app is built so
// an unset variable leaves tuning disabled.
func newApp() *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:   fmt.Sprintf("%s v%s", appName, appVersion),
		BodyLimit: 100 * 1024 * 1024, // 100MB
	})

	app.Use(compress.New())
	app.Use(cors.New(cors.Config{
		ExposeHeaders: fmt.Sprintf("%s,%s", fiber.HeaderContentDisposition, fiber.HeaderContentType),
	}))
	app.Use(recover.New())

	// Check if APP_TUNING is true. Read at app build time; unset stays off.
	appTuning := os.Getenv("APP_TUNING")
	if appTuning != "" && strings.ToLower(appTuning) == "true" {
		app.Use(pprof.New(pprof.Config{Prefix: "/api/tuning"})) // 启用 pprof
		app.Get("/api/tuning/monitor", monitor.New())           // 监控页面
	}

	// Setup WebSocket
	app.Use("/app/ws", api.WebSocketUpgrade)
	app.Get("/app/ws", socketio.New(api.WebSocketHandler))

	// Setup Fiber API Router
	app.Route("/api", api.ApiRoute, "api.")

	return app
}

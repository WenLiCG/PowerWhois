package api

import (
	"time"
	"typonamer/scheduler"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/redis/go-redis/v9"
)

func ApiRoute(router fiber.Router, bulk *scheduler.BulkCheck, client *redis.Client) {
	router.Post("/login", limiter.New(limiter.Config{Max: 10, Expiration: time.Minute, SkipSuccessfulRequests: true}), Login)
	router.Get("/web/setting", WebSettingList)

	admin := router.Group("/admin", LoginRequired)
	admin.Get("/setting", AdminSettingList)
	admin.Put("/setting", SettingUpdate)
	admin.Get("/log", DownloadLog)
	admin.Delete("/log", ResetLog)
	admin.Post("/bulkcheckupload", BulkCheckDomainUpload(bulk))
	admin.Get("/bulkcheckresultdownload", BulkCheckResultDownload(bulk))
	admin.Put("/registration/:operation", ReconcileRegistration(client))
}

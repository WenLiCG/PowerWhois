package api

import (
	"crypto/sha256"
	"fmt"
	"os"
	"time"
	"typonamer/scheduler"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/redis/go-redis/v9"
)

func ApiRoute(router fiber.Router, bulk *scheduler.BulkCheck, client *redis.Client) {
	setAPIKeys(os.Getenv("POWERWHOIS_API_KEYS"))

	router.Post("/login", limiter.New(limiter.Config{Max: 10, Expiration: time.Minute, SkipSuccessfulRequests: true, KeyGenerator: LimitKey}), Login)
	router.Get("/web/setting", WebSettingList)
	// The limiter runs after key validation so invalid keys cannot spend a valid key's budget.
	router.Get("/v1/check", APIKeyRequired, limiter.New(limiter.Config{
		Max: 600, Expiration: time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string { return fmt.Sprintf("api:%x", sha256.Sum256([]byte(requestAPIKey(c)))) },
	}), CheckDomain)

	admin := router.Group("/admin", LoginRequired)
	admin.Post("/logout", Logout)
	admin.Post("/ws-ticket", IssueWSTicket)
	admin.Get("/setting", AdminSettingList)
	admin.Put("/setting", SettingUpdate)
	admin.Get("/log", DownloadLog)
	admin.Delete("/log", ResetLog)
	admin.Post("/bulkcheckupload", BulkCheckDomainUpload(bulk))
	admin.Get("/bulkcheckresultdownload", BulkCheckResultDownload(bulk))
	admin.Get("/registrations", PendingRegistrations(client))
	admin.Put("/registration/:operation", ReconcileRegistration(client))
}

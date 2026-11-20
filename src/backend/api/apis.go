package api

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"typonamer/config"
	"typonamer/log"
	"typonamer/lookup/customize"
	"typonamer/register"
	"typonamer/scheduler"
	"typonamer/utils"

	"github.com/dromara/carbon/v2"
	"github.com/duke-git/lancet/v2/slice"
	"github.com/gofiber/fiber/v2"
)

// utf8BomData represents the UTF-8 Byte Order Mark (BOM) sequence.
// It is prepended to files to indicate UTF-8 encoding.
var utf8BomData = []byte{0xEF, 0xBB, 0xBF}

const secretSetMarker = "__PW_SECRET_SET__"

// maskedSettings changes only a response copy, keeping runtime secrets intact.
func maskedSettings() config.Config {
	cfg := config.GetConfig()
	for _, secret := range []*string{&cfg.AuthPassword, &cfg.JwtSecretKey, &cfg.SocketProxyPassword} {
		if strings.TrimSpace(*secret) == "" {
			*secret = ""
		} else {
			*secret = secretSetMarker
		}
	}
	return cfg
}

func AdminSettingList(c *fiber.Ctx) error {
	cfg := maskedSettings()
	log.Info("Getting admin setting list success")
	return c.JSON(cfg)
}

func WebSettingList(c *fiber.Ctx) error {
	log.Debug("Getting web setting list success")

	cfg := config.GetConfig()

	registerApis := make([]string, 0)
	for _, api := range cfg.RegisterApis {
		registerApis = append(registerApis, api.ApiName)
	}

	whoisApis := make([]string, 0)
	for _, api := range cfg.WhoisApis {
		whoisApis = append(whoisApis, api.ApiName)
	}

	return c.JSON(fiber.Map{
		"webCheckDomainLimit": cfg.WebCheckDomainLimit,
		"typoDefaultCcTlds":   cfg.TypoDefaultCcTlds,
		"registerApis":        registerApis,
		"whoisApis":           whoisApis,
	})
}

func SettingUpdate(c *fiber.Ctx) error {
	newConfig := new(config.Config)
	if err := c.BodyParser(newConfig); err != nil {
		// Error parsing the config info
		log.Error("Parse config info error: ", err)
		return c.Status(500).SendString(err.Error())
	}

	// Missing/null string fields parse as empty. Blank and marker values mean
	// keep the stored proxy password; real new values are preserved verbatim.
	password := strings.TrimSpace(newConfig.SocketProxyPassword)
	if password == "" || password == secretSetMarker {
		newConfig.SocketProxyPassword = config.GetConfig().SocketProxyPassword
	}

	err := config.UpdateConfig(*newConfig)
	if err != nil {
		// Error updating the config
		log.Error("Update config error: ", err)
		return c.Status(500).SendString(err.Error())
	}

	// Update the custom whois API limiter
	customize.SetupLimiter()

	// Update the register API limiter
	register.SetupLimiter()

	// Update the config success
	log.Info("Update config success")

	return c.JSON(maskedSettings())
}

func DownloadLog(c *fiber.Ctx) error {
	zipLogFile, err := log.GetZipLogsFile()
	if err != nil {
		log.Error("Get zip log file error: ", err)
		return c.Status(500).SendString(err.Error())
	}

	log.Info("Download log: ", zipLogFile)

	return c.Download(zipLogFile)
}

func ResetLog(c *fiber.Ctx) error {
	err := log.ResetLogsFile()
	if err != nil {
		// Error resetting the log file
		log.Error("Reset log error: ", err)
		return c.Status(500).SendString(err.Error())
	}

	// Reset the log success
	log.Info("Reset log success")
	return c.SendStatus(200)
}

func BulkCheckDomainUpload(c *fiber.Ctx) error {
	uploadFile, err := c.FormFile("file")
	if err != nil {
		log.Error("Get file error: ", err)
		return c.Status(500).SendString(err.Error())
	}

	log.Info("Bulk check domain upload: ", uploadFile.Filename)

	f, err := uploadFile.Open()
	if err != nil {
		log.Error("Open file error: ", err)
		return c.Status(500).SendString(err.Error())
	}
	defer f.Close()

	// Read the file content into a buffer
	buffer := bytes.NewBuffer(nil)
	io.Copy(buffer, f)

	// Add the raw domains to redis
	err = scheduler.BulkCheckAddRawDomains(buffer)
	if err != nil {
		log.Error("Bulk check domain save error: ", err)
		return c.Status(500).SendString(err.Error())
	}

	log.Info("Bulk check domain save to redis success")

	return c.SendStatus(200)
}

func BulkCheckResultDownload(c *fiber.Ctx) error {
	// Get the taken, free and error domains
	takenDomains := scheduler.GetBulkCheckTakenDomains()
	freeDomains := scheduler.GetBulkCheckFreeDomains()
	errorDomains := scheduler.GetBulkCheckErrorDomains()

	// Combine the taken, free and error domains
	domainJsonResults := slice.Concat(takenDomains, freeDomains, errorDomains)

	domainResults := utils.GetOrderedQueryResult(domainJsonResults)
	csvData, err := utils.ConvertQueryResultToCSV(domainResults)
	if err != nil {
		log.Error("Convert query result to csv error: ", err)
		return c.Status(500).SendString(err.Error())
	}

	// Add the utf8 bom data to the csv data
	csvData = append(utf8BomData, csvData...)

	log.Debug("Convert query result to csv success")
	log.Info("Download query result success")

	// Set the filename and content type
	filename := fmt.Sprintf("bulk_check_result_%s.csv", carbon.Now().ToShortDateTimeString())
	c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Set("Content-Type", "text/csv")

	// Send the csv data to the client
	return c.SendStream(bytes.NewReader(csvData))
}

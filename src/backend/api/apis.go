package api

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"typonamer/config"
	"typonamer/jobs"
	"typonamer/log"
	"typonamer/lookup/customize"
	"typonamer/register"
	"typonamer/scheduler"
	"typonamer/utils"

	"github.com/dromara/carbon/v2"
	"github.com/gofiber/fiber/v2"
)

// utf8BomData represents the UTF-8 Byte Order Mark (BOM) sequence.
// It is prepended to files to indicate UTF-8 encoding.
var utf8BomData = []byte{0xEF, 0xBB, 0xBF}

func AdminSettingList(c *fiber.Ctx) error {
	log.Info("Getting admin setting list success")
	return c.JSON(config.PublicSettings())
}

func WebSettingList(c *fiber.Ctx) error {
	log.Debug("Getting web setting list success")

	cfg := config.GetConfig()

	registerApis := make([]string, 0)
	whoisApis := make([]string, 0)
	if token, ok := strings.CutPrefix(c.Get("Authorization"), "Bearer "); ok && validAdminToken(token) {
		for _, api := range cfg.RegisterApis {
			registerApis = append(registerApis, api.ApiName)
		}
		for _, api := range cfg.WhoisApis {
			whoisApis = append(whoisApis, api.ApiName)
		}
	}

	c.Set("Cache-Control", "no-store")
	return c.JSON(fiber.Map{
		"webCheckDomainLimit": cfg.WebCheckDomainLimit,
		"typoDefaultCcTlds":   cfg.TypoDefaultCcTlds,
		"registerApis":        registerApis,
		"whoisApis":           whoisApis,
	})
}

func SettingUpdate(c *fiber.Ctx) error {
	newConfig := new(struct {
		config.Config
		SecretActions map[string]string `json:"secretActions"`
	})
	if err := c.BodyParser(newConfig); err != nil {
		// Error parsing the config info
		log.Error("Parse config info error: ", err)
		return c.Status(400).SendString("invalid settings payload")
	}

	err := config.UpdateConfig(newConfig.Config, newConfig.SecretActions)
	if err != nil {
		// Error updating the config
		log.Error("Update config error: ", err)
		return c.Status(400).SendString(err.Error())
	}

	// Update the custom whois API limiter
	customize.SetupLimiter()

	// Update the register API limiter
	register.SetupLimiter()

	// Update the config success
	log.Info("Update config success")

	return c.JSON(config.PublicSettings())
}

func DownloadLog(c *fiber.Ctx) error {
	filename := fmt.Sprintf("typonamer_logs_%s.zip", carbon.Now().ToShortDateTimeString())
	return sendDownload(c, filename, log.WriteZipLogs)
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
		return c.Status(400).SendString("missing upload file")
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
	if _, err := io.Copy(buffer, f); err != nil {
		return c.SendStatus(400)
	}

	// Add the raw domains to redis
	err = scheduler.BulkCheckAddRawDomains(buffer)
	if err != nil {
		log.Error("Bulk check domain save error: ", err)
		if errors.Is(err, jobs.ErrConflict) {
			return c.Status(409).SendString(err.Error())
		}
		return c.Status(500).SendString(err.Error())
	}

	log.Info("Bulk check domain save to redis success")

	return c.SendStatus(200)
}

func BulkCheckResultDownload(c *fiber.Ctx) error {
	ctx := c.UserContext()
	id := scheduler.CurrentTaskID()
	task, err := scheduler.Tasks.Get(ctx, id)
	if err != nil {
		return taskError(c, err)
	}
	filename := fmt.Sprintf("bulk_check_result_%s.csv", carbon.Now().ToShortDateTimeString())
	return sendDownload(c, filename, func(w io.Writer) error {
		if _, err := w.Write(utf8BomData); err != nil {
			return err
		}
		header, err := utils.ConvertQueryResultToCSV(nil)
		if err != nil {
			return err
		}
		if _, err := w.Write(header); err != nil {
			return err
		}
		stable := task.State == "completed" || task.State == "canceled"
		written := 0
		for offset := 0; offset >= 0 && !(stable && written == task.Completed); {
			page, err := scheduler.Tasks.Results(ctx, id, offset, 1000, false)
			if err != nil {
				return err
			}
			data, err := utils.ConvertQueryResultToCSV(page.Items)
			if err != nil {
				return err
			}
			if _, err = w.Write(bytes.TrimPrefix(data, header)); err != nil {
				return err
			}
			offset = page.NextOffset
			written += len(page.Items)
		}
		return nil
	})
}

type tempDownload struct{ *os.File }

func (f tempDownload) Close() error {
	return errors.Join(f.File.Close(), os.Remove(f.Name()))
}

// Stage a private file so failures can still return an error before HTTP headers are sent.
func sendDownload(c *fiber.Ctx, name string, write func(io.Writer) error) error {
	file, err := os.CreateTemp("", "powerwhois-download-*")
	if err != nil {
		return fiber.ErrInternalServerError
	}
	stream := tempDownload{file}
	if err = write(file); err == nil {
		_, err = file.Seek(0, io.SeekStart)
	}
	if err != nil {
		stream.Close()
		log.Error("Cannot prepare download: ", err)
		return fiber.ErrInternalServerError
	}
	c.Attachment(name)
	c.Set("Cache-Control", "no-store")
	return c.SendStream(stream)
}

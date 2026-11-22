package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"os"
	"strings"
	"time"

	"typonamer/config"
	"typonamer/log"
	"typonamer/register"
	"typonamer/scheduler"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

func readJSON(c *fiber.Ctx, dst any, size int64) error {
	if v := c.Get(fiber.HeaderContentEncoding); v != "" && !strings.EqualFold(v, "identity") {
		return fiber.ErrUnsupportedMediaType
	}
	if int64(c.Context().Request.Header.ContentLength()) > size {
		return fiber.ErrRequestEntityTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(body(c), size+1))
	if int64(len(data)) > size {
		return fiber.ErrRequestEntityTooLarge
	}
	if err != nil || json.Unmarshal(data, dst) != nil {
		return fiber.ErrBadRequest
	}
	return nil
}

func body(c *fiber.Ctx) io.Reader {
	if r := c.Context().RequestBodyStream(); r != nil {
		return r
	}
	return bytes.NewReader(c.BodyRaw())
}

func AdminSettingList(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	return c.JSON(config.PublicSettings(config.GetConfig()))
}

func WebSettingList(c *fiber.Ctx) error {
	cfg := config.GetConfig()
	registrars, providers := []string{}, []string{}
	scheme, token, _ := strings.Cut(c.Get("Authorization"), " ")
	if strings.EqualFold(scheme, "Bearer") {
		if _, err := tokenExpiry(token); err == nil {
			for _, p := range cfg.RegisterApis {
				registrars = append(registrars, p.ApiName)
			}
			for _, p := range cfg.WhoisApis {
				providers = append(providers, p.ApiName)
			}
		}
	}
	c.Set("Cache-Control", "no-store")
	return c.JSON(fiber.Map{
		"webCheckDomainLimit": cfg.WebCheckDomainLimit,
		"typoDefaultCcTlds":   cfg.TypoDefaultCcTlds,
		"registerApis":        registrars,
		"whoisApis":           providers,
	})
}

func SettingUpdate(c *fiber.Ctx) error {
	var next config.Config
	if err := readJSON(c, &next, 64<<10); err != nil {
		return err
	}
	safe, err := config.UpdateConfig(next)
	if errors.Is(err, config.ErrWriteConfig) {
		return fiber.ErrInternalServerError
	}
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	c.Set("Cache-Control", "no-store")
	return c.JSON(safe)
}

func DownloadLog(c *fiber.Ctx) error {
	file, err := os.CreateTemp("", "powerwhois-logs-*.zip")
	if err != nil {
		return fiber.ErrInternalServerError
	}
	path := file.Name()
	file.Close()
	if err := log.ZipLogs(path); err != nil {
		os.Remove(path)
		return fiber.ErrInternalServerError
	}
	archive, err := os.Open(path)
	if err != nil {
		os.Remove(path)
		return fiber.ErrInternalServerError
	}
	return download(c, archive, "application/zip", "typonamer_logs_"+time.Now().Format("20060102_150405")+".zip")
}

func ResetLog(c *fiber.Ctx) error {
	if err := log.ResetLogsFile(); err != nil {
		return fiber.ErrInternalServerError
	}
	return c.SendStatus(fiber.StatusOK)
}

func BulkCheckDomainUpload(bulk *scheduler.BulkCheck) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if bulk == nil {
			return fiber.ErrServiceUnavailable
		}
		const maxBody = scheduler.MaxUploadBytes + (1 << 20)
		if c.Context().Request.Header.ContentLength() > maxBody {
			return fiber.ErrRequestEntityTooLarge
		}
		media, attrs, err := mime.ParseMediaType(c.Get(fiber.HeaderContentType))
		if err != nil || media != "multipart/form-data" || attrs["boundary"] == "" {
			return fiber.ErrBadRequest
		}
		reader := multipart.NewReader(io.LimitReader(body(c), maxBody), attrs["boundary"])
		var part *multipart.Part
		for {
			part, err = reader.NextPart()
			if err != nil {
				return fiber.ErrBadRequest
			}
			if part.FormName() == "file" && part.FileName() != "" {
				break
			}
		}
		ctx, cancel := context.WithTimeout(c.UserContext(), 2*time.Minute)
		defer cancel()
		if err := bulk.Upload(ctx, part); err != nil {
			switch {
			case errors.Is(err, scheduler.ErrBulkState):
				return fiber.NewError(fiber.StatusConflict, err.Error())
			case errors.Is(err, scheduler.ErrUploadSize):
				return fiber.NewError(fiber.StatusRequestEntityTooLarge, err.Error())
			default:
				log.Error("Bulk upload failed: ", err)
				return fiber.ErrInternalServerError
			}
		}
		return c.SendStatus(fiber.StatusOK)
	}
}

func BulkCheckResultDownload(bulk *scheduler.BulkCheck) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if bulk == nil {
			return fiber.ErrServiceUnavailable
		}
		file, err := os.CreateTemp("", "powerwhois-results-*.csv")
		if err != nil {
			return fiber.ErrInternalServerError
		}
		ctx, cancel := context.WithTimeout(c.UserContext(), 2*time.Minute)
		defer cancel()
		err = bulk.Export(ctx, file)
		if err == nil {
			_, err = file.Seek(0, io.SeekStart)
		}
		if err != nil {
			file.Close()
			os.Remove(file.Name())
			if errors.Is(err, scheduler.ErrBulkState) {
				return fiber.NewError(fiber.StatusConflict, err.Error())
			}
			log.Error("Bulk export failed: ", err)
			return fiber.ErrInternalServerError
		}
		return download(c, file, "text/csv; charset=utf-8", "bulk_check_result_"+time.Now().Format("20060102_150405")+".csv")
	}
}

type temporaryDownload struct{ *os.File }

func (t temporaryDownload) Close() error {
	err := t.File.Close()
	os.Remove(t.File.Name())
	return err
}

func download(c *fiber.Ctx, file *os.File, media, name string) error {
	c.Set(fiber.HeaderContentType, media)
	c.Set(fiber.HeaderContentDisposition, fmt.Sprintf("attachment; filename=%q", name))
	c.Set("Cache-Control", "no-store")
	return c.SendStream(temporaryDownload{file})
}

func ReconcileRegistration(client *redis.Client) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if client == nil {
			return fiber.ErrServiceUnavailable
		}
		var form struct {
			Domain  string `json:"domain"`
			Outcome string `json:"outcome"`
		}
		if err := readJSON(c, &form, 8<<10); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(c.UserContext(), 3*time.Second)
		defer cancel()
		info, err := register.Reconcile(ctx, client, form.Domain, c.Params("operation"), form.Outcome)
		if errors.Is(err, register.ErrReconcile) {
			return fiber.NewError(fiber.StatusConflict, err.Error())
		}
		if err != nil {
			return fiber.ErrInternalServerError
		}
		c.Set("Cache-Control", "no-store")
		return c.JSON(info)
	}
}

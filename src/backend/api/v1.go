package api

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"github.com/gofiber/fiber/v2"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"typonamer/jobs"
	"typonamer/lookup/lookupinfo"
	"typonamer/scheduler"
	"typonamer/typo"
	"typonamer/utils"
)

func apiError(c *fiber.Ctx, status int, code, message string) error {
	return c.Status(status).JSON(fiber.Map{"error": fiber.Map{"code": code, "message": message}})
}
func QueryAuth(c *fiber.Ctx) error {
	token := strings.TrimPrefix(c.Get("Authorization"), "Bearer ")
	if token == "" || token == c.Get("Authorization") {
		return apiError(c, 401, "unauthorized", "Bearer token required")
	}
	expected := os.Getenv("POWERWHOIS_QUERY_TOKEN")
	if len(expected) >= 32 && subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1 {
		return c.Next()
	}
	valid, _ := ValidateToken(token)
	if valid {
		return c.Next()
	}
	return apiError(c, 401, "unauthorized", "invalid or expired credential")
}
func decode(c *fiber.Ctx, v interface{}) error {
	d := json.NewDecoder(bytes.NewReader(c.Body()))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	if d.Decode(&struct{}{}) != io.EOF {
		return errors.New("expected one JSON object")
	}
	return nil
}
func taskError(c *fiber.Ctx, err error) error {
	if errors.Is(err, jobs.ErrResultTooLarge) {
		return apiError(c, 413, "result_too_large", err.Error())
	}
	if errors.Is(err, jobs.ErrNoRetryResults) || errors.Is(err, jobs.ErrInvalidRequest) {
		return apiError(c, 400, "invalid_request", err.Error())
	}
	if errors.Is(err, jobs.ErrNotFound) {
		return apiError(c, 404, "not_found", "task not found")
	}
	if errors.Is(err, jobs.ErrConflict) {
		return apiError(c, 409, "conflict", err.Error())
	}
	return apiError(c, 503, "storage_unavailable", "task storage unavailable; retry with the same idempotency key")
}
func pageParams(c *fiber.Ctx) (int, int, error) {
	offset, e := strconv.Atoi(c.Query("offset", "0"))
	if e != nil || offset < 0 {
		return 0, 0, errors.New("invalid offset")
	}
	limit, e := strconv.Atoi(c.Query("limit", "100"))
	if e != nil || limit < 1 || limit > 1000 {
		return 0, 0, errors.New("limit must be 1..1000")
	}
	return offset, limit, nil
}
func V1Routes(router fiber.Router) {
	router.Get("/openapi.json", func(c *fiber.Ctx) error { return c.Type("json").Send(openAPISpec) })
	v := router.Group("/v1", QueryAuth)
	v.Post("/query", func(c *fiber.Ctx) error {
		var r jobs.Request
		if err := decode(c, &r); err != nil {
			return apiError(c, 400, "invalid_request", err.Error())
		}
		if len(r.Domains) > 100 {
			return apiError(c, 400, "invalid_request", "use tasks for more than 100 domains")
		}
		normalized, err := jobs.Normalize(r)
		if err != nil {
			return apiError(c, 400, "invalid_request", err.Error())
		}
		r = normalized
		ctx, cancel := context.WithTimeout(c.UserContext(), 120*time.Second)
		defer cancel()
		result := make([]lookupinfo.QueryResult, len(r.Domains))
		var responseBytes atomic.Int64
		var responseTooLarge atomic.Bool
		var wg sync.WaitGroup
		work := make(chan int)
		for i := 0; i < min(r.Concurrency, len(r.Domains)); i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for index := range work {
					item := scheduler.Tasks.Query(ctx, r.Domains[index], r.Mode, r.Timeout, r.Retries)
					item.Order = index
					// Full evidence is retrieved from durable tasks; synchronous responses stay bounded.
					if len(item.RawResponse) > 65536 {
						item.RawResponse = item.RawResponse[:65536]
						item.RawResponseTruncated = true
					}
					encoded, _ := json.Marshal(item)
					if responseBytes.Add(int64(len(encoded))) > 8*1024*1024 {
						responseTooLarge.Store(true)
						cancel()
						continue
					}
					result[index] = item
				}
			}()
		}
		for i := range r.Domains {
			work <- i
		}
		close(work)
		wg.Wait()
		if responseTooLarge.Load() {
			return apiError(c, 413, "response_too_large", "use durable tasks and paged results for large responses")
		}
		return c.JSON(fiber.Map{"items": result})
	})
	v.Post("/typo", func(c *fiber.Ctx) error {
		var r typo.Request
		if err := decode(c, &r); err != nil {
			return apiError(c, 400, "invalid_request", err.Error())
		}
		result, err := typo.Generate(r)
		if err != nil {
			return apiError(c, 400, "invalid_request", err.Error())
		}
		return c.JSON(result)
	})
	v.Post("/tasks", func(c *fiber.Ctx) error {
		var r jobs.Request
		if err := decode(c, &r); err != nil {
			return apiError(c, 400, "invalid_request", err.Error())
		}
		key := c.Get("Idempotency-Key")
		t, reused, err := scheduler.Tasks.Create(c.UserContext(), r, key)
		if err != nil {
			return taskError(c, err)
		}
		code := 202
		if reused {
			code = 200
		}
		return c.Status(code).JSON(t)
	})
	v.Get("/tasks", func(c *fiber.Ctx) error {
		offset, limit, err := pageParams(c)
		if err != nil {
			return apiError(c, 400, "invalid_request", err.Error())
		}
		tasks, err := scheduler.Tasks.List(c.UserContext(), offset, limit)
		if err != nil {
			return taskError(c, err)
		}
		next := -1
		if len(tasks) == limit {
			next = offset + limit
		}
		return c.JSON(fiber.Map{"items": tasks, "offset": offset, "nextOffset": next})
	})
	v.Get("/tasks/:id", func(c *fiber.Ctx) error {
		t, err := scheduler.Tasks.Get(c.UserContext(), c.Params("id"))
		if err != nil {
			return taskError(c, err)
		}
		return c.JSON(t)
	})
	v.Get("/tasks/:id/results", func(c *fiber.Ctx) error {
		offset, limit, err := pageParams(c)
		if err != nil {
			return apiError(c, 400, "invalid_request", err.Error())
		}
		p, err := scheduler.Tasks.Results(c.UserContext(), c.Params("id"), offset, limit, c.QueryBool("includeRaw", false))
		if err != nil {
			return taskError(c, err)
		}
		return c.JSON(p)
	})
	v.Get("/tasks/:id/results/:index", func(c *fiber.Ctx) error {
		index, e := strconv.Atoi(c.Params("index"))
		if e != nil || index < 0 {
			return apiError(c, 400, "invalid_request", "invalid result index")
		}
		p, e := scheduler.Tasks.Results(c.UserContext(), c.Params("id"), index, 1)
		if e != nil {
			return taskError(c, e)
		}
		if len(p.Items) == 0 {
			return apiError(c, 404, "not_found", "result not completed")
		}
		return c.JSON(p.Items[0])
	})
	v.Post("/tasks/:id/:action", func(c *fiber.Ctx) error {
		action := c.Params("action")
		var t jobs.Task
		var err error
		if action == "retry" {
			t, err = scheduler.Tasks.RetryFailed(c.UserContext(), c.Params("id"))
		} else if action == "pause" || action == "resume" || action == "cancel" {
			t, err = scheduler.Tasks.Control(c.UserContext(), c.Params("id"), action)
		} else {
			return apiError(c, 404, "not_found", "unknown task action")
		}
		if err != nil {
			return taskError(c, err)
		}
		return c.JSON(t)
	})
	v.Get("/tasks/:id/export", func(c *fiber.Ctx) error {
		format := c.Query("format", "json")
		if format != "json" && format != "csv" {
			return apiError(c, 400, "invalid_request", "format must be json or csv")
		}
		t, err := scheduler.Tasks.Get(c.UserContext(), c.Params("id"))
		if err != nil {
			return taskError(c, err)
		}
		// Export is paged too: bounded memory and a stable input-index cursor during active tasks.
		offset, limit, err := pageParams(c)
		if err != nil {
			return apiError(c, 400, "invalid_request", err.Error())
		}
		p, err := scheduler.Tasks.Results(c.UserContext(), t.ID, offset, limit, format != "csv")
		if err != nil {
			return taskError(c, err)
		}
		c.Set("X-Task-State", t.State)
		c.Set("X-Export-Consistency", "partial")
		if t.State == "completed" {
			c.Set("X-Export-Consistency", "complete")
		}
		c.Set("X-Next-Offset", strconv.Itoa(p.NextOffset))
		c.Set("X-Result-Count", strconv.Itoa(len(p.Items)))
		c.Set("X-Task-Completed", strconv.Itoa(t.Completed))
		c.Set("Content-Disposition", `attachment; filename="powerwhois-`+t.ID+`.`+format+`"`)
		if format == "json" {
			return c.JSON(p)
		}
		csv, err := utils.ConvertQueryResultToCSV(p.Items)
		if err != nil {
			return apiError(c, 500, "export_failed", "could not encode CSV")
		}
		return c.Type("csv").Send(csv)
	})
	router.Post("/admin/tasks/:id/select", LoginRequired(), func(c *fiber.Ctx) error {
		if err := scheduler.SelectTask(c.Params("id")); err != nil {
			return taskError(c, err)
		}
		return c.SendStatus(204)
	})
}

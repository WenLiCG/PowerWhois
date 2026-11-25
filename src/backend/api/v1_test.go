package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
	"typonamer/internal/testutil"
	"typonamer/jobs"
	"typonamer/lookup/lookuper"
	"typonamer/lookup/lookuperror"
	"typonamer/lookup/lookupinfo"
	"typonamer/scheduler"
	"typonamer/utils"
)

func TestQueryAPIAndTasks(t *testing.T) {
	r := testutil.Redis(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scheduler.Init(ctx, r)
	scheduler.Tasks.Query = func(ctx context.Context, d, m string, timeout, retries int) lookupinfo.QueryResult {
		info := lookupinfo.DomainInfo{LookupType: "dns", Source: "fixture"}
		return lookuper.Result(d, info, lookuperror.ErrorNsNotFound)
	}
	done := make(chan error, 1)
	go func() { done <- scheduler.Tasks.Run(ctx) }()
	select {
	case <-scheduler.Tasks.Ready:
	case <-time.After(3 * time.Second):
		t.Fatal("worker startup")
	}
	defer func() {
		cancel()
		if e := <-done; e != nil {
			t.Error(e)
		}
	}()
	token := "test-query-token-with-at-least-32-characters"
	t.Setenv("POWERWHOIS_QUERY_TOKEN", token)
	app := fiber.New()
	app.Route("/api", ApiRoute)
	var lastHeaders map[string][]string
	request := func(method, path string, body interface{}, authorized bool, key string) (int, []byte) {
		t.Helper()
		data, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		if authorized {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		req.Header.Set("Idempotency-Key", key)
		resp, e := app.Test(req, 5000)
		if e != nil {
			t.Fatal(e)
		}
		defer resp.Body.Close()
		lastHeaders = resp.Header
		raw, e := io.ReadAll(resp.Body)
		if e != nil {
			t.Fatal(e)
		}
		return resp.StatusCode, raw
	}
	if code, _ := request("GET", "/api/v1/tasks", nil, false, ""); code != 401 {
		t.Fatalf("anonymous access %d", code)
	}
	if code, _ := request("GET", "/api/admin/setting", nil, true, ""); code != 401 {
		t.Fatalf("query token elevated %d", code)
	}
	req := jobs.Request{Domains: []string{"example.com", "example.org"}, Mode: "dns"}
	code, raw := request("POST", "/api/v1/query", req, true, "")
	if code != 200 {
		t.Fatalf("query %d %s", code, raw)
	}
	var direct struct {
		Items []lookupinfo.QueryResult `json:"items"`
	}
	json.Unmarshal(raw, &direct)
	if len(direct.Items) != 2 || direct.Items[0].Status != "unknown" {
		t.Fatalf("DNS misclassified: %s", raw)
	}
	code, raw = request("POST", "/api/v1/tasks", req, true, "once")
	var task jobs.Task
	json.Unmarshal(raw, &task)
	if code != 202 || task.ID == "" {
		t.Fatalf("create %d %s", code, raw)
	}
	code, replay := request("POST", "/api/v1/tasks", req, true, "once")
	var same jobs.Task
	json.Unmarshal(replay, &same)
	if code != 200 || same.ID != task.ID {
		t.Fatal("idempotency replay")
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		current, e := scheduler.Tasks.Get(ctx, task.ID)
		if e == nil && current.State == "completed" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	code, raw = request("GET", "/api/v1/tasks/"+task.ID+"/export?limit=100", nil, true, "")
	var page jobs.Page
	json.Unmarshal(raw, &page)
	if code != 200 || len(page.Items) != 2 {
		t.Fatalf("export %d %s", code, raw)
	}
	for i, item := range page.Items {
		if item.Domain != direct.Items[i].Domain || item.Status != direct.Items[i].Status || item.Source != direct.Items[i].Source {
			t.Fatal("query and task differ")
		}
	}
	code, csv := request("GET", "/api/v1/tasks/"+task.ID+"/export?format=csv", nil, true, "")
	if code != 200 || !bytes.Contains(csv, []byte("Queried At")) || !bytes.Contains(csv, []byte("unknown")) {
		t.Fatalf("CSV %d %s", code, csv)
	}
	if strings.Join(lastHeaders["X-Result-Count"], "") != "2" || strings.Join(lastHeaders["X-Task-Completed"], "") != "2" {
		t.Fatal("export result counts missing", lastHeaders)
	}
	if code, _ := request("GET", "/api/v1/tasks?limit=-1", nil, true, ""); code != 400 {
		t.Fatal("invalid pagination accepted")
	}
	if code, _ := request("POST", "/api/v1/tasks", map[string]interface{}{"domains": []string{"example.com"}, "register": true}, true, ""); code != 400 {
		t.Fatal("unknown field accepted")
	}
	if code, _ := request("POST", "/api/v1/tasks", jobs.Request{Domains: []string{"invalid"}}, true, ""); code != 400 {
		t.Fatal("invalid domain must return an input error")
	}
	if code, _ := request("POST", "/api/v1/typo", map[string]interface{}{"domain": "example.com", "types": []string{"skipLetter"}}, true, ""); code != 200 {
		t.Fatal("typo failed")
	}
	code, spec := request("GET", "/api/openapi.json", nil, false, "")
	if code != 200 || !json.Valid(spec) {
		t.Fatal("invalid OpenAPI")
	}
	if code, data := request("GET", "/api/v1/tasks/"+task.ID+"/results/0", nil, true, ""); code != 200 || !bytes.Contains(data, []byte("example.com")) {
		t.Fatal("individual evidence unavailable", code)
	}
	scheduler.Tasks.Query = func(context.Context, string, string, int, int) lookupinfo.QueryResult {
		return lookupinfo.QueryResult{Domain: "example.com", Status: "registered", RawResponse: strings.Repeat("x", 200000)}
	}
	code, raw = request("POST", "/api/v1/query", req, true, "")
	json.Unmarshal(raw, &direct)
	if code != 200 || !direct.Items[0].RawResponseTruncated || len(direct.Items[0].RawResponse) != 65536 {
		t.Fatal("unbounded synchronous evidence")
	}
	scheduler.Tasks.Query = func(context.Context, string, string, int, int) lookupinfo.QueryResult {
		return lookupinfo.QueryResult{Domain: "example.com", Status: "registered", NameServer: []string{strings.Repeat("x", 9*1024*1024)}}
	}
	if code, _ := request("POST", "/api/v1/query", req, true, ""); code != 413 {
		t.Fatal("oversized synchronous response accepted", code)
	}
}

func TestLegacyCSVExportAcrossPages(t *testing.T) {
	r := testutil.Redis(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scheduler.Init(ctx, r)
	domains := make([]string, 1001)
	for i := range domains {
		domains[i] = fmt.Sprintf("row%d.com", i)
	}
	task, _, err := scheduler.Tasks.Create(ctx, jobs.Request{Domains: domains}, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := scheduler.SelectTask(task.ID); err != nil {
		t.Fatal(err)
	}
	items := []lookupinfo.QueryResult{}
	key := scheduler.Tasks.Prefix + "task:" + task.ID
	for _, index := range []int{0, 1000} {
		item := lookupinfo.QueryResult{Domain: domains[index], Order: index, Status: "error", QueryError: "first line\nsecond line"}
		data, _ := json.Marshal(item)
		if err := r.HSet(ctx, key+":results", index, data).Err(); err != nil {
			t.Fatal(err)
		}
		items = append(items, item)
	}
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	app := fiber.New()
	app.Get("/export", BulkCheckResultDownload)
	resp, err := app.Test(httptest.NewRequest("GET", "/export", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	want, _ := utils.ConvertQueryResultToCSV(items)
	if err != nil || resp.StatusCode != 200 || !bytes.Equal(data, append(utf8BomData, want...)) {
		t.Fatalf("CSV across two pages differs: %d, %v", resp.StatusCode, err)
	}
	if files, err := os.ReadDir(dir); err != nil || len(files) != 0 {
		t.Fatalf("temporary download left behind: %v, %v", files, err)
	}
}

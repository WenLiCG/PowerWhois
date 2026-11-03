package api

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/gofiber/fiber/v2"
	"io"
	"net/http/httptest"
	"testing"
	"time"
	"typonamer/internal/testutil"
	"typonamer/jobs"
	"typonamer/lookup/lookuper"
	"typonamer/lookup/lookuperror"
	"typonamer/lookup/lookupinfo"
	"typonamer/scheduler"
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
	if code, _ := request("GET", "/api/v1/tasks?limit=-1", nil, true, ""); code != 400 {
		t.Fatal("invalid pagination accepted")
	}
	if code, _ := request("POST", "/api/v1/tasks", map[string]interface{}{"domains": []string{"example.com"}, "register": true}, true, ""); code != 400 {
		t.Fatal("unknown field accepted")
	}
	if code, _ := request("POST", "/api/v1/typo", map[string]interface{}{"domain": "example.com", "types": []string{"skipLetter"}}, true, ""); code != 200 {
		t.Fatal("typo failed")
	}
	code, spec := request("GET", "/api/openapi.json", nil, false, "")
	if code != 200 || !json.Valid(spec) {
		t.Fatal("invalid OpenAPI")
	}
}

package command_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
	"typonamer/command"
	"typonamer/workflow"
)

type provider struct{}

func TestServerRejectsNonLoopbackBeforeOpeningDatabase(t *testing.T) {
	for _, listen := range []string{"0.0.0.0:8080", "[::]:8080", "192.0.2.1:8080", "example.com:8080"} {
		t.Run(listen, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "must-not-create.db")
			var out, stderr bytes.Buffer
			if code := command.Run([]string{"serve", "--listen", listen, "--data", path}, &out, &stderr); code != 2 {
				t.Fatalf("nonloopback listen accepted: %d", code)
			}
			if _, e := os.Stat(path); !os.IsNotExist(e) {
				t.Fatal("listener validation happened after side effects")
			}
		})
	}
}

func TestCLIRejectsRedirects(t *testing.T) {
	var followed atomic.Bool
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { followed.Store(true) }))
	defer destination.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	var out, stderr bytes.Buffer
	if code := command.Run([]string{"jobs", "--url", redirect.URL}, &out, &stderr); code != 1 || followed.Load() {
		t.Fatalf("redirect was followed or not rejected: exit=%d followed=%v", code, followed.Load())
	}
}

func (provider) Lookup(context.Context, string) workflow.Result {
	return workflow.Result{Status: "registered", Provider: "synthetic integration provider", CheckedAt: time.Now().UTC()}
}

func TestCLIAndAPISameJob(t *testing.T) {
	dir := t.TempDir()
	store, e := workflow.Open(filepath.Join(dir, "jobs.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	svc, e := workflow.NewService(store, provider{}, 2)
	if e != nil {
		t.Fatal(e)
	}
	defer svc.Close()
	server := httptest.NewServer(svc.Handler(""))
	defer server.Close()
	f := excelize.NewFile()
	_ = f.SetCellStr("Sheet1", "B1", "Domain")
	_ = f.SetCellStr("Sheet1", "B2", "example.com")
	_ = f.SetCellStr("Sheet1", "B3", "EXAMPLE.COM")
	_ = f.SetCellStr("Sheet1", "B4", "-example.com")
	input := filepath.Join(dir, "input.xlsx")
	if e = f.SaveAs(input); e != nil {
		t.Fatal(e)
	}
	f.Close()
	run := func(args ...string) []byte {
		t.Helper()
		var out, err bytes.Buffer
		args = append(args, "--url", server.URL)
		if code := command.Run(args, &out, &err); code != 0 {
			t.Fatalf("%v exit=%d: %s", args, code, err.String())
		}
		return out.Bytes()
	}
	var j workflow.Job
	if e = json.Unmarshal(run("import", "--file", input, "--header"), &j); e != nil {
		t.Fatal(e)
	}
	if j.UniqueSeeds != 1 || j.InvalidRows != 1 {
		t.Fatal("CLI import differs")
	}
	if e = json.Unmarshal(run("generate", "--job", j.ID, "--rules", "omission", "--include-seeds"), &j); e != nil {
		t.Fatal(e)
	}
	run("start", "--job", j.ID)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if e = json.Unmarshal(run("status", "--job", j.ID), &j); e != nil {
			t.Fatal(e)
		}
		if j.State == "completed" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if j.State != "completed" {
		t.Fatal("job did not complete")
	}
	apiJob, e := store.Get(j.ID)
	if e != nil || apiJob.Completed != j.Completed || j.Completed != j.Candidates {
		t.Fatal("CLI/API count mismatch")
	}
	run("results", "--job", j.ID, "--limit", "2")
	destination := filepath.Join(dir, "report.xlsx")
	run("export", "--job", j.ID, "--file", destination, "--denominator", "known")
	xlsx, e := excelize.OpenFile(destination)
	if e != nil {
		t.Fatal(e)
	}
	defer xlsx.Close()
	rows, e := xlsx.GetRows("Results")
	if e != nil || len(rows) != j.Candidates+1 {
		t.Fatal("XLSX count mismatch")
	}
	var stdout, stderr bytes.Buffer
	if code := command.Run([]string{"status"}, &stdout, &stderr); code != 2 {
		t.Fatal("missing args exit code")
	}
	if _, e = os.Stat(destination); e != nil {
		t.Fatal(e)
	}
}

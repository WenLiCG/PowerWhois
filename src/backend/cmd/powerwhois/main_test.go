package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCLIContract(t *testing.T) {
	token := "test-query-token-with-at-least-32-characters"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Error("missing authentication")
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/tasks/exported/export" {
			w.Header().Set("Content-Type", "text/csv")
			offset := r.URL.Query().Get("offset")
			if next, ok := map[string]string{"0": "37", "37": "-1", "38": "oops", "39": "39"}[offset]; ok {
				w.Header().Set("X-Next-Offset", next)
			}
			w.Write([]byte("Domain\nexample.com\n"))
			return
		}
		if r.URL.Path == "/api/v1/tasks/missing" {
			w.WriteHeader(404)
			w.Write([]byte(`{"error":{"code":"not_found","message":"task not found"}}`))
			return
		}
		if r.Method == "POST" {
			var body struct {
				Domains     []string `json:"domains"`
				Concurrency int      `json:"concurrency"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			if len(body.Domains) != 2 {
				t.Errorf("stdin parse: %v", body.Domains)
			}
			if body.Concurrency != 0 {
				t.Errorf("CLI overrode server default concurrency: %d", body.Concurrency)
			}
			if r.Header.Get("Idempotency-Key") != "reconnect" {
				t.Error("lost idempotency key")
			}
		}
		w.Write([]byte(`{"id":"same-task","state":"queued"}`))
	}))
	defer server.Close()
	t.Setenv("POWERWHOIS_URL", server.URL)
	t.Setenv("POWERWHOIS_QUERY_TOKEN", token)
	var out, errOut bytes.Buffer
	if code := run([]string{"tasks", "create", "--file", "-", "--key", "reconnect"}, bytes.NewBufferString("example.com\nexample.org\n"), &out, &errOut); code != 0 || !json.Valid(out.Bytes()) || errOut.Len() != 0 {
		t.Fatalf("stdout contract: %d %s %s", code, out.String(), errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if code := run([]string{"tasks", "get", "--id", "missing"}, nil, &out, &errOut); code != 4 || out.Len() != 0 || errOut.Len() == 0 {
		t.Fatal("exit/error contract")
	}
	offset := "0"
	for _, next := range []string{"37", "-1"} {
		out.Reset()
		errOut.Reset()
		if code := run([]string{"tasks", "export", "--id", "exported", "--format", "csv", "--offset", offset}, nil, &out, &errOut); code != 0 || out.String() != "Domain\nexample.com\n" || errOut.String() != "nextOffset="+next+"\n" {
			t.Fatalf("CSV or cursor differs: exit %d, stdout %q, stderr %q", code, out.String(), errOut.String())
		}
		offset = strings.TrimPrefix(strings.TrimSpace(errOut.String()), "nextOffset=")
	}
	for _, bad := range []string{"38", "39", "40"} {
		out.Reset()
		errOut.Reset()
		if code := run([]string{"tasks", "export", "--id", "exported", "--format", "csv", "--offset", bad}, nil, &out, &errOut); code != 1 || out.Len() != 0 || !strings.Contains(errOut.String(), "cursor") {
			t.Fatalf("unusable CSV cursor accepted at %s", bad)
		}
	}
}

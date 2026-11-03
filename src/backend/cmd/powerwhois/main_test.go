package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCLIContract(t *testing.T) {
	token := "test-query-token-with-at-least-32-characters"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Error("missing authentication")
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/tasks/missing" {
			w.WriteHeader(404)
			w.Write([]byte(`{"error":{"code":"not_found","message":"task not found"}}`))
			return
		}
		if r.Method == "POST" {
			var body struct {
				Domains []string `json:"domains"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			if len(body.Domains) != 2 {
				t.Errorf("stdin parse: %v", body.Domains)
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
}

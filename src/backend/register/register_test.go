package register

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"typonamer/config"
	"typonamer/internal/testutil"
)

func TestRegistrationUsesMocksAndDoesNotFollowRedirect(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch r.URL.Path {
		case "/redirect":
			w.Header().Set("Location", "/success")
			w.WriteHeader(302)
		case "/failure":
			w.WriteHeader(500)
			w.Write([]byte("SUCCESS"))
		default:
			w.Write([]byte("SUCCESS"))
		}
	}))
	defer server.Close()
	if body, e := sendRegisterRequest(context.Background(), server.URL+"/success"); e != nil || body != "SUCCESS" {
		t.Fatal(body, e)
	}
	for _, path := range []string{"/redirect", "/failure"} {
		if _, e := sendRegisterRequest(context.Background(), server.URL+path+"?key=secret"); e == nil || strings.Contains(e.Error(), "secret") {
			t.Fatal("unsafe HTTP error", e)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := sendRegisterRequest(ctx, server.URL); e == nil {
		t.Fatal("canceled request sent")
	}
	if calls.Load() != 3 {
		t.Fatalf("unexpected retry/redirect: %d", calls.Load())
	}
}

func TestGETTransactionIsNotReplayedAfterUpstreamAcceptsAndDisconnects(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 2 {
			conn, _, e := w.(http.Hijacker).Hijack()
			if e != nil {
				t.Error(e)
				return
			}
			conn.Close()
			return
		}
		w.Write([]byte("SUCCESS"))
	}))
	defer server.Close()
	if _, e := sendRegisterRequest(context.Background(), server.URL); e != nil {
		t.Fatal(e)
	}
	if _, e := sendRegisterRequest(context.Background(), server.URL); e == nil {
		t.Fatal("uncertain transaction reported success")
	}
	if calls.Load() != 2 {
		t.Fatalf("transaction replayed: %d requests", calls.Load())
	}
}

func TestRegistrationPersistsIntentAndBlocksDuplicateSubmission(t *testing.T) {
	r := testutil.Redis(t)
	SetAuditStore(r)
	t.Cleanup(func() { SetAuditStore(nil) })
	path := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(path, []byte("logLevel: Error\n"), 0600)
	t.Setenv("POWERWHOIS_ADMIN_USER", "fixture")
	t.Setenv("POWERWHOIS_ADMIN_PASSWORD", "fixture-password-strong")
	t.Setenv("POWERWHOIS_JWT_SECRET", strings.Repeat("fixture!", 8))
	if e := config.Load(path); e != nil {
		t.Fatal(e)
	}
	var calls atomic.Int32
	accepted := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Idempotency-Key") == "" {
			t.Error("missing registrar idempotency key")
		}
		if r.URL.Query().Get("domain") == "slow.com" {
			close(accepted)
			<-r.Context().Done()
			return
		}
		w.Write([]byte("SUCCESS"))
	}))
	defer server.Close()
	c := config.GetConfig()
	c.RegisterApis = []config.RegisterApi{{ApiName: "fixture", ApiUrl: server.URL + "/?domain={domain}", SuccessText: []string{"SUCCESS"}, FailText: []string{"FAILED"}, ConcurrencyLimit: 1}}
	if e := config.UpdateConfig(c); e != nil {
		t.Fatal(e)
	}
	SetupLimiter()
	result, e := RegisterContext(context.Background(), "example.com", "fixture")
	if e != nil || result.RegisterStatus != "success" {
		t.Fatal(result, e)
	}
	data, e := r.Get(context.Background(), "powerwhois:registration:"+result.AuditID).Bytes()
	if e != nil {
		t.Fatal(e)
	}
	var record map[string]string
	json.Unmarshal(data, &record)
	if record["status"] != "success" || record["domain"] != "example.com" || strings.Contains(string(data), server.URL) {
		t.Fatal("invalid audit record")
	}
	if duplicate, e := RegisterContext(context.Background(), "example.com", "fixture"); e == nil || duplicate.RegisterStatus != "outcome_unknown" || calls.Load() != 1 {
		t.Fatal("duplicate transaction was sent")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan struct{})
	go func() { result, e = RegisterContext(ctx, "slow.com", "fixture"); close(finished) }()
	select {
	case <-accepted:
	case <-time.After(3 * time.Second):
		t.Fatal("mock registrar never received request")
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("registration did not cancel")
	}
	if e == nil || result.RegisterStatus != "outcome_unknown" {
		t.Fatal(result, e)
	}
	data, _ = r.Get(context.Background(), "powerwhois:registration:"+result.AuditID).Bytes()
	if !strings.Contains(string(data), "outcome_unknown") {
		t.Fatal("uncertain result not persisted")
	}
}

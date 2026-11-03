package register

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
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

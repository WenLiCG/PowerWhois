package register

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"typonamer/config"
	"typonamer/constant"
	"typonamer/internal/testredis"

	"github.com/google/uuid"
)

func TestRegistrationRecordsOneRequestAndReconcilesAmbiguity(t *testing.T) {
	store := testredis.Start(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("domain") != "example.com" || r.Header.Get("Idempotency-Key") == "" {
			t.Error("probe did not receive normalized target and operation ID")
		}
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		w.Write([]byte("success marker"))
	}))
	defer server.Close()
	provider := config.RegisterApi{
		ApiName: "fixture", ApiUrl: server.URL + "/?domain={domain}", ConcurrencyLimit: 1,
		SuccessText: []string{"success marker"}, FailText: []string{"failed marker"},
	}
	ctx := context.Background()
	id := uuid.NewString()
	if first := Register(ctx, store, "Example.com", id, provider, nil); first.RegisterStatus != constant.RegisterStatusUnknown {
		t.Fatal("a failed HTTP response was treated as a confirmed registration")
	}
	for _, operation := range []string{id, uuid.NewString()} {
		if result := Register(ctx, store, "example.com", operation, provider, nil); result.OperationID != id || calls.Load() != 1 {
			t.Fatal("another registrar request was issued before reconciliation")
		}
	}
	if _, err := Reconcile(ctx, store, "example.com", id, constant.RegisterStatusFailed); err != nil {
		t.Fatal(err)
	}
	if result := Register(ctx, store, "example.com", id, provider, nil); result.RegisterStatus != constant.RegisterStatusFailed || calls.Load() != 1 {
		t.Fatal("the same confirmation was replayed after reconciliation")
	}
	newID := uuid.NewString()
	if result := Register(ctx, store, "example.com", newID, provider, nil); result.RegisterStatus != constant.RegisterStatusSuccess || calls.Load() != 2 {
		t.Fatal("a new confirmed registration should be able to succeed")
	}
	if result := Register(ctx, store, "example.com", uuid.NewString(), provider, nil); result.OperationID != newID || calls.Load() != 2 {
		t.Fatal("immediate duplicate after a lost success acknowledgement reached the provider")
	}
}

func TestRevokedSessionSubmitsNothing(t *testing.T) {
	store := testredis.Start(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Write([]byte("success marker"))
	}))
	defer server.Close()
	provider := config.RegisterApi{
		ApiName: "fixture", ApiUrl: server.URL + "/?domain={domain}", ConcurrencyLimit: 1,
		SuccessText: []string{"success marker"}, FailText: []string{"failed marker"},
	}
	ctx := context.Background()

	// Revoked before the claim: nothing is recorded or sent.
	first := Register(ctx, store, "before.com", uuid.NewString(), provider, func() bool { return false })
	if first.RegisterStatus != constant.RegisterStatusError || calls.Load() != 0 || store.Exists(ctx, key("before.com")).Val() != 0 {
		t.Fatalf("revoked before claim: %+v, %d calls", first, calls.Load())
	}

	// Revoked between the claim and the request: a definite failure is stored,
	// the domain is released, and nothing is sent.
	checks := 0
	id := uuid.NewString()
	second := Register(ctx, store, "between.com", id, provider, func() bool { checks++; return checks == 1 })
	if second.RegisterStatus != constant.RegisterStatusFailed || calls.Load() != 0 {
		t.Fatalf("revoked after claim: %+v, %d calls", second, calls.Load())
	}
	if stored := store.HGet(ctx, key("between.com"), id).Val(); !strings.Contains(stored, `"registerStatus":"failed"`) {
		t.Fatalf("stored record = %q", stored)
	}
	if pending, err := Pending(ctx, store); err != nil || len(pending) != 0 {
		t.Fatalf("unsent domain left pending: %+v, %v", pending, err)
	}
	if retry := Register(ctx, store, "between.com", uuid.NewString(), provider, nil); retry.RegisterStatus != constant.RegisterStatusSuccess || calls.Load() != 1 {
		t.Fatalf("domain stayed locked after an unsent operation: %+v", retry)
	}
}

func TestPendingListsOnlyUnresolvedOperations(t *testing.T) {
	store := testredis.Start(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("domain") == "unclear.com" {
			w.Write([]byte("no marker"))
			return
		}
		w.Write([]byte("success marker"))
	}))
	defer server.Close()
	provider := config.RegisterApi{
		ApiName: "fixture", ApiUrl: server.URL + "/?domain={domain}", ConcurrencyLimit: 1,
		SuccessText: []string{"success marker"}, FailText: []string{"failed marker"},
	}
	ctx := context.Background()
	unclear := uuid.NewString()
	if result := Register(ctx, store, "unclear.com", unclear, provider, nil); result.RegisterStatus != constant.RegisterStatusUnknown {
		t.Fatalf("ambiguous reply = %s", result.RegisterStatus)
	}
	if result := Register(ctx, store, "confirmed.com", uuid.NewString(), provider, nil); result.RegisterStatus != constant.RegisterStatusSuccess {
		t.Fatalf("confirmed reply = %s", result.RegisterStatus)
	}
	if err := store.Set(ctx, "unrelated", "keep", 0).Err(); err != nil {
		t.Fatal(err)
	}
	pending, err := Pending(ctx, store)
	if err != nil || len(pending) != 1 || pending[0].OperationID != unclear || pending[0].DomainName != "unclear.com" || pending[0].StartedAt == 0 {
		t.Fatalf("pending = %+v, %v", pending, err)
	}
	if _, err := Reconcile(ctx, store, "unclear.com", unclear, constant.RegisterStatusFailed); err != nil {
		t.Fatal(err)
	}
	if pending, err := Pending(ctx, store); err != nil || len(pending) != 0 {
		t.Fatalf("reconciled operation still pending: %+v, %v", pending, err)
	}
}

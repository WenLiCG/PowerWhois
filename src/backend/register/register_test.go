package register

import (
	"context"
	"net/http"
	"net/http/httptest"
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
	if first := Register(ctx, store, "Example.com", id, provider); first.RegisterStatus != constant.RegisterStatusUnknown {
		t.Fatal("a failed HTTP response was treated as a confirmed registration")
	}
	for _, operation := range []string{id, uuid.NewString()} {
		if result := Register(ctx, store, "example.com", operation, provider); result.OperationID != id || calls.Load() != 1 {
			t.Fatal("another registrar request was issued before reconciliation")
		}
	}
	if _, err := Reconcile(ctx, store, "example.com", id, constant.RegisterStatusFailed); err != nil {
		t.Fatal(err)
	}
	if result := Register(ctx, store, "example.com", id, provider); result.RegisterStatus != constant.RegisterStatusFailed || calls.Load() != 1 {
		t.Fatal("the same confirmation was replayed after reconciliation")
	}
	newID := uuid.NewString()
	if result := Register(ctx, store, "example.com", newID, provider); result.RegisterStatus != constant.RegisterStatusSuccess || calls.Load() != 2 {
		t.Fatal("a new confirmed registration should be able to succeed")
	}
	if result := Register(ctx, store, "example.com", uuid.NewString(), provider); result.OperationID != newID || calls.Load() != 2 {
		t.Fatal("immediate duplicate after a lost success acknowledgement reached the provider")
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
	if result := Register(ctx, store, "unclear.com", unclear, provider); result.RegisterStatus != constant.RegisterStatusUnknown {
		t.Fatalf("ambiguous reply = %s", result.RegisterStatus)
	}
	if result := Register(ctx, store, "confirmed.com", uuid.NewString(), provider); result.RegisterStatus != constant.RegisterStatusSuccess {
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

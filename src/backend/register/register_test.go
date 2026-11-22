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

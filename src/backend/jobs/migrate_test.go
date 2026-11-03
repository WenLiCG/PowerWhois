package jobs

import (
	"context"
	"encoding/json"
	"testing"
	"typonamer/constant"
	"typonamer/internal/testutil"
	"typonamer/lookup/lookupinfo"
)

func TestLegacyMigrationPreservesOriginalAndIsRepeatable(t *testing.T) {
	r := testutil.Redis(t)
	ctx := context.Background()
	s := New(r, "migration:")
	pending := `{"Domain":"example.org","Order":1}`
	r.HSet(ctx, constant.BulkCheckUniqueDomainsRedisKey, "example.org", pending)
	raw, _ := json.Marshal(lookupinfo.QueryResult{Domain: "example.com", Order: 0, LookupType: "dns", RegisterStatus: "Free"})
	r.RPush(ctx, constant.BulkCheckFreeResultRedisKey, raw)
	r.Set(ctx, "unrelated", "retain", 0)
	for i := 0; i < 2; i++ {
		if e := s.ImportLegacy(ctx); e != nil {
			t.Fatal(e)
		}
	}
	tasks, e := s.List(ctx, 0, 10)
	if e != nil || len(tasks) != 1 {
		t.Fatal(tasks, e)
	}
	task := tasks[0]
	if task.State != "paused" || task.Total != 2 || task.Completed != 1 {
		t.Fatalf("bad migration: %+v", task)
	}
	page, e := s.Results(ctx, task.ID, 0, 10)
	if e != nil || len(page.Items) != 1 || page.Items[0].Status != "unknown" {
		t.Fatal(page, e)
	}
	if r.HGet(ctx, constant.BulkCheckUniqueDomainsRedisKey, "example.org").Val() != pending || r.LIndex(ctx, constant.BulkCheckFreeResultRedisKey, 0).Val() != string(raw) || r.Get(ctx, "unrelated").Val() != "retain" {
		t.Fatal("migration mutated original keys")
	}
}

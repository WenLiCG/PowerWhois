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
	kept := constant.BulkCheckErrorResultRedisKey + ":kept"
	reserved, _ := json.Marshal(lookupinfo.QueryResult{Domain: "example.net", Order: 2, RegisterStatus: "Reserved", QueryError: "注册局保留"})
	unknown, _ := json.Marshal(lookupinfo.QueryResult{Domain: "example.info", Order: 3, RegisterStatus: "Unknown", QueryError: "缺少查询证据"})
	if err := r.RPush(ctx, kept, reserved, unknown).Err(); err != nil {
		t.Fatal(err)
	}
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
	if task.State != "paused" || task.Total != 4 || task.Completed != 3 {
		t.Fatalf("bad migration: %+v", task)
	}
	page, e := s.Results(ctx, task.ID, 0, 10)
	if e != nil || len(page.Items) != 3 || page.Items[0].Status != "unknown" {
		t.Fatal(page, e)
	}
	if item := page.Items[1]; item.Status != "unknown" || item.RegisterStatus != "Reserved" || item.QueryError != "注册局保留" {
		t.Fatalf("reserved result lost during migration: %+v", item)
	}
	if item := page.Items[2]; item.Status != "unknown" || item.QueryError != "缺少查询证据" {
		t.Fatalf("unknown result lost during migration: %+v", item)
	}
	if r.HGet(ctx, constant.BulkCheckUniqueDomainsRedisKey, "example.org").Val() != pending || r.LIndex(ctx, constant.BulkCheckFreeResultRedisKey, 0).Val() != string(raw) || r.Get(ctx, "unrelated").Val() != "retain" {
		t.Fatal("migration mutated original keys")
	}
	if rows := r.LRange(ctx, kept, 0, -1).Val(); len(rows) != 2 || rows[0] != string(reserved) || rows[1] != string(unknown) {
		t.Fatal("migration mutated kept results")
	}
}

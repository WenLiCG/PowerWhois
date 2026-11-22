package scheduler

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"typonamer/config"
	"typonamer/constant"
	"typonamer/internal/testredis"
	"typonamer/lookup/lookupinfo"
)

func TestBulkPauseResumeAndClear(t *testing.T) {
	client := testredis.Start(t)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	unblock := make(chan struct{})
	b := &BulkCheck{redis: client, query: func(ctx context.Context, name, _ string, _ bool, _ config.Config) lookupinfo.QueryResult {
		if name == "bravo.com" {
			select {
			case <-ctx.Done():
			case <-unblock:
			}
		}
		return lookupinfo.QueryResult{Domain: name, RegisterStatus: constant.DomainRegisterStatusTaken}
	}}
	defer b.Shutdown(ctx)
	if err := client.Set(ctx, "unrelated", "keep", 0).Err(); err != nil {
		t.Fatal(err)
	}
	if err := b.Upload(ctx, strings.NewReader("\uFEFFhttps://bravo.com/path\nalpha.com\nbravo.com\n")); err != nil {
		t.Fatal(err)
	}
	if err := b.Start(ctx, constant.WhoisQuery); err != nil {
		t.Fatal(err)
	}
	waitBulk(t, ctx, b, func(in BulkCheckStatusInfo) bool {
		return in.Status == constant.BulkCheckStatusRunning && in.TakenDomains == 1
	})
	if err := b.Clear(ctx); err == nil {
		t.Fatal("active task was clearable")
	}
	if err := b.Stop(ctx, constant.BulkCheckStatusPaused); err != nil {
		t.Fatal(err)
	}
	paused := waitBulk(t, ctx, b, func(in BulkCheckStatusInfo) bool { return in.Status == constant.BulkCheckStatusPaused })
	if paused.TakenDomains != 1 || paused.RemainDomains != 1 || paused.TotalDomains != 2 {
		t.Fatalf("unexpected counts after pause: %+v", paused)
	}
	var partial bytes.Buffer
	if err := b.Export(ctx, &partial); err != nil || !strings.Contains(partial.String(), "alpha.com") || strings.Contains(partial.String(), "bravo.com") {
		t.Fatalf("unexpected partial export: %s (%v)", partial.String(), err)
	}
	close(unblock)
	if err := b.Resume(ctx); err != nil {
		t.Fatal(err)
	}
	done := waitBulk(t, ctx, b, func(in BulkCheckStatusInfo) bool { return in.Status == constant.BulkCheckStatusDone })
	if done.TakenDomains != 2 || done.RemainDomains != 0 {
		t.Fatalf("unexpected recovered counts: %+v", done)
	}
	var complete bytes.Buffer
	if err := b.Export(ctx, &complete); err != nil || strings.Count(complete.String(), "bravo.com") != 1 ||
		strings.Index(complete.String(), "bravo.com") >= strings.Index(complete.String(), "alpha.com") {
		t.Fatalf("full export lost original order or uniqueness: %s (%v)", complete.String(), err)
	}
	if err := b.Clear(ctx); err != nil || client.Get(ctx, "unrelated").Val() != "keep" {
		t.Fatalf("clear affected unrelated data: %v", err)
	}
}

func TestCommitIsAtomicAndRecoveryCanFinishWithNoPending(t *testing.T) {
	client := testredis.Start(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	b := &BulkCheck{redis: client}
	result := lookupinfo.QueryResult{Domain: "example.com", RegisterStatus: constant.DomainRegisterStatusTaken, RawResponse: "large response fixture"}
	if err := client.HSet(ctx, pendingKey, result.Domain, "{\"Domain\":\"example.com\",\"Order\":0}").Err(); err != nil {
		t.Fatal(err)
	}
	if err := client.Set(ctx, takenKey, "wrong storage type", 0).Err(); err != nil {
		t.Fatal(err)
	}
	if err := b.commit(ctx, result); err == nil || !client.HExists(ctx, pendingKey, result.Domain).Val() {
		t.Fatal("failed write lost the pending item")
	}
	if err := client.Del(ctx, takenKey).Err(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := b.commit(ctx, result); err != nil {
			t.Fatal(err)
		}
	}
	if client.LLen(ctx, takenKey).Val() != 1 || client.HLen(ctx, pendingKey).Val() != 0 {
		t.Fatal("retried commit was not idempotent")
	}
	if strings.Contains(client.LIndex(ctx, takenKey, 0).Val(), result.RawResponse) {
		t.Fatal("bulk storage retained the raw response body")
	}
	if err := client.MSet(ctx, statusKey, constant.BulkCheckStatusRunning, modeKey, constant.WhoisQuery, totalKey, "1").Err(); err != nil {
		t.Fatal(err)
	}
	recovered, err := NewBulkCheck(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Shutdown(ctx)
	waitBulk(t, ctx, recovered, func(in BulkCheckStatusInfo) bool { return in.Status == constant.BulkCheckStatusDone })
	if err := client.RPush(ctx, freeKey, "invalid-json").Err(); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := recovered.Export(ctx, &output); err == nil || output.Len() != 0 {
		t.Fatal("export accepted an unreadable stored row")
	}
}

func waitBulk(t *testing.T, ctx context.Context, b *BulkCheck, ready func(BulkCheckStatusInfo) bool) BulkCheckStatusInfo {
	t.Helper()
	for {
		info, err := b.Info(ctx)
		if err == nil && ready(info) {
			return info
		}
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for bulk task: %+v, %v", info, err)
		case <-time.After(15 * time.Millisecond):
		}
	}
}

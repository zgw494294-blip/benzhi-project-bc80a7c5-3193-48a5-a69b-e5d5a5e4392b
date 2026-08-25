package persistence

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"scenicpermit/internal/application"
	"scenicpermit/internal/audit"
	"scenicpermit/internal/domain"
)

func TestStorePersistsProjectionEventAndIdempotency(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	aggregate, _ := domain.NewAggregate("batch-store", "落盘测试", "剧场", "协调员", now.Add(time.Hour), now)
	event, _ := audit.NewEvent("e1", aggregate.Batch.ID, "batch.created", "协调员", aggregate.Batch.Revision, now, aggregate.Batch)
	if err := store.Create(context.Background(), aggregate, event, "key-1", []byte(`{"revision":1}`)); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	loaded, err := reopened.Load(context.Background(), aggregate.Batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Batch.Title != aggregate.Batch.Title {
		t.Fatalf("标题未持久化")
	}
	response, ok, err := reopened.IdempotentResponse(context.Background(), aggregate.Batch.ID, "key-1")
	if err != nil || !ok || len(response) == 0 {
		t.Fatalf("幂等响应缺失")
	}
}

func TestStoreRejectsStaleCommit(t *testing.T) {
	store, _ := Open(filepath.Join(t.TempDir(), "store.json"))
	defer store.Close()
	now := time.Now().UTC()
	aggregate, _ := domain.NewAggregate("batch-stale", "并发测试", "剧场", "协调员", now.Add(time.Hour), now)
	event, _ := audit.NewEvent("e1", aggregate.Batch.ID, "batch.created", "协调员", 1, now, aggregate.Batch)
	_ = store.Create(context.Background(), aggregate, event, "create", []byte(`{}`))
	clone, _ := domain.CloneAggregate(aggregate)
	clone.Batch.Revision = 2
	event2, _ := audit.NewEvent("e2", aggregate.Batch.ID, "changed", "协调员", 2, now, clone.Batch)
	err := store.Commit(context.Background(), application.Commit{Aggregate: clone, ExpectedRev: 99, Event: event2, IdempotencyKey: "change", Response: []byte(`{}`)})
	if err == nil {
		t.Fatal("陈旧提交应被拒绝")
	}
}

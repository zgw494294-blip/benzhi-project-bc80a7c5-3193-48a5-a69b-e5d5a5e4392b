package canceled_write_after_rename_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"scenicpermit/internal/application"
	"scenicpermit/internal/audit"
	"scenicpermit/internal/domain"
	"scenicpermit/internal/persistence"
)

func TestCanceledWriteMustNotAppearAfterReopen(t *testing.T) {
	now := time.Date(2027, 3, 18, 9, 0, 0, 0, time.UTC)
	performanceAt := now.Add(72 * time.Hour)

	t.Run("create", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "create.json")
		store, err := persistence.Open(path)
		if err != nil {
			t.Fatalf("open store: %v", err)
		}
		aggregate, err := domain.NewAggregate("batch-canceled-create", "取消创建", "实验剧场", "协调员", performanceAt, now)
		if err != nil {
			t.Fatalf("new aggregate: %v", err)
		}
		event, err := audit.NewEvent("event-create", aggregate.Batch.ID, "batch.created", "协调员", aggregate.Batch.Revision, now, aggregate.Batch)
		if err != nil {
			t.Fatalf("new event: %v", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err = store.Create(ctx, aggregate, event, "create-key", []byte(`{"revision":1}`))
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("pre-canceled create error = %v, want context.Canceled", err)
		}
		reopened, err := persistence.Open(path)
		if err != nil {
			t.Fatalf("reopen store: %v", err)
		}
		if _, err := reopened.Load(context.Background(), aggregate.Batch.ID); err == nil {
			t.Errorf("canceled create became durable after reopen")
		}
	})

	t.Run("commit", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "commit.json")
		store, err := persistence.Open(path)
		if err != nil {
			t.Fatalf("open store: %v", err)
		}
		aggregate, err := domain.NewAggregate("batch-canceled-commit", "原始标题", "实验剧场", "协调员", performanceAt, now)
		if err != nil {
			t.Fatalf("new aggregate: %v", err)
		}
		created, err := audit.NewEvent("event-initial", aggregate.Batch.ID, "batch.created", "协调员", aggregate.Batch.Revision, now, aggregate.Batch)
		if err != nil {
			t.Fatalf("new create event: %v", err)
		}
		if err := store.Create(context.Background(), aggregate, created, "initial-key", []byte(`{"revision":1}`)); err != nil {
			t.Fatalf("create initial batch: %v", err)
		}
		updated, err := domain.CloneAggregate(aggregate)
		if err != nil {
			t.Fatalf("clone aggregate: %v", err)
		}
		if err := updated.UpdateBatch("不应持久化的标题", "实验剧场", "协调员", performanceAt); err != nil {
			t.Fatalf("update aggregate: %v", err)
		}
		changed, err := audit.NewEvent("event-update", updated.Batch.ID, "batch.updated", "协调员", updated.Batch.Revision, now.Add(time.Minute), updated.Batch)
		if err != nil {
			t.Fatalf("new update event: %v", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err = store.Commit(ctx, application.Commit{Aggregate: updated, ExpectedRev: aggregate.Batch.Revision, Event: changed, IdempotencyKey: "update-key", Response: []byte(`{"revision":2}`)})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("pre-canceled commit error = %v, want context.Canceled", err)
		}
		reopened, err := persistence.Open(path)
		if err != nil {
			t.Fatalf("reopen store: %v", err)
		}
		loaded, err := reopened.Load(context.Background(), aggregate.Batch.ID)
		if err != nil {
			t.Fatalf("load reopened batch: %v", err)
		}
		if loaded.Batch.Title != aggregate.Batch.Title || loaded.Batch.Revision != aggregate.Batch.Revision {
			t.Errorf("canceled commit became durable after reopen: title=%q revision=%d", loaded.Batch.Title, loaded.Batch.Revision)
		}
	})
}

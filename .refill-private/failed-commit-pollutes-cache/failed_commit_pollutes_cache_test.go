package failedcommitpollutescache_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"scenicpermit/internal/application"
	"scenicpermit/internal/domain"
	"scenicpermit/internal/persistence"
)

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time { return c.now }

type failingCommitRepository struct {
	application.Repository
	err error
}

func (r *failingCommitRepository) Commit(context.Context, application.Commit) error {
	return r.err
}

func TestFailedCommitMustNotPolluteProjectionCache(t *testing.T) {
	store, err := persistence.Open(filepath.Join(t.TempDir(), "store.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	commitErr := errors.New("受控提交失败")
	repo := &failingCommitRepository{Repository: store, err: commitErr}
	now := time.Date(2032, time.March, 4, 10, 0, 0, 0, time.UTC)
	service := application.NewServiceWithClock(repo, fixedClock{now: now})
	created, err := service.CreateBatch(context.Background(), application.CreateBatchCommand{
		ID: "batch-cache-rollback", Title: "原始标题", Venue: "实验剧场",
		PerformanceAt: now.Add(48 * time.Hour), Coordinator: "制作协调员",
		Actor: "制作协调员", IdempotencyKey: "create-cache-rollback",
	})
	if err != nil {
		t.Fatal(err)
	}

	before, err := service.BatchDetail(context.Background(), created.BatchID)
	if err != nil {
		t.Fatal(err)
	}
	if before.Aggregate.Batch.Title != "原始标题" || before.Aggregate.Batch.Revision != 1 {
		t.Fatalf("初始投影异常: title=%q revision=%d", before.Aggregate.Batch.Title, before.Aggregate.Batch.Revision)
	}

	_, err = service.UpdateBatch(context.Background(), application.UpdateBatchCommand{
		Meta:  application.Meta{BatchID: created.BatchID, Revision: created.Revision, Actor: "制作协调员", IdempotencyKey: "failed-update"},
		Title: "从未提交的标题", Venue: "实验剧场", PerformanceAt: now.Add(72 * time.Hour), Coordinator: "制作协调员",
	})
	if !errors.Is(err, commitErr) {
		t.Fatalf("应返回受控提交错误，得到 %v", err)
	}

	after, err := service.BatchDetail(context.Background(), created.BatchID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Aggregate.Batch.Title != "原始标题" || after.Aggregate.Batch.Revision != 1 || after.Aggregate.Batch.State != domain.BatchDraft {
		t.Fatalf("提交失败后缓存遭到污染: title=%q revision=%d state=%s", after.Aggregate.Batch.Title, after.Aggregate.Batch.Revision, after.Aggregate.Batch.State)
	}
}

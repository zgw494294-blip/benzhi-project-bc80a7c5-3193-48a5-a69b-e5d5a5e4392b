package replay_leader_cancellation_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"scenicpermit/internal/application"
	"scenicpermit/internal/audit"
	"scenicpermit/internal/domain"
)

type observedContext struct {
	context.Context
	once    sync.Once
	reached chan struct{}
}

func (c *observedContext) markReached() {
	c.once.Do(func() { close(c.reached) })
}

func (c *observedContext) Done() <-chan struct{} {
	c.markReached()
	return c.Context.Done()
}

type blockingReplayRepository struct {
	firstEntered chan struct{}
	lookups      atomic.Int32
	creates      atomic.Int32
}

func (r *blockingReplayRepository) IdempotentResponse(ctx context.Context, _, _ string) ([]byte, bool, error) {
	if r.lookups.Add(1) == 1 {
		close(r.firstEntered)
		<-ctx.Done()
		return nil, false, ctx.Err()
	}
	if observed, ok := ctx.(*observedContext); ok {
		observed.markReached()
	}
	return nil, false, nil
}

func (r *blockingReplayRepository) Create(context.Context, *domain.Aggregate, audit.Event, string, []byte) error {
	r.creates.Add(1)
	return nil
}

func (*blockingReplayRepository) Load(context.Context, string) (*domain.Aggregate, error) {
	panic("unexpected Load")
}

func (*blockingReplayRepository) List(context.Context) ([]domain.InspectionBatch, error) {
	panic("unexpected List")
}

func (*blockingReplayRepository) Commit(context.Context, application.Commit) error {
	panic("unexpected Commit")
}

func (*blockingReplayRepository) Events(context.Context, string) ([]audit.Event, error) {
	panic("unexpected Events")
}

func (*blockingReplayRepository) Permits(context.Context) ([]domain.AdmissionPermit, error) {
	panic("unexpected Permits")
}

func (*blockingReplayRepository) Close() error { return nil }

func TestReplayLeaderCancellationMustNotFailLiveCaller(t *testing.T) {
	repo := &blockingReplayRepository{firstEntered: make(chan struct{})}
	service := application.NewService(repo)
	command := application.CreateBatchCommand{
		ID:             "batch-replay-cancel",
		Title:          "并发取消复现",
		Venue:          "测试剧场",
		PerformanceAt:  time.Now().UTC().Add(time.Hour),
		Coordinator:    "制作协调员",
		Actor:          "制作协调员",
		IdempotencyKey: "shared-create-key",
	}

	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	leaderErr := make(chan error, 1)
	go func() {
		_, err := service.CreateBatch(leaderCtx, command)
		leaderErr <- err
	}()
	<-repo.firstEntered

	waiterCtx := &observedContext{Context: context.Background(), reached: make(chan struct{})}
	waiterErr := make(chan error, 1)
	go func() {
		_, err := service.CreateBatch(waiterCtx, command)
		waiterErr <- err
	}()
	<-waiterCtx.reached
	cancelLeader()

	if err := <-leaderErr; !errors.Is(err, context.Canceled) {
		t.Fatalf("首调用应返回 context canceled，实际为 %v", err)
	}
	if err := <-waiterErr; err != nil {
		t.Fatalf("有效调用者继承了首调用的取消错误: %v", err)
	}
	if got := repo.creates.Load(); got != 1 {
		t.Fatalf("有效调用者应完成一次创建，实际创建次数为 %d", got)
	}
}

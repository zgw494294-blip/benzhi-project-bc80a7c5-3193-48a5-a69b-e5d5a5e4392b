package panicpoisonsbatchlock

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"scenicpermit/internal/application"
	"scenicpermit/internal/audit"
	"scenicpermit/internal/domain"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type panicOnceRepository struct {
	loads atomic.Int32
	now   time.Time
}

func (r *panicOnceRepository) Create(context.Context, *domain.Aggregate, audit.Event, string, []byte) error {
	return nil
}
func (r *panicOnceRepository) Load(context.Context, string) (*domain.Aggregate, error) {
	if r.loads.Add(1) == 1 {
		panic("injected repository panic")
	}
	return domain.NewAggregate("batch-lock", "锁恢复检查", "剧场", "协调员", r.now.Add(time.Hour), r.now)
}
func (r *panicOnceRepository) List(context.Context) ([]domain.InspectionBatch, error) {
	return nil, nil
}
func (r *panicOnceRepository) Commit(context.Context, application.Commit) error { return nil }
func (r *panicOnceRepository) Events(context.Context, string) ([]audit.Event, error) {
	return nil, nil
}
func (r *panicOnceRepository) Permits(context.Context) ([]domain.AdmissionPermit, error) {
	return nil, nil
}
func (r *panicOnceRepository) IdempotentResponse(context.Context, string, string) ([]byte, bool, error) {
	return nil, false, nil
}
func (r *panicOnceRepository) Close() error { return nil }

func TestPanicMustNotPoisonBatchSerialLock(t *testing.T) {
	now := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	repo := &panicOnceRepository{now: now}
	service := application.NewServiceWithClock(repo, fixedClock{now: now})
	command := application.AddUnitCommand{
		Meta: application.Meta{BatchID: "batch-lock", Revision: 1, Actor: "协调员", IdempotencyKey: "unit-one"},
		Unit: domain.SceneryUnit{ID: "unit-one", UnitCode: "SC-1", Name: "景片", StageZone: "主舞台", MaterialClass: "木质", Supplier: "制作组", TreatmentLot: "LOT-1", EvidenceRefs: []domain.EvidenceRef{{Name: "处理单", Digest: "digest-1"}}},
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("首次调用应触发注入的 panic")
			}
		}()
		_, _ = service.AddUnit(context.Background(), command)
	}()

	command.IdempotencyKey = "unit-two"
	done := make(chan error, 1)
	go func() {
		_, err := service.AddUnit(context.Background(), command)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("panic 恢复后的同批次请求失败: %v", err)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("panic 后同批次请求被遗留锁永久阻塞")
	}
}

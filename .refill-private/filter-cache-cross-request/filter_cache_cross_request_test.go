package filtercachecrossrequest

import (
	"context"
	"testing"
	"time"

	"scenicpermit/internal/application"
	"scenicpermit/internal/audit"
	"scenicpermit/internal/domain"
)

type detailRepository struct {
	aggregate *domain.Aggregate
}

func (r *detailRepository) Create(context.Context, *domain.Aggregate, audit.Event, string, []byte) error {
	panic("unexpected Create")
}

func (r *detailRepository) Load(context.Context, string) (*domain.Aggregate, error) {
	return domain.CloneAggregate(r.aggregate)
}

func (r *detailRepository) List(context.Context) ([]domain.InspectionBatch, error) {
	panic("unexpected List")
}

func (r *detailRepository) Commit(context.Context, application.Commit) error {
	panic("unexpected Commit")
}

func (r *detailRepository) Events(context.Context, string) ([]audit.Event, error) {
	return nil, nil
}

func (r *detailRepository) Permits(context.Context) ([]domain.AdmissionPermit, error) {
	return nil, nil
}

func (r *detailRepository) IdempotentResponse(context.Context, string, string) ([]byte, bool, error) {
	panic("unexpected IdempotentResponse")
}

func (r *detailRepository) Close() error { return nil }

func TestBatchDetailCacheMustIsolateFilters(t *testing.T) {
	now := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	aggregate := &domain.Aggregate{
		Batch: domain.InspectionBatch{
			ID: "batch-filter-cache", Title: "过滤缓存复现", Venue: "实验剧场",
			PerformanceAt: now.Add(48 * time.Hour), Coordinator: "协调员",
			State: domain.BatchSubmitted, Revision: 4, CreatedAt: now,
		},
		Units: []domain.SceneryUnit{
			{ID: "unit-left", BatchID: "batch-filter-cache", UnitCode: "L-01", Name: "左台景片", StageZone: "左台", MaterialClass: "木质"},
			{ID: "unit-right", BatchID: "batch-filter-cache", UnitCode: "R-01", Name: "右台景片", StageZone: "右台", MaterialClass: "织物"},
		},
		Plan: &domain.InspectionPlan{
			ID: "plan-filter-cache", BatchID: "batch-filter-cache", PlanRevision: 4,
			CheckDefinitions: []domain.CheckDefinition{{Code: "flame", Name: "续燃检查", Criterion: "无续燃", Required: true, Blocking: true}},
			FrozenUnitIDs:    []string{"unit-left", "unit-right"}, CreatedBy: "检查员", FrozenAt: now,
		},
	}
	service := application.NewService(&detailRepository{aggregate: aggregate})

	left, err := service.BatchDetailFiltered(context.Background(), aggregate.Batch.ID, application.BatchDetailFilter{
		Progress: domain.ProgressFilter{StageZone: "左台"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(left.Matrix) != 1 || left.Matrix[0].UnitID != "unit-left" {
		t.Fatalf("首次左台过滤结果异常: %+v", left.Matrix)
	}

	right, err := service.BatchDetailFiltered(context.Background(), aggregate.Batch.ID, application.BatchDetailFilter{
		Progress: domain.ProgressFilter{StageZone: "右台"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(right.Matrix) != 1 || right.Matrix[0].UnitID != "unit-right" {
		t.Fatalf("右台请求复用了其他过滤条件的缓存: %+v", right.Matrix)
	}
}

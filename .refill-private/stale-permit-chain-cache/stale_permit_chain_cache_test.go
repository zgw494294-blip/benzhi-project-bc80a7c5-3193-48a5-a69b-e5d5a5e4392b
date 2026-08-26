package stalepermitcache_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"scenicpermit/internal/application"
	"scenicpermit/internal/domain"
	"scenicpermit/internal/persistence"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

func prepareReadyBatch(t *testing.T, service *application.Service, batchID string, now time.Time) {
	t.Helper()
	ctx := context.Background()
	created, err := service.CreateBatch(ctx, application.CreateBatchCommand{
		ID: batchID, Title: "巡演布景", Venue: "实验剧场", Coordinator: "制作协调员",
		PerformanceAt: now.Add(24 * time.Hour), Actor: "制作协调员", IdempotencyKey: "create-" + batchID,
	})
	if err != nil {
		t.Fatalf("create %s: %v", batchID, err)
	}
	unitID := "unit-" + batchID
	added, err := service.AddUnit(ctx, application.AddUnitCommand{
		Meta: application.Meta{BatchID: batchID, Revision: created.Revision, Actor: "制作协调员", IdempotencyKey: "unit-" + batchID},
		Unit: domain.SceneryUnit{
			ID: unitID, UnitCode: unitID, Name: "主景片", StageZone: "左台", MaterialClass: "木质",
			Supplier: "制作部", TreatmentLot: "FR-1", EvidenceRefs: []domain.EvidenceRef{{Name: "处理记录", Digest: "evidence-1"}},
		},
	})
	if err != nil {
		t.Fatalf("add unit to %s: %v", batchID, err)
	}
	definitions := []domain.CheckDefinition{{
		Code: "flame", Name: "续燃检查", Criterion: "无续燃", Required: true, Blocking: true,
	}}
	preflight, err := service.PreflightPlan(ctx, batchID, definitions)
	if err != nil {
		t.Fatalf("preflight %s: %v", batchID, err)
	}
	submitted, err := service.SubmitPlan(ctx, application.SubmitPlanCommand{
		Meta:   application.Meta{BatchID: batchID, Revision: added.Revision, Actor: "检查员", IdempotencyKey: "submit-" + batchID},
		PlanID: "plan-" + batchID, Definitions: definitions, ConfirmationDigest: preflight.ConfirmationDigest,
	})
	if err != nil {
		t.Fatalf("submit %s: %v", batchID, err)
	}
	ready, err := service.RecordResult(ctx, application.RecordResultCommand{
		Meta: application.Meta{BatchID: batchID, Revision: submitted.Revision, Actor: "检查员", IdempotencyKey: "result-" + batchID},
		Result: domain.CheckResult{
			ID: "result-" + batchID, UnitID: unitID, CheckCode: "flame", Outcome: domain.OutcomePass,
			MeasuredValue: "0s", EvidenceDigest: "result-evidence", Inspector: "检查员",
		},
	})
	if err != nil {
		t.Fatalf("record result for %s: %v", batchID, err)
	}
	if ready.State != domain.BatchReady {
		t.Fatalf("prepared batch %s state = %s, want %s", batchID, ready.State, domain.BatchReady)
	}
}

func TestSequentialApprovalsMustRefreshPermitChain(t *testing.T) {
	now := time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)
	repo, err := persistence.Open(filepath.Join(t.TempDir(), "scenicpermit.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	service := application.NewServiceWithClock(repo, fixedClock{now: now})
	prepareReadyBatch(t, service, "batch-a", now)
	prepareReadyBatch(t, service, "batch-b", now)

	first, err := service.Approve(context.Background(), application.ApproveCommand{
		Meta:       application.Meta{BatchID: "batch-a", Revision: 4, Actor: "安全负责人", IdempotencyKey: "approve-a"},
		ApprovedBy: "安全负责人",
	})
	if err != nil {
		t.Fatalf("first approval failed: %v", err)
	}
	if first.State != domain.BatchApproved {
		t.Fatalf("first approval state = %s, want %s", first.State, domain.BatchApproved)
	}

	second, err := service.Approve(context.Background(), application.ApproveCommand{
		Meta:       application.Meta{BatchID: "batch-b", Revision: 4, Actor: "安全负责人", IdempotencyKey: "approve-b"},
		ApprovedBy: "安全负责人",
	})
	if err != nil {
		t.Fatalf("second sequential approval should allocate the next permit sequence: %v", err)
	}
	if second.State != domain.BatchApproved {
		t.Fatalf("second approval state = %s, want %s", second.State, domain.BatchApproved)
	}

	permits, err := repo.Permits(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(permits) != 2 || permits[0].Sequence != 1 || permits[1].Sequence != 2 {
		t.Fatalf("permit sequences are not consecutive: %+v", permits)
	}
}

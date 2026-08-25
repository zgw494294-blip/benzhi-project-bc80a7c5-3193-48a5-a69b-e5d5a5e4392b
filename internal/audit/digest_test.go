package audit

import (
	"scenicpermit/internal/domain"
	"testing"
	"time"
)

func TestPermitChainDetectsTampering(t *testing.T) {
	issued := time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC)
	first, err := SignPermit(domain.AdmissionPermit{ID: "p1", BatchID: "b1", Sequence: 1, ApprovedUnitIDs: []string{"u1"}, ManifestDigest: "m1", PreviousDigest: GenesisDigest, ApprovedBy: "安全负责人", IssuedAt: issued})
	if err != nil {
		t.Fatal(err)
	}
	second, err := SignPermit(domain.AdmissionPermit{ID: "p2", BatchID: "b2", Sequence: 2, ApprovedUnitIDs: []string{"u2"}, ManifestDigest: "m2", PreviousDigest: first.PermitDigest, ApprovedBy: "安全负责人", IssuedAt: issued.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if result := VerifyChain([]domain.AdmissionPermit{second, first}); !result.Valid {
		t.Fatal(result.Message)
	}
	second.ApprovedBy = "篡改者"
	if result := VerifyChain([]domain.AdmissionPermit{first, second}); result.Valid {
		t.Fatal("篡改凭据不应通过")
	}
}

func TestPlanConfirmationDigestChangesWithDraftSnapshot(t *testing.T) {
	now := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	a, _ := domain.NewAggregate("batch-digest", "预检", "剧场", "协调员", now.Add(time.Hour), now)
	unit := domain.SceneryUnit{ID: "unit-1", UnitCode: "SC-1", Name: "景片", StageZone: "主舞台", MaterialClass: "织物", Supplier: "制作组", TreatmentLot: "L1", EvidenceRefs: []domain.EvidenceRef{{Name: "处理单", Digest: "evidence-1"}}}
	_ = a.AddUnit(unit, now)
	definitions := []domain.CheckDefinition{{Code: "C1", Name: "检查", Criterion: "合格", Required: true, Blocking: true}}
	first, err := PlanConfirmationDigest(a.PreflightPlan(definitions))
	if err != nil {
		t.Fatal(err)
	}
	unit.EvidenceRefs = []domain.EvidenceRef{{Name: "处理单", Digest: "evidence-2"}}
	if _, err := a.UpdateUnit("unit-1", unit); err != nil {
		t.Fatal(err)
	}
	second, err := PlanConfirmationDigest(a.PreflightPlan(definitions))
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("登记快照变化后确认摘要必须变化")
	}
}

package detachedapprovedpermit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"scenicpermit/internal/audit"
	"scenicpermit/internal/domain"
	"scenicpermit/internal/persistence"
)

func approvedAggregate(t *testing.T, now time.Time) *domain.Aggregate {
	t.Helper()
	a, err := domain.NewAggregate("batch-approved", "已批准批次", "剧场", "协调员", now.Add(time.Hour), now)
	if err != nil {
		t.Fatal(err)
	}
	unit := domain.SceneryUnit{ID: "unit-one", UnitCode: "SC-1", Name: "景片", StageZone: "主舞台", MaterialClass: "木质", Supplier: "制作组", TreatmentLot: "LOT-1", EvidenceRefs: []domain.EvidenceRef{{Name: "处理单", Digest: "digest-1"}}}
	if err := a.AddUnit(unit, now); err != nil {
		t.Fatal(err)
	}
	definition := domain.CheckDefinition{Code: "FLAME", Name: "续燃检查", Criterion: "无续燃", Required: true, Blocking: true}
	if err := a.FreezePlan("plan-one", "协调员", []domain.CheckDefinition{definition}, now); err != nil {
		t.Fatal(err)
	}
	result := domain.CheckResult{ID: "result-one", UnitID: unit.ID, CheckCode: definition.Code, Outcome: domain.OutcomePass, EvidenceDigest: "result-digest", Inspector: "检查员"}
	if err := a.RecordResult(result, now); err != nil {
		t.Fatal(err)
	}
	manifest, err := a.BuildManifest()
	if err != nil {
		t.Fatal(err)
	}
	manifestDigest, err := audit.ManifestDigest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	permit, err := audit.SignPermit(domain.AdmissionPermit{ID: "permit-one", BatchID: a.Batch.ID, Sequence: 1, ManifestDigest: manifestDigest, PreviousDigest: audit.GenesisDigest, ApprovedBy: "安全负责人", IssuedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Approve(permit, now); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestOpenRejectsApprovedProjectionMissingGlobalPermit(t *testing.T) {
	now := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	aggregate := approvedAggregate(t, now)
	events := make([]audit.Event, 0, aggregate.Batch.Revision)
	for revision := int64(1); revision <= aggregate.Batch.Revision; revision++ {
		event, err := audit.NewEvent("event-"+time.Unix(revision, 0).UTC().Format("150405"), aggregate.Batch.ID, "workflow.step", "操作人", revision, now.Add(time.Duration(revision)*time.Second), map[string]any{"revision": revision})
		if err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	document := map[string]any{
		"schemaVersion": 1,
		"updatedAt":     now,
		"batches":       map[string]any{aggregate.Batch.ID: aggregate},
		"events":        map[string]any{aggregate.Batch.ID: events},
		"idempotency":   map[string]any{aggregate.Batch.ID: map[string]any{}},
		"permits":       []any{},
	}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "store.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	store, err := persistence.Open(path)
	if store != nil {
		_ = store.Close()
	}
	if err == nil {
		t.Fatal("聚合内已有批准凭据但全局凭据链缺失时仍允许启动")
	}
}

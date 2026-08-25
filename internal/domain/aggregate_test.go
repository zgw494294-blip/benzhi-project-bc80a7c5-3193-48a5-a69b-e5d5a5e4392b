package domain

import (
	"testing"
	"time"
)

func TestAggregateCompleteWorkflow(t *testing.T) {
	now := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	a, err := NewAggregate("batch-1", "首演检查", "实验剧场", "制作协调员", now.Add(48*time.Hour), now)
	if err != nil {
		t.Fatal(err)
	}
	unit := SceneryUnit{ID: "unit-1", UnitCode: "SC-001", Name: "主幕", StageZone: "主舞台", MaterialClass: "阻燃织物", Supplier: "制作组", TreatmentLot: "LOT-1", EvidenceRefs: []EvidenceRef{{Name: "处理单", Digest: "digest-1"}}}
	if err := a.AddUnit(unit, now); err != nil {
		t.Fatal(err)
	}
	definitions := []CheckDefinition{{Code: "FLAME", Name: "续燃", Criterion: "无续燃", Required: true, Blocking: true}}
	if err := a.FreezePlan("plan-1", "协调员", definitions, now); err != nil {
		t.Fatal(err)
	}
	failure := CheckResult{ID: "result-1", UnitID: "unit-1", CheckCode: "FLAME", Outcome: OutcomeFail, MeasuredValue: "4秒", EvidenceDigest: "fail-digest", Inspector: "检查员"}
	if err := a.RecordResult(failure, now); err != nil {
		t.Fatal(err)
	}
	if err := a.OpenRemediation(Remediation{ID: "rem-1", CheckResultID: "result-1", Owner: "制作负责人", DueAt: now.Add(time.Hour)}, now); err != nil {
		t.Fatal(err)
	}
	if err := a.CompleteRemediation("rem-1", "重新处理", []EvidenceRef{{Name: "整改记录", Digest: "rem-digest"}}, now); err != nil {
		t.Fatal(err)
	}
	pass := CheckResult{ID: "result-2", UnitID: "unit-1", CheckCode: "FLAME", Outcome: OutcomePass, MeasuredValue: "0秒", EvidenceDigest: "pass-digest", Inspector: "检查员"}
	if err := a.RecordResult(pass, now); err != nil {
		t.Fatal(err)
	}
	if err := a.CanApprove(); err != nil {
		t.Fatalf("应可批准: %v", err)
	}
	if a.Batch.State != BatchReady {
		t.Fatalf("状态 = %s", a.Batch.State)
	}
	manifest, err := a.BuildManifest()
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Units) != 1 || len(manifest.Checks) != 1 {
		t.Fatalf("清单不完整: %#v", manifest)
	}
}

func TestAggregateRejectsRetestBeforeRemediation(t *testing.T) {
	now := time.Now().UTC()
	a, _ := NewAggregate("batch-2", "测试", "剧场", "协调员", now.Add(time.Hour), now)
	_ = a.AddUnit(SceneryUnit{ID: "unit-2", UnitCode: "SC-2", Name: "景片", StageZone: "右区", MaterialClass: "木质", Supplier: "制作组", TreatmentLot: "LOT-2", EvidenceRefs: []EvidenceRef{{Name: "证据", Digest: "a"}}}, now)
	_ = a.FreezePlan("plan-2", "协调员", []CheckDefinition{{Code: "C1", Name: "项目", Criterion: "标准", Required: true, Blocking: true}}, now)
	_ = a.RecordResult(CheckResult{ID: "r-1", UnitID: "unit-2", CheckCode: "C1", Outcome: OutcomeFail, EvidenceDigest: "a", Inspector: "检查员"}, now)
	err := a.RecordResult(CheckResult{ID: "r-2", UnitID: "unit-2", CheckCode: "C1", Outcome: OutcomePass, EvidenceDigest: "b", Inspector: "检查员"}, now)
	if err == nil {
		t.Fatal("未完成整改的复测应被拒绝")
	}
}

func TestDraftUnitUpdateAndWithdrawalAreAtomic(t *testing.T) {
	now := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	a, _ := NewAggregate("batch-units", "登记纠错", "剧场", "协调员", now.Add(48*time.Hour), now)
	unit1 := SceneryUnit{ID: "unit-1", UnitCode: "SC-001", Name: "主幕", StageZone: "主舞台", MaterialClass: "织物", Supplier: "制作组", TreatmentLot: "LOT-1", EvidenceRefs: []EvidenceRef{{Name: "处理单", Digest: "digest-1"}}}
	unit2 := SceneryUnit{ID: "unit-2", UnitCode: "SC-002", Name: "侧幕", StageZone: "侧台", MaterialClass: "织物", Supplier: "制作组", TreatmentLot: "LOT-2", EvidenceRefs: []EvidenceRef{{Name: "处理单", Digest: "digest-2"}}}
	_ = a.AddUnit(unit1, now)
	_ = a.AddUnit(unit2, now)
	revision := a.Batch.Revision
	replacement := unit2
	replacement.UnitCode = "sc-001"
	if _, err := a.UpdateUnit("unit-2", replacement); err == nil {
		t.Fatal("不区分大小写的重复编号应被拒绝")
	}
	stored, _ := a.Unit("unit-2")
	if stored.UnitCode != "SC-002" || a.Batch.Revision != revision {
		t.Fatal("失败更新不应改变登记或修订号")
	}
	replacement.UnitCode, replacement.EvidenceRefs = "SC-003", []EvidenceRef{{Name: "新证据", Digest: "digest-3"}}
	if _, err := a.UpdateUnit("unit-2", replacement); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RemoveUnit("unit-1"); err != nil {
		t.Fatal(err)
	}
	if len(a.Units) != 1 || a.Units[0].UnitCode != "SC-003" {
		t.Fatalf("撤回后的登记不符合预期: %#v", a.Units)
	}
}

func TestPreflightCoverageIsStableAndBlocking(t *testing.T) {
	now := time.Now().UTC()
	a, _ := NewAggregate("batch-preview", "预检", "剧场", "协调员", now.Add(time.Hour), now)
	for _, unit := range []SceneryUnit{
		{ID: "unit-b", UnitCode: "SC-002", Name: "二号", StageZone: "侧台", MaterialClass: "木质", Supplier: "制作组", TreatmentLot: "L2", EvidenceRefs: []EvidenceRef{{Name: "单据", Digest: "d2"}}},
		{ID: "unit-a", UnitCode: "SC-001", Name: "一号", StageZone: "主舞台", MaterialClass: "织物", Supplier: "制作组", TreatmentLot: "L1", EvidenceRefs: []EvidenceRef{{Name: "单据", Digest: "d1"}}},
	} {
		_ = a.AddUnit(unit, now)
	}
	definitions := []CheckDefinition{{Code: "Z", Name: "追溯", Criterion: "一致", Required: true}, {Code: "A", Name: "续燃", Criterion: "无续燃", Required: true, Blocking: true}}
	view := a.PreflightPlan(definitions)
	if !view.Confirmable || view.Summary.TotalCheckCount != 4 || view.Summary.BlockingCheckCount != 1 {
		t.Fatalf("预检汇总错误: %#v", view)
	}
	if view.Coverage[0].UnitCode != "SC-001" || view.Coverage[0].CheckCode != "A" || view.Coverage[3].CheckCode != "Z" {
		t.Fatalf("覆盖排序不稳定: %#v", view.Coverage)
	}
	blocked := a.PreflightPlan([]CheckDefinition{{Code: "DUP", Name: "一", Criterion: "一", Required: true}, {Code: "dup", Name: "二", Criterion: "二", Required: true}})
	if blocked.Confirmable || len(blocked.Diagnostics) == 0 || blocked.ConfirmationDigest != "" {
		t.Fatal("重复检查编号必须形成阻断诊断且不能确认")
	}
}

func TestRecordResultsIsAllOrNothingAndBumpsOnce(t *testing.T) {
	now := time.Now().UTC()
	a, _ := NewAggregate("batch-bulk", "批量", "剧场", "协调员", now.Add(time.Hour), now)
	for _, unit := range []SceneryUnit{
		{ID: "u1", UnitCode: "SC-1", Name: "一号", StageZone: "主舞台", MaterialClass: "木质", Supplier: "制作组", TreatmentLot: "L1", EvidenceRefs: []EvidenceRef{{Name: "证据", Digest: "d1"}}},
		{ID: "u2", UnitCode: "SC-2", Name: "二号", StageZone: "主舞台", MaterialClass: "木质", Supplier: "制作组", TreatmentLot: "L2", EvidenceRefs: []EvidenceRef{{Name: "证据", Digest: "d2"}}},
	} {
		_ = a.AddUnit(unit, now)
	}
	_ = a.FreezePlan("plan-bulk", "协调员", []CheckDefinition{{Code: "C1", Name: "检查", Criterion: "合格", Required: true, Blocking: true}}, now)
	_ = a.RecordResult(CheckResult{ID: "r1", UnitID: "u1", CheckCode: "C1", Outcome: OutcomePass, EvidenceDigest: "pass-1", Inspector: "甲"}, now)
	revision, count := a.Batch.Revision, len(a.Results)
	_, err := a.RecordResults([]CheckResult{
		{ID: "r2", UnitID: "u1", CheckCode: "C1", Outcome: OutcomePass, EvidenceDigest: "pass-2", Inspector: "甲"},
		{ID: "r3", UnitID: "u2", CheckCode: "C1", Outcome: OutcomePass, EvidenceDigest: "pass-3", Inspector: "甲"},
	}, now)
	if err == nil || a.Batch.Revision != revision || len(a.Results) != count || a.LatestResult("u2", "C1") != nil {
		t.Fatal("批量中任一项目失败时必须全批回滚")
	}
	applied, err := a.RecordResults([]CheckResult{{ID: "r3", UnitID: "u2", CheckCode: "C1", Outcome: OutcomePass, EvidenceDigest: "pass-3", Inspector: "甲"}}, now)
	if err != nil || len(applied) != 1 || a.Batch.Revision != revision+1 || applied[0].Attempt != 1 {
		t.Fatalf("合法批量应只递增一次修订: %#v, %v", applied, err)
	}
}

func TestProgressUsesLatestAttemptAndRemediationRisk(t *testing.T) {
	now := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	a, _ := NewAggregate("batch-progress", "进度", "剧场", "协调员", now.Add(time.Hour), now)
	_ = a.AddUnit(SceneryUnit{ID: "u1", UnitCode: "SC-1", Name: "一号", StageZone: "主舞台", MaterialClass: "织物", Supplier: "制作组", TreatmentLot: "L1", EvidenceRefs: []EvidenceRef{{Name: "证据", Digest: "d1"}}}, now)
	_ = a.FreezePlan("plan-progress", "协调员", []CheckDefinition{{Code: "C1", Name: "检查", Criterion: "合格", Required: true, Blocking: true}}, now)
	_ = a.RecordResult(CheckResult{ID: "fail", UnitID: "u1", CheckCode: "C1", Outcome: OutcomeFail, EvidenceDigest: "fail", Inspector: "甲"}, now)
	_ = a.OpenRemediation(Remediation{ID: "rem", CheckResultID: "fail", Owner: "张三", DueAt: now.Add(time.Hour)}, now)
	before := a.Batch.Revision
	old, err := a.ChangeRemediation("rem", "李四", now.Add(2*time.Hour), "排期调整", now)
	if err != nil || old.Owner != "张三" || a.Batch.Revision != before+1 {
		t.Fatalf("整改变更失败: %v", err)
	}
	queue, _ := a.RemediationQueue("李四", "open", "overdue", now.Add(3*time.Hour))
	if len(queue) != 1 || queue[0].OverdueSeconds != 3600 {
		t.Fatalf("逾期队列错误: %#v", queue)
	}
	_ = a.CompleteRemediation("rem", "重新处理", []EvidenceRef{{Name: "整改", Digest: "fixed"}}, now.Add(3*time.Hour))
	_ = a.RecordResult(CheckResult{ID: "pass", UnitID: "u1", CheckCode: "C1", Outcome: OutcomePass, EvidenceDigest: "pass", Inspector: "乙"}, now.Add(3*time.Hour))
	progress, _ := a.Progress(ProgressFilter{Status: "fail"})
	if len(progress.Matrix) != 0 {
		t.Fatal("失败初检不应覆盖最新合格复测状态")
	}
	progress, _ = a.Progress(ProgressFilter{Inspector: "乙", Status: "pass"})
	if len(progress.Matrix) != 1 || progress.Groups[0].Passed != 1 {
		t.Fatalf("最新结果筛选错误: %#v", progress)
	}
}
